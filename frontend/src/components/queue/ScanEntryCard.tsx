import { useEffect, useRef, useState } from 'react';
import { Alert, Avatar, Badge, Button, Card, Checkbox, Group, NumberInput, Stack, Text, TextInput, Title } from '@mantine/core';
import { commitScanEntry, updateScanEntry } from '../../api/client';
import type { ScanEntry } from '../../types';
import { FlaggedEntryResolver } from './FlaggedEntryResolver';
import { StockOutInstanceSelector } from './StockOutInstanceSelector';
import { ProvenanceBadge } from '../product/ProvenanceBadge';
import { expiryDateToISOString, formatExpiryDate, isValidUnitCount } from './queueUtils';

// A zero timestamp clears expires_at. JSON null leaves the stored date unchanged.
const CLEARED_EXPIRY_ISO = '0001-01-01T00:00:00.000Z';

const expiryDatePart = (expiresAt: string | null): string =>
  expiresAt === null ? '' : expiresAt.substring(0, 10);

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
  const unitCountRef = useRef(entry.unitCount);
  const [expiryDraft, setExpiryDraft] = useState(expiryDatePart(entry.expiresAt));
  const [expiryCommitted, setExpiryCommitted] = useState(expiryDatePart(entry.expiresAt));
  const [expiryEditorOpen, setExpiryEditorOpen] = useState(false);
  const [expiryError, setExpiryError] = useState('');
  const expiryInputRef = useRef<HTMLInputElement>(null);
  const [instancePickerOpen, setInstancePickerOpen] = useState(false);
  const [showChangeIndicator, setShowChangeIndicator] = useState(true);

  const prefersReducedMotion = () =>
    typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  // Sync draft state when unitCount or expiresAt changes (server updates)
  useEffect(() => {
    // Deliberate prop->draft sync: reset the controlled draft when the server value changes.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setUnitCountDraft(entry.unitCount);
    unitCountRef.current = entry.unitCount;
  }, [entry.unitCount]);

  useEffect(() => {
    // Deliberate prop->draft sync: reset the controlled draft when the server value changes.
    const next = expiryDatePart(entry.expiresAt);
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setExpiryDraft(next);
    setExpiryCommitted(next);
  }, [entry.expiresAt]);

  useEffect(() => {
    if (!expiryEditorOpen) return;
    expiryInputRef.current?.focus();
  }, [expiryEditorOpen]);

  // Show change indicator on mount and when unitCount changes
  useEffect(() => {
    // Intentional transient animation trigger, cleared by the cleanup timer below.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setShowChangeIndicator(true);
    const timer = setTimeout(() => setShowChangeIndicator(false), 600);
    return () => clearTimeout(timer);
  }, [entry.unitCount]);

  const isStockIn = entry.direction === 'stock_in';
  const isStockOut = entry.direction === 'stock_out';
  // Pending confirmation is a list row. Flagged review stays a card.
  const flatRow = entry.status === 'pending';

  const changeIndicatorClass = [
    'scan-entry-card',
    flatRow ? 'scan-entry-row' : undefined,
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
      setApproveError(requestError instanceof Error ? requestError.message : 'Unable to confirm scan.');
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

  const persistUnitCount = async (draft: number) => {
    if (!isValidUnitCount(draft)) {
      unitCountRef.current = entry.unitCount;
      setUnitCountDraft(entry.unitCount);
      return;
    }
    if (draft === entry.unitCount) return;
    try {
      await updateScanEntry(entry.id, { unitCount: draft });
      onChanged();
    } catch {
      setUnitCountError('Unable to update unit count.');
      unitCountRef.current = entry.unitCount;
      setUnitCountDraft(entry.unitCount);
    }
  };

  const handleUnitCountBlur = () => {
    void persistUnitCount(Number(unitCountDraft));
  };

  const adjustUnitCount = (delta: number) => {
    const base = isValidUnitCount(unitCountRef.current) ? unitCountRef.current : entry.unitCount;
    const next = base + delta;
    if (!isValidUnitCount(next)) return;
    unitCountRef.current = next;
    setUnitCountDraft(next);
    setUnitCountError('');
    void persistUnitCount(next);
  };

  const handleUnitCountKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      (event.target as HTMLInputElement).blur();
    }
  };

  const handleUnitCountChange = (value: number | string | null) => {
    const next = value === null || value === '' ? entry.unitCount : Number(value);
    unitCountRef.current = next;
    setUnitCountDraft(next);
    if (value !== null && value !== '') {
      setUnitCountError('');
    }
  };

  const handleExpiryBlur = async () => {
    if (expiryDraft !== expiryCommitted) {
      try {
        const expiresAt = expiryDraft === '' ? CLEARED_EXPIRY_ISO : expiryDateToISOString(expiryDraft);
        await updateScanEntry(entry.id, { expiresAt });
        setExpiryCommitted(expiryDraft);
        onChanged();
      } catch {
        setExpiryError('Unable to update expiration date.');
        setExpiryDraft(expiryCommitted);
        return;
      }
    }
    if (entry.direction === 'stock_in') {
      setExpiryEditorOpen(false);
    }
  };

  const openExpiryEditor = () => {
    setExpiryError('');
    setExpiryEditorOpen(true);
  };

  const closeExpiryEditor = () => {
    setExpiryDraft(expiryCommitted);
    setExpiryError('');
    setExpiryEditorOpen(false);
  };

  const clearExpiry = async () => {
    setExpiryError('');
    try {
      await updateScanEntry(entry.id, { expiresAt: CLEARED_EXPIRY_ISO });
      setExpiryDraft('');
      setExpiryCommitted('');
      setExpiryEditorOpen(false);
      onChanged();
    } catch {
      setExpiryError('Unable to update expiration date.');
    }
  };

  const queueItem = (
    <>
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
            <Text size="xs" c="dimmed" component="span" className="copyable-barcode">Barcode: {entry.barcode}</Text>
            <Text size="xs" c="dimmed" component="span">Scanned: {scannedAtLabel}</Text>
            <ProvenanceBadge externalSource={entry.product?.externalSource} />
          </div>
        </div>
      </div>

      {entry.status === 'pending' && (
        <Group
          className={flatRow ? 'scan-entry-actions scan-entry-row-controls' : 'scan-entry-actions'}
          gap={flatRow ? 6 : 4}
          align="center"
          wrap="wrap"
          justify="flex-start"
        >
          <div className="scan-entry-stepper">
            <button
              type="button"
              className="scan-entry-stepper-btn"
              aria-label="Decrease unit count"
              disabled={!isValidUnitCount(Number(unitCountDraft)) || Number(unitCountDraft) <= 1}
              onClick={() => adjustUnitCount(-1)}
            >
              <span aria-hidden="true">−</span>
            </button>
            <NumberInput
              className="scan-entry-stepper-value"
              hideControls
              min={1}
              step={1}
              allowDecimal={false}
              value={unitCountDraft}
              onChange={handleUnitCountChange}
              onBlur={handleUnitCountBlur}
              onKeyDown={handleUnitCountKeyDown}
              aria-label="Unit count"
            />
            <button
              type="button"
              className="scan-entry-stepper-btn"
              aria-label="Increase unit count"
              onClick={() => adjustUnitCount(1)}
            >
              <span aria-hidden="true">+</span>
            </button>
          </div>
          {entry.direction !== null && (
            <Group gap={4} wrap="nowrap">
              <Button
                size="compact-xs"
                loading={approving}
                onClick={() => void handleApprove()}
              >
                Confirm
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
            </Group>
          )}
          {isStockIn && (
            <div className="scan-entry-expiry-slot">
              {expiryEditorOpen ? (
                <div className="scan-entry-expiry-editor">
                  <TextInput
                    ref={expiryInputRef}
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
                  <button
                    type="button"
                    className="scan-entry-expiry-action"
                    aria-label="Cancel expiration"
                    onMouseDown={(event) => event.preventDefault()}
                    onClick={closeExpiryEditor}
                  >
                    Cancel
                  </button>
                </div>
              ) : expiryCommitted !== '' ? (
                <div className="scan-entry-expiry-summary">
                  <span>Expires {formatExpiryDate(`${expiryCommitted}T00:00:00.000Z`)}</span>
                  <button type="button" className="scan-entry-expiry-action" aria-label="Change expiration" onClick={openExpiryEditor}>
                    Change
                  </button>
                  <button type="button" className="scan-entry-expiry-action" aria-label="Clear expiration" onClick={() => void clearExpiry()}>
                    Clear
                  </button>
                </div>
              ) : (
                <button type="button" className="scan-entry-add-expiry" onClick={openExpiryEditor}>
                  Add expiration
                </button>
              )}
            </div>
          )}
          {isStockOut && (
            <button
              type="button"
              className="scan-entry-more"
              aria-expanded={instancePickerOpen}
              onClick={() => setInstancePickerOpen((open) => !open)}
            >
              More
            </button>
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

      {instancePickerOpen && isStockOut && itemId !== undefined && (
        <div className="scan-entry-extra">
          <StockOutInstanceSelector itemId={itemId} value={instanceId} onChange={setInstanceId} />
        </div>
      )}
      {instancePickerOpen && isStockOut && itemId === undefined && (
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
    </>
  );

  if (flatRow) {
    return (
      <article className={changeIndicatorClass} aria-label={`Scan ${entry.barcode}`}>
        {queueItem}
      </article>
    );
  }

  return (
    <Card component="article" withBorder padding="xs" radius="md" className={changeIndicatorClass} aria-label={`Scan ${entry.barcode}`}>
      {queueItem}
    </Card>
  );
};
