import { useCallback, useEffect, useMemo, useState } from 'react';
import { trackEventSource } from '../../telemetry/client';
import { Alert, Badge, Checkbox, Loader, Tabs, SimpleGrid, Stack, Text, Title } from '@mantine/core';
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

  const setEntrySelected = (entryId: string, selected: boolean) => {
    setSelectedIds((current) =>
      selected ? [...new Set([...current, entryId])] : current.filter((id) => id !== entryId),
    );
  };

  return (
    <Stack gap="sm">
      <Title order={1} size="h3">Scan queue</Title>
      <BarcodeInputField onScan={(barcode) => void captureBarcode(barcode)} />
      {scanError !== '' && (
        <Alert color="red" py="xs">
          {scanError}
        </Alert>
      )}
      {scannerConnected !== null && (
        <Alert color={scannerConnected ? 'green' : 'red'} py="xs" title={scannerConnected ? 'Scanner connected' : 'Scanner disconnected'}>
          {scannerConnected
            ? 'The barcode scanner is connected.'
            : 'The barcode scanner is not connected. You can scan with this device\'s camera instead.'}
        </Alert>
      )}
      <CameraScanner onScan={(barcode) => void captureBarcode(barcode)} />
      <Alert color={scannerMode === 'stock_in' ? 'blue' : 'orange'} py="xs" title={`Mode: ${scannerMode}`}>
        Current scanner mode: <strong>{scannerMode === 'stock_in' ? 'STOCK IN' : 'STOCK OUT'}</strong>
      </Alert>
      <Tabs value={activeView} onChange={handleViewChange}>
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
      <Checkbox
        label="Select all for approval"
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
      {loading && <Loader aria-label="Loading scan queue" />}
      {error !== '' && (
        <Alert color="red" py="xs">
          {error}
        </Alert>
      )}
      {!loading && error === '' && entries.length === 0 && processing.length === 0 && (
        <Text c="dimmed">No pending scans.</Text>
      )}
      <SimpleGrid cols={{ base: 1, sm: 2, lg: 3 }} spacing="xs">
        {processingForView.map((notice) => (
          <ProcessingScanCard key={notice.id} notice={notice} />
        ))}
        {viewEntries.map((entry) => (
          <ScanEntryCard
            key={entry.id}
            entry={entry}
            itemId={entry.productId === null ? undefined : itemIdByProductId.get(entry.productId)}
            selected={selectedIds.includes(entry.id)}
            onSelectedChange={(selected) => setEntrySelected(entry.id, selected)}
            onChanged={() => void loadQueue()}
          />
        ))}
      </SimpleGrid>
    </Stack>
  );
};
