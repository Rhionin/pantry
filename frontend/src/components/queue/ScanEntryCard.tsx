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
  justCaptured?: boolean;
  onSelectedChange: (selected: boolean) => void;
  onChanged: () => void;
}

export const ScanEntryCard = ({
  entry,
  selected,
  itemId,
  justCaptured = false,
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

  const changeIndicatorClass = [
    'scan-entry-card',
    !showChangeIndicator
      ? undefined
      : prefersReducedMotion()
        ? 'scan-entry-card--changed-static'
        : 'scan-entry-card--changed-animated',
    justCaptured ? 'scan-entry-card--just-captured' : undefined,
  ].filter(Boolean).join(' ');

  const productName = entry.product?.name ?? 'Unknown product';
  const scannedAtLabel = new Date(entry.scannedAt).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });

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
    <Card component="article" withBorder padding="xs" radius="md" className={changeIndicatorClass} aria-label={`Scan ${entry.barcode}`}>
      <div className="scan-entry-layout">
        <div className="scan-entry-check">
          {entry.status === 'pending' && (
            <Checkbox
              size="xs"
              aria-label="Select scan for batch approval"
              checked={selected}
              onChange={(event) => onSelectedChange(event.currentTarget.checked)}
            />
          )}
        </div>
        <Avatar
          className="scan-entry-avatar"
          src={entry.product?.imageUrl}
          name={productName}
          radius="sm"
          size={32}
        />
        <div className="scan-entry-body">
          <div className="scan-entry-titleline">
            <Title order={3} size="sm" lineClamp={1} mb={0}>{productName}</Title>
            {entry.status === 'flagged' && <Badge size="xs" color="orange">Flagged</Badge>}
          </div>
          <div className="scan-entry-meta">
            <Text size="xs" c="dimmed" component="span">Barcode: {entry.barcode}</Text>
            <Text size="xs" c="dimmed" component="span">Scanned: {scannedAtLabel}</Text>
            <ProvenanceBadge externalSource={entry.product?.externalSource} />
          </div>
        </div>
      </div>

      {entry.status === 'pending' && (
        <Group className="scan-entry-actions" gap={4} align="center" wrap="wrap">
          <NumberInput
            className="scan-entry-qty"
            size="xs"
            min={1}
            step={1}
            allowDecimal={false}
            value={unitCountDraft}
            onChange={handleUnitCountChange}
            onBlur={handleUnitCountBlur}
            onKeyDown={handleUnitCountKeyDown}
            aria-label="Unit count"
          />
          <TextInput
            className="scan-entry-expiry"
            size="xs"
            type="date"
            placeholder="mm/dd/yyyy"
            value={expiryDraft}
            onChange={(event) => {
              setExpiryDraft(event.currentTarget.value);
              setExpiryError('');
            }}
            onBlur={handleExpiryBlur}
            aria-label="Expiration date"
          />
          {entry.direction !== null && (
            <>
              <Button
                size="compact-xs"
                loading={approving}
                onClick={() => void handleApprove()}
              >
                Approve
              </Button>
              <Button
                size="compact-xs"
                variant="light"
                color="red"
                loading={removing}
                onClick={() => void handleRemove()}
              >
                Remove
              </Button>
            </>
          )}
        </Group>
      )}

      {(unitCountError !== '' || expiryError !== '') && (
        <Stack gap={4} mt={4}>
          {unitCountError !== '' && (
            <Alert color="red" py={4}>
              {unitCountError}
            </Alert>
          )}
          {expiryError !== '' && (
            <Alert color="red" py={4}>
              {expiryError}
            </Alert>
          )}
        </Stack>
      )}

      {entry.status === 'pending' && entry.direction === 'stock_out' && itemId !== undefined && (
        <div className="scan-entry-extra">
          <StockOutInstanceSelector itemId={itemId} value={instanceId} onChange={setInstanceId} />
        </div>
      )}
      {entry.status === 'pending' && entry.direction === 'stock_out' && itemId === undefined && (
        <Alert color="yellow" py={4} mt={4}>No inventory item is available for this product.</Alert>
      )}

      {entry.status === 'flagged' && (
        <div className="scan-entry-extra">
          <FlaggedEntryResolver entry={entry} onResolved={onChanged} />
        </div>
      )}

      {entry.status === 'flagged' && (
        <Group justify="flex-end" mt={4}>
          <Button
            size="compact-xs"
            variant="light"
            color="red"
            loading={removing}
            onClick={() => void handleRemove()}
          >
            Remove
          </Button>
        </Group>
      )}

      {approveError !== '' && <Alert color="red" py={4} mt={4}>{approveError}</Alert>}
      {removeError !== '' && <Alert color="red" py={4} mt={4}>{removeError}</Alert>}
    </Card>
  );
};
