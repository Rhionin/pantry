# Implementation Plan

## Overview

Build `internal/scanlistener` bottom-up: mode state and Control_Barcode classification
first (no dependencies), then the shared entry-creation helper (`scan.NewEntryFromLookup`)
and the `ScanCreateHandler` refactor that adopts it — landed early since `ScanListener`
depends on the same helper and Requirement 3.4 (structural identity between the two
paths) only holds if both call one function — then the `ScanListener` itself, which
reads complete lines from standard input via `bufio.Scanner` and dispatches each line
through mode classification or the shared entry-creation helper. `cmd/server/main.go`
wiring comes next, then the remaining property tests, then an `apitest` case in
`internal/server` that closes the loop by exercising Requirement 3.4 through the real
HTTP response path.

There is no hardware-only or OS-specific file in this design — `ScanListener` reads an
injectable `io.Reader` (defaulting to `os.Stdin`), so every code path is testable with a
`strings.Reader` or `io.Pipe`.

Language: Go. Verification follows AGENTS.md: table-driven unit tests, `rapid`-based
property tests (`pgregory.net/rapid`, already a dependency), and the `apitest`
framework in `internal/server` via `handlerTestCase` / `runHandlerTests`.

Requirement 5 (no new network surface) is a structural property of this design —
`scanlistener` never imports `net`, and no task in this plan touches `server.go`'s
route table — so it has no dedicated test task; it is verified by code review and by
`go vet`/`go build` succeeding. Requirement 6 (coexistence with browser-based capture)
is verified by the existing `internal/server` suite continuing to pass unmodified at
every checkpoint below.

## Tasks

- [x] 1. Implement mode state and Control_Barcode classification
  - [x] 1.1 Create `internal/scanlistener/mode.go` with `modeState` (`mu
    sync.Mutex`, `current scan.ScanDirection`), `newModeState()` defaulting to
    `scan.StockIn`, `get()`/`set(d scan.ScanDirection)`, and `classify(barcode,
    stockInBarcode, stockOutBarcode string) (mode scan.ScanDirection, isControl
    bool)`
    - _Requirements: 2.1, 2.2, 2.3, 2.5, 2.6_

  - [x] 1.2 Write unit tests for `modeState` and `classify`
    - `newModeState().get()` returns `scan.StockIn` with no prior `set` call
    - `set` followed by `get` returns the value just set, repeatedly, without
      drifting back to the default
    - `classify`: an exact match on the configured stock-in barcode returns
      `(StockIn, true)`; an exact match on stock-out returns `(StockOut, true)`;
      any other string returns `(_, false)`; a barcode that differs only in case
      or whitespace from a control barcode is NOT classified as control (exact
      match only)
    - _Requirements: 2.1, 2.2, 2.3, 2.5, 2.6_

- [x] 2. Shared entry-creation helper and `ScanCreateHandler` refactor
  - [x] 2.1 Add `scan.NewEntryFromLookup` to `internal/scan/scan.go`
    - `NewEntryFromLookup(userID, barcode string, lookup product.LookupResult,
      direction *ScanDirection, scannedAt time.Time) ScanEntry` — found lookup sets
      `ProductID` and `Status: Pending`; not-found sets `ProductID: nil` and
      `Status: Flagged`; `UnitCount` is always `1`
    - This is the single rule both the HTTP scan-create path and `ScanListener`
      use, so entries created through either path cannot silently drift apart
    - _Requirements: 3.1, 3.4_

  - [x] 2.2 Write unit tests for `NewEntryFromLookup`
    - In `internal/scan/scan_test.go`: a found lookup result yields `ProductID`
      set to the found product's ID, `Status: Pending`, `UnitCount: 1`; a
      not-found result yields `ProductID: nil`, `Status: Flagged`, `UnitCount: 1`;
      `UserID`, `Barcode`, `ScannedAt`, and `Direction` are carried through
      unchanged in both cases
    - _Requirements: 3.1_

  - [x] 2.3 Refactor `ScanCreateHandler.Handle` in
    `internal/server/handler_scan_create.go` to call `scan.NewEntryFromLookup`
    instead of inlining the found/not-found branch
    - After the `Lookup` call, build `entry := scan.NewEntryFromLookup(...)`, then
      apply the two HTTP-only overrides in place: set `entry.ExpiresAt` when
      `req.Body.ExpiresAt != nil`, and set `entry.UnitCount = req.Body.UnitCount`
      when it is non-zero
    - _Requirements: 3.1, 3.4_

  - [x] 2.4 Add regression coverage for the refactored `ScanCreateHandler`
    - Handler test asserting a found-product scan-create request still returns
      `Status: pending` with the correct `productId`, and a not-found scan-create
      request still returns `Status: flagged` with a nil `productId`
    - _Requirements: 3.1, 3.4_

- [x] 3. Checkpoint - mode logic and handler refactor compile and their tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 4. Implement `ScanListener` reading from standard input
  - [x] 4.1 Create `internal/scanlistener/listener.go` with the `ScanListener`
    struct (`StockInBarcode`, `StockOutBarcode`, `HeadlessUserID`, inline `Queue`
    and `LookupService` interfaces, `Stdin io.Reader`, `Now func() time.Time`),
    `defaultHeadlessUserID = "user-1"`, `Run(ctx context.Context)`,
    `handleLine`, `createEntry`, and `now()`
    - `Run` defaults `Stdin` to `os.Stdin` when nil, reads lines via
      `bufio.Scanner` until EOF, a read error, or `ctx` is cancelled, and logs
      when it stops (Requirement 1.3)
    - `handleLine` discards an empty line with no entry created (Requirement
      1.4), classifies a non-empty line and updates mode state on a
      Control_Barcode match with no entry created (Requirements 2.2, 2.3), or
      calls `createEntry` with the mode in effect at read time (Requirement 2.4)
    - `createEntry` defaults `HeadlessUserID` to `defaultHeadlessUserID` when
      empty (Requirement 4.2), looks up the product, and logs-and-continues on a
      `LookupService` or `Queue.CreateScanEntry` error without terminating
      (Requirement 3.5)
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 2.4, 3.1, 3.2, 3.3, 3.5, 4.1, 4.2_

  - [x] 4.2 Write unit tests for `ScanListener.Run`
    - New `internal/scanlistener/listener_test.go` with a `strings.Reader` or
      `io.Pipe` as `Stdin`, a `fakeLookupService`, and a `fakeQueue`
    - `Stdin` reaches EOF → `Run` returns after processing every line already
      available, without hanging or erroring (Requirement 1.3)
    - A blank line in the input produces no created entry and does not change
      mode (Requirement 1.4)
    - A control-barcode line updates mode and creates no entry; a subsequent
      product-barcode line creates an entry with that mode
    - `fakeLookupService.Lookup` returns an error for a Product_Barcode → no
      entry is created and `Run` keeps reading subsequent lines
    - `fakeQueue.CreateScanEntry` returns an error → the error is not surfaced
      from `Run`, and a later barcode in the same input still creates an entry
      (Requirement 3.5)
    - _Requirements: 1.3, 1.4, 3.5_

- [x] 5. Checkpoint - `ScanListener` compiles and its tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 6. Configuration and lifecycle wiring in `cmd/server/main.go`
  - [x] 6.1 Implement `loadScanListenerConfig() (*scanlistener.ScanListener,
    bool)` in `cmd/server/main.go`
    - Reads `STOCK_IN_CONTROL_BARCODE` (default `STOCK_IN`),
      `STOCK_OUT_CONTROL_BARCODE` (default `STOCK_OUT`) via the existing
      `envOrDefault` helper, and `HEADLESS_USER_ID` (default `user-1`)
    - Equal stock-in/stock-out barcodes → logs a configuration error and returns
      `nil, false` (Requirement 2.7)
    - _Requirements: 2.2, 2.3, 2.7, 4.1, 4.2_

  - [x] 6.2 Wire `ScanListener` startup into `main()`
    - After `lookupService` is constructed and before `http.ListenAndServe`: call
      `loadScanListenerConfig()`; when `ok`, assign `listener.Queue =
      scan.NewQueue(sqlDB)` and `listener.LookupService = lookupService`, then
      start `go listener.Run(context.Background())`
    - _Requirements: 1.1_

  - [x] 6.3 Write unit tests for `loadScanListenerConfig`
    - In `cmd/server/main_test.go`: default control barcodes → `ok=true` with
      `STOCK_IN`/`STOCK_OUT`/`user-1` defaults; explicit env values override each
      default; equal `STOCK_IN_CONTROL_BARCODE`/`STOCK_OUT_CONTROL_BARCODE` →
      `ok=false`
    - Use `t.Setenv` for env values, matching the existing `TestProductCacheTTL`
      pattern in this file
    - _Requirements: 2.7, 4.2_

- [x] 7. Checkpoint - server builds and starts, all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 8. Property tests for line handling, mode transitions, and entry construction
  - [x] 8.1 Write a property test for the empty-line discard rule
    - New file `internal/scanlistener/listener_properties_test.go`
    - **Property 1: Empty-line discard rule**
    - **Validates: Requirements 1.2, 1.4**
    - Generate an arbitrary string with `rapid` (including the empty string),
      pass it through `handleLine`, and assert: empty line → no entry created,
      mode unchanged; non-empty non-control line → exactly one entry created
      with that barcode
    - Minimum 100 iterations, tagged `Feature: background-scan-listener, Property
      1: Empty-line discard rule`
    - _Requirements: 1.2, 1.4_

  - [x] 8.2 Write a property test for mode transitions and classification
    - **Property 2: Mode transitions and Control_Barcode vs Product_Barcode classification**
    - **Validates: Requirements 2.2, 2.3, 2.4, 2.5, 2.6**
    - In `listener_properties_test.go`: generate a random starting mode and a
      random sequence of lines (each either the stock-in barcode, the stock-out
      barcode, or an arbitrary non-empty Product_Barcode string), drive them
      through `ScanListener.Run` via a multi-line `Stdin` reader against a
      `fakeQueue` that records created entries, and assert: every
      Control_Barcode occurrence updates the tracked mode and creates no entry;
      every Product_Barcode creates exactly one entry whose direction equals the
      mode in effect immediately before it, without itself changing the mode;
      the final mode equals the last Control_Barcode's mode, or the starting
      mode if none occurred
    - Minimum 100 iterations, tagged `Feature: background-scan-listener, Property
      2: Mode transitions and Control_Barcode vs Product_Barcode classification`
    - _Requirements: 2.2, 2.3, 2.4, 2.5, 2.6_

  - [x] 8.3 Write a property test for lookup-based entry construction
    - **Property 3: Headless entries follow the shared lookup-based creation rule**
    - **Validates: Requirements 3.1**
    - In `listener_properties_test.go`: generate an arbitrary barcode and a
      generated `product.LookupResult` (found with a generated product ID, or
      not found), call `scan.NewEntryFromLookup` with a generated `ScanDirection`,
      and assert `ProductID`/`Status`/`UnitCount` follow the found/not-found rule
    - Minimum 100 iterations, tagged `Feature: background-scan-listener, Property
      3: Headless entries follow the shared lookup-based creation rule`
    - _Requirements: 3.1_

  - [x] 8.4 Write a property test for structural identity with HTTP-created entries
    - **Property 4: Structural identity with HTTP-created entries**
    - **Validates: Requirements 3.4**
    - In `listener_properties_test.go`: for a generated barcode, `LookupResult`,
      direction, and pair of distinct user IDs (headless vs. HTTP-supplied), call
      `scan.NewEntryFromLookup` twice with the same barcode/lookup/direction but
      different `userID` and equal `scannedAt`; assert every field is equal
      between the two results except `UserID`
    - Minimum 100 iterations, tagged `Feature: background-scan-listener, Property
      4: Structural identity with HTTP-created entries`
    - _Requirements: 3.4_

  - [x] 8.5 Write a property test for headless user attribution and default fallback
    - **Property 5: Headless user attribution and default fallback**
    - **Validates: Requirements 4.1, 4.2**
    - In `listener_properties_test.go`: construct a `ScanListener` with a
      generated `HeadlessUserID` (including the empty string) and a
      `fakeQueue`/`fakeLookupService`, drive one Product_Barcode through
      `createEntry`, and assert the created entry's `UserID` equals the
      configured value when non-empty, and equals `"user-1"` when empty
    - Minimum 100 iterations, tagged `Feature: background-scan-listener, Property
      5: Headless user attribution and default fallback`
    - _Requirements: 4.1, 4.2_

- [x] 9. Add `apitest` coverage for headless-created entries in `internal/server`
  - [x] 9.1 Add a handler test asserting headless entries have the same shape as browser-created entries
    - New `internal/server/handler_scan_headless_test.go` using `handlerTestCase`
      / `runHandlerTests`
    - In `setup`, seed a `ScanEntry` directly through `scan.NewEntryFromLookup` +
      `env.DB`-backed `scan.Queue.CreateScanEntry` with a distinct `UserID`,
      standing in for a headless-created entry with no real terminal input
      involved
    - Issue `GET /api/scans` for both that headless `UserID` and a
      browser-created entry's `UserID` (created via the normal `POST /api/scans`
      exchange), and assert the two returned JSON objects have identical field
      sets and value shapes aside from `userId`
    - _Requirements: 3.3, 3.4_

- [x] 10. Final checkpoint - full suite, race detector, and coverage
  - Run the full suite and confirm every test from tasks 1-9 passes
  - Run `go test -race ./internal/scanlistener/...` for the concurrency-adjacent
    mode-state mutex and the `Run` dispatch loop
  - Run `./scripts/test-coverage.sh` to enforce coverage thresholds and commit the
    updated script if the threshold increased, per AGENTS.md
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Requirement 5 (no new network surface) has no dedicated test task: it is a
  structural property enforced by `scanlistener` never importing `net` and by no
  task in this plan touching `internal/server/server.go`'s route table.
- Requirement 6 (coexistence) is verified implicitly — every checkpoint (3, 5, 7,
  10) runs the full existing suite, which would fail if browser-based scan capture
  regressed.
- Task 2 (shared helper + handler refactor) is sequenced before task 4
  (`ScanListener`) because `ScanListener.createEntry` calls
  `scan.NewEntryFromLookup` — Requirement 3.4's structural-identity guarantee only
  holds once both call sites exist.
- Tasks 6.1 and 6.2 both edit `cmd/server/main.go` and are therefore sequenced,
  not parallelized, to avoid a same-file conflict.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "2.1"] },
    { "id": 1, "tasks": ["1.2", "2.2", "2.3"] },
    { "id": 2, "tasks": ["2.4"] },
    { "id": 3, "tasks": ["4.1"] },
    { "id": 4, "tasks": ["4.2", "8.1"] },
    { "id": 5, "tasks": ["6.1"] },
    { "id": 6, "tasks": ["6.2", "8.2"] },
    { "id": 7, "tasks": ["6.3", "8.3", "9.1"] },
    { "id": 8, "tasks": ["8.4"] },
    { "id": 9, "tasks": ["8.5"] }
  ]
}
```
