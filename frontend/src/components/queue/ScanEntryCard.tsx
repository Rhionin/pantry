import { useState } from 'react';
import { Alert, Avatar, Badge, Button, Card, Checkbox, Group, Stack, Text, Title } from '@mantine/core';
import { commitScanEntry } from '../../api/client';
import type { ScanEntry } from '../../types';
import { FlaggedEntryResolver } from './FlaggedEntryResolver';
import { StockOutInstanceSelector } from './StockOutInstanceSelector';
import { formatExpiryDate } from './queueUtils';

export interface ScanEntryCardProps {
  entry: ScanEntry;
  selected: boolean;
  itemId?: string;
  onSelectedChange: (selected: boolean) => void;
  onChanged: () => void;
}

const directionLabel = (entry: ScanEntry): string => {
  if (entry.direction === 'stock_in') return 'Stock in';
  if (entry.direction === 'stock_out') return 'Stock out';
  return 'Not set';
};

export const ScanEntryCard = ({
  entry,
  selected,
  itemId,
  onSelectedChange,
  onChanged,
}: ScanEntryCardProps) => {
  const [instanceId, setInstanceId] = useState('');
  const [commitError, setCommitError] = useState('');
  const [committing, setCommitting] = useState(false);

  const handleCommit = async () => {
    setCommitting(true);
    setCommitError('');
    try {
      await commitScanEntry(entry.id, instanceId === '' ? undefined : instanceId);
      onChanged();
    } catch (requestError) {
      setCommitError(requestError instanceof Error ? requestError.message : 'Unable to commit scan.');
    } finally {
      setCommitting(false);
    }
  };

  return (
    <Card component="article" withBorder padding="sm" aria-label={`Scan ${entry.barcode}`}>
      <Stack gap="xs">
        <Group justify="space-between" align="flex-start" wrap="nowrap">
          <Group gap="xs" wrap="nowrap" align="flex-start">
            <Avatar src={entry.product?.imageUrl} name={entry.product?.name ?? '?'} radius="sm" />
            <div>
              <Title order={3} size="h5">{entry.product?.name ?? 'Unknown product'}</Title>
              <Text size="xs" c="dimmed">Barcode: {entry.barcode}</Text>
            </div>
          </Group>
          <Group gap="xs" wrap="nowrap">
            {entry.status === 'flagged' && <Badge color="orange">Flagged</Badge>}
            {entry.status === 'pending' && (
              <Checkbox
                size="xs"
                label="Select for batch review"
                checked={selected}
                onChange={(event) => onSelectedChange(event.currentTarget.checked)}
              />
            )}
          </Group>
        </Group>
        <Text size="xs" c="dimmed">
          Scanned: {new Date(entry.scannedAt).toLocaleString()} · Direction: {directionLabel(entry)} · Unit
          count: {entry.unitCount}
          {entry.expiresAt !== null && ` · Expires: ${formatExpiryDate(entry.expiresAt)}`}
        </Text>
        {entry.status === 'flagged' && (
          <FlaggedEntryResolver entry={entry} onResolved={onChanged} />
        )}
        {entry.status === 'pending' && entry.direction === 'stock_out' && itemId !== undefined && (
          <StockOutInstanceSelector itemId={itemId} value={instanceId} onChange={setInstanceId} />
        )}
        {entry.status === 'pending' && entry.direction === 'stock_out' && itemId === undefined && (
          <Alert color="yellow" py="xs">No inventory item is available for this product.</Alert>
        )}
        {entry.status === 'pending' && entry.direction !== null && (
          <Button size="xs" loading={committing} onClick={() => void handleCommit()}>
            Commit scan
          </Button>
        )}
        {commitError !== '' && <Alert color="red" py="xs">{commitError}</Alert>}
      </Stack>
    </Card>
  );
};
