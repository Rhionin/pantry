import { describe, expect, it } from 'vitest';
import fc from 'fast-check';
import type { ScanEntry, ScanStatus } from '../../types';
import { mergeScanEvent, pruneSelection } from './queueUtils';

const DISPLAYABLE_SCAN_STATUSES: ScanStatus[] = ['pending', 'flagged'];

const scanEntry = (overrides: Partial<ScanEntry>): ScanEntry => ({
  id: 'scan-1',
  userId: 'user-1',
  barcode: '111',
  scannedAt: '2026-03-20T10:00:00Z',
  direction: null,
  unitCount: 1,
  expiresAt: null,
  status: 'pending',
  productId: null,
  product: null,
  committedAt: null,
  createdAt: '2026-03-20T10:00:00Z',
  ...overrides,
});

describe('mergeScanEvent', () => {
  it('appends a pending event for a new id', () => {
    const entries = [scanEntry({ id: 'existing' })];
    const event = scanEntry({ id: 'new', status: 'pending' });

    expect(mergeScanEvent(entries, event)).toEqual([entries[0], event]);
  });

  it('replaces an existing entry in place with a flagged event', () => {
    const entries = [
      scanEntry({ id: 'a', barcode: '111' }),
      scanEntry({ id: 'b', barcode: '222' }),
      scanEntry({ id: 'c', barcode: '333' }),
    ];
    const event = scanEntry({ id: 'b', barcode: '222', status: 'flagged' });

    expect(mergeScanEvent(entries, event)).toEqual([entries[0], event, entries[2]]);
  });

  it('removes an existing entry when the event is committed', () => {
    const entries = [scanEntry({ id: 'a' }), scanEntry({ id: 'b' })];
    const event = scanEntry({ id: 'a', status: 'committed' });

    expect(mergeScanEvent(entries, event)).toEqual([entries[1]]);
  });

  it('removes an existing entry when the event is cancelled', () => {
    const entries = [scanEntry({ id: 'a' }), scanEntry({ id: 'b' })];
    const event = scanEntry({ id: 'b', status: 'cancelled' });

    expect(mergeScanEvent(entries, event)).toEqual([entries[0]]);
  });

  // Feature: realtime-scan-updates, Property 9: Merging a Scan_Event upserts
  // displayable entries and removes non-displayable ones
  //
  // **Validates: Requirements 4.2, 4.3**
  //
  // For any list of displayed scan entries and any incoming Scan_Event, mergeScanEvent
  // SHALL produce a list containing an entry with the event's id, equal to the event,
  // exactly once when the event's status is pending or flagged, and SHALL produce a list
  // containing no entry with the event's id when the event's status is committed or
  // cancelled; in both cases every other entry already in the list SHALL remain unchanged
  // and in the same relative order.
  it('Property 9: upserts displayable entries and removes non-displayable ones', () => {
    const statusArbitrary: fc.Arbitrary<ScanStatus> = fc.constantFrom(
      'pending',
      'flagged',
      'committed',
      'cancelled',
    );
    const entryArbitrary = fc.record({
      id: fc.uuid(),
      status: statusArbitrary,
    }).map(({ id, status }) => scanEntry({ id, status }));

    fc.assert(fc.property(
      fc.uniqueArray(entryArbitrary, { maxLength: 20, selector: (entry) => entry.id }),
      entryArbitrary,
      (entries, event) => {
        const result = mergeScanEvent(entries, event);
        const others = entries.filter((entry) => entry.id !== event.id);
        const matchesForEventId = result.filter((entry) => entry.id === event.id);

        if (DISPLAYABLE_SCAN_STATUSES.includes(event.status)) {
          expect(matchesForEventId).toEqual([event]);
        } else {
          expect(matchesForEventId).toEqual([]);
        }
        expect(result.filter((entry) => entry.id !== event.id)).toEqual(others);
      },
    ), { numRuns: 100 });
  });
});

describe('pruneSelection', () => {
  it('drops an id no longer present in the entries list and keeps the rest', () => {
    const entries = [scanEntry({ id: 'a' }), scanEntry({ id: 'c' })];

    expect(pruneSelection(['a', 'b', 'c'], entries)).toEqual(['a', 'c']);
  });

  // Feature: realtime-scan-updates, Property 10: Removing an entry from the displayed
  // list prunes it from the selection
  //
  // **Validates: Requirements 4.4**
  //
  // For any list of selected entry ids and any list of currently displayed entries,
  // pruneSelection SHALL produce a list containing exactly the ids from the input that
  // name an entry present in the displayed list, in the same relative order, and no others.
  it('Property 10: keeps exactly the selected ids present in the displayed list, in order', () => {
    fc.assert(fc.property(
      fc.array(fc.uuid(), { maxLength: 20 }),
      fc.array(fc.uuid(), { maxLength: 20 }).map((ids) => ids.map((id) => scanEntry({ id }))),
      (selectedIds, entries) => {
        const expected = selectedIds.filter((id) => entries.some((entry) => entry.id === id));

        expect(pruneSelection(selectedIds, entries)).toEqual(expected);
      },
    ), { numRuns: 100 });
  });
});
