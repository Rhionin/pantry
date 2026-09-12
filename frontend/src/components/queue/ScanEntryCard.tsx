import { useState } from 'react';
import { Alert, Avatar, Badge, Button, Card, Checkbox, Group, NumberInput, Stack, Text, TextInput, Title } from '@mantine/core';
import { commitScanEntry, updateScanEntry } from '../../api/client';
import type { ScanEntry } from '../../types';
import { FlaggedEntryResolver } from './FlaggedEntryResolver';
import { StockOutInstanceSelector } from './StockOutInstanceSelector';
import { expiryDateToISOString, formatExpiryDate, isValidUnitCount } from './queueUtils';

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
  const [approveError, setApproveError] = useState('');
  const [approving, setApproving] = useState(false);
  const [removeError, setRemoveError] = useState('');
  const [removing, setRemoving] = useState(false);
  const [unitCountDraft, setUnitCountDraft] = useState(entry.unitCount);
  const [unitCountError, setUnitCountError] = useState('');
  const [expiryDraft, setExpiryDraft] = useState(
    entry.expiresAt === null ? '' : entry.expiresAt.substring(0, 10),
  );
  const [expiryError, setExpiryError] = useState('');

  const handleApprove = async () => {
    setApproving(true);
    setApproveError('');
    try {
      await commitScanEntry(entry.id, instanceId === '' ? undefined : instanceId);
      onChanged();
    } catch (requestError) {
      setApproveError(requestError instanceof Error ? requestError.message : 'Unable to approve scan.');
    } finally {
      setApproving(false);
    }
  };

  const handleRemove = async () => {
    setRemoving(true);
    setRemoveError('');
    try {
      await updateScanEntry(entry.id, { status: 'cancelled' });
      onChanged();
    } catch (requestError) {
      setRemoveError(requestError instanceof Error ? requestError.message : 'Unable to remove scan.');
    } finally {
      setRemoving(false);
    }
  };

  const handleIncrement = async () => {
    try {
      await updateScanEntry(entry.id, { unitCount: entry.unitCount + 1 });
      setUnitCountDraft(entry.unitCount + 1);
      onChanged();
    } catch {
      // Error handling is done by the API layer - just reset draft on failure
      setUnitCountDraft(entry.unitCount);
    }
  };

  const handleDecrement = async () => {
    try {
      await updateScanEntry(entry.id, { unitCount: entry.unitCount - 1 });
      setUnitCountDraft(entry.unitCount - 1);
      onChanged();
    } catch {
      // Error handling is done by the API layer - just reset draft on failure
      setUnitCountDraft(entry.unitCount);
    }
  };

  const handleUnitCountBlur = async () => {
    const draft = Number(unitCountDraft);
    
    // Validate the draft - must be an integer >= 1
    if (!isValidUnitCount(draft)) {
      setUnitCountDraft(entry.unitCount);
      return;
    }
    
    // Only send PATCH if value changed
    if (draft !== entry.unitCount) {
      try {
        await updateScanEntry(entry.id, { unitCount: draft });
        onChanged();
      } catch {
        setUnitCountError('Unable to update unit count.');
        setUnitCountDraft(entry.unitCount);
      }
    }
  };

  const handleUnitCountKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      (event.target as HTMLInputElement).blur();
    }
  };

  const handleUnitCountChange = (value: number | string | null) => {
    setUnitCountDraft(value === null || value === '' ? entry.unitCount : Number(value));
    // Clear any previous error when user starts typing
    if (value !== null && value !== '') {
      setUnitCountError('');
    }
  };

  const handleExpiryBlur = async () => {
    const isoString = expiryDateToISOString(expiryDraft);
    
    // Only send PATCH if value changed
    const currentExpiresAt = entry.expiresAt === null ? undefined : entry.expiresAt.substring(0, 10);
    if (expiryDraft !== currentExpiresAt) {
      try {
        await updateScanEntry(entry.id, { expiresAt: isoString });
        onChanged();
      } catch {
        setExpiryError('Unable to update expiration date.');
        // Reset draft to current value
        setExpiryDraft(entry.expiresAt === null ? '' : entry.expiresAt.substring(0, 10));
      }
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
                aria-label="Select scan for batch approval"
                checked={selected}
                onChange={(event) => onSelectedChange(event.currentTarget.checked)}
              />
            )}
          </Group>
        </Group>
        <Text size="xs" c="dimmed">
          Scanned: {new Date(entry.scannedAt).toLocaleString()} · Direction: {directionLabel(entry)}
        </Text>
        {entry.status === 'pending' && (
          <Group gap="xs" align="center" wrap="nowrap">
            <Button
              size="xs"
              onClick={() => void handleDecrement()}
              disabled={entry.unitCount <= 1}
              aria-label="Decrease unit count"
            >
              -
            </Button>
            <NumberInput
              size="xs"
              min={1}
              step={1}
              allowDecimal={false}
              value={unitCountDraft}
              onChange={handleUnitCountChange}
              onBlur={handleUnitCountBlur}
              onKeyDown={handleUnitCountKeyDown}
              w={160}
              aria-label="Unit count"
            />
            {unitCountError !== '' && (
              <Alert color="red" py="xs" style={{ flex: 1 }}>
                {unitCountError}
              </Alert>
            )}
            <Button
              size="xs"
              onClick={() => void handleIncrement()}
              aria-label="Increase unit count"
            >
              +
            </Button>
          </Group>
        )}
        {entry.status === 'pending' && (
          <Group gap="xs" align="center" wrap="nowrap">
            <TextInput
              size="xs"
              type="date"
              value={expiryDraft}
              onChange={(event) => {
                setExpiryDraft(event.currentTarget.value);
                // Clear any previous error when user starts typing
                setExpiryError('');
              }}
              onBlur={handleExpiryBlur}
              w={160}
              aria-label="Expiration date"
            />
            {expiryError !== '' && (
              <Alert color="red" py="xs" style={{ flex: 1 }}>
                {expiryError}
              </Alert>
            )}
          </Group>
        )}
        <Text size="xs" c="dimmed">
          Unit count: {entry.unitCount}
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
          <Button size="xs" loading={approving} onClick={() => void handleApprove()} aria-label="Approve scan">
            Approve scan
          </Button>
        )}
        {(entry.status === 'pending' || entry.status === 'flagged') && (
          <Button
            size="xs"
            variant="subtle"
            color="red"
            loading={removing}
            onClick={() => void handleRemove()}
            aria-label="Remove scan"
          >
            Remove
          </Button>
        )}
        {approveError !== '' && <Alert color="red" py="xs">{approveError}</Alert>}
        {removeError !== '' && <Alert color="red" py="xs">{removeError}</Alert>}
      </Stack>
    </Card>
  );
};
