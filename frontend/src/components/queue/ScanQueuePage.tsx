import { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Loader, Stack, Text, Title } from '@mantine/core';
import { getInventoryList, listScanEntries } from '../../api/client';
import type { InventoryItem, ScanEntry } from '../../types';
import { BatchReviewPanel } from './BatchReviewPanel';
import { ScanEntryCard } from './ScanEntryCard';
import { sortScansChronologically } from './queueUtils';

const DEFAULT_USER_ID = 'default-user';

export interface ScanQueuePageProps {
  userId?: string;
}

export const ScanQueuePage = ({ userId = DEFAULT_USER_ID }: ScanQueuePageProps) => {
  const [entries, setEntries] = useState<ScanEntry[]>([]);
  const [inventory, setInventory] = useState<InventoryItem[]>([]);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

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

  const itemIdByProductId = useMemo(
    () => new Map(inventory.map((inventoryItem) => [inventoryItem.item.productId, inventoryItem.item.id])),
    [inventory],
  );

  const setEntrySelected = (entryId: string, selected: boolean) => {
    setSelectedIds((current) =>
      selected ? [...new Set([...current, entryId])] : current.filter((id) => id !== entryId),
    );
  };

  return (
    <Stack gap="lg">
      <Title order={1}>Scan queue</Title>
      <BatchReviewPanel
        selectedIds={selectedIds}
        onComplete={() => {
          setSelectedIds([]);
          void loadQueue();
        }}
      />
      {loading && <Loader aria-label="Loading scan queue" />}
      {error !== '' && <Alert color="red">{error}</Alert>}
      {!loading && error === '' && entries.length === 0 && (
        <Text c="dimmed">No pending scans.</Text>
      )}
      {entries.map((entry) => (
        <ScanEntryCard
          key={entry.id}
          entry={entry}
          itemId={entry.productId === null ? undefined : itemIdByProductId.get(entry.productId)}
          selected={selectedIds.includes(entry.id)}
          onSelectedChange={(selected) => setEntrySelected(entry.id, selected)}
          onChanged={() => void loadQueue()}
        />
      ))}
    </Stack>
  );
};
