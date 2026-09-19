import { useEffect, useState } from 'react';
import { Alert, Avatar, Badge, Button, Card, Checkbox, Group, NumberInput, Stack, Text, TextInput, Title } from '@mantine/core';
import { commitScanEntry, updateScanEntry } from '../../api/client';
import type { ScanEntry } from '../../types';
import { FlaggedEntryResolver } from './FlaggedEntryResolver';
import { StockOutInstanceSelector } from './StockOutInstanceSelector';
import { ProvenanceBadge } from '../product/ProvenanceBadge';
import { expiryDateToISOString, isValidUnitCount } from './queueUtils';

export interface ScanEntryCardProps {
  entry: ScanEntry;
  selected: boolean;
  itemId?: string;
  onSelectedChange: (selected: boolean) => void;
  onChanged: () => void;
}

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
  const [showChangeIndicator, setShowChangeIndicator] = useState(true);

  const prefersReducedMotion = () =>
    typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  // Sync draft state when unitCount or expiresAt changes (server updates)
  useEffect(() => {
    // Deliberate prop->draft sync: reset the controlled draft when the server value changes.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setUnitCountDraft(entry.unitCount);
  }, [entry.unitCount]);

  useEffect(() => {
    // Deliberate prop->draft sync: reset the controlled draft when the server value changes.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setExpiryDraft(entry.expiresAt === null ? '' : entry.expiresAt.substring(0, 10));
  }, [entry.expiresAt]);

  // Show change indicator on mount and when unitCount changes
  useEffect(() => {
    // Intentional transient animation trigger, cleared by the cleanup timer below.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setShowChangeIndicator(true);
    const timer = setTimeout(() => setShowChangeIndicator(false), 600);
    return () => clearTimeout(timer);
  }, [entry.unitCount]);

  const changeIndicatorClass = !showChangeIndicator
    ? undefined
    : prefersReducedMotion()
      ? 'scan-entry-card--changed-static'
      : 'scan-entry-card--changed-animated';

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
    <Card component="article" withBorder padding="sm" className={changeIndicatorClass} aria-label={`Scan ${entry.barcode}`}>
      <Stack gap="xs">
        {/* Header: Product info + checkbox */}
        <Group justify="space-between" align="flex-start" wrap="nowrap">
          <Group gap="xs" wrap="nowrap" align="flex-start" style={{ flex: 1 }}>
            <Avatar src={entry.product?.imageUrl} name={entry.product?.name ?? '?'} radius="sm" />
            <div style={{ flex: 1 }}>
              <Title order={3} size="h5">{entry.product?.name ?? 'Unknown product'}</Title>
              <Text size="xs" c="dimmed">Barcode: {entry.barcode}</Text>
              <ProvenanceBadge externalSource={entry.product?.externalSource} />
            </div>
          </Group>
          <Group gap="xs" wrap="nowrap" align="center">
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

        {/* Timestamp */}
        <Text size="xs" c="dimmed">
          Scanned: {new Date(entry.scannedAt).toLocaleString()}
        </Text>

        {/* Unit count + Expiration date controls (pending only) */}
        {entry.status === 'pending' && (
          <Group gap="xs" align="center" wrap="nowrap">
            <NumberInput
              size="xs"
              min={1}
              step={1}
              allowDecimal={false}
              value={unitCountDraft}
              onChange={handleUnitCountChange}
              onBlur={handleUnitCountBlur}
              onKeyDown={handleUnitCountKeyDown}
              w={80}
              aria-label="Unit count"
            />
            <TextInput
              size="xs"
              type="date"
              placeholder="mm/dd/yyyy"
              value={expiryDraft}
              onChange={(event) => {
                setExpiryDraft(event.currentTarget.value);
                // Clear any previous error when user starts typing
                setExpiryError('');
              }}
              onBlur={handleExpiryBlur}
              w={120}
              aria-label="Expiration date"
            />
          </Group>
        )}

        {/* Errors from unit count and expiration date */}
        {(unitCountError !== '' || expiryError !== '') && (
          <Stack gap="xs">
            {unitCountError !== '' && (
              <Alert color="red" py="xs">
                {unitCountError}
              </Alert>
            )}
            {expiryError !== '' && (
              <Alert color="red" py="xs">
                {expiryError}
              </Alert>
            )}
          </Stack>
        )}

        {/* Instance selector for stock_out entries */}
        {entry.status === 'pending' && entry.direction === 'stock_out' && itemId !== undefined && (
          <StockOutInstanceSelector itemId={itemId} value={instanceId} onChange={setInstanceId} />
        )}
        {entry.status === 'pending' && entry.direction === 'stock_out' && itemId === undefined && (
          <Alert color="yellow" py="xs">No inventory item is available for this product.</Alert>
        )}

        {/* Flagged entry resolver */}
        {entry.status === 'flagged' && (
          <FlaggedEntryResolver entry={entry} onResolved={onChanged} />
        )}

        {/* Approve and Remove buttons on same line (pending with direction only) */}
        {entry.status === 'pending' && entry.direction !== null && (
          <Group gap="xs">
            <Button
              size="xs"
              loading={approving}
              onClick={() => void handleApprove()}
              style={{ flex: 1 }}
            >
              Approve
            </Button>
            <Button
              size="xs"
              variant="subtle"
              color="red"
              loading={removing}
              onClick={() => void handleRemove()}
            >
              Remove
            </Button>
          </Group>
        )}

        {/* Remove-only button for flagged entries */}
        {entry.status === 'flagged' && (
          <Button
            size="xs"
            variant="subtle"
            color="red"
            loading={removing}
            onClick={() => void handleRemove()}
            w="100%"
          >
            Remove
          </Button>
        )}

        {/* Error alerts */}
        {approveError !== '' && <Alert color="red" py="xs">{approveError}</Alert>}
        {removeError !== '' && <Alert color="red" py="xs">{removeError}</Alert>}
      </Stack>
    </Card>
  );
};
