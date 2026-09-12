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
