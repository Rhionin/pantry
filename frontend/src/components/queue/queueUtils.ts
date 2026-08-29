import type { ItemInstanceWithStatus, ScanEntry } from '../../types';

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
