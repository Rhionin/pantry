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
  const isCompactStockIn = isStockIn && entry.status === 'pending';
  const changeIndicatorClass = [
    'scan-entry-card',
    isCompactStockIn ? 'scan-entry-card--stock-in' : undefined,
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

  return (
    <Card
      component="article"
      withBorder
      padding={isCompactStockIn ? 6 : 'xs'}
      radius="md"
      className={changeIndicatorClass}
      aria-label={`Scan ${entry.barcode}`}
    >
      {isCompactStockIn ? (
        <>
          <div className="scan-entry-stock-in-identity" data-layout-row="identity">
            <div className="scan-entry-check">
              <Checkbox
                size="xs"
                aria-label="Select scan for batch approval"
                checked={selected}
                onChange={(event) => onSelectedChange(event.currentTarget.checked)}
              />
            </div>
            <Avatar
              className="scan-entry-avatar"
              src={entry.product?.imageUrl}
              name={productName}
              radius="sm"
              size={28}
            />
            <div className="scan-entry-stock-in-product">
              <Title order={3} size="sm" lineClamp={1} mb={0}>{productName}</Title>
              <Text className="scan-entry-stock-in-barcode" size="xs" c="dimmed" component="span">
                Barcode: {entry.barcode}
              </Text>
              <Text className="scan-entry-stock-in-scanned" size="xs" c="dimmed" component="span">
                Scanned: {scannedAtLabel}
              </Text>
            </div>
            <div className="scan-entry-stock-in-expiry">
              {expiryEditorOpen ? (
                <Text size="xs" c="dimmed" component="span">Expiration</Text>
              ) : expiryCommitted !== '' ? (
                <div className="scan-entry-expiry-summary">
                  <button
                    type="button"
                    className="scan-entry-expiry-action scan-entry-expiry-change"
                    aria-label="Change expiration"
                    onClick={openExpiryEditor}
                  >
                    Expires {formatExpiryDate(`${expiryCommitted}T00:00:00.000Z`)}
                  </button>
                  <button
                    type="button"
                    className="scan-entry-expiry-action scan-entry-expiry-clear"
                    aria-label="Clear expiration"
                    onClick={() => void clearExpiry()}
                  >
                    <span aria-hidden="true">×</span>
                  </button>
                </div>
              ) : (
                <button
                  type="button"
                  className="scan-entry-add-expiry"
                  aria-label="Add expiration"
                  onClick={openExpiryEditor}
                >
                  Add expiration
                </button>
              )}
            </div>
          </div>

          <div className="scan-entry-stock-in-toolbar" data-layout-row="toolbar">
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
            <div className="scan-entry-stock-in-actions">
              <Button
                className="scan-entry-stock-in-action"
                size="compact-sm"
                loading={approving}
                onClick={() => void handleApprove()}
              >
                Approve
              </Button>
              <Button
                className="scan-entry-stock-in-action"
                size="compact-sm"
                variant="light"
                color="red"
                loading={removing}
                onClick={() => void handleRemove()}
              >
                Remove
              </Button>
            </div>
          </div>

          {expiryEditorOpen && (
            <div className="scan-entry-expiry-editor" data-layout-row="expiration">
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
          )}
        </>
      ) : (
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
        </>
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
