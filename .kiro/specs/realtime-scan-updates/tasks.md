# Implementation Plan

## Overview

Build `internal/events.Broadcaster` bottom-up: the standalone package first (no
dependencies on `scan`/`inventory` beyond their public structs), verified with its own
unit and property tests, then the two collaborator fields (`Queue.Broadcaster`/`Pantry`,
`Pantry.Broadcaster`) and publish calls added to existing `scan`/`inventory` methods —
these are internal collaboration contracts tested with a fake `Broadcaster`, per
AGENTS.md, not through `apitest`. `Pantry.GetInventoryItem` and the `aggregateItem`
extraction land before the `Queue` changes, since `CommitStockIn`/`CommitStockOut` call
`Pantry.GetInventoryItem` to build their `Inventory_Event`. `internal/server` wiring
(the new `EventsHandler`, route registration, and `server.go`'s broadcaster/collaborator
assignments) comes next, followed by the one integration test SSE requires. Frontend
work follows the same bottom-up order: pure merge/prune helpers first (property-tested
directly, no rendering), then the `ScanQueuePage`/`InventoryPage` `EventSource` wiring
that consumes them.

Language: Go (backend) and TypeScript/React (frontend). Verification follows AGENTS.md:
`apitest`/`handlerTestCase`/`exchanges()` for HTTP-observable behavior, `pgregory.net/rapid`
property tests for internal collaboration contracts (already a dependency), and
`./scripts/test-coverage.sh` after each task.

## Tasks

- [x] 1. Implement `internal/events.Broadcaster`
  - [x] 1.1 Create `internal/events/broadcaster.go` with `Message`, `Broadcaster`,
    `NewBroadcaster`, `Subscribe`, `PublishScanEvent`, `PublishInventoryEvent`, and the
    internal `publish` method exactly as specified in design.md's Components section
    - `subscriberBuffer = 16`; a full subscriber channel is closed and dropped rather
      than blocking delivery to others
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5_

  - [x] 1.2 Write unit tests for `Broadcaster` in `internal/events/broadcaster_test.go`
    - `Subscribe` then `PublishScanEvent`/`PublishInventoryEvent` delivers exactly one
      `Message` with the expected `EventType` ("scan"/"inventory") and JSON-encoded
      `Data` on the returned channel
    - Two independent subscribers both receive a published message
    - Calling the `unsubscribe` function stops further delivery to that subscriber
      without affecting a second, still-subscribed subscriber
    - A publish before any `Subscribe()` call is not observed by a subscriber that
      subscribes afterward (no replay/buffering)
    - A subscriber whose channel buffer is already full when a publish occurs is
      dropped (its channel is closed) while a second, draining subscriber still
      receives the message
    - _Requirements: 1.2, 1.3, 1.4_

  - [x] 1.3 Write property tests for `Broadcaster` in
    `internal/events/broadcaster_properties_test.go`
    - **Property 1: Subscribers only receive events published after they subscribe**
    - **Validates: Requirements 1.2**
    - **Property 2: Unsubscribing removes exactly that subscriber without affecting the rest**
    - **Validates: Requirements 1.3**
    - **Property 3: A subscriber that cannot keep up is dropped without blocking delivery to others**
    - **Validates: Requirements 1.4**
    - Use `pgregory.net/rapid` to generate: a "before" and "after" publish sequence
      split by a `Subscribe()` call (Property 1); a set of concurrently subscribed
      connections and a subset unsubscribed before a publish (Property 2); a set of
      subscribed connections where an arbitrary non-empty subset never drains its
      channel until it overflows (Property 3, drive the buffer past
      `subscriberBuffer` and assert every subscriber outside that subset still
      receives every message)
    - Minimum 100 iterations per property, tagged `Feature: realtime-scan-updates,
      Property N: <title>`
    - _Requirements: 1.2, 1.3, 1.4_

- [x] 2. Checkpoint - `internal/events` compiles and its tests pass
  - Run `./scripts/test-coverage.sh`
  - Ensure all tests pass, ask the user if questions arise.

- [x] 3. Add `Pantry.GetInventoryItem` and extract shared aggregation
  - [x] 3.1 Extract `aggregateItem` in `internal/inventory/aggregate.go` and add
    `DefaultWarningDays` and `GetInventoryItem`
    - Add `const DefaultWarningDays = 7`
    - Extract `GetInventoryList`'s per-item instance-fetch-and-tally body into
      `aggregateItem(ctx context.Context, item Item, now time.Time, warningDays int)
      (InventoryItem, error)`; `GetInventoryList` calls it in its loop, behavior
      unchanged
    - Add `GetInventoryItem(ctx context.Context, itemID string, now time.Time,
      warningDays int) (*InventoryItem, error)`: `getItemByID`, return `nil, nil` if
      not found, otherwise `aggregateItem` and return its address
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5_

  - [x] 3.2 Update `internal/server/handler_inventory_list.go` to use
    `inventory.DefaultWarningDays` instead of the hardcoded `warningDays := 7`
    - _Requirements: — (no behavior change; keeps the two call sites from drifting)_

  - [x] 3.3 Write unit tests for `GetInventoryItem` in
    `internal/inventory/aggregate_test.go`
    - A known item with instances of mixed expiry status returns an `InventoryItem`
      whose counts match `GetInventoryList`'s result for the same item
    - An unknown `itemID` returns `nil, nil`
    - _Requirements: 3.1, 3.2, 3.3_

- [x] 4. Checkpoint - inventory aggregation changes compile and existing tests pass
  - Run `./scripts/test-coverage.sh`
  - Ensure all tests pass, ask the user if questions arise.

- [x] 5. Add `Pantry.Broadcaster` and publish calls to `Pantry` mutations
  - [x] 5.1 Add the `Broadcaster` field and publish calls in
    `internal/inventory/inventory.go`
    - Add the nil-safe `Broadcaster interface { PublishInventoryEvent(item
      InventoryItem) }` field to `Pantry`, exactly as specified in design.md
    - `AddInstance`: after `r.GetInstance(ctx, instance.ID)` succeeds, if
      `r.Broadcaster != nil`, fetch `r.GetInventoryItem(ctx, instance.ItemID,
      time.Now(), DefaultWarningDays)` and publish if non-nil
    - `RemoveInstance`: after the `UPDATE` succeeds, if `r.Broadcaster != nil`, fetch
      `r.GetInventoryItem(ctx, existing.ItemID, time.Now(), DefaultWarningDays)` and
      publish if non-nil
    - `UpdateTargetQuantity`: after the `UPDATE` succeeds, if `r.Broadcaster != nil`,
      fetch `r.GetInventoryItem(ctx, itemID, time.Now(), DefaultWarningDays)` and
      publish if non-nil
    - No publish call on any existing error return path
    - _Requirements: 3.1, 3.2, 3.3, 3.6_

  - [x] 5.2 Add a `fakeBroadcaster` test double and unit tests in
    `internal/inventory/inventory_test.go`
    - `fakeBroadcaster` records every `PublishInventoryEvent` call (append to a
      slice); assign it to `pantry.Broadcaster` in the relevant test cases
    - `AddInstance`, `RemoveInstance` (success), and `UpdateTargetQuantity` (success)
      each record exactly one `PublishInventoryEvent` call whose argument matches
      `GetInventoryItem`'s result for the affected item read back afterward
    - `RemoveInstance` on an already-removed/unknown instance and
      `UpdateTargetQuantity` on an unknown item record zero calls
    - With `pantry.Broadcaster` left nil (as in every pre-existing test), all three
      methods behave exactly as before (no panic, same return values)
    - _Requirements: 3.1, 3.2, 3.3, 3.6_

  - [x] 5.3 Write property tests for `Pantry` publish behavior in
    `internal/inventory/aggregate_properties_test.go`
    - **Property 7 (Pantry half): Every successful inventory-affecting operation
      publishes exactly one matching Inventory_Event**
    - **Validates: Requirements 3.1, 3.2, 3.3**
    - **Property 8 (Pantry half): A failed inventory-affecting call publishes no
      Inventory_Event**
    - **Validates: Requirements 3.6**
    - Reuse this file's existing item/instance generators; assign a
      `fakeBroadcaster` to `pantry.Broadcaster` and assert the recorded-call
      invariant for `AddInstance`/`RemoveInstance`/`UpdateTargetQuantity` across
      generated success and failure inputs (unknown instance/item IDs for the
      failure case)
    - Minimum 100 iterations per property, tagged `Feature: realtime-scan-updates,
      Property N: <title>`
    - _Requirements: 3.1, 3.2, 3.3, 3.6_

- [x] 6. Checkpoint - `Pantry` publish behavior compiles and its tests pass
  - Run `./scripts/test-coverage.sh`
  - Ensure all tests pass, ask the user if questions arise.

- [x] 7. Add `Queue.Broadcaster`/`Queue.Pantry` and publish calls to scan mutations
  - [x] 7.1 Add the `Broadcaster` and `Pantry` fields to `internal/scan/scan.go` and
    publish calls to `CreateScanEntry`, `UpdateScanEntry`, `BatchUpdateScanEntries`
    - Add the two nil-safe fields to `Queue` exactly as specified in design.md
      (`Broadcaster` needs `PublishScanEvent`/`PublishInventoryEvent`; `Pantry` needs
      `GetInventoryItem`); import `internal/inventory` for the field types
    - `CreateScanEntry`: publish the entry returned by `r.GetScanEntry(ctx,
      entry.ID)` when non-nil and `r.Broadcaster != nil`, then return it unchanged
    - `UpdateScanEntry`: after `RowsAffected() > 0`, fetch with `r.GetScanEntry(ctx,
      id)`; if that fetch errors, return the error; otherwise publish if non-nil and
      `r.Broadcaster != nil`
    - `BatchUpdateScanEntries`: after `tx.Commit()` succeeds, loop over `ids` again,
      fetch each with `r.GetScanEntry(ctx, id)`, and publish one `Scan_Event` per ID
      when `r.Broadcaster != nil`
    - _Requirements: 2.1, 2.2, 2.4, 2.6_

  - [x] 7.2 Extend `fakeBroadcaster`-style test doubles and unit tests in
    `internal/scan/scan_test.go`
    - Add a `fakeBroadcaster` recording `PublishScanEvent`/`PublishInventoryEvent`
      calls, assigned to `queue.Broadcaster` in the relevant cases
    - `CreateScanEntry` success records exactly one `PublishScanEvent` matching the
      created entry
    - `UpdateScanEntry` success records exactly one `PublishScanEvent` matching the
      updated entry read back via `GetScanEntry`; `UpdateScanEntry` on an unknown ID
      records zero calls
    - `BatchUpdateScanEntries` records exactly one `PublishScanEvent` per updated ID,
      each matching that entry's post-update state
    - With `queue.Broadcaster` left nil, all three methods behave exactly as before
    - _Requirements: 2.1, 2.2, 2.4, 2.6_

  - [x] 7.3 Write property tests for Requirement 2's Queue methods in
    `internal/scan/scan_properties_test.go`
    - **Property 4 (partial): Every successful scan-mutating operation publishes
      exactly one matching Scan_Event** — covering `CreateScanEntry` and
      `UpdateScanEntry` in this task (`ResolveFlaggedEntry`/`CommitStockIn`/
      `CommitStockOut` are covered in task 8.3)
    - **Validates: Requirements 2.1, 2.2**
    - **Property 5: Batch update publishes one Scan_Event per updated entry**
    - **Validates: Requirements 2.4**
    - **Property 6 (partial): A failed scan-mutating call publishes no Scan_Event**
      — covering `CreateScanEntry`, `UpdateScanEntry`, and `BatchUpdateScanEntries`
      in this task
    - **Validates: Requirements 2.6**
    - Reuse the existing `TestProperty4_BatchUpdateApplies` selection generator for
      the batch-publish property; assign a `fakeBroadcaster` and assert the
      recorded-call invariant across generated inputs, including an unknown-ID case
      for the failure property
    - Minimum 100 iterations per property, tagged `Feature: realtime-scan-updates,
      Property N: <title>`
    - _Requirements: 2.1, 2.2, 2.4, 2.6_

- [x] 8. Add publish calls to `ResolveFlaggedEntry`, `CommitStockIn`, `CommitStockOut`
  - [x] 8.1 Add publish calls in `internal/scan/commit.go`
    - `ResolveFlaggedEntry`: after `tx.Commit()` succeeds, fetch with
      `r.GetScanEntry(ctx, scanEntryID)` and publish if non-nil and
      `r.Broadcaster != nil`
    - `CommitStockIn`: after `tx.Commit()` succeeds, fetch the committed entry with
      `r.GetScanEntry(ctx, scanEntry.ID)` and publish a `Scan_Event` if
      `r.Broadcaster != nil`; then, if `r.Pantry != nil` and `r.Broadcaster != nil`,
      fetch `r.Pantry.GetInventoryItem(ctx, itemID, time.Now(),
      inventory.DefaultWarningDays)` and publish an `Inventory_Event` when non-nil
    - `CommitStockOut`: same pattern as `CommitStockIn`, using its own `itemID`
      local variable
    - No publish call on any existing error return path in any of the three methods
    - _Requirements: 2.3, 2.5, 2.6, 3.4, 3.5, 3.6_

  - [x] 8.2 Extend `internal/scan/commit_test.go` with publish-behavior unit tests
    - Using the `fakeBroadcaster` from task 7.2 (and a fake or real `Pantry`
      implementing `GetInventoryItem`, e.g. `newTestPantry` sharing the same DB
      connection as the test's `Queue`): `ResolveFlaggedEntry` success records one
      `PublishScanEvent`; `CommitStockIn`/`CommitStockOut` success each record one
      `PublishScanEvent` and, when `Pantry` is set, exactly one
      `PublishInventoryEvent` matching `GetInventoryItem`'s result for the affected
      item
    - Each method's existing documented error conditions record zero publish calls
    - With `queue.Broadcaster` (and `queue.Pantry`) left nil, all three methods
      behave exactly as before
    - _Requirements: 2.3, 2.5, 2.6, 3.4, 3.5, 3.6_

  - [x] 8.3 Complete the Requirement 2/3 property tests in
    `internal/scan/scan_properties_test.go`
    - **Property 4 (completing): Every successful scan-mutating operation publishes
      exactly one matching Scan_Event** — extend to cover `ResolveFlaggedEntry`,
      `CommitStockIn`, `CommitStockOut`
    - **Validates: Requirements 2.3, 2.5**
    - **Property 6 (completing): A failed scan-mutating call publishes no
      Scan_Event** — extend to cover the same three methods' documented error
      conditions
    - **Validates: Requirements 2.6**
    - **Property 7 (Queue half): Every successful inventory-affecting operation
      publishes exactly one matching Inventory_Event** — covering
      `CommitStockIn`/`CommitStockOut`
    - **Validates: Requirements 3.4, 3.5**
    - **Property 8 (Queue half): A failed inventory-affecting call publishes no
      Inventory_Event** — covering `CommitStockIn`/`CommitStockOut`
    - **Validates: Requirements 3.6**
    - Minimum 100 iterations per property, tagged `Feature: realtime-scan-updates,
      Property N: <title>`
    - _Requirements: 2.3, 2.5, 2.6, 3.4, 3.5, 3.6_

- [x] 9. Checkpoint - `Queue` publish behavior compiles and its tests pass
  - Run `./scripts/test-coverage.sh`
  - Ensure all tests pass, ask the user if questions arise.

- [x] 10. Add the `GET /api/events` route and wire the broadcaster
  - [x] 10.1 Create `internal/server/handler_events.go` with `EventsHandler`
    - `EventsHandler{ Broadcaster *events.Broadcaster }` and `Handle(w
      http.ResponseWriter, r *http.Request)` exactly as specified in design.md:
      sets SSE headers, subscribes, loops writing `event: <type>\ndata:
      <json>\n\n` frames and flushing until the channel closes, a write fails, or
      `r.Context().Done()`, then `defer unsubscribe()`
    - Responds `500` immediately if `w` doesn't implement `http.Flusher`
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5_

  - [x] 10.2 Wire the broadcaster into `internal/server/server.go`
    - Construct `broadcaster := events.NewBroadcaster()` before the scan/inventory
      handler blocks
    - Move the inventory handlers block (`pantry := inventory.NewPantry(db)` and its
      handlers) above the scan handlers block, or keep current ordering and assign
      `scanQueue.Pantry = pantry` after both `scanQueue` and `pantry` exist — either
      is acceptable per design.md; assign `scanQueue.Broadcaster = broadcaster`,
      `pantry.Broadcaster = broadcaster`, and `scanQueue.Pantry = pantry`
    - Register `mux.HandleFunc("GET /api/events", (&EventsHandler{Broadcaster:
      broadcaster}).Handle)`
    - _Requirements: 1.1, 1.5, 7.2_

- [x] 11. Checkpoint - server builds, existing `internal/server` suite passes unmodified
  - Run `./scripts/test-coverage.sh`
  - Ensure all tests pass, ask the user if questions arise.

- [x] 12. Add the SSE integration test
  - [x] 12.1 Add `internal/server/handler_events_test.go`
    - Follow the `httptest.NewServer` + raw `http.Client` pattern
      `handler_scan_headless_test.go`'s `getScanEntry` helper already uses: start a
      real server from `NewHandler(...)`, open a streaming `GET /api/events` request
      in a goroutine using an `http.Client` with no response timeout, read frames
      line-by-line from the response body
    - Trigger a mutation through a normal `POST /api/scans` call on the same
      handler/DB, then assert an `event: scan\ndata: {...}\n\n` frame arrives on the
      stream whose decoded JSON `id` matches the created entry's ID
    - Close the streaming request (or cancel its context) at test end so the
      goroutine and connection are cleaned up
    - _Requirements: 1.1, 1.2, 1.5, 2.1_

- [x] 13. Checkpoint - full backend suite passes
  - Run `./scripts/test-coverage.sh`
  - Ensure all tests pass, ask the user if questions arise.

- [x] 14. Add frontend merge/prune helpers
  - [x] 14.1 Add `mergeScanEvent` and `pruneSelection` to
    `frontend/src/components/queue/queueUtils.ts`
    - Implement exactly as specified in design.md: `DISPLAYABLE_SCAN_STATUSES =
      ['pending', 'flagged']`, upsert-or-remove semantics for `mergeScanEvent`,
      filter semantics for `pruneSelection`
    - _Requirements: 4.2, 4.3, 4.4_

  - [x] 14.2 Write example and property tests for `mergeScanEvent`/`pruneSelection`
    in a new `frontend/src/components/queue/queueUtils.test.ts`
    - Example tests: a pending event for a new ID appends it; a flagged event for an
      existing ID replaces it in place; a committed/cancelled event for an existing
      ID removes it; `pruneSelection` drops an ID no longer present in the entries
      list and keeps the rest
    - **Property 9: Merging a Scan_Event upserts displayable entries and removes
      non-displayable ones**
    - **Validates: Requirements 4.2, 4.3**
    - **Property 10: Removing an entry from the displayed list prunes it from the
      selection**
    - **Validates: Requirements 4.4**
    - Use a small hand-rolled generator (e.g. `fast-check` if already a dependency,
      otherwise randomized example generation in a loop with a fixed seed range) —
      check `frontend/package.json` first and match whatever property-testing
      convention (or lack of one) already exists for this codebase's frontend tests;
      if none exists, implement these two as thorough example-based tests covering
      upsert-new, upsert-existing, remove-existing, remove-absent, and
      order-preservation cases instead
    - Minimum 100 iterations per property if a property-testing library is used
    - _Requirements: 4.2, 4.3, 4.4_

  - [x] 14.3 Create `frontend/src/components/inventory/inventoryUtils.ts` with
    `mergeInventoryEvent`
    - Implement exactly as specified in design.md
    - _Requirements: 5.2_

  - [x] 14.4 Write example and property tests for `mergeInventoryEvent` in a new
    `frontend/src/components/inventory/inventoryUtils.test.ts`
    - Example tests: an event for a new item id appends it; an event for an existing
      item id replaces it in place while other items keep their relative order
    - **Property 11: Merging an Inventory_Event upserts the matching item**
    - **Validates: Requirements 5.2**
    - Same generator approach note as task 14.2
    - _Requirements: 5.2_

- [x] 15. Checkpoint - frontend util tests pass
  - Run the frontend test suite (e.g. `npm test -- --run` in `frontend/`)
  - Ensure all tests pass, ask the user if questions arise.

- [x] 16. Wire `EventSource` into `ScanQueuePage` and `InventoryPage`
  - [x] 16.1 Add the `EventSource`-backed `useEffect` to
    `frontend/src/components/queue/ScanQueuePage.tsx`
    - Implement exactly as specified in design.md: open `new
      EventSource('/api/events')` on mount, listen for `'scan'` events, merge each
      parsed `ScanEntry` via `mergeScanEvent` + `sortScansChronologically`, prune
      `selectedIds` via `pruneSelection` against the merged list in the same
      updater, and close the connection on unmount
    - No `'error'` listener is added
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 6.3_

  - [x] 16.2 Add the `EventSource`-backed `useEffect` to
    `frontend/src/components/inventory/InventoryPage.tsx`
    - Implement exactly as specified in design.md: open `new
      EventSource('/api/events')` on mount, listen for `'inventory'` events, merge
      each parsed `InventoryItem` via `mergeInventoryEvent`, and close the
      connection on unmount
    - No `'error'` listener is added
    - _Requirements: 5.1, 5.2, 5.3, 6.3_

  - [x] 16.3 Add a fake `EventSource` test double and extend
    `ScanQueuePage.test.tsx` and `InventoryPage.test.tsx`
    - Add a small `FakeEventSource` class (constructor records the URL passed,
      `addEventListener` records the registered type/listener, `close` records that
      it was called) installed via `vi.stubGlobal('EventSource', FakeEventSource)`,
      matching this codebase's existing `vi.stubGlobal('fetch', ...)` convention
    - `ScanQueuePage.test.tsx`: mounting opens a connection to `/api/events`;
      dispatching a `'scan'` event with a pending entry's JSON adds it to the
      displayed list without a fetch call; unmounting calls `close()` on the
      `EventSource` instance
    - `InventoryPage.test.tsx`: mounting opens a connection to `/api/events`;
      dispatching an `'inventory'` event updates the matching displayed item;
      unmounting calls `close()`
    - Both: no `'error'` listener is registered, and dispatching a manufactured
      `'error'` event (if the fake supports it) leaves displayed state unchanged
    - _Requirements: 4.1, 4.5, 5.1, 5.3, 6.3_

- [x] 17. Final checkpoint - full suite, coverage, and race detector
  - Run `./scripts/test-coverage.sh` from the repository root and confirm it passes,
    committing the updated script if the threshold increased, per AGENTS.md
  - Run `go test -race ./internal/events/... ./internal/scan/... ./internal/inventory/...
    ./internal/server/...` for the new concurrency-adjacent broadcaster and its
    collaborators
  - Run the frontend test suite (e.g. `npm test -- --run` in `frontend/`)
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP.
- Requirement 6.1 (browser automatic reconnect) and 6.2 (no redelivery on reconnect)
  are structural properties of the native `EventSource` API and `Broadcaster.Subscribe`'s
  "only messages published after Subscribe" contract (already covered by Property 1 in
  task 1.3) — neither has a dedicated frontend test task.
- Requirement 7 (scope boundaries) has no dedicated test task: it is verified by this
  plan never touching `internal/shopping`, `internal/suggestion`,
  `ShoppingListPage.tsx`, or the `suggestions` components, and by every checkpoint
  running the existing full suite, which would fail if those areas regressed.
- Task 3 (Pantry aggregation extraction) is sequenced before task 5 (Pantry publish
  calls) and task 8 (Queue's `CommitStockIn`/`CommitStockOut` publish calls), because
  both depend on `Pantry.GetInventoryItem` existing.
- Task 7 is sequenced before task 8 because both edit files in `internal/scan`
  (`scan.go` vs. `commit.go` are different files, but 8.1 relies on the `Broadcaster`/
  `Pantry` fields task 7.1 adds to the shared `Queue` struct).
- Task 10.2 edits `internal/server/server.go`; task 3.2 edits
  `internal/server/handler_inventory_list.go`. These are different files, so there is
  no same-file conflict between them — task 3.2's early placement is just a side
  effect of it having no dependencies, not a requirement for task 10.2's ordering.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "3.1"] },
    { "id": 1, "tasks": ["1.2", "3.2", "3.3"] },
    { "id": 2, "tasks": ["1.3"] },
    { "id": 3, "tasks": ["5.1"] },
    { "id": 4, "tasks": ["5.2", "7.1"] },
    { "id": 5, "tasks": ["5.3", "7.2"] },
    { "id": 6, "tasks": ["7.3"] },
    { "id": 7, "tasks": ["8.1"] },
    { "id": 8, "tasks": ["8.2"] },
    { "id": 9, "tasks": ["8.3", "10.1"] },
    { "id": 10, "tasks": ["10.2"] },
    { "id": 11, "tasks": ["12.1"] },
    { "id": 12, "tasks": ["14.1"] },
    { "id": 13, "tasks": ["14.2", "14.3"] },
    { "id": 14, "tasks": ["14.4", "16.1"] },
    { "id": 15, "tasks": ["16.2"] },
    { "id": 16, "tasks": ["16.3"] }
  ]
}
```
