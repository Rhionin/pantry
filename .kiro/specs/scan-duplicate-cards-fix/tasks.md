# Implementation Plan

## Overview

This plan fixes duplicate scan-queue cards by adding a merge-or-create check to
`Queue.CreateScanEntry` (`internal/scan/scan.go`). It follows the exploratory bugfix
workflow: surface counterexamples on the UNFIXED code first (Task 1), lock in the
behavior that must not change (Task 2), then apply the fix — a new
`findMergeableScanEntry` query method plus a merge-or-create branch — and re-run the
same tests to confirm the bug is resolved with no regressions (Task 3). Per AGENTS.md,
tests live in `internal/scan/scan_test.go` (table-driven, `newTestQueue`) and
`internal/scan/scan_properties_test.go` (`rapid`, modeled on the existing properties
in that file); integration coverage closes the loop through both entry points — the
HTTP handler (`internal/server/handler_scan_create_test.go`, `handlerTestCase` /
`runHandlerTests` / `exchanges()`) and the headless listener
(`internal/scanlistener/listener_test.go`, `fakeQueue`) — since `design.md` requires
both to pick up the fix with no caller changes.

The **Bug Condition** is `findMergeableScanEntry(userID, barcode, direction) != nil`
(design "Bug Details" — a `pending` or `flagged` entry already exists for the same
`user_id` + `barcode` + `direction`). The fix increments that entry's `unit_count`
through the existing `UpdateScanEntry` path instead of inserting a new row.

---

## Tasks

- [x] 1. Write bug condition exploration tests (BEFORE implementing the fix)
  - **Property 1: Bug Condition** - Repeat Scan Creates A Duplicate Instead Of Merging
  - **CRITICAL**: These tests MUST FAIL on the UNFIXED code - failure confirms the bug exists
  - **DO NOT attempt to fix the test or the code when it fails at this stage**
  - **NOTE**: These tests encode the expected behavior - they will validate the fix when they pass after implementation
  - **GOAL**: Surface counterexamples showing N repeat scans of the same
    user/barcode/direction produce N one-count rows instead of one N-count row
  - **Scoped PBT Approach**: This is a deterministic bug — scope the exploration to
    concrete failing cases (a `pending` entry, a `flagged` entry, and a timestamp
    check) rather than the full input domain, matching design.md's "Exploratory Bug
    Condition Checking" test plan
  - Add table-driven cases to `internal/scan/scan_test.go` using `newTestQueue`:
    - **Repeat pending scan**: call `CreateScanEntry` twice with identical
      `UserID`/`Barcode`/`Direction` while the first entry is still `pending`; assert
      (on unfixed code) two rows exist via `ListScanEntries`, each `UnitCount == 1`
    - **Repeat flagged scan**: same shape, but the lookup misses so the first entry is
      `flagged`; assert (on unfixed code) two `flagged` rows exist
    - **Timestamp check**: create at `t0`, "rescan" at `t1`; assert (on unfixed code)
      the second row's `ScannedAt == t1` rather than `t0` being preserved on a single
      merged row
  - Run the tests on UNFIXED code
  - **EXPECTED OUTCOME**: All three cases FAIL (this is correct — it proves the bug exists)
  - Document the counterexamples found (e.g., "two `CreateScanEntry` calls with the
    same user/barcode/direction while the first is `pending` produce two rows with
    `unit_count = 1` each, instead of one row with `unit_count = 2`")
  - Mark this task complete when the tests are written, run, and their failure is documented
  - _Requirements: 1.1, 1.2, 1.3_

- [x] 2. Write preservation property tests (BEFORE implementing the fix)
  - **Property 2: Preservation** - Insert-New-Entry Behavior For Non-Mergeable Scans
  - **IMPORTANT**: Follow observation-first methodology — run the UNFIXED code for
    non-bug-condition inputs, record the actual outputs, then encode those observed
    outputs as assertions
  - **GOAL**: Lock in that every scan without a mergeable prior entry keeps inserting a
    new row, so the fix cannot regress it
  - Add table-driven cases to `internal/scan/scan_test.go` (observe on UNFIXED code
    first, per design.md's "Preservation Checking" test plan):
    - **No prior entry**: first scan of a barcode/user/direction creates a
      `unit_count = 1` row with status derived from lookup
    - **Different direction**: existing `pending` stock-in entry, new scan is
      stock-out for the same barcode/user → new row created, stock-in entry untouched
    - **Different user**: existing `pending` entry for user A, same barcode/direction
      scanned by user B → new row created for user B, user A's entry untouched
    - **Only committed/cancelled entries exist**: existing entry for the same
      barcode/user/direction is `committed` (and, separately, `cancelled`) → new row
      created rather than merging into the closed entry
    - **Broadcaster behavior preserved**: using `fakeBroadcaster` (already defined in
      `scan_test.go`), assert exactly one `PublishScanEvent` call per `CreateScanEntry`
      call for each of the above cases
  - Add a property-based test to `internal/scan/scan_properties_test.go` (`rapid`,
    package `scan_test`, `newTestQueue`) per design.md's recommendation that PBT is
    well suited to catching an accidentally-too-broad merge condition:
    - **Property 2: Preservation - Direction/User/Status Mismatch Never Merges** —
      generate a random existing entry (random `direction` including `nil`, random
      `user_id`, random `status` from `pending`/`flagged`/`committed`/`cancelled`) and
      a random incoming scan for a *different* direction, a *different* user_id, or
      the *same* direction+user_id but only against the generated status; assert a new
      row is always inserted (row count for that exact user/barcode/direction/status
      tuple increases by exactly one) whenever the generated combination does not
      match the mergeable definition (same user, same direction including nil==nil,
      status `pending` or `flagged`)
  - Run all of the above on UNFIXED code
  - **EXPECTED OUTCOME**: All cases and the property test PASS (this confirms the
    baseline behavior to preserve)
  - Mark this task complete when the tests are written, run, and passing on unfixed code
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5_

- [x] 3. Fix for duplicate scan-queue cards

  - [x] 3.1 Add `findMergeableScanEntry` query method to `Queue`
    - In `internal/scan/scan.go`, add
      `func (r *Queue) findMergeableScanEntry(ctx context.Context, userID, barcode string, direction *ScanDirection) (*ScanEntry, error)`
    - Query `scan_entries` joined with `products` (reuse `scanScanEntry`) with
      `WHERE se.user_id = ? AND se.barcode = ? AND se.direction IS ? AND se.status IN ('pending', 'flagged') ORDER BY se.scanned_at ASC, se.id ASC LIMIT 1`
    - Use `direction IS ?` (not `=`) so a `nil` direction on the incoming entry only
      matches an existing entry whose direction is also `nil` (SQLite's `IS` is
      null-safe equality) — keeps direction an exact-match key including the
      not-yet-set case
    - Order by `scanned_at ASC, id ASC` so the earliest entry is treated as the
      "original" whose timestamp is preserved, with a deterministic tie-breaker
    - Return `nil, nil` when no row matches (mirror `GetScanEntry`'s `sql.ErrNoRows` handling)
    - _Bug_Condition: isBugCondition(input) == (findMergeableScanEntry(input.UserID, input.Barcode, input.Direction) != nil)_
    - _Requirements: 2.1, 2.2_

  - [x] 3.2 Add the merge-or-create branch to `CreateScanEntry`
    - Before the existing `INSERT` in `CreateScanEntry`, call
      `findMergeableScanEntry(ctx, entry.UserID, entry.Barcode, entry.Direction)`
    - If an existing entry is found:
      - Compute the unit delta: `entry.UnitCount` if positive, else `1` (matches the
        existing default-to-1 convention in `NewEntryFromLookup`)
      - Call `r.UpdateScanEntry(ctx, existing.ID, nil, &newUnitCount, nil, nil, nil)`
        with `newUnitCount = existing.UnitCount + delta`; passing `nil` for direction,
        expiresAt, productID, and status leaves them — and, critically,
        `scanned_at`, which `UpdateScanEntry` never touches — unchanged
      - Return the entry via `r.GetScanEntry(ctx, existing.ID)` instead of falling
        through to the insert; `UpdateScanEntry` already publishes the
        `Scan_Event` when `Broadcaster` is set, so no separate publish call is needed
    - If no existing entry is found, fall through to the current insert logic
      unchanged (including its own `GetScanEntry` + `PublishScanEvent` call)
    - Do not change `handler_scan_create.go` or `scanlistener/listener.go` — both
      already call `Queue.CreateScanEntry`, so they pick up the new behavior for free
    - _Bug_Condition: isBugCondition(input) == (findMergeableScanEntry(input.UserID, input.Barcode, input.Direction) != nil)_
    - _Expected_Behavior: result.ID == existing.ID AND result.UnitCount == existing.UnitCount_before + max(input.UnitCount, 1) AND result.ScannedAt == existing.ScannedAt_before AND no new row inserted_
    - _Preservation: when findMergeableScanEntry returns nil, CreateScanEntry inserts a new row identical in shape to the original function's insert_
    - _Requirements: 2.1, 2.2, 2.3, 3.1, 3.2, 3.3, 3.4, 3.5_

  - [x] 3.3 Verify bug condition exploration tests now pass
    - **Property 1: Expected Behavior** - Repeat Scan Merges Into The Existing Entry
    - **IMPORTANT**: Re-run the SAME tests from Task 1 — do NOT write new tests
    - The Task 1 tests encode the expected behavior; when they pass they confirm a
      repeat `pending` or `flagged` scan increments `unit_count` on the existing
      entry, leaves its `scanned_at` unchanged, and creates no second row
    - **EXPECTED OUTCOME**: Tests PASS (confirms the bug is fixed)
    - _Requirements: 2.1, 2.2, 2.3_

  - [x] 3.4 Verify preservation tests still pass
    - **Property 2: Preservation** - Insert-New-Entry Behavior For Non-Mergeable Scans
    - **IMPORTANT**: Re-run the SAME tests from Task 2 — do NOT write new tests
    - Confirm the no-prior-entry, different-direction, different-user, and
      only-committed/cancelled cases still insert a new row, and the
      direction/user/status-mismatch property test still holds
    - **EXPECTED OUTCOME**: Tests PASS (confirms no regressions)
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5_

- [x] 4. Add unit test coverage for `findMergeableScanEntry` (logic the tests above don't isolate directly)
  - [x] 4.1 Add direct unit tests for `findMergeableScanEntry` to `internal/scan/scan_test.go`
    - Returns the matching entry for an exact `user_id`/`barcode`/`direction`/status match
    - Returns `nil` when no match exists, including when `direction` is `nil` on both
      the query and a candidate row (the `IS`-based null-safe comparison)
    - Returns `nil` when the only candidate is `committed` or `cancelled`
    - When more than one mergeable entry exists, returns the one with the earliest
      `scanned_at` (defensive tie-breaker)
    - _Requirements: 2.1, 2.2_

  - [x] 4.2 Add a unit test for the zero/unset `UnitCount` default on merge
    - `CreateScanEntry` merging a scan with `UnitCount == 0` increments the existing
      entry's `unit_count` by exactly 1 (not 0), matching today's create-path default
    - _Requirements: 2.1, 2.2, 2.3_

- [x] 5. Add property-based coverage for merge-vs-create accounting across random scan sequences
  - [x] 5.1 Write a property test asserting final row/unit-count accounting in `internal/scan/scan_properties_test.go`
    - **Property 3: Merge Accounting** - Row Count And Total Unit Count Match Expected Merges
    - **Validates: Requirements 2.1, 2.2, 2.3, 3.1, 3.2, 3.3, 3.4, 3.5**
    - Generate a random sequence of scans (varying barcode, user, direction including
      `nil`, and lookup hit/miss) via `rapid`, feed each through the fixed
      `CreateScanEntry`, and after the sequence assert — per (barcode, user,
      direction) group — that the resulting row count equals the number of "runs" of
      consecutive mergeable scans in that group (a run breaks whenever the group's
      current open entry is not `pending`/`flagged`, which doesn't happen via
      `CreateScanEntry` alone but keeps the model correct), and that each row's
      `unit_count` equals the number of scans merged into it
    - Minimum 100 iterations, tagged `Feature: scan-duplicate-cards-fix, Property 3:
      Merge Accounting`
    - _Requirements: 2.1, 2.2, 2.3, 3.1, 3.2, 3.3, 3.4, 3.5_

- [x] 6. Add integration coverage across both entry points
  - [x] 6.1 Add an HTTP handler test for merge behavior via `POST /api/scans`
    - New case in `internal/server/handler_scan_create_test.go` (`handlerTestCase` /
      `runHandlerTests`): issue two `POST /api/scans` requests with the same barcode,
      direction, and userId while the first stays `pending`; use
      `afterRequest: exchanges(...)` with `GET /api/scans` to assert exactly one entry
      exists for that user with `unitCount == 2`, rather than querying the DB directly
    - Add a second case covering the `flagged` path (barcode with no matching product)
    - _Requirements: 2.1, 2.2, 3.5_

  - [x] 6.2 Add a `ScanListener` test for merge behavior via stdin
    - New case in `internal/scanlistener/listener_test.go` using the existing
      `fakeQueue`/`fakeLookupService` pattern: feed two identical barcode lines through
      `ScanListener.Run` via a multi-line `Stdin` reader and assert `fakeQueue`
      recorded the merge — either via a `fakeQueue.CreateScanEntry` variant that
      models merge-or-create against its in-memory slice, or by exercising the real
      `scan.Queue` (`newTestQueue`) as `l.Queue` instead of `fakeQueue` for this
      specific test so the real merge logic runs; assert exactly one entry exists with
      `unit_count == 2`
    - _Requirements: 2.1, 2.2, 3.5_

  - [x] 6.3 Add a test confirming the broadcast fires exactly once per scan on merge
    - Using `fakeBroadcaster` (`internal/scan/scan_test.go`) attached to a real
      `scan.Queue`: two `CreateScanEntry` calls that merge produce exactly two
      `PublishScanEvent` calls total (one per call, not one per row), and the second
      call's published entry carries the incremented `unitCount`
    - _Requirements: 2.1, 2.2, 2.3, 3.5_

- [x] 7. Checkpoint - Ensure all tests pass and coverage is enforced
  - Run the full suite and confirm every test from Tasks 1-6 passes (exploration tests
    now green, preservation tests still green, unit/property/integration tests green)
  - Run `./scripts/test-coverage.sh` to enforce (and lock in) coverage thresholds;
    commit the updated script if the threshold increased, per AGENTS.md
  - Ask the user if any questions or ambiguities arise

---

## Task Dependency Graph

```
Task 1 (Bug Condition exploration tests — must FAIL on unfixed code)
   │
   ├─ independent of ─┐
   │                  │
Task 2 (Preservation tests — must PASS on unfixed code)
   │                  │
   └──────┬───────────┘
          ▼
Task 3 (Fix)
   3.1 findMergeableScanEntry                    (new query method)
        ▼
   3.2 Merge-or-create branch in CreateScanEntry (depends on 3.1)
        ▼
   3.3 Re-run Task 1 tests → now PASS            (depends on 3.1, 3.2)
   3.4 Re-run Task 2 tests → still PASS          (depends on 3.1, 3.2)
          ▼
Task 4 (Unit coverage for findMergeableScanEntry + zero-UnitCount default)  (depends on 3.1, 3.2)
          ▼
Task 5 (Property 3: merge accounting across random sequences)              (depends on 3.1, 3.2)
          ▼
Task 6 (Integration: HTTP handler, ScanListener, broadcast-once)           (depends on 3.1, 3.2)
          ▼
Task 7 (Checkpoint: all tests pass + ./scripts/test-coverage.sh)
```

```json
{"waves": [["1", "2"], ["3.1"], ["3.2"], ["3.3", "3.4"], ["4.1", "4.2", "5.1"], ["6.1", "6.2", "6.3"], ["7"]]}
```

## Notes

- Tasks 1 and 2 are written FIRST, against UNFIXED code (test-first): Task 1 must
  fail, Task 2 must pass.
- Task 3's subtasks are ordered so the query method (3.1) precedes the branch that
  calls it (3.2); the verification subtasks (3.3, 3.4) re-run the existing tests
  rather than writing new ones.
- Tasks 4-6 depend on the fix (3.1, 3.2) being in place, since `findMergeableScanEntry`
  doesn't exist until then and the integration tests need real merge behavior to
  observe.
- Task 6.2 notes an implementation choice for the reviewer: `fakeQueue` in
  `scanlistener` is a plain recorder today, so exercising real merge behavior through
  `ScanListener` means either extending `fakeQueue` to model merging or swapping in the
  real `scan.Queue` (`newTestQueue`) for that one test — either is acceptable, but
  don't duplicate `CreateScanEntry`'s merge logic inside `fakeQueue`, since that would
  test the fake instead of the fix.
