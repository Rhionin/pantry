import { describe, expect, it } from 'vitest';
import fc from 'fast-check';
import type { ScanEntry, ScanStatus } from '../../types';
import { getEntriesForView, isValidUnitCount, mergeScanEvent, pruneSelection, toggleSelectAll, isBatchEligible } from './queueUtils';

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
describe('isValidUnitCount', () => {
  it('returns false for zero', () => {
    expect(isValidUnitCount(0)).toBe(false);
  });

  it('returns true for one', () => {
    expect(isValidUnitCount(1)).toBe(true);
  });

  it('returns false for negative numbers', () => {
    expect(isValidUnitCount(-1)).toBe(false);
  });

  it('returns false for decimal numbers', () => {
    expect(isValidUnitCount(1.5)).toBe(false);
  });

  // Feature: scan-queue-polish, Property 1: Unit count validity
  //
  // **Validates: Requirements 2.6**
  //
  // For any number, isValidUnitCount SHALL return true if and only if that number
  // is an integer greater than or equal to one.
  it('Property 1: returns true for positive integers and false otherwise', () => {
    const numberArbitrary = fc.integer();

    fc.assert(fc.property(
      numberArbitrary,
      (value) => {
        const expected = Number.isInteger(value) && value >= 1;
        expect(isValidUnitCount(value)).toBe(expected);
      },
    ), { numRuns: 100 });
  });
});

describe('isBatchEligible', () => {
  it('returns true for pending entries with direction set', () => {
    const entry = scanEntry({ status: 'pending', direction: 'stock_in' });
    expect(isBatchEligible(entry)).toBe(true);
  });

  it('returns false for pending entries with direction null', () => {
    const entry = scanEntry({ status: 'pending', direction: null });
    expect(isBatchEligible(entry)).toBe(false);
  });

  it('returns false for flagged entries with direction set', () => {
    const entry = scanEntry({ status: 'flagged', direction: 'stock_out' });
    expect(isBatchEligible(entry)).toBe(false);
  });

  it('returns false for committed entries', () => {
    const entry = scanEntry({ status: 'committed', direction: 'stock_in' });
    expect(isBatchEligible(entry)).toBe(false);
  });

  it('returns false for cancelled entries', () => {
    const entry = scanEntry({ status: 'cancelled', direction: 'stock_out' });
    expect(isBatchEligible(entry)).toBe(false);
  });
});

describe('toggleSelectAll', () => {
  it('selects all eligible entries when none are selected', () => {
    const entries = [
      scanEntry({ id: 'a', status: 'pending', direction: 'stock_in' }),
      scanEntry({ id: 'b', status: 'pending', direction: 'stock_out' }),
      scanEntry({ id: 'c', status: 'pending', direction: null }),
      scanEntry({ id: 'd', status: 'flagged', direction: 'stock_in' }),
    ];

    expect(toggleSelectAll(entries, [])).toEqual(['a', 'b']);
  });

  it('deselects all eligible entries when all are selected', () => {
    const entries = [
      scanEntry({ id: 'a', status: 'pending', direction: 'stock_in' }),
      scanEntry({ id: 'b', status: 'pending', direction: 'stock_out' }),
    ];

    expect(toggleSelectAll(entries, ['a', 'b'])).toEqual([]);
  });

  it('preserves non-eligible entries that were already selected', () => {
    const entries = [
      scanEntry({ id: 'a', status: 'pending', direction: 'stock_in' }),
      scanEntry({ id: 'b', status: 'flagged', direction: null }),
      scanEntry({ id: 'c', status: 'pending', direction: 'stock_out' }),
    ];

    expect(toggleSelectAll(entries, ['b'])).toEqual(['b', 'a', 'c']);
  });

  it('adds all eligible entries when only some are already selected', () => {
    const entries = [
      scanEntry({ id: 'a', status: 'pending', direction: 'stock_in' }),
      scanEntry({ id: 'b', status: 'flagged', direction: null }),
      scanEntry({ id: 'c', status: 'pending', direction: 'stock_out' }),
    ];

    // When not all eligible are selected, all eligible are added while preserving selected non-eligible
    // selectedIds=['b','a'] means 'b' (non-eligible) is selected and 'a' (eligible) is selected
    // Result: 'b' preserved, 'a' already in result, 'c' added as eligible
    expect(toggleSelectAll(entries, ['b', 'a'])).toEqual(['b', 'a', 'c']);
  });

  it('preserves the order of non-eligible entries', () => {
    const entries = [
      scanEntry({ id: 'a', status: 'pending', direction: 'stock_in' }),
      scanEntry({ id: 'b', status: 'flagged', direction: null }),
      scanEntry({ id: 'c', status: 'pending', direction: 'stock_out' }),
      scanEntry({ id: 'd', status: 'flagged', direction: 'stock_out' }),
    ];

    expect(toggleSelectAll(entries, ['b', 'd'])).toEqual(['b', 'd', 'a', 'c']);
  });

  // Feature: scan-queue-polish, Property 2: Select-all toggles exactly the eligible
  // entries and preserves the rest
  //
  // **Validates: Requirements 3.2, 3.3, 3.4**
  //
  // For any list of displayed scan entries and any current batch selection,
  // toggleSelectAll SHALL produce a selection in which every displayed
  // Batch_Eligible_Entry is selected if and only if at least one Batch_Eligible_Entry
  // was unselected beforehand, and in which the selected/unselected state of every
  // entry that is not a Batch_Eligible_Entry is unchanged from the input selection.
  it('Property 2: toggles eligible entries and preserves non-eligible entries', () => {
    const directionArbitrary = fc.oneof(
      fc.constant('stock_in'),
      fc.constant('stock_out'),
      fc.constant(null),
    );
    const statusArbitrary = fc.oneof(
      fc.constant('pending'),
      fc.constant('flagged'),
      fc.constant('committed'),
      fc.constant('cancelled'),
    );
    const entryArbitrary = fc.record({
      id: fc.uuid(),
      status: statusArbitrary,
      direction: directionArbitrary,
    }).map(({ id, status, direction }) => scanEntry({ id, status, direction }));

    fc.assert(fc.property(
      fc.uniqueArray(entryArbitrary, { maxLength: 20, selector: (entry) => entry.id }),
      fc.array(fc.uuid(), { maxLength: 20 }),
      (entries, selectedIds) => {
        const eligibleIds = entries.filter(isBatchEligible).map((entry) => entry.id);
        const nonEligibleIds = entries.filter((entry) => !isBatchEligible).map((entry) => entry.id);

        const result = toggleSelectAll(entries, selectedIds);

        // Verify non-eligible entries are preserved in order
        const preservedNonEligible = selectedIds.filter((id) => nonEligibleIds.includes(id));
        const resultNonEligible = result.filter((id) => nonEligibleIds.includes(id));
        expect(resultNonEligible).toEqual(preservedNonEligible);

        // Verify eligible entries are all selected or none selected
        const selectedEligible = result.filter((id) => eligibleIds.includes(id));
        const previouslySelectedEligible = selectedIds.filter((id) => eligibleIds.includes(id));

        if (previouslySelectedEligible.length === eligibleIds.length && eligibleIds.length > 0) {
          // If all eligible were selected, none should be selected now
          expect(selectedEligible).toEqual([]);
        } else {
          // Otherwise, all eligible should be selected
          expect(selectedEligible.sort()).toEqual(eligibleIds.sort());
        }
      },
    ), { numRuns: 100 });
  });
});
describe('getEntriesForView', () => {
  // Feature: scan-queue-polish, Property 3: Every entry belongs to exactly one queue view
  //
  // **Validates: Requirements 9.1, 9.2, 9.3, 9.4**
  //
  // For any list of scan entries and either queue view, getEntriesForView partitions the entries
  // so that an entry appears in the Stock_In_View result if and only if its direction equals
  // 'stock_in', appears in the Stock_Out_View result if and only if its direction equals
  // 'stock_out' or is null, and appears in exactly one of the two results — never both,
  // never neither.

  it('Property 3: partitions entries so each belongs to exactly one view', () => {
    const directionArbitrary = fc.oneof(
      fc.constant('stock_in'),
      fc.constant('stock_out'),
      fc.constant(null),
    );
    const entryArbitrary = fc.record({
      id: fc.uuid(),
      direction: directionArbitrary,
    }).map(({ id, direction }) => scanEntry({ id, direction }));

    fc.assert(fc.property(
      fc.array(entryArbitrary, { maxLength: 20 }),
      (entries) => {
        const stockInEntries = getEntriesForView(entries, 'stock_in');
        const stockOutEntries = getEntriesForView(entries, 'stock_out');

        // Every entry appears in exactly one of the two results
        entries.forEach((entry) => {
          const inStockIn = stockInEntries.some((e) => e.id === entry.id);
          const inStockOut = stockOutEntries.some((e) => e.id === entry.id);

          if (entry.direction === 'stock_in') {
            expect(inStockIn).toBe(true);
            expect(inStockOut).toBe(false);
          } else {
            expect(inStockIn).toBe(false);
            expect(inStockOut).toBe(true);
          }
        });

        // The two results are disjoint
        stockInEntries.forEach((entry) => {
          expect(stockOutEntries.some((e) => e.id === entry.id)).toBe(false);
        });

        // Together they cover all input entries
        const allViewEntries = [...stockInEntries, ...stockOutEntries];
        expect(allViewEntries.length).toBe(entries.length);
        entries.forEach((entry) => {
          expect(allViewEntries.some((e) => e.id === entry.id)).toBe(true);
        });
      },
    ), { numRuns: 100 });
  });
});
