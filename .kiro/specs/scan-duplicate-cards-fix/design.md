# Scan Duplicate Cards Fix Bugfix Design

## Overview

`Queue.CreateScanEntry` (`internal/scan/scan.go`) unconditionally inserts a new
`scan_entries` row for every scan it's given. Both entry points funnel
through this one method — the web scan-in field via
`internal/server/handler_scan_create.go`, and the headless barcode-scanner
listener via `internal/scanlistener/listener.go` — so neither one currently
has any notion of "this barcode is already sitting in the queue."

The fix adds a merge-or-create check at the top of `CreateScanEntry`: before
inserting, look up an existing `pending` or `flagged` entry for the same
`user_id` + `barcode` + `direction`. If one exists, increment its
`unit_count` through the existing `UpdateScanEntry` path (leaving
`scanned_at` untouched) and return that entry instead of inserting a new
row. If none exists, fall through to today's insert behavior unchanged.
Because the check lives inside `CreateScanEntry` itself, both entry points
get the new behavior for free with no changes to either caller.

## Glossary

- **Bug_Condition (C)**: A scan entry is being created for a barcode/user/direction
  combination that already has a mergeable entry (status `pending` or `flagged`)
  in the queue.
- **Property (P)**: The existing mergeable entry's `unit_count` is incremented by
  the new scan's unit count, its `scanned_at` is left unchanged, and it — not a
  new row — is the entry returned and broadcast.
- **Preservation**: The existing insert-a-new-entry behavior, unchanged for every
  input where no mergeable entry exists (different direction, different user,
  only committed/cancelled entries, or no prior entry at all).
- **Mergeable entry**: An existing `scan_entries` row with the same `user_id`,
  `barcode`, and `direction` as the incoming scan, and `status` of `pending` or
  `flagged`.
- **CreateScanEntry**: The method in `internal/scan/scan.go` that both entry
  points call to record a scan. Currently always inserts; this is the fix site.
- **findMergeableScanEntry**: The new query method added to `Queue` that looks
  up the mergeable entry described above.

## Bug Details

### Bug Condition

The bug manifests whenever `CreateScanEntry` is called with a barcode/user/direction
tuple for which a `pending` or `flagged` entry already exists in `scan_entries`.
Instead of recognizing that entry as still "open" for merging, `CreateScanEntry`
inserts a brand-new row with `unit_count` 1, so the queue ends up with N
one-count cards for N scans of the same item instead of one N-count card.

**Formal Specification:**
```
FUNCTION isBugCondition(input)
  INPUT: input of type ScanEntry (the entry about to be created)
  OUTPUT: boolean

  existing := findMergeableScanEntry(input.UserID, input.Barcode, input.Direction)
  // mergeable := status IN ('pending', 'flagged')

  RETURN existing != NULL
END FUNCTION
```

### Examples

- A user scans barcode `111` for stock-in. No prior entry exists → a `pending`
  entry E1 is created with `unit_count = 1`. The user scans `111` again before
  resolving E1. Current (buggy) behavior: a second entry E2 is created with
  `unit_count = 1`, so the queue shows two one-count cards for `111`. Expected:
  E1's `unit_count` becomes 2 and no E2 is created.
- Barcode `222` isn't recognized by product lookup, so entry E1 is created
  `flagged` with `unit_count = 1`. The user rescans `222` before resolving the
  flag. Current (buggy) behavior: a second flagged entry E2 is created.
  Expected: E1's `unit_count` becomes 2.
- Barcode `111` is scanned 5 times in a row for stock-in before the user opens
  the queue. Current (buggy) behavior: 5 separate one-count cards. Expected:
  1 card with `unit_count = 5`, `scanned_at` equal to the *first* scan's
  timestamp.
- Edge case: barcode `333` has a `pending` stock-in entry, then the user
  switches the scanner to stock-out mode and scans `333` again. Because the
  direction differs, this must NOT merge — a new stock-out entry is created
  (see Preservation Requirements).

## Expected Behavior

### Preservation Requirements

**Unchanged Behaviors:**
- Creating the very first scan entry for a barcode/user/direction (no existing
  entry at all) continues to insert a new row with `unit_count` 1 and status
  derived from the product lookup, exactly as today.
- Scans for a different `direction` on the same barcode/user never merge with
  an existing entry — a new, separate entry is created.
- Scans for a different `user_id` on the same barcode never merge with another
  user's existing entry — a new, separate entry is created.
- When the only existing entries for a barcode/user/direction are `committed`
  or `cancelled`, a new entry is created rather than merging into a closed entry.
- The web scan-in field and the headless listener continue to apply identical
  behavior, since both still funnel through the same `CreateScanEntry` method.
- `UpdateScanEntry` remains the single allowed UPDATE path for scan entries;
  the merge path reuses it rather than issuing its own UPDATE.

**Scope:**
All inputs that do NOT have a mergeable entry (per the bug condition above)
should be completely unaffected by this fix. This includes:
- First-time scans of a barcode for a given user/direction
- Scans whose only prior entries differ by direction or user
- Scans whose only prior entries are already `committed` or `cancelled`

## Hypothesized Root Cause

1. **No pre-insert lookup**: `CreateScanEntry` never queries `scan_entries`
   before inserting — it has no way of knowing a mergeable entry exists,
   because the method was designed only for the "create" case.
2. **No merge concept in the data-access layer**: There's no query method on
   `Queue` that finds an existing entry by `user_id` + `barcode` + `direction`
   + status, so there's currently no building block a merge check could use.
3. **Both entry points call the same under-powered method**: `handler_scan_create.go`
   and `scanlistener/listener.go` both call `CreateScanEntry` expecting
   "create," which is consistent with each other but consistently wrong for
   repeat scans — confirming this is a single-fix-point bug, not two
   independent ones.

## Correctness Properties

Property 1: Bug Condition - Merge Into Existing Pending/Flagged Entry

_For any_ scan entry creation request where a mergeable entry exists (same
`user_id`, `barcode`, `direction`, and status `pending` or `flagged`), the
fixed `CreateScanEntry` SHALL increment that existing entry's `unit_count` by
the new scan's unit count (defaulting to 1), leave its `scanned_at` unchanged,
NOT insert a new row, and return/broadcast the updated existing entry.

**Validates: Requirements 2.1, 2.2, 2.3**

Property 2: Preservation - Insert-New-Entry Behavior

_For any_ scan entry creation request where no mergeable entry exists (no
prior entry, a different direction, a different user, or only
committed/cancelled entries for that barcode/user/direction), the fixed
`CreateScanEntry` SHALL produce exactly the same result as the original
function: a newly inserted row with unit count as given (default 1) and
status derived from the product lookup, returned and broadcast as today.

**Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5**

## Fix Implementation

### Changes Required

**File**: `internal/scan/scan.go`

**Function**: `Queue.CreateScanEntry`

**Specific Changes**:

1. **New query method `findMergeableScanEntry`**: Add a method on `Queue`
   that looks up a mergeable entry:
   ```go
   func (r *Queue) findMergeableScanEntry(ctx context.Context, userID, barcode string, direction *ScanDirection) (*ScanEntry, error)
   ```
   It runs a `SELECT` against `scan_entries` (joined with `products`, reusing
   `scanScanEntry`) with:
   ```sql
   WHERE se.user_id = ?
     AND se.barcode = ?
     AND se.direction IS ?
     AND se.status IN ('pending', 'flagged')
   ORDER BY se.scanned_at ASC, se.id ASC
   LIMIT 1
   ```
   `direction IS ?` is used instead of `=` so a `NULL` direction on the
   incoming entry only matches an existing entry whose direction is also
   `NULL` (SQLite's `IS` is null-safe equality), which keeps direction an
   exact-match key including the not-yet-set case. Ordering by `scanned_at`
   ascending (oldest first) is the defensive tie-breaker requested: if more
   than one mergeable entry somehow exists, the earliest one is treated as
   the "original" so its timestamp is the one preserved. `se.id ASC` breaks
   ties deterministically if `scanned_at` values are identical.

2. **Merge-or-create branch in `CreateScanEntry`**: Before the existing
   `INSERT`, call `findMergeableScanEntry(ctx, entry.UserID, entry.Barcode, entry.Direction)`.
   - If it returns an existing entry:
     - Compute the unit delta: `entry.UnitCount` if positive, else `1`
       (matches the existing default-to-1 convention used elsewhere, e.g.
       `NewEntryFromLookup`).
     - Call `r.UpdateScanEntry(ctx, existing.ID, nil, &newUnitCount, nil, nil, nil)`
       with `newUnitCount = existing.UnitCount + delta`. Passing `nil` for
       direction, `expiresAt`, `productID`, and `status` leaves them (and,
       critically, `scanned_at`, which `UpdateScanEntry` never touches)
       unchanged.
     - `UpdateScanEntry` already re-fetches the row and calls
       `Broadcaster.PublishScanEvent` when set, so the merge path gets a
       Scan_Event for free — no separate publish call needed.
     - Return the entry `UpdateScanEntry` leaves current (fetch via
       `r.GetScanEntry(ctx, existing.ID)`) instead of falling through to the
       insert.
   - If no existing entry is found, fall through to the current insert logic
     unchanged (including its own `GetScanEntry` + `PublishScanEvent` call).

3. **No changes to `handler_scan_create.go` or `scanlistener/listener.go`**:
   both already call `Queue.CreateScanEntry`, so they pick up the new
   behavior automatically.

## Testing Strategy

### Validation Approach

The testing strategy follows a two-phase approach: first, surface
counterexamples that demonstrate the bug on unfixed code, then verify the fix
works correctly and preserves existing behavior. All tests live in
`internal/scan/scan_test.go`, following the `newTestQueue` pattern
(in-memory SQLite via `app.RunMigrations`) already established there.

### Exploratory Bug Condition Checking

**Goal**: Surface counterexamples that demonstrate the bug BEFORE implementing
the fix. Confirm or refute the root cause analysis.

**Test Plan**: Call `CreateScanEntry` twice with the same `user_id`,
`barcode`, and `direction` while the first entry is still `pending` (and
again while it's `flagged`), then list entries for that user and observe two
separate one-count rows instead of one two-count row. Run against the
UNFIXED code to confirm the failure mode.

**Test Cases**:
1. **Repeat pending scan**: create entry with status `pending`, create a
   second entry with identical `user_id`/`barcode`/`direction` (will produce
   2 rows on unfixed code).
2. **Repeat flagged scan**: same as above but the product lookup misses, so
   status is `flagged` (will produce 2 rows on unfixed code).
3. **Timestamp check**: create entry at `t0`, "rescan" at `t1` (will produce
   a second row with its own `scanned_at = t1` on unfixed code, rather than
   `t0` being preserved on a single merged row).

**Expected Counterexamples**:
- Two `scan_entries` rows instead of one, each `unit_count = 1`.
- Possible causes: no pre-insert lookup, no merge query method, both entry
  points relying on the same under-powered `CreateScanEntry`.

### Fix Checking

**Goal**: Verify that for all inputs where the bug condition holds, the fixed
function produces the expected behavior.

**Pseudocode:**
```
FOR ALL entry WHERE isBugCondition(entry) DO
  result := CreateScanEntry_fixed(entry)
  ASSERT result.ID == existingMergeableEntry.ID
  ASSERT result.UnitCount == existingMergeableEntry.UnitCount_before + max(entry.UnitCount, 1)
  ASSERT result.ScannedAt == existingMergeableEntry.ScannedAt_before
  ASSERT countRowsFor(entry.UserID, entry.Barcode, entry.Direction) unchanged (no new row)
END FOR
```

### Preservation Checking

**Goal**: Verify that for all inputs where the bug condition does NOT hold,
the fixed function produces the same result as the original function.

**Pseudocode:**
```
FOR ALL entry WHERE NOT isBugCondition(entry) DO
  ASSERT CreateScanEntry_fixed(entry) inserts a new row identical in shape
         to what CreateScanEntry_original(entry) would have inserted
END FOR
```

**Testing Approach**: Property-based testing is recommended for preservation
checking because it can generate many combinations of direction/user/status
mismatches automatically and catch a merge condition that's accidentally too
broad (e.g. matching across directions).

**Test Plan**: Observe on UNFIXED code that creating entries with a different
direction, a different user, or only committed/cancelled prior entries always
inserts a new row, then write tests (including property-based ones) asserting
that continues to hold after the fix.

**Test Cases**:
1. **No prior entry**: first scan of a barcode/user/direction still creates a
   `unit_count = 1` row with status derived from lookup.
2. **Different direction**: existing `pending` stock-in entry, new scan is
   stock-out for the same barcode/user → new row created, stock-in entry
   untouched.
3. **Different user**: existing `pending` entry for user A, same barcode/direction
   scanned by user B → new row created for user B, user A's entry untouched.
4. **Only committed/cancelled entries exist**: existing entry for the same
   barcode/user/direction is `committed` (and, separately, `cancelled`) → new
   row created rather than merging into the closed entry.
5. **Broadcaster behavior preserved**: a `Broadcaster` stub records the
   entries it's asked to publish; assert exactly one `PublishScanEvent` call
   per `CreateScanEntry` call, carrying the merged entry on a merge and the
   newly created entry otherwise.

### Unit Tests

- `findMergeableScanEntry` returns the right entry for exact-match
  user/barcode/direction/status, and `nil` when no match (including when
  direction is `nil` on both the query and a candidate row).
- `CreateScanEntry` merges into a `pending` entry, incrementing `unit_count`
  and leaving `scanned_at` unchanged.
- `CreateScanEntry` merges into a `flagged` entry the same way.
- `CreateScanEntry` does not merge across direction, user, or
  committed/cancelled status (see Preservation test cases above).
- `CreateScanEntry` defaults a zero/unset `UnitCount` on the incoming scan to
  a delta of 1 when merging, matching today's create-path default.

### Property-Based Tests

- Generate random sequences of scans (varying barcode, user, direction,
  lookup hit/miss) and assert the final `scan_entries` row count and total
  `unit_count` per barcode/user/direction group matches the number of scans
  that should have merged versus not.
- Generate random existing-entry statuses (`pending`, `flagged`, `committed`,
  `cancelled`) and assert merging only occurs for `pending`/`flagged`.
- Generate random direction pairings (same, different, both nil) and assert
  merging only occurs when directions are identical (including nil-to-nil).

### Integration Tests

- Full flow: two HTTP `POST` scan-create calls for the same barcode via
  `handler_scan_create.go` result in one queue entry with `unit_count = 2`.
- Full flow: two lines fed through `scanlistener.ScanListener` for the same
  barcode/mode result in one queue entry with `unit_count = 2`, confirming
  parity between the two entry points.
- Confirm a `Scan_Event` is broadcast exactly once per scan, carrying the
  merged entry's updated `unit_count` on merge.
