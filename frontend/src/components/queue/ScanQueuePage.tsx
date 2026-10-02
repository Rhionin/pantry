import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { notifications } from '@mantine/notifications';
import { trackEventSource } from '../../telemetry/client';
import { Alert, Badge, Checkbox, Group, Loader, Tabs, SimpleGrid, Stack, Text, Title } from '@mantine/core';
import { createScanEntry, getInventoryList, getScannerConfig, listScanEntries, setScannerMode } from '../../api/client';
import type { InventoryItem, ProcessingFailure, ProcessingNotice, ScanEntry, ScannerConfig } from '../../types';
import { BarcodeInputField } from '../scanner/BarcodeInputField';
import { CameraScanner } from '../scanner/CameraScanner';
import { BatchReviewPanel } from './BatchReviewPanel';
import { ProcessingScanCard } from './ProcessingScanCard';
import { ScanEntryCard } from './ScanEntryCard';
import { addProcessingNotice, entryMatchesView, formatReviewCount, getEntriesForView, isBatchEligible, mergeScanEvent, pruneSelection, removeProcessingNotice, settleProcessingNotice, sortScansNewestFirst, toggleSelectAll } from './queueUtils';

const DEFAULT_USER_ID = 'user-1';

export type QueueView = 'stock_in' | 'stock_out';

export interface ScannerModeEvent {
  mode: 'stock_in' | 'stock_out';
}

export interface ScanQueuePageProps {
  userId?: string;
}

export const ScanQueuePage = ({ userId = DEFAULT_USER_ID }: ScanQueuePageProps) => {
  const [entries, setEntries] = useState<ScanEntry[]>([]);
  const [inventory, setInventory] = useState<InventoryItem[]>([]);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [activeView, setActiveView] = useState<QueueView>('stock_in');
  const [scannerMode, setScannerModeState] = useState<QueueView>('stock_in');
  const [scannerConfig, setScannerConfig] = useState<ScannerConfig | null>(null);
  const [scannerConnected, setScannerConnected] = useState<boolean | null>(null);
  const [processing, setProcessing] = useState<ProcessingNotice[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [scanError, setScanError] = useState('');
  const [cameraOpen, setCameraOpen] = useState(false);
  const [capturedBarcode, setCapturedBarcode] = useState('');
  const captureHighlightTimer = useRef(0);
  
  // Move the scanner mode and keep the visible tab following it, so a scan
  // taken in the current direction lands in the tab the user is looking at.
  const applyScannerMode = useCallback((mode: QueueView) => {
    setScannerModeState(mode);
    setActiveView(mode);
  }, []);

  const handleViewChange = (value: string | null) => {
    if (value !== 'stock_in' && value !== 'stock_out') return;
    setActiveView(value);
    setSelectedIds([]);
  };

  const stockInCount = useMemo(() => getEntriesForView(entries, 'stock_in').length, [entries]);
  const stockOutCount = useMemo(() => getEntriesForView(entries, 'stock_out').length, [entries]);

  const viewEntries = useMemo(() => getEntriesForView(entries, activeView), [entries, activeView]);
  const processingForView = useMemo(
    () => processing.filter((notice) => notice.userId === userId && entryMatchesView(notice.direction, activeView)),
    [processing, userId, activeView],
  );
  const viewEntryIds = useMemo(() => new Set(viewEntries.map((entry) => entry.id)), [viewEntries]);
  const eligibleEntries = useMemo(() => viewEntries.filter(isBatchEligible), [viewEntries]);
  const allEligibleSelected = useMemo(
    () => eligibleEntries.length > 0 && eligibleEntries.every((entry) => selectedIds.includes(entry.id)),
    [eligibleEntries, selectedIds],
  );

  const loadQueue = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const [pending, flagged] = await Promise.all([
        listScanEntries(userId, 'pending'),
        listScanEntries(userId, 'flagged'),
      ]);
      setEntries(sortScansNewestFirst([...pending, ...flagged]));
      setSelectedIds((current) => current.filter((id) => pending.some((entry) => entry.id === id)));
      void getInventoryList().then(setInventory).catch(() => setInventory([]));
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to load scan queue.');
    } finally {
      setLoading(false);
    }
  }, [userId]);

  useEffect(() => {
    void Promise.resolve().then(loadQueue);
  }, [loadQueue]);

  useEffect(() => () => window.clearTimeout(captureHighlightTimer.current), []);

  useEffect(() => {
    if (capturedBarcode === '') return;
    const node = document.querySelector('.scan-entry-card--just-captured, .processing-scan-card--just-captured');
    node?.scrollIntoView?.({ block: 'nearest' });
  }, [capturedBarcode, entries, processing]);

  // Load the reserved control-barcode strings so captureBarcode can classify a
  // scan as a mode switch instead of a product barcode, and seed the displayed
  // mode from the backend's current direction so a browser that connects after
  // a switch starts on the right mode. If the config cannot be loaded,
  // classification falls back to off (no crash) and scans post as usual.
  useEffect(() => {
    let cancelled = false;
    getScannerConfig()
      .then((config) => {
        if (cancelled) return;
        setScannerConfig(config);
        if (typeof config.connected === 'boolean') {
          setScannerConnected(config.connected);
        }
        applyScannerMode(config.currentMode);
      })
      .catch(() => {
        if (!cancelled) setScannerConfig(null);
      });
    return () => {
      cancelled = true;
    };
  }, [applyScannerMode]);

  useEffect(() => {
    let cancelled = false;
    const refreshConnection = () => {
      getScannerConfig()
        .then((config) => {
          if (!cancelled && typeof config.connected === 'boolean') {
            setScannerConnected(config.connected);
          }
        })
        .catch(() => undefined);
    };
    const timer = window.setInterval(refreshConnection, 2000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    const eventSource = new EventSource('/api/events');
    trackEventSource(eventSource);
    eventSource.addEventListener('scan', (message) => {
      const scanEntry = JSON.parse((message as MessageEvent).data) as ScanEntry;
      setEntries((current) => {
        const next = sortScansNewestFirst(mergeScanEvent(current, scanEntry));
        setSelectedIds((selected) => pruneSelection(selected, next));
        return next;
      });
      setProcessing((current) => settleProcessingNotice(current, scanEntry));
    });
    eventSource.addEventListener('scan_processing', (message) => {
      const notice = JSON.parse((message as MessageEvent).data) as ProcessingNotice;
      setProcessing((current) => addProcessingNotice(current, notice));
    });
    eventSource.addEventListener('scan_processing_failed', (message) => {
      const failure = JSON.parse((message as MessageEvent).data) as ProcessingFailure;
      setProcessing((current) => removeProcessingNotice(current, failure.id));
      setScanError(failure.message);
    });
    eventSource.addEventListener('scanner_mode', (message) => {
      const event = JSON.parse((message as MessageEvent).data) as ScannerModeEvent;
      if (event.mode === 'stock_in' || event.mode === 'stock_out') {
        applyScannerMode(event.mode);
      }
    });
    return () => eventSource.close();
  }, [applyScannerMode]);

  const itemIdByProductId = useMemo(
    () => new Map(inventory.map((inventoryItem) => [inventoryItem.item.productId, inventoryItem.item.id])),
    [inventory],
  );

  // Classify a scanned string against the configured control barcodes using the
  // same exact-match rule the backend classify() applies (no trimming or
  // case-folding beyond what BarcodeInputField already trims). Returns the mode
  // to switch to, or null when the string is an ordinary product barcode or the
  // config has not loaded yet.
  const classifyControlBarcode = (barcode: string): QueueView | null => {
    if (scannerConfig === null) return null;
    if (barcode === scannerConfig.stockInBarcode) return 'stock_in';
    if (barcode === scannerConfig.stockOutBarcode) return 'stock_out';
    return null;
  };

  const captureBarcode = async (barcode: string) => {
    setScanError('');
    const targetMode = classifyControlBarcode(barcode);
    if (targetMode !== null) {
      // Optimistically reflect the switch; the resulting scanner_mode SSE event
      // keeps every subscriber consistent with this value.
      applyScannerMode(targetMode);
      try {
        await setScannerMode(targetMode);
      } catch (requestError) {
        setScanError(requestError instanceof Error ? requestError.message : 'Unable to switch scanner mode.');
      }
      return;
    }
    try {
      // Stamp the entry with the selected mode so browser scans land in the
      // same view as the current direction, matching the headless listener
      // which stamps each entry with its Current_Mode.
      await createScanEntry({ barcode, direction: scannerMode, userId });
      await loadQueue();
    } catch (requestError) {
      setScanError(requestError instanceof Error ? requestError.message : 'Unable to add the scan.');
    }
  };

  const noteCameraCapture = (barcode: string) => {
    const targetMode = classifyControlBarcode(barcode);
    if (targetMode !== null) {
      notifications.show({
        message: targetMode === 'stock_in' ? 'Switched to stock in' : 'Switched to stock out',
        color: targetMode === 'stock_in' ? 'blue' : 'orange',
        autoClose: 2000,
        position: 'top-center',
      });
    } else {
      setCapturedBarcode(barcode);
      window.clearTimeout(captureHighlightTimer.current);
      captureHighlightTimer.current = window.setTimeout(() => setCapturedBarcode(''), 4000);
      notifications.show({
        message: `Captured ${barcode}`,
        color: 'teal',
        autoClose: 2000,
        position: 'top-center',
      });
    }
    void captureBarcode(barcode);
  };

  const setEntrySelected = (entryId: string, selected: boolean) => {
    setSelectedIds((current) =>
      selected ? [...new Set([...current, entryId])] : current.filter((id) => id !== entryId),
    );
  };

  const modeLabel = scannerMode === 'stock_in' ? 'STOCK IN' : 'STOCK OUT';

  return (
    <Stack gap={6} className="scan-page">
      <div className="scan-page-tools scan-toolbar">
        <BarcodeInputField onScan={(barcode) => void captureBarcode(barcode)} />
        <div className="scan-toolbar-primary">
          <Title order={1} className="scan-toolbar-title">Scan queue</Title>
          {/* "Mode: stock_…" stays in the DOM for the queue banner matcher.
              The visible label is the short STOCK IN / STOCK OUT text. */}
          <div role="alert" className={`scan-mode-chip scan-mode-chip--${scannerMode}`}>
            <span className="scan-mode-chip-key" aria-hidden="true">Mode: {scannerMode}</span>
            <strong className="scan-mode-chip-label">{modeLabel}</strong>
          </div>
          <CameraScanner onScan={noteCameraCapture} onOpenChange={setCameraOpen} />
        </div>
        {scanError !== '' && (
          <Alert color="red" py={4}>
            {scanError}
          </Alert>
        )}
        {scannerConnected !== null && !cameraOpen && (
          <Text
            size="xs"
            className={scannerConnected ? 'scan-status scan-status--on' : 'scan-status scan-status--off'}
          >
            {scannerConnected ? 'Scanner connected' : (
              <>
                <span>Scanner disconnected</span>
                {'. You can scan with this device\'s camera instead.'}
              </>
            )}
          </Text>
        )}
        <Tabs value={activeView} onChange={handleViewChange} className="scan-view-tabs">
          <Tabs.List>
            <Tabs.Tab
              value="stock_in"
              rightSection={stockInCount > 0
                ? <Badge size="sm" circle aria-hidden>{formatReviewCount(stockInCount)}</Badge>
                : undefined}
            >
              Stock in
            </Tabs.Tab>
            <Tabs.Tab
              value="stock_out"
              rightSection={stockOutCount > 0
                ? <Badge size="sm" circle aria-hidden>{formatReviewCount(stockOutCount)}</Badge>
                : undefined}
            >
              Stock out
            </Tabs.Tab>
          </Tabs.List>
        </Tabs>
        {viewEntries.length > 0 && (
          <Group className="scan-batch-row" justify="space-between" align="center" wrap="nowrap" gap="xs">
            <Checkbox
              size="xs"
              label="Select all"
              aria-label="Select all eligible scans for batch approval"
              checked={allEligibleSelected}
              disabled={eligibleEntries.length === 0}
              onChange={() => setSelectedIds((current) => toggleSelectAll(viewEntries, current))}
            />
            <BatchReviewPanel
              selectedIds={selectedIds.filter((id) => viewEntryIds.has(id))}
              onComplete={() => {
                setSelectedIds([]);
                void loadQueue();
              }}
            />
          </Group>
        )}
      </div>
      <Stack gap={6} className="scan-page-queue" aria-label="Scan queue entries">
        {loading && <Loader size="sm" aria-label="Loading scan queue" />}
        {error !== '' && (
          <Alert color="red" py={4}>
            {error}
          </Alert>
        )}
        {!loading && error === '' && viewEntries.length === 0 && processingForView.length === 0 && (
          <Text c="dimmed" size="sm">No pending scans.</Text>
        )}
        <SimpleGrid className="scan-entry-list" cols={{ base: 1, sm: 2, lg: 3 }} spacing={6}>
          {processingForView.map((notice) => (
            <ProcessingScanCard
              key={notice.id}
              notice={notice}
              justCaptured={capturedBarcode !== '' && notice.barcode === capturedBarcode}
            />
          ))}
          {viewEntries.map((entry) => (
            <ScanEntryCard
              key={entry.id}
              entry={entry}
              itemId={entry.productId === null ? undefined : itemIdByProductId.get(entry.productId)}
              justCaptured={capturedBarcode !== '' && entry.barcode === capturedBarcode}
              selected={selectedIds.includes(entry.id)}
              onSelectedChange={(selected) => setEntrySelected(entry.id, selected)}
              onChanged={() => void loadQueue()}
            />
          ))}
        </SimpleGrid>
      </Stack>
    </Stack>
  );
};
