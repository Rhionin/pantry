import { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Checkbox, Loader, Tabs, SimpleGrid, Stack, Text, Title } from '@mantine/core';
import { createScanEntry, getInventoryList, listScanEntries } from '../../api/client';
import type { InventoryItem, ScanEntry } from '../../types';
import { BarcodeInputField } from '../scanner/BarcodeInputField';
import { BatchReviewPanel } from './BatchReviewPanel';
import { ScanEntryCard } from './ScanEntryCard';
import { getEntriesForView, isBatchEligible, mergeScanEvent, pruneSelection, sortScansChronologically, toggleSelectAll } from './queueUtils';

const DEFAULT_USER_ID = 'user-1';

export type QueueView = 'stock_in' | 'stock_out';

export interface ScanQueuePageProps {
  userId?: string;
}

export const ScanQueuePage = ({ userId = DEFAULT_USER_ID }: ScanQueuePageProps) => {
  const [entries, setEntries] = useState<ScanEntry[]>([]);
  const [inventory, setInventory] = useState<InventoryItem[]>([]);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [activeView, setActiveView] = useState<QueueView>('stock_out');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [scanError, setScanError] = useState('');
  
  const handleViewChange = (value: string | null) => {
    if (value !== 'stock_in' && value !== 'stock_out') return;
    setActiveView(value);
    setSelectedIds([]);
  };

  const viewEntries = useMemo(() => getEntriesForView(entries, activeView), [entries, activeView]);
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
      setEntries(sortScansChronologically([...pending, ...flagged]));
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

  useEffect(() => {
    const eventSource = new EventSource('/api/events');
    eventSource.addEventListener('scan', (message) => {
      const scanEntry = JSON.parse((message as MessageEvent).data) as ScanEntry;
      setEntries((current) => {
        const next = sortScansChronologically(mergeScanEvent(current, scanEntry));
        setSelectedIds((selected) => pruneSelection(selected, next));
        return next;
      });
    });
    return () => eventSource.close();
  }, []);

  const itemIdByProductId = useMemo(
    () => new Map(inventory.map((inventoryItem) => [inventoryItem.item.productId, inventoryItem.item.id])),
    [inventory],
  );

  const captureBarcode = async (barcode: string) => {
    setScanError('');
    try {
      await createScanEntry({ barcode, userId });
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
      <Tabs value={activeView} onChange={handleViewChange}>
        <Tabs.List>
          <Tabs.Tab value="stock_in">Stock in</Tabs.Tab>
          <Tabs.Tab value="stock_out">Stock out</Tabs.Tab>
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
      {!loading && error === '' && entries.length === 0 && (
        <Text c="dimmed">No pending scans.</Text>
      )}
      <SimpleGrid cols={{ base: 1, sm: 2, lg: 3 }} spacing="xs">
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
