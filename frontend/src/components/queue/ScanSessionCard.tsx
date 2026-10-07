import { useId, type ReactNode } from 'react';
import { Card, Checkbox, VisuallyHidden } from '@mantine/core';
import type { ScanEntry } from '../../types';
import {
  batchTimeBounds,
  formatBatchScanCount,
  formatBatchTimeRange,
  isBatchEligible,
  toggleSelectAll,
} from './queueUtils';

export interface ScanSessionCardProps {
  entries: ScanEntry[];
  expanded: boolean;
  onToggleExpanded: () => void;
  directionLabel: string;
  selectedIds: string[];
  onSelectionChange: (selectedIds: string[]) => void;
  flat?: boolean;
  children: ReactNode;
}

export const ScanSessionCard = ({
  entries,
  expanded,
  onToggleExpanded,
  directionLabel,
  selectedIds,
  onSelectionChange,
  flat = false,
  children,
}: ScanSessionCardProps) => {
  const panelId = useId();
  const { earliest, latest } = batchTimeBounds(entries);
  const timeRange = formatBatchTimeRange(earliest, latest);
  const countLabel = formatBatchScanCount(entries.length);
  const flagged = entries.some((entry) => entry.status === 'flagged');
  const eligibleCount = entries.filter(isBatchEligible).length;
  const allEligibleSelected = eligibleCount > 0
    && entries.filter(isBatchEligible).every((entry) => selectedIds.includes(entry.id));

  const className = [
    'scan-session-card',
    expanded ? 'scan-session-card--expanded' : 'scan-session-card--collapsed',
    flagged ? 'scan-session-card--flagged' : undefined,
  ].filter(Boolean).join(' ');

  return (
    <Card
      component="section"
      withBorder
      padding={0}
      radius="md"
      className={className}
      aria-label={`Scan session ${timeRange}`}
    >
      <button
        type="button"
        className="scan-session-toggle"
        aria-expanded={expanded}
        aria-controls={expanded ? panelId : undefined}
        onClick={onToggleExpanded}
      >
        <span className="scan-session-toggle-copy">
          {expanded ? (
            <>
              <span className="scan-session-time">{timeRange}</span>
              <span className="scan-session-meta">{countLabel} · {directionLabel}</span>
            </>
          ) : (
            <span className="scan-session-time">
              {`${timeRange} · ${countLabel}`}<VisuallyHidden>{`, ${directionLabel}`}</VisuallyHidden>
            </span>
          )}
        </span>
        <span className="scan-session-chevron" aria-hidden="true" />
      </button>
      {expanded && (
        <div id={panelId} className="scan-session-body">
          <Checkbox
            className="scan-session-select"
            size="xs"
            label="Select all"
            aria-label={`Select all eligible scans in ${timeRange}`}
            checked={allEligibleSelected}
            disabled={eligibleCount === 0}
            onChange={() => onSelectionChange(toggleSelectAll(entries, selectedIds))}
          />
          <div className={flat ? 'scan-entry-list scan-entry-list--flat' : 'scan-session-grid'}>
            {children}
          </div>
        </div>
      )}
    </Card>
  );
};
