import type { ItemInstanceWithStatus, ScanEntry, ScanStatus } from '../../types';

const DISPLAYABLE_SCAN_STATUSES: ScanStatus[] = ['pending', 'flagged'];

export const sortScansChronologically = (entries: ScanEntry[]): ScanEntry[] =>
  [...entries].sort(
    (left, right) =>
      new Date(left.scannedAt).getTime() - new Date(right.scannedAt).getTime(),
  );

export const sortInstancesUseOldestFirst = (
  instances: ItemInstanceWithStatus[],
): ItemInstanceWithStatus[] =>
  [...instances].sort((left, right) => {
    if (left.expiresAt === null) return right.expiresAt === null ? left.id.localeCompare(right.id) : 1;
    if (right.expiresAt === null) return -1;
    const dateOrder = new Date(left.expiresAt).getTime() - new Date(right.expiresAt).getTime();
    return dateOrder === 0 ? left.id.localeCompare(right.id) : dateOrder;
  });

export const formatExpiryDate = (value: string): string =>
  new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeZone: 'UTC' }).format(
    new Date(value),
  );

export const expiryDateToISOString = (expiryDate: string): string | undefined =>
  expiryDate === '' ? undefined : new Date(`${expiryDate}T00:00:00.000Z`).toISOString();

// A Batch_Eligible_Entry is a pending entry whose direction has been
// determined. Requirement 3.
export const isBatchEligible = (entry: ScanEntry): boolean =>
  entry.status === 'pending' && entry.direction !== null;

// Computes the next batch selection when the Select_All_Control is
// activated. If every displayed Batch_Eligible_Entry is currently selected,
// deselects all of them; otherwise selects all of them. Ids belonging to
// entries that are not Batch_Eligible_Entry are left untouched either way.
// Requirements 3.2, 3.3, 3.4.
export const toggleSelectAll = (entries: ScanEntry[], selectedIds: string[]): string[] => {
  const eligibleIds = entries.filter(isBatchEligible).map((entry) => entry.id);
  const allEligibleSelected = eligibleIds.length > 0 && eligibleIds.every((id) => selectedIds.includes(id));
  const preserved = selectedIds.filter((id) => !eligibleIds.includes(id));
  return allEligibleSelected ? preserved : [...new Set([...preserved, ...eligibleIds])];
};

// A confirmed unit-count entry is only accepted when it is a positive whole
// number. Requirement 2.6.
export const isValidUnitCount = (value: number): boolean =>
  Number.isInteger(value) && value >= 1;

// Partitions displayed entries into the two Queue_Direction_Tabs views.
// Stock_In_View is exactly `direction === 'stock_in'`; Stock_Out_View is the
// catchall for `direction === 'stock_out'` and `direction === null`, so a
// flagged/undirected entry always has somewhere to be reviewed. Every entry
// belongs to exactly one view — this is a total partition, not a filter that
// can drop entries. Requirements 9.1, 9.2, 9.3, 9.4.
export type QueueView = 'stock_in' | 'stock_out';

export const getEntriesForView = (entries: ScanEntry[], view: QueueView): ScanEntry[] =>
  view === 'stock_in'
    ? entries.filter((entry) => entry.direction === 'stock_in')
    : entries.filter((entry) => entry.direction === 'stock_out' || entry.direction === null);

// Applies one incoming Scan_Event to the currently displayed list: upserts it
// if its status is still displayable (pending/flagged), or removes any entry
// with the same id if it isn't (committed/cancelled). Requirements 4.2, 4.3.
export const mergeScanEvent = (entries: ScanEntry[], event: ScanEntry): ScanEntry[] => {
  if (!DISPLAYABLE_SCAN_STATUSES.includes(event.status)) {
    return entries.filter((entry) => entry.id !== event.id);
  }
  const index = entries.findIndex((entry) => entry.id === event.id);
  if (index === -1) return [...entries, event];
  return entries.map((entry) => (entry.id === event.id ? event : entry));
};

// Drops any id from a selection that no longer names a displayed entry.
// Requirement 4.4.
export const pruneSelection = (selectedIds: string[], entries: ScanEntry[]): string[] =>
  selectedIds.filter((id) => entries.some((entry) => entry.id === id));
