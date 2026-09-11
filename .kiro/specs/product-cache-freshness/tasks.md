# Implementation Plan

## Overview

Turn the `products` table into a TTL'd cache of Open Food Facts data. The work lands
bottom-up: schema and model first (nothing else reads correctly without the columns),
then `Catalog` reads and writes, then the new `product.Refresher` that owns
revalidation, then the test seams the API tests need, then the two entry points
(background stale-while-revalidate from `LookupService`, synchronous
`POST /api/products/{id}/refresh`), then the API tests themselves.

Verification follows AGENTS.md: API tests in `internal/server/` using
`handlerTestCase` / `runHandlerTests` with `afterRequest: exchanges(...)` are the
default. Unit and `rapid` property tests are reserved for the eight targets the
design's unit-test table names — pure functions, concurrency, environment parsing,
and schema-level behavior the HTTP surface cannot reach. Language: Go.

## Tasks

- [x] 1. Schema and model foundation

  - [x] 1.1 Add migration `004_add_product_freshness.sql`
    - Create `internal/app/migrations/004_add_product_freshness.sql` with three
      `ALTER TABLE products ADD COLUMN` statements: `source TEXT NOT NULL DEFAULT 'external'`,
      `refreshed_at DATETIME` (nullable), `name_overridden INTEGER NOT NULL DEFAULT 0`
    - Follow with the explicit blanket `UPDATE products SET source = 'external', refreshed_at = NULL, name_overridden = 0`
      so Requirement 1.4 is stated in the file rather than implied by the column defaults
    - Comment why the `{'external','user'}` value set is enforced in Go: SQLite cannot
      attach a CHECK constraint via `ALTER TABLE`
    - Leave the `'Product ' || id` placeholder rows migration `002` created in place —
      a real Open Food Facts fetch overwrites them
    - _Requirements: 1.1, 1.2, 1.3, 1.4_
    - _Properties: 1_

  - [x] 1.2 Add the three freshness fields to `product.Product`
    - In `internal/product/product.go`, add `Source string`, `RefreshedAt *time.Time`,
      and `NameOverridden bool` with tags `source,omitempty`, `refreshedAt,omitempty`,
      `nameOverridden,omitempty`
    - `RefreshedAt` is a pointer because NULL means "never checked" and is semantically
      distinct from the zero time — that distinction drives Requirement 1.5
    - `omitempty` on all three keeps `GET /api/products` and `PUT /api/products/{id}`
      responses byte-identical when the fields are unset (`UpdateHandler` assembles its
      response in memory and never re-reads the row, so without `omitempty` it would
      sprout a misleading `"source": ""`)
    - Do not extend `ProductSummary` — Requirement 8.6 freezes its wire shape
    - _Requirements: 1.2, 1.5_

  - [x] 1.3 Unit test migration 004 backfill and idempotence
    - **Property 1: Migration 004 backfills provenance without disturbing data**
    - In `internal/app/migrate_test.go`, build a database migrated only through `003`,
      seed `products` rows, then run `app.RunMigrations(conn)` and assert every row has
      `source = 'external'`, `refreshed_at` NULL, `name_overridden` false while `id`,
      `name`, `category`, `unit_of_measure`, `image_url`, and `created_at` are unchanged
    - Assert a second `RunMigrations` changes nothing (`schema_migrations` count stable)
    - Use `app.RunMigrations` — never duplicate the schema inline
    - _Requirements: 1.1, 1.2, 1.3, 1.4_
    - _Properties: 1_

- [x] 2. `product.Catalog` reads and writes

  - [x] 2.1 Bind provenance in `CreateProduct`
    - Extend the INSERT column list with `source`, `refreshed_at`, `name_overridden`
    - Empty `Source` defaults to `SourceUser`; any value other than `external` or `user`
      returns an error — this is the CHECK constraint SQLite will not let us add
    - Nil `RefreshedAt` binds NULL
    - Every existing caller omits `Source`, so no pre-existing product (seeded in a test
      or created through `POST /api/products`) becomes a revalidation target by accident
    - _Requirements: 1.6, 1.7_
    - _Properties: 2_

  - [x] 2.2 Read the freshness columns in `GetProductByID` and `ListProducts`
    - Add `source`, `refreshed_at`, and `COALESCE(name_overridden, 0)` to both column lists
    - Scan `refreshed_at` into a `sql.NullTime` and convert to `*time.Time`
    - `GetProductByID` is required by the `Refresher`, which gates on all three values;
      `ListProducts` carries them so the UI can show provenance and last-checked time
    - Leave `LookupByBarcode` untouched — it is the Tier-2 hot path whose result shape
      Requirement 8.6 freezes, and the `Refresher` re-reads by primary key anyway
    - _Requirements: 1.5_
    - _Properties: 3_

  - [x] 2.3 Add `SaveRefresh` and `MarkRefreshed`
    - `SaveRefresh(ctx, p Product, at time.Time) error` writes `name`, `category`,
      `unit_of_measure`, `image_url`, and `refreshed_at = at` — and touches neither `id`,
      `source`, `created_at`, nor `name_overridden`
    - `MarkRefreshed(ctx, id string, at time.Time) error` writes only `refreshed_at = at`,
      for the two failure paths where no upstream field values exist to write
    - Neither method writes the `barcodes` table, so every mapping keeps its `barcode`,
      `source`, and `user_id`
    - _Requirements: 3.6, 3.7, 5.2, 5.3, 5.4, 7.4_
    - _Properties: 8, 9_

  - [x] 2.4 Set `name_overridden` inside the `UpdateProduct` UPDATE
    - Add a `name_overridden = CASE WHEN source = 'external' AND name <> ? THEN 1 ELSE name_overridden END`
      clause; bind the submitted name twice (once as the assignment, once for the comparison)
    - SQLite evaluates `SET` expressions against the pre-update row, so the stored name is
      still available to compare against — no read-then-compare, no race window, no extra
      round trip on a single-writer connection
    - A name change on an external row claims the name; a category-only edit that submits
      the same name leaves the flag alone (Requirement 3.5); user rows are never touched
    - The flag is sticky by design: renaming back to the upstream string does not surrender
      ownership
    - Signature is unchanged, so `server.UpdateHandler` needs no edit
    - _Requirements: 3.4, 3.5_
    - _Properties: 10_

  - [x] 2.5 Unit test `CreateProduct` source validation
    - In `internal/product/product_test.go`, add a new test case asserting an invalid
      `Source` is rejected and that `external` / `user` / empty are accepted with the
      expected stored value. The API never submits an invalid source, so this is the only
      guard on the constraint SQLite cannot hold
    - Add new cases only — do not modify the existing test functions in this file (see Notes)
    - _Requirements: 1.7_
    - _Properties: 2_

- [x] 3. `product.Refresher`

  - [x] 3.1 Create `internal/product/refresh.go` with the type and the freshness predicate
    - New file. Declare `SourceExternal` / `SourceUser`, the `RefreshOutcome` type with
      `OutcomeUpdated` / `OutcomeUnchanged` / `OutcomeNotFoundUpstream`, and the
      `ErrRefreshTargetMissing` sentinel the handler maps to 404
    - Declare the `Refresher` struct: inline `Catalog` interface (`GetProductByID`,
      `SaveRefresh`, `MarkRefreshed`), inline `OpenFoodFacts` interface (`LookupBarcode`),
      `TTL`, `Now func() time.Time`, `ExternalLookupEnabled`, `BackgroundTimeout`, plus the
      unexported `mu sync.Mutex`, `inFlight map[string]struct{}`, and `wg sync.WaitGroup`
    - `now()` returns `time.Now` when `Now` is nil, so existing construction sites behave
      unchanged; `isStale(p)` returns true when `p.RefreshedAt == nil` and otherwise
      `now().Sub(*p.RefreshedAt) > r.TTL`
    - No sleep, no timer, no wall-clock wait anywhere in this file
    - Document the invariant the upstream call depends on: for `source = 'external'` rows the
      product ID *is* the barcode (`persistExternalProduct` sets `Product.ID = barcode`, and
      migration `002`'s placeholders have `id = product_id = barcode`), so no second query
      against `barcodes` is needed
    - _Requirements: 1.5, 8.2, 8.5_
    - _Properties: 3_

  - [x] 3.2 Implement `mergeRefresh` and `classify`
    - `mergeRefresh(cached Product, upstream ProductSummary) Product` takes an upstream
      field **only when that upstream field is non-empty**; `name` additionally requires
      `!cached.NameOverridden`. `source`, `created_at`, `id`, and `name_overridden` are
      never touched
    - **Keep the non-empty guard.** It is what Requirements 3.1 and 3.2 ask for:
      `OpenFoodFactsClient.LookupBarcode` hardcodes `UnitOfMeasure: ""` — Open Food Facts is
      never asked for a unit and never supplies one — so an unconditional overwrite would
      blank the unit on **every** product the first time it is refreshed. Requirement 5's
      governing intent is that a refresh never degrades a cached product, so an empty
      upstream field means "no opinion", not "clear this". Do not "fix" this back to an
      unconditional overwrite. The same guard is what keeps a rotated-but-missing thumbnail
      from wiping a working one
    - Take `upstream` by value: the fake OFF client returns a pointer into its own store,
      and `mergeRefresh` must not mutate it
    - `classify(before, after Product) RefreshOutcome` compares only the four field values
      and returns `OutcomeUpdated` on any difference, else `OutcomeUnchanged` — so
      `refreshed_at` moving never by itself counts as an update
    - _Requirements: 3.1, 3.2, 3.3, 4.4, 4.5, 5.1, 5.3_
    - _Properties: 7, 12_

  - [x] 3.3 Implement `Refresh` — the single core path both entry points call
    - `Refresh(ctx, productID) (RefreshOutcome, error)` ignores the TTL entirely
      (Requirement 4.2); staleness is only ever consulted by `ScheduleRefresh`
    - Control flow: (1) `!ExternalLookupEnabled` → return `OutcomeUnchanged, nil` with no
      read and no write; (2) load the row, missing → `ErrRefreshTargetMissing`;
      (3) `row.Source != SourceExternal` → return `OutcomeUnchanged, nil` writing nothing;
      (4) `OpenFoodFacts.LookupBarcode(ctx, row.ID)`
    - Upstream `ErrProductNotFound` → `MarkRefreshed(row.ID, now)`, return
      `OutcomeNotFoundUpstream, nil`. Any other upstream error → `MarkRefreshed` **first**,
      then return `OutcomeUnchanged, err`, so an outage does not make every subsequent
      lookup retry immediately. Success → `merged := mergeRefresh(*row, *upstream)`,
      `SaveRefresh(merged, now)`, return `classify(*row, merged), nil`
    - Never infer disabled-ness from an error value: `disabledProductLookup` returns
      `ErrProductNotFound`, and treating that as an upstream not-found would stamp
      `refreshed_at` and silently suppress real revalidation for a full TTL
    - _Requirements: 3.6, 3.7, 4.2, 4.6, 4.8, 5.1, 5.2, 5.3, 5.4, 6.4, 7.1_
    - _Properties: 8, 9, 12, 13, 17_

  - [x] 3.4 Implement `ScheduleRefresh` and `Wait`
    - Steps 1–4 run **synchronously on the caller's context**: return if
      `!ExternalLookupEnabled`; load the row and return when it is nil or
      `row.Source != SourceExternal`; return when `!isStale(row)`; then under `mu`, return
      if the ID is already in `inFlight`, otherwise insert it and `wg.Add(1)`
    - A fresh row therefore schedules *nothing* — no goroutine, no `WaitGroup` entry — which
      is Requirement 2.4 taken literally, and it keeps the clock and TTL out of `LookupService`
    - The goroutine `defer`s removing the ID from `inFlight` and `wg.Done()`, builds
      `context.WithTimeout(context.WithoutCancel(context.Background()), backgroundTimeout())`
      (default 30s), calls `Refresh`, and logs failures as `refresh %s failed: %v` — one line
      per failure. The request context is unusable: `net/http` cancels it exactly when the
      revalidation starts
    - `Wait()` blocks on the `WaitGroup`. The ordering that makes tests deterministic is that
      `wg.Add(1)` happens before `Lookup` returns, so by the time a response reaches a test
      the counter is already incremented and `Wait()` cannot return early. The per-refresh
      timeout is what guarantees `Wait()` always terminates and no goroutine outlives the test
    - Key the in-flight map by product ID, which equals the barcode for external rows — that
      is exactly "one upstream request per barcode in flight". Hand-rolled map plus mutex,
      not `golang.org/x/sync/singleflight`: the callers want no result, the second caller
      should drop rather than block, and the type already carries a `WaitGroup`
    - _Requirements: 2.3, 2.4, 2.5, 2.6, 5.5, 8.3, 8.4_
    - _Properties: 5, 6_

  - [x] 3.5 Wire the `Refresher` into `LookupService` and stamp newly cached products
    - Add a nil-tolerant `Refresher interface { ScheduleRefresh(ctx, productID string) }`
      field and a `Now func() time.Time` field to `LookupService`. Nil `Refresher` disables
      revalidation so every existing literal keeps compiling and behaving as it does today
    - The inline `Catalog` interface needs **no widening** — `GetProductByID` and
      `CreateProduct` are already listed with unchanged signatures
    - In the Tier-2 branch, after resolving `product` and before returning, call
      `s.Refresher.ScheduleRefresh(ctx, product.ID)` when `Refresher != nil`. Build the
      `LookupResult` from values read before that call so the response is identical whether
      a revalidation was scheduled or not, and whether it later succeeded or failed
    - In `persistExternalProduct`, set `Source: SourceExternal` and `RefreshedAt: &now`
      where `now = s.now()`; `NameOverridden` stays false by zero value
    - Tier 1, not-found, and the `Source` contract (`user_override` / `global` / `external` /
      empty) are untouched; the Tier-2 branch keeps returning the conservative `"global"`
    - _Requirements: 1.6, 2.1, 2.2, 2.3, 8.6_
    - _Properties: 2, 4_

  - [x] 3.6 Unit and property tests for the pure functions
    - **Property 7: A merge takes an upstream field only when upstream supplied one**
    - **Property 12: The outcome classifies the field delta**
    - **Property 3: Staleness is exactly "never checked, or older than the TTL"**
    - In a new `internal/product/refresh_test.go`: a `rapid` property test over generated
      cached rows and upstream summaries asserting each of the four merged fields equals the
      upstream value when non-empty (and, for `name`, when `name_overridden` is false) and
      the prior cached value otherwise; plus a table enumerating the field-presence
      combinations including all-empty upstream
    - A `classify` table over field tuples covering each of the four fields differing
      individually and all four equal
    - An `isStale` table across NULL `refreshed_at`, inside the TTL, outside the TTL, and
      exactly at the TTL — the boundary is only reachable by calling the predicate directly
    - `rapid` is already in `go.mod`; minimum 100 iterations, tagged
      `Feature: product-cache-freshness, Property N: <property text>`
    - _Requirements: 1.5, 3.1, 3.2, 3.3, 4.4, 4.5, 5.1_
    - _Properties: 3, 7, 12_

  - [x] 3.7 Concurrency tests for single-flight and goroutine lifetime
    - **Property 6: Concurrent revalidations of one barcode collapse to one upstream request**
    - In `internal/product/refresh_test.go`, hold a fake upstream inside `LookupBarcode` on a
      release channel, fire N concurrent `ScheduleRefresh` calls for one barcode, release,
      `Wait()`, then assert the upstream call count for that barcode is exactly 1. The
      blocking upstream is what proves the in-flight guard *rejected* the duplicates rather
      than merely serializing them — a count of 1 with a non-blocking fake proves nothing
    - Assert `Wait()` returns and leaves no goroutine the `Refresher` started
      (Requirement 8.4), and that a second `ScheduleRefresh` after `Wait()` is accepted, so
      the in-flight entry was actually released
    - Run these under `go test -race`
    - _Requirements: 2.5, 8.3, 8.4_
    - _Properties: 6_

- [x] 4. Checkpoint - schema, catalog, and refresher compile and their tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 5. Test infrastructure for the API tests

  - [x] 5.1 Add `fakeClock`
    - A `sync.Mutex`-guarded `time.Time` with `Now() time.Time` and
      `Advance(d time.Duration)`, in `internal/server/`
    - It gets wired in as both `Refresher.Now` and `LookupService.Now`, so tests move time
      instead of waiting for it — that is how Requirement 8.5 stays satisfied in the tests
      as well as in the implementation
    - _Requirements: 8.2, 8.5_

  - [x] 5.2 Extend `fakeOpenFoodFacts`
    - Add a `sync.Mutex` guarding `store`, the new `errs` map, and the new `calls` map.
      Background revalidation goroutines call `LookupBarcode` concurrently with the test
      body, so the fake is now shared mutable state and the race detector will say so
    - `SeedError(barcode string, err error)` records an error to return instead of a product,
      distinct from an absent barcode (which keeps yielding `ErrProductNotFound`). This is
      what drives the 502 path and Requirement 5.3
    - `CallCount(barcode string) int` is the observable behind "no upstream request was made"
      and "exactly one was made"
    - A blocking seed (`SeedBlocking(barcode string, release <-chan struct{})` or equivalent)
      holds a call inside `LookupBarcode` for the concurrency assertions
    - Document the re-seeding contract: `Seed` overwrites its map entry and `LookupBarcode`
      reads at call time, so re-seeding a barcode between the cache fill and the refresh is
      how every merge test makes the upstream answer differ from what was cached
    - _Requirements: 5.3, 8.3_
    - _Properties: 5, 6, 11, 17_

  - [x] 5.3 Widen `testEnv` and make `exchanges()` reuse the same `Refresher`
    - Add `Refresher *product.Refresher` and `Clock *fakeClock` to `testEnv` in
      `internal/server/test_runner_test.go`
    - `setupTestWithDB` constructs the `fakeClock` and the `Refresher` (TTL, the fake OFF
      client, `ExternalLookupEnabled: true`, `Now` from the clock) and passes the same
      instance to both `LookupService` and `NewHandler`
    - **`exchanges()` must thread `env.Refresher` through instead of rebuilding one.** It
      currently constructs a fresh `LookupService` and calls `NewHandler` from scratch; a
      second `Refresher` owns a different `WaitGroup` and in-flight map, so
      `env.Refresher.Wait()` would return without awaiting goroutines the exchange started
      and the tests would flake silently. This is not optional cleanup
    - Add a `setupExternalProduct(id, name, category string, refreshedAt *time.Time)` helper
      alongside the existing `setupProduct*` family. Leave those helpers creating
      `source = 'user'` rows (empty `Source` defaults to user) — that is correct for them and
      is what keeps existing tests from starting revalidations
    - Lands together with the widened `NewHandler` signature from task 6.4
    - _Requirements: 8.2, 8.3_

- [x] 6. Configuration, the refresh endpoint, and wiring

  - [x] 6.1 Add the `BadGateway` error constructor
    - In `internal/server/handler_errors.go`, add
      `func BadGateway(message string) error` returning
      `&HTTPError{Code: http.StatusBadGateway, Message: message}` — 502 has no existing helper
    - _Requirements: 4.9_
    - _Properties: 14_

  - [x] 6.2 Add `RefreshHandler`
    - New file `internal/server/handler_refresh.go` with `RefreshHandler` holding inline
      `Refresher` (`Refresh(ctx, productID) (product.RefreshOutcome, error)`) and `Catalog`
      (`GetProductByID`) interfaces, a `refreshProductPathParams{ID string}`, and a
      `refreshProductResponse{Product product.Product; Outcome product.RefreshOutcome}`
    - No request body. Flow: empty `ID` → `BadRequest("product id is required")` matching
      `UpdateHandler` (unreachable through the route pattern, but consistent);
      `errors.Is(err, product.ErrRefreshTargetMissing)` → `NotFound("product not found")` → 404;
      any other error → `BadGateway("could not refresh product from Open Food Facts")` → 502,
      with the row already intact because `Refresh` writes no field values on that path;
      otherwise re-read via `GetProductByID` and return 200
    - The re-read is what makes "responds after the revalidation completes" observable: the
      body carries post-merge values including the new `refreshedAt`
    - A `source = 'user'` row falls out as 200 `"outcome":"unchanged"` with untouched values,
      because `Refresh` short-circuits before writing
    - _Requirements: 4.1, 4.3, 4.7, 4.8, 4.9_
    - _Properties: 11, 13, 14_

  - [x] 6.3 Add `productCacheTTL()` and construct the `Refresher` in `main.go`
    - `const defaultProductCacheTTL = 30 * 24 * time.Hour`. `productCacheTTL()` reads
      `PRODUCT_CACHE_TTL`: empty or unset → default; valid Go duration → that duration;
      unparseable → `log.Printf` naming the offending value, then the default. Startup never
      fails on this value
    - A value that parses is used as given even if non-positive: `0s` is a legitimate
      "always revalidate" debugging setting, and the single-flight guard keeps it from
      stampeding upstream
    - Hoist `externalLookupEnabled := os.Getenv("DISABLE_EXTERNAL_PRODUCT_LOOKUP") != "true"`
      and use it both to select `disabledProductLookup` and to set
      `Refresher.ExternalLookupEnabled`. The stub stays where it is, serving the Tier-3
      lookup path unchanged
    - Construct the `Refresher` (catalog, external client, TTL, flag) and pass it to
      `LookupService.Refresher`
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 8.1_
    - _Properties: 16, 17_

  - [x] 6.4 Widen `NewHandler` and register the route
    - `server.NewHandler` gains a `refresher *product.Refresher` parameter, constructed in
      `main.go` where the OFF client and the TTL config already live
    - Register `mux.HandleFunc("POST /api/products/{id}/refresh", HandleJSON(refreshHandler.Handle))`
      alongside the other product routes, in the existing `HandleJSON` style
    - Update the `main.go` call site
    - _Requirements: 4.1, 8.1_

  - [x] 6.5 Unit test `productCacheTTL`
    - **Property 16: TTL configuration is a total function**
    - In `cmd/server/main_test.go`, table-drive unset, empty, valid durations, and several
      unparseable values, asserting the parsed duration or the 30-day default and that no
      call panics or exits. This reads the environment before any server exists, so it
      cannot be an API test
    - _Requirements: 6.1, 6.2, 6.3_
    - _Properties: 16_

- [x] 7. API tests

  - [x] 7.1 Refresh outcome cases
    - **Property 11: The endpoint refreshes regardless of freshness and reports the stored result**
    - New `internal/server/handler_refresh_test.go` using `handlerTestCase` / `runHandlerTests`
    - Upstream answer changed → 200, `"outcome":"updated"`, body carries the new values, and
      `afterRequest: exchanges(GET /api/products)` confirms the row was persisted rather than
      echoed
    - Upstream answer identical → 200, `"outcome":"unchanged"`
    - Fresh external row (clock inside the TTL) → still calls upstream and still returns 200,
      because `Refresh` ignores the TTL
    - Barcode absent from the fake → 200, `"outcome":"not_found_upstream"`, and
      `exchanges(GET /api/products)` confirms the cached name and image survived
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 5.1, 5.4_
    - _Properties: 8, 9, 11, 12_

  - [x] 7.2 Refresh failure and ownership cases
    - **Property 13: Refresh never writes a row it does not own**
    - **Property 14: The endpoint's failure statuses are total**
    - **Property 17: Disabling external lookup makes refresh inert**
    - `SeedError` returns a transport error → 502, and `exchanges(GET /api/products)` confirms
      all four field values are untouched
    - Unknown product id → 404 with a message stating the product was not found
    - `source = 'user'` row → 200, `"outcome":"unchanged"`, values untouched, and
      `env.OpenFoodFacts.CallCount` for that id is 0
    - `Refresher.ExternalLookupEnabled` false → 200, `"outcome":"unchanged"`, `CallCount` 0,
      and every column of the row unchanged including `refreshed_at`
    - _Requirements: 4.7, 4.8, 4.9, 5.1, 6.4, 7.1_
    - _Properties: 13, 14, 17_

  - [x] 7.3 Stale-while-revalidate lookup tests
    - **Property 5: A stale lookup revalidates behind the response**
    - **Property 4: A lookup response is unaffected by revalidation**
    - **Property 8: Every attempt stamps `refreshed_at`**
    - **Property 2: Writes record provenance**
    - Add to `internal/server/handler_lookup_test.go`. Stale external row: re-seed the fake
      with a different name, `GET /api/products/lookup` returns the **cached** name and
      `CallCount` is 0 at that moment, then `env.Refresher.Wait()` and
      `exchanges(GET /api/products)` shows the merged name
    - Fresh external row (`env.Clock` inside the TTL): response as before, `Wait()`,
      `CallCount` still 0
    - Stale lookup where the fake errors: the lookup response is byte-identical to the
      no-revalidation case, and the row survives
    - Tier-3 persistence provenance: seed the fake so the barcode is **absent** from the local
      catalog, `GET /api/products/lookup` resolves it at Tier 3, and the persisted row has
      `source = 'external'`, `name_overridden` false, and `refreshed_at` equal to the
      `env.Clock` time at persist time
    - `refreshed_at` precision is the one documented place a direct `env.DB` query in
      `afterRequest` is warranted — asserting the exact stamp against `env.Clock` is not
      expressible through the API. Everything else in this task goes through `exchanges()`
    - _Requirements: 1.6, 2.1, 2.2, 2.3, 2.4, 2.6, 3.6, 5.2, 5.3, 5.6, 8.6_
    - _Properties: 2, 3, 4, 5, 8_

  - [x] 7.4 Name-override flow
    - **Property 10: A name edit on an external row claims the name**
    - **Property 7: A merge takes an upstream field only when upstream supplied one**
    - In `handler_refresh_test.go`: PUT a new name on an external row, then refresh with a
      different upstream name → the user's name survives while category and image update
    - PUT a category-only change that submits the **same** name, then refresh → the name
      **does** update, which is what proves Requirement 3.5 rather than merely asserting it
    - Verify both through `exchanges(GET /api/products)`
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5_
    - _Properties: 7, 10_

  - [x] 7.5 User-override precedence across a revalidation
    - **Property 15: A user override outranks freshness**
    - In `handler_lookup_test.go`: seed one barcode with both a `user_override` mapping for
      the requesting user and a stale `global` mapping to an external row. Look up → the
      override product. Refresh the shadowed global row → look up again → still the override
      product, and the `barcodes` mappings retain their `barcode`, `source`, and `user_id`
    - _Requirements: 7.2, 7.3, 7.4_
    - _Properties: 9, 15_

- [x] 8. Final checkpoint - full suite, race detector, and coverage
  - Run the full suite and confirm every test from tasks 1–7 passes
  - Run `go test -race ./...` for the concurrency properties (Properties 6, 5 and the
    goroutine-lifetime assertion; the shared `fakeOpenFoodFacts` is now touched by background
    goroutines, so the race detector is the gate on its mutex)
  - Run `./scripts/test-coverage.sh` to enforce coverage thresholds and commit the updated
    script if the threshold increased, per AGENTS.md
  - Ensure all tests pass, ask the user if questions arise.

## Notes

**Ordering.** Waves follow dependencies, not task numbers. Three places where the two differ:

- Task 6.4 (the widened `NewHandler` signature) lands before task 5.3, because
  `setupTestWithDB` and `exchanges()` cannot pass a `Refresher` to a `NewHandler` that does
  not accept one.
- Tasks 2.1–2.4 all edit `internal/product/product.go` and tasks 3.1–3.4 all edit
  `internal/product/refresh.go`, so each is its own wave rather than running in parallel.
- Task 1.2 (the `Product` fields) precedes every `Catalog` change, and migration 1.1 precedes
  everything — without the columns nothing compiles or reads correctly.

**The non-empty guard in `mergeRefresh`.** `mergeRefresh` guards each upstream field with a
non-empty check, which is exactly what Requirements 3.1 and 3.2 specify.
`OpenFoodFactsClient.LookupBarcode` hardcodes `UnitOfMeasure: ""`, so an unconditional
overwrite would blank the unit on every product's first refresh. Requirement 5's intent — a
refresh never degrades a cached product — governs. The guard is localized to `mergeRefresh` and
is called out in task 3.2 so it does not get "fixed" back to an unconditional overwrite. If the
client later learns to parse `quantity` or `product_quantity_unit`, real units start propagating
with no further change.

**Preserved behavior; these files are the regression witness.**
`internal/product/lookup_properties_test.go`, `internal/product/product_test.go`, and
`internal/server/handler_external_persistence_test.go` encode behavior this feature must not
change, and they should keep passing **unmodified**. An edit to an existing test in any of them
signals design drift, not a test that needs updating. The single exception is a change strictly
required by the widened `testEnv` struct. Task 2.5 *adds* a new case to `product_test.go`; it
does not touch the existing ones.

**Verifying task per property.** Each of the 17 correctness properties, with the task that
verifies it:
1 → 1.3; 2 → 7.3, 2.5; 3 → 3.6, 7.3; 4 → 7.3; 5 → 7.3; 6 → 3.7; 7 → 3.6, 7.4;
8 → 7.1, 7.3 (the exact-stamp DB assertion); 9 → 7.1, 7.5; 10 → 7.4; 11 → 7.1; 12 → 3.6, 7.1;
13 → 7.2; 14 → 7.2; 15 → 7.5; 16 → 6.5; 17 → 7.2.

**Testing split.** API tests via `handlerTestCase` / `runHandlerTests` with
`afterRequest: exchanges(...)` are the default. Unit and `rapid` property tests are limited to
the eight targets the design's unit-test table names: `mergeRefresh`, `classify`, single-flight,
`isStale`, `productCacheTTL`, migration 004, `CreateProduct` source validation, and the
no-leaked-goroutine check. Sub-tasks marked `*` are those supplementary unit and property
tests; the API tests in task 7 are not optional, because they are where most properties live.

## Task Dependency Graph

```
1.1 migration 004 ─┬─ 1.3* migrate_test
1.2 Product fields ─┴─ 2.1 CreateProduct ── 2.2 reads ── 2.3 SaveRefresh/MarkRefreshed ── 2.4 UpdateProduct CASE
                                                                                   └─ 2.5* source validation
                                        ▼
3.1 refresh.go scaffolding + isStale
   ▼
3.2 mergeRefresh + classify ─────────────────────────── 3.6* pure-function tests
   ▼
3.3 Refresh
   ▼
3.4 ScheduleRefresh + Wait ───────────────────────────── 3.7* concurrency tests
   ▼
3.5 LookupService wiring        6.1 BadGateway
   ▼                              ▼
6.3 main.go TTL + Refresher     6.2 RefreshHandler
   ▼                              ▼
6.4 NewHandler + route ── 6.5* main_test
   ▼
5.1 fakeClock   5.2 fakeOpenFoodFacts
   ▼
4. checkpoint (schema, catalog, and refresher tests pass)
   ▼
5.3 testEnv + setupTestWithDB + exchanges()
   ▼
7.1 outcomes   7.3 stale-while-revalidate
7.2 failures   7.5 override precedence
7.4 name override
   ▼
8. checkpoint (all tests + -race + coverage)
```

```json
{"waves": [["1.1", "1.2"], ["1.3", "2.1"], ["2.2"], ["2.3"], ["2.4", "2.5"], ["3.1"], ["3.2"], ["3.3"], ["3.4"], ["3.5", "6.1"], ["6.2", "6.3"], ["3.6", "6.4", "6.5"], ["3.7", "5.1", "5.2"], ["4"], ["5.3"], ["7.1", "7.3"], ["7.2", "7.5"], ["7.4"], ["8"]]}
```
