# Implementation Plan: Open Products Facts Lookup

## Overview

Tier 3 widens from one hardcoded upstream to all four Product Opener databases. The plan
follows the design's dependency order: the schema lands first, then the generalized client,
then the provenance column plumbing, then the fan-out, then the two collaborators that
consume it, then configuration, then the test harness that can express per-database
outcomes, and only then the frontend — so the `externalSource` field exists and is proven
over HTTP before any UI reads it.

Per AGENTS.md, behavior is verified through the HTTP contract using `handlerTestCase` /
`runHandlerTests` with `afterRequest: exchanges(...)`. Unit and property tests in
`internal/product/` cover only what the API cannot reach: client decoding, the three miss
conditions, `ExternalSource.valid()`, `classifyFanOut`, goroutine accounting, and
`productMissTTL()` parsing. Property tests use `rapid` with a minimum of 100 iterations and
are tagged `Feature: open-products-facts-lookup, Property {n}: {property text}`.

Every task ends by running `./scripts/test-coverage.sh` so the threshold is enforced and the
auto-updated script is committed with the code, per AGENTS.md.

## Tasks

- [x] 1. Migration 005: provenance column and confirmed-miss table

  - [x] 1.1 Add migration `005_add_external_source_and_barcode_misses.sql`
    - Create `internal/app/migrations/005_add_external_source_and_barcode_misses.sql`
    - `ALTER TABLE products ADD COLUMN external_source TEXT` — nullable, no default, and
      deliberately **no backfill UPDATE**: a row cached before this feature genuinely has
      unknown provenance, so NULL is the correct value
    - `CREATE TABLE barcode_misses (barcode TEXT PRIMARY KEY, checked_at DATETIME NOT NULL)`
      — its own table, so no existing query has to learn to exclude it
    - Include the design's comment explaining that SQLite cannot add a CHECK constraint via
      `ALTER TABLE`, so the four-value set is enforced in Go (`Catalog.CreateProduct`),
      exactly as `products.source` already is
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 3.1, 3.2, 3.3_

  - [x] 1.2 Write migration tests for value preservation and NULL provenance
    - In `internal/app/migrate_test.go`, seed `products` rows with non-default `source`,
      `refreshed_at`, and `name_overridden` values, run `app.RunMigrations(conn)`, and assert
      those three columns are unchanged and `external_source` is NULL for every pre-existing row
    - Assert `barcode_misses` exists and that a second `RunMigrations` call stays idempotent
      (`schema_migrations` count unchanged)
    - Use `app.RunMigrations` — never inline schema
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 3.2, 3.3_

- [x] 2. Generalize the upstream client to any Product Opener database

  - [x] 2.1 Rename `OpenFoodFactsClient` to `ProductOpenerClient` with a construction-time source and base URL
    - Move `internal/product/openfoodfacts.go` to `internal/product/product_opener.go`
    - Add the `ExternalSource` string type with `ExternalSourceOpenFoodFacts`,
      `ExternalSourceOpenProductsFacts`, `ExternalSourceOpenBeautyFacts`, and
      `ExternalSourceOpenPetFoodFacts`; the prefixed names keep this value set visually
      distinct from the existing `SourceExternal` / `SourceUser` constants for `products.source`
    - Rename the type to `ProductOpenerClient` with `source` and `baseURL` fields;
      `NewProductOpenerClient(source, baseURL)` and
      `NewProductOpenerClientWithHTTPClient(source, baseURL, hc)`; add `Source()` and
      `BaseURL()` accessors so a test can confirm a fake server stands in for the intended database
    - Add the `productOpenerBaseURLs` map (data, not code) and `DefaultProductOpenerClients()`
      returning one client per database with the same retry transport and 10s timeout
    - Build the request with `http.NewRequestWithContext` instead of `httpClient.Get`, so a
      cancelled caller stops four requests rather than zero; the 10s `http.Client.Timeout`
      still bounds each request independently
    - Keep `ProductSummary.ID` set from the **requested** barcode, never the response `code`
      field, and keep the three miss conditions (HTTP 404, `status != 1`, empty
      `product_name`) and the hardcoded empty `UnitOfMeasure` exactly as they are
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 7.7_

  - [x] 2.2 Update the two call sites of the old constructor
    - `cmd/server/main.go` and `internal/product/openfoodfacts_test.go` are the only two;
      no `NewOpenFoodFactsClient` alias is kept, because retaining an OFF-specific
      constructor would assert OFF is architecturally special when the whole point is that
      it is not
    - Rename `internal/product/openfoodfacts_test.go` to `internal/product/product_opener_test.go`
      and update its mock-transport table to construct clients through `NewProductOpenerClientWithHTTPClient`
    - Confirm the package builds and the existing suite is green before moving on
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 7.1_

  - [x] 2.3 Write client decoding and miss-condition tests across all four databases
    - **Property 23: The wire format round-trips through the client**
    - **Property 24: Three conditions each mean unknown**
    - **Property 25: A resolved product's ID is the requested barcode**
    - **Validates: Requirements 7.4, 7.5, 7.6**
    - In `internal/product/product_opener_test.go`, generalize the existing table to run
      against all four `ExternalSource` values using `httptest` servers and real clients
      (this is what `BaseURL()` exists for)
    - Cover: `product_name` / `categories` / `image_front_small_url` decoding; HTTP 404,
      `status != 1`, and empty `product_name` each yielding `ErrProductNotFound`; and a
      response whose own `code` differs from the requested barcode still yielding a summary
      whose ID is the requested barcode
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 7.4, 7.5, 7.6_

  - [x] 2.4 Write a configuration-parity test for `DefaultProductOpenerClients()`
    - Assert all four clients carry the identical 10s timeout and the same retry transport
      configuration, and that each `BaseURL()` matches its `productOpenerBaseURLs` entry
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 7.2, 7.3_

- [x] 3. Carry provenance through the catalog and every response family

  - [x] 3.1 Add `ExternalSource.valid()` and the `products.external_source` column plumbing
    - Add `func (s ExternalSource) valid() bool` — the empty value is valid and means "no
      external provenance"
    - Add an `ExternalSource` field tagged `json:"externalSource,omitempty"` to both
      `Product` and `ProductSummary`; a value type with `""` meaning SQL NULL via
      `nullableString`, matching how `Category`, `UnitOfMeasure`, and `ImageURL` are already
      handled, so `omitempty` gives the "absent, not null" wire shape for free
    - In `internal/product/product.go`, add the column to `ListProducts`, `GetProductByID`,
      `CreateProduct`, and `SaveRefresh`, and add `COALESCE(p.external_source, '')` to
      `LookupByBarcode`
    - `CreateProduct` rejects an out-of-set value with a user-friendly message before
      touching the database, following the existing `source` precedent
    - `CreateProduct` through the product API stores NULL provenance
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 3.4, 3.5, 3.7, 9.1_

  - [x] 3.2 Add `COALESCE(p.external_source, '')` to the inventory and scan join queries
    - `internal/inventory/inventory.go`: the three item queries (list, by ID, by product)
    - `internal/scan/scan.go`: the three scan-entry queries, all scanned by `scanScanEntry`
    - These six queries plus `LookupByBarcode` are the complete set that builds a
      `ProductSummary`, which is what makes Requirements 9.1–9.3 one change rather than three
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 9.2, 9.3_

  - [x] 3.3 Write validation tests for the four-value set
    - **Property 9: Only the four values are storable**
    - **Validates: Requirements 3.7**
    - In `internal/product/product_test.go`, property-test that any string that is neither
      empty nor one of the four defined values causes `CreateProduct` to fail and write no row
    - Use `newTestCatalog` and `app.RunMigrations`, per AGENTS.md
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 3.7_

- [x] 4. Build the concurrent fan-out with a three-state outcome

  - [x] 4.1 Implement `ExternalLookup`, `FanOutResult`, and `classifyFanOut`
    - Create `internal/product/external_lookup.go`
    - `databasePrecedence` as a slice literal in the order Open Food Facts, Open Products
      Facts, Open Beauty Facts, Open Pet Food Facts — Requirement 2.2's confirmed pair and
      2.3's recommended remainder are both this literal, and reordering is a one-line edit
    - `FanOutOutcome` with `FanOutHit`, `FanOutConfirmedMiss`, `FanOutUnresolved`, and
      `FanOutResult{Outcome, Product, Source}` where `Product`/`Source` are set if and only
      if the outcome is a hit; the type carries no upstream error value at all, which is what
      makes "no failure detail in the response" structural rather than a discipline
    - `ExternalLookup{Clients map[ExternalSource]barcodeLookup}` plus `NewExternalLookup`; a
      database absent from the map is an `errNoUpstreamClient` **error**, never a miss, so a
      misconfigured deployment cannot manufacture a confirmed miss
    - `Lookup` allocates `replies := make([]upstreamReply, len(databasePrecedence))`, one
      goroutine per database each writing only its own precedence-indexed slot (no mutex, no
      shared mutable state), then `wg.Wait()` before returning — no goroutine outlives the call
    - No goroutine touches the database: the fan-out's collaborator interface has no catalog
      method, so every write happens on the calling goroutine after the join, which is what
      the `SetMaxOpenConns(1)` SQLite connection requires
    - `classifyFanOut` scans `replies` in precedence order and returns on the first hit
      (a hit outranks a later miss and a concurrent error); `errors.Is(err, ErrProductNotFound)`
      is a miss; anything else logs one line naming barcode, database, and reason and sets
      `sawError`, which downgrades a would-be confirmed miss to `FanOutUnresolved`
    - `LookupIn(ctx, source, barcode)` queries exactly one named database, for `Refresher`
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 1.1, 1.2, 1.3, 1.5, 2.1, 2.2, 2.3, 2.4, 6.4, 6.5_

  - [x] 4.2 Write `classifyFanOut` outcome and determinism tests
    - **Property 3: The precedence-earliest hit wins, every time**
    - **Property 21: A failure anywhere keeps the barcode retryable**
    - **Validates: Requirements 2.1, 2.2, 2.3, 2.4, 6.1, 6.4**
    - In `internal/product/external_lookup_test.go`, property-test over generated non-empty
      subsets of hitting databases that the precedence-earliest hit is always selected and
      that repeating the same reply set yields the same selection
    - Table-test the three outcomes: all-miss with no error is `FanOutConfirmedMiss`;
      any error with no hit is `FanOutUnresolved`; a hit alongside an error is `FanOutHit`
    - Include the missing-client case: a database absent from `Clients` classifies as an
      error, so an all-miss-except-absent fan-out is unresolved, not a confirmed miss
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 6.1, 6.4_

  - [x] 4.3 Write the goroutine-accounting and every-database-asked tests
    - **Property 1: Every database is asked**
    - **Property 32: A fan-out leaves nothing running**
    - **Validates: Requirements 1.1, 10.4**
    - In `internal/product/external_lookup_properties_test.go`, property-test over generated
      combinations of hits, misses, and failures that `runtime.NumGoroutine()` after `Lookup`
      returns equals the count before it started
    - Assert each of the four clients received exactly one `LookupBarcode` call per fan-out
    - No sleeps and no wall-clock waits anywhere in these tests
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 1.1, 10.4_

- [x] 5. Checkpoint - client, schema, and fan-out in place
  - Ensure all tests pass, ask the user if questions arise.

- [x] 6. Split the upstream into two consumer-side interfaces

  - [x] 6.1 Replace the `OpenFoodFacts` field with `Upstream` on both collaborators
    - `LookupService.Upstream` declares only `Lookup(ctx, barcode) FanOutResult`;
      `Refresher.Upstream` declares only `LookupIn(ctx, source, barcode) (*ProductSummary, error)`
      — narrow interfaces declared at the consuming field, the pattern already used for
      `LookupService.Catalog` and `Refresher.Catalog`
    - Add the `UpstreamDatabases` union in `internal/product/external_lookup.go` so one value
      satisfies both collaborators and the disable switch cannot half-apply
    - The rename is deliberate: after this change the field holds four databases, and a field
      named `OpenFoodFacts` would be actively misleading at every call site
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 1.1, 4.1, 4.2_

  - [x] 6.2 Update the four call sites the field rename reaches
    - `cmd/server/main.go`, `internal/server/setup_test.go`,
      `internal/server/test_runner_test.go`, and `internal/server/handler_scan_headless_test.go`
    - This subtask is compile-level only: inject the existing single-database behavior through
      the new field names and keep the suite green; per-database test expression arrives with
      the harness generalization in task 11
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 1.1, 4.1_

- [x] 7. Add the confirmed-miss data-access methods

  - [x] 7.1 Implement `RecordBarcodeMiss`, `GetBarcodeMiss`, and `DeleteBarcodeMiss` on `Catalog`
    - A barcode's known-unknown status belongs to the same lookup concern as `products` and
      `barcodes`, so the three methods go on the existing `Catalog` rather than introducing a
      second data-access type (AGENTS.md allows exactly one per feature package)
    - `RecordBarcodeMiss` is a single upsert
      (`ON CONFLICT(barcode) DO UPDATE SET checked_at = excluded.checked_at`), which serves
      both the first recording and the re-stamp of an existing miss with no read-modify-write
    - `GetBarcodeMiss` returns `(*time.Time, error)`, nil when there is no record
    - Requirement 5.7 holds by construction: no product, inventory, or scan query mentions
      `barcode_misses`, and `Refresher` iterates `products` rows with `source = 'external'`
      so it never sees a miss
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 5.1, 5.5, 5.7_

- [x] 8. Rewrite `LookupService.Lookup` with the miss gate and the three-way outcome switch

  - [x] 8.1 Add the miss gate after Tier 1/2 and branch on the fan-out outcome
    - Keep the empty-argument validation, Tier 1, Tier 2, the `ScheduleRefresh` call, and the
      conservative `"global"` Source default exactly as they are
    - Place the `GetBarcodeMiss` gate **after** Tier 1/2, not before: a user who resolved a
      flagged entry by creating a product has a `barcodes` mapping, so Tier 2 answers and the
      stale miss row is never consulted. Reordering these two steps breaks Requirement 5.8
    - Expiry is `s.now().Sub(*miss) <= s.missTTL()` — subtraction against the injected clock,
      no sleep and no wall-clock wait
    - A live miss returns the same empty `LookupResult` and empty `Source` a fresh all-miss
      returns, so the flagged-entry workflow cannot tell the difference
    - Switch on the outcome: `FanOutHit` persists and returns `Source: "external"`;
      `FanOutConfirmedMiss` calls `RecordBarcodeMiss(ctx, barcode, s.now())` and returns
      not-found; `FanOutUnresolved` writes nothing and returns not-found, leaving the barcode
      retryable and telling the caller nothing about the upstream failure
    - Surface a `GetBarcodeMiss` read failure as a lookup error with a user-friendly message
      containing no function name
    - `persistExternalProduct(ctx, p, source)` takes the winning source, stores
      `Source: SourceExternal` with `ExternalSource: source`, keeps the existing
      `UpsertBarcodeMapping(ctx, p.ID, p.ID, "global", "")`, and then calls
      `DeleteBarcodeMiss(ctx, p.ID)` to clear now-dead data
    - Add the `MissTTL time.Duration` field with a `missTTL()` accessor defaulting to
      `defaultMissTTL` when zero, so existing struct literals keep compiling
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 1.4, 1.6, 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.8, 6.1, 6.2, 6.6, 10.5, 10.6_

- [x] 9. Target refreshes at the database a row's provenance names

  - [x] 9.1 Resolve the refresh target from `external_source` with a legacy fallback
    - Leave the `ExternalLookupEnabled` gate, `GetProductByID`, the nil check, and the
      `source != SourceExternal` early return unchanged — that early return is what makes a
      user row query nothing and change nothing
    - Preserve the `ExternalLookupEnabled` gate's existing reasoning comment: inferring
      "disabled" from an `ErrProductNotFound` value would stamp `refreshed_at` and suppress
      real revalidation for a full TTL, and it now matters more because a disabled fan-out
      must also not manufacture a confirmed miss
    - `target := row.ExternalSource`, falling back to `ExternalSourceOpenFoodFacts` when
      empty — the only database a legacy row could have come from
    - Call `r.Upstream.LookupIn(ctx, target, row.ID)` — exactly one database, and `row.ID`
      passed directly because the external-row invariant "product ID is the barcode" holds
    - Leave both failure paths alone: a miss and a transport error each stamp `refreshed_at`
      via `MarkRefreshed`, which writes `refreshed_at` and nothing else. That is precisely
      why a miss preserves `name`, `category`, `unit_of_measure`, `image_url`, and
      `external_source`; broadening `MarkRefreshed` would regress Requirement 4.7
    - Set `merged.ExternalSource = target` unconditionally after `mergeRefresh` — a no-op
      rewrite for a row that already had provenance (3.6) and the stamp that records origin
      for a legacy row (4.4)
    - Add `external_source = ?` to `SaveRefresh`'s `UPDATE`; leave `classify` and
      `mergeRefresh` untouched
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 3.6, 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7_

- [x] 10. Wire configuration in `cmd/server/main.go`

  - [x] 10.1 Add `productMissTTL()` and the disabled-lookup stub, and inject one upstream value
    - `productMissTTL()` reads `PRODUCT_MISS_TTL`, shaped exactly like the existing
      `productCacheTTL()`: empty returns `defaultMissTTL` (7 days), an unparseable value logs
      once naming the offending value and defaults rather than failing startup, and a value
      that parses is used as given even if non-positive, since `0s` is a legitimate
      "never cache a miss" debugging setting
    - Build `var upstream product.UpstreamDatabases = product.NewExternalLookup(product.DefaultProductOpenerClients())`
      and swap in `disabledExternalLookup{}` when `DISABLE_EXTERNAL_PRODUCT_LOOKUP` is `true`
    - `disabledExternalLookup.Lookup` returns `FanOutUnresolved`, **not**
      `FanOutConfirmedMiss`: reporting a confirmed miss would record one for every barcode
      scanned while lookup is off, and each would then stay unresolvable for a full
      `Miss_TTL` after lookup is switched back on
    - `disabledExternalLookup.LookupIn` preserves the existing stub's `ErrProductNotFound`
      return; `Refresher` never reaches it because `ExternalLookupEnabled` short-circuits first
    - Inject the single `upstream` value into **both** `Refresher.Upstream` and
      `LookupService.Upstream`, as today, so the disable switch cannot half-apply; pass
      `MissTTL: productMissTTL()` to `LookupService` and leave `PRODUCT_CACHE_TTL` as the
      single `Refresher.TTL` with no per-database variant
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7, 8.8_

  - [x] 10.2 Write `productMissTTL()` parsing tests
    - **Property 29: A duration setting round-trips, and a non-duration defaults**
    - **Validates: Requirements 8.6, 8.7**
    - In `cmd/server/main_test.go`, property-test that any duration's string form round-trips
      through `productMissTTL()`, and that any string no duration parser accepts yields 7 days
    - Assert the invalid value is logged exactly once and names the offending value; assert
      unset and empty both yield 7 days
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 8.5, 8.6, 8.7_

- [x] 11. Generalize the test harness to four independently controllable databases

  - [x] 11.1 Rename `fakeOpenFoodFacts` to `fakeProductOpener` with no behavior change
    - Rename `internal/server/open_food_facts_fake_test.go` to
      `internal/server/product_opener_fake_test.go` and the type to `fakeProductOpener`
    - The existing shape is already right for one database — an in-memory store plus
      per-barcode error and blocking seeding, mutex-guarded because background goroutines call
      it concurrently. Change nothing about that behavior in this subtask
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 10.1_

  - [x] 11.2 Add `fakeUpstream` and change `setupTestWithDB` to return `(http.Handler, testEnv)`
    - `fakeUpstream` **embeds** `*product.ExternalLookup` and holds
      `databases map[product.ExternalSource]*fakeProductOpener`, with a
      `Database(source)` accessor. Embedding the real value means a test that seeds one
      database and asserts a winner is exercising the shipped `classifyFanOut` and
      `databasePrecedence`, not a test copy of them — and it supplies both `Lookup` and
      `LookupIn` for free
    - Keep `testEnv.OpenFoodFacts` pointing at the Open Food Facts fake, so every existing
      `env.OpenFoodFacts.Seed(...)` and `env.OpenFoodFacts.CallCount(...)` call compiles
      unchanged and means the same thing; add `Upstream *fakeUpstream` for per-database seeding
    - `setupTestWithDB` would need seven return values; change its signature to
      `(http.Handler, testEnv)` instead, building and returning the fully populated `testEnv`.
      This stops the signature growing with every feature and keeps the "same `Refresher`,
      same `fakeClock`" contract its doc comment already explains
    - Update the three in-package call sites: `runHandlerTests`, `newExternalHarness`, and the
      ad-hoc handler builder in `handler_scan_headless_test.go`
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 10.1, 10.2_

  - [x] 11.3 Propagate `MissTTL` through `exchanges()`
    - `exchanges()` builds a second `LookupService` against the same state; it must receive
      the same `Upstream`, `Refresher`, `Now`, **and** `MissTTL`
    - A missing `MissTTL` there silently defaults to zero and makes every cached-miss
      assertion in an `afterRequest` sequence pass for the wrong reason. Add a comment saying
      so, alongside the existing one about reusing `env.Refresher`
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 10.1, 10.2, 10.3_

- [x] 12. Prove the backend behavior through the HTTP API

  - [x] 12.1 Test the fan-out, precedence, and failure safety over HTTP
    - **Property 1: Every database is asked**
    - **Property 2: A lone hit resolves with source external**
    - **Property 3: The precedence-earliest hit wins, every time**
    - **Property 4: A hit outranks a concurrent failure**
    - **Property 21: A failure anywhere keeps the barcode retryable**
    - **Property 22: Upstream failure detail never reaches the caller**
    - **Validates: Requirements 1.1, 1.4, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 6.1, 6.2, 6.3, 6.6**
    - New `internal/server/handler_product_opener_lookup_test.go` using `handlerTestCase` /
      `runHandlerTests` and `env.Upstream.Database(source)` to express per-database outcomes
    - Cover: a lone hit from each of the four databases resolving with `source: "external"`;
      overlapping hits selecting the precedence-earliest and creating **no** flagged scan
      entry; a hit alongside a failure returning and persisting the hit; every-database
      failure returning not-found with no `products` row and a retry on the next lookup
      (assert via `afterRequest: exchanges(...)` plus `CallCount`)
    - Assert the response body contains no part of the seeded upstream failure message
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 1.1, 1.4, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 6.1, 6.2, 6.3, 6.6_

  - [x] 12.2 Test provenance in every response family and the unchanged fast tiers
    - **Property 5: The persisted external row keeps its pre-feature shape**
    - **Property 6: Provenance round-trips through storage and every response family**
    - **Property 7: A user-created product has no provenance**
    - **Property 33: The fast tiers are untouched**
    - **Validates: Requirements 1.6, 3.4, 3.5, 9.1, 9.2, 9.3, 10.5, 10.6**
    - New `internal/server/handler_provenance_test.go` asserting `externalSource` appears at
      `$.product.externalSource` on `GET /api/products/lookup`,
      `$[*].product.externalSource` on `GET /api/scans`,
      `$[*].item.product.externalSource` on `GET /api/inventory`, and
      `$[*].externalSource` on `GET /api/products`
    - Assert the persisted row still has `source: external` with a `global` barcodes mapping,
      and that an Open Food Facts resolution differs from its pre-feature values only by the
      added `externalSource`
    - Assert a product created through the product API carries **no** `externalSource` key
      (absent, not null) in any response
    - Assert a Tier 1 override and a Tier 2 catalog hit return the same result shape and
      `Source` value as before and query no database (`CallCount` zero on all four)
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 1.6, 3.4, 3.5, 9.1, 9.2, 9.3, 10.5, 10.6_

  - [x] 12.3 Test the confirmed-miss lifecycle over HTTP
    - **Property 15: A confirmed miss suppresses the fan-out for its whole window**
    - **Property 16: A re-confirmed miss restarts its window**
    - **Property 17: A barcode that becomes known stops being a miss**
    - **Property 18: A cached miss is indistinguishable from a fresh unknown**
    - **Property 19: Confirmed misses are invisible to every response**
    - **Property 20: A user-created product outranks a cached miss**
    - **Validates: Requirements 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8**
    - New `internal/server/handler_barcode_miss_test.go`; expire misses with
      `env.Clock.Advance`, never a sleep
    - Cover: an all-miss fan-out suppressing every upstream call within the TTL; expiry
      re-querying all four; a now-resolving barcode being persisted and returned on every
      later lookup; a second all-miss restarting the window; a cached miss producing the same
      result shape, `Source` value, and flagged scan entry as a fresh unknown
    - Assert Requirement 5.7 positively over HTTP: record misses, then confirm
      `GET /api/products`, `GET /api/products/lookup`, and `GET /api/inventory` contain nothing
      for those barcodes
    - Assert a user-created product for a missed barcode wins on every subsequent lookup with
      zero upstream calls, because the miss gate sits after Tier 2
    - A direct `env.DB` read of `barcode_misses.checked_at` is the one acceptable last resort
      here, and only for timestamp precision — the behavior itself is fully assertable over
      HTTP, so skip the DB read unless precision is genuinely in question
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8_

  - [x] 12.4 Test refresh targeting, the legacy stamp, and merge preservation
    - **Property 8: A refresh never rewrites provenance**
    - **Property 10: A refresh queries exactly the database its row names**
    - **Property 11: A successful refresh of a legacy row records where the data came from**
    - **Property 12: A user row is inert under refresh**
    - **Property 13: A refresh hit merges as it did before the feature**
    - **Property 14: A refresh miss preserves every cached field**
    - **Property 28: One cache TTL governs every provenance**
    - **Validates: Requirements 3.6, 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 8.4**
    - New `internal/server/handler_refresh_provenance_test.go` driving refreshes through the
      refresh endpoint and asserting per-database `CallCount`: exactly one call, against the
      database the row names, and zero against the other three
    - Cover: a legacy row (NULL provenance) targeting Open Food Facts and reporting
      `openfoodfacts` after a successful revalidation; a provenance-carrying row keeping its
      value across hit, miss, and error outcomes; a user row querying nothing and changing
      nothing; a hit taking upstream fields only where upstream supplied non-empty values,
      stamping `refreshedAt`, and reporting the pre-feature outcome classification; a miss
      leaving name, category, unit of measure, image URL, and provenance intact
    - Assert staleness is decided by the single configured `PRODUCT_CACHE_TTL` regardless of
      which database supplied the row
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 3.6, 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 8.4_

  - [x] 12.5 Test the disable switch
    - **Property 26: Disabling upstream lookup disables it completely and cheaply**
    - **Property 27: Disabling upstream lookup freezes every row**
    - **Validates: Requirements 8.1, 8.2, 8.3**
    - New `internal/server/handler_external_disabled_test.go` wiring a `LookupService` and
      `Refresher` with the disabled upstream
    - Assert a Tier-3 lookup queries no database, records no confirmed miss, and that
      re-enabling lookup resolves the barcode immediately **with no clock advance** — this is
      what distinguishes `FanOutUnresolved` from `FanOutConfirmedMiss` and the assertion that
      would fail if the stub reported a confirmed miss
    - Assert a refresh with lookup disabled leaves every field value and `refreshedAt` unchanged
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 8.1, 8.2, 8.3_

- [x] 13. Checkpoint - backend complete and the API field is proven
  - Ensure all tests pass, ask the user if questions arise.

- [x] 14. Surface provenance on the two frontend surfaces

  - [x] 14.1 Add the `ExternalSource` type and the optional `ProductSummary` field
    - In `frontend/src/types/index.ts`, add the four-member `ExternalSource` union and
      `externalSource?: ExternalSource` on `ProductSummary`
    - `frontend/src/api/client.ts` needs no change: the field is additive and optional and the
      client returns these types structurally
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 9.1, 9.2, 9.3_

  - [x] 14.2 Create the `ProvenanceBadge` component
    - New `frontend/src/components/product/ProvenanceBadge.tsx` owning the
      `DATABASE_NAMES: Record<ExternalSource, string>` map, so the two surfaces cannot drift
    - `return null` when the value is absent **or** unrecognized, so a future fifth database
      shipped to the backend before the frontend cannot render a badge reading "undefined"
    - Render a `Badge` with `size="xs"`, `variant="light"`, and an `aria-label` of
      "Product data from {name}", keeping the bare database name as the visible text, so the
      accessible name is a superset of the visual label rather than a contradiction of it
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 9.4, 9.5, 9.6, 9.7_

  - [x] 14.3 Write `ProvenanceBadge` component tests
    - **Property 30: A provenance label names its database accessibly**
    - **Property 31: No provenance, no label**
    - **Validates: Requirements 9.4, 9.5, 9.6, 9.7**
    - Vitest tests in `frontend/src/components/product/ProvenanceBadge.test.tsx`: each of the
      four values renders the mapped name and carries the matching accessible name; an absent
      value and an unrecognized value each render nothing
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 9.4, 9.5, 9.6, 9.7_

  - [x] 14.4 Place the badge on the scan card and the inventory row
    - `ScanEntryCard.tsx`: beneath the existing `Barcode: {entry.barcode}` line in the header
      stack, as `<ProvenanceBadge externalSource={entry.product?.externalSource} />` — with
      the product metadata rather than the `Flagged` status badge, because it describes the
      data's origin, not the entry's state
    - `ItemRow.tsx`: in the `Stack` under the category `Text`, as
      `<ProvenanceBadge externalSource={item.product.externalSource} />`. `ItemRow` is
      `memo`-wrapped and the value is read off the existing `inventoryItem` object, so no
      memo boundary changes
    - `FlaggedEntryResolver` needs no change: a flagged entry has no product, so there is no
      provenance to show
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 9.4, 9.5, 9.6_

  - [x] 14.5 Write placement tests for both surfaces
    - **Property 30: A provenance label names its database accessibly**
    - **Property 31: No provenance, no label**
    - **Validates: Requirements 9.4, 9.5, 9.6, 9.7**
    - One test each in `ScanEntryCard.test.tsx` and a new `ItemRow.test.tsx`: the badge appears
      with a provenance-carrying product and does not appear otherwise
    - Run `./scripts/test-coverage.sh`
    - _Requirements: 9.4, 9.5, 9.6, 9.7_

- [x] 15. Final checkpoint - full suite and coverage threshold
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional test polish and can be skipped for a faster MVP; the
  core implementation subtasks are never optional.
- Every subtask ends with `./scripts/test-coverage.sh`, which enforces the threshold and
  auto-updates it on an increase so the change commits with the code, per AGENTS.md.
- The load-bearing invariant across tasks 4, 8, and 10: **a hit beats an error, and an error
  beats a cached miss.** Anything short of a clean all-miss leaves the barcode retryable, so a
  transient outage cannot harden into a permanent "unknown".
- Two orderings are behavioral, not stylistic. The miss gate in task 8.1 sits **after**
  Tier 1/2, which is what satisfies Requirement 5.8. The disabled stub in task 10.1 reports
  `FanOutUnresolved`, not a confirmed miss, which is what keeps barcodes scanned during a
  disabled window resolvable the moment lookup is re-enabled.
- Task 11.2 embeds the real `*product.ExternalLookup` in `fakeUpstream` on purpose: the fakes
  substitute at the HTTP boundary only, so every precedence and classification assertion in
  task 12 exercises shipped code rather than a test reimplementation.
- Tasks 1–13 complete and prove the backend before task 14 touches the frontend, so the
  `externalSource` field exists on the wire before any UI consumes it.
- Property tests run a minimum of 100 iterations and are tagged
  `Feature: open-products-facts-lookup, Property {n}: {property text}`.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "2.1"] },
    { "id": 1, "tasks": ["1.2", "2.2", "3.1"] },
    { "id": 2, "tasks": ["2.3", "3.2", "4.1"] },
    { "id": 3, "tasks": ["2.4", "3.3", "4.2", "6.1"] },
    { "id": 4, "tasks": ["4.3", "6.2", "7.1"] },
    { "id": 5, "tasks": ["8.1", "9.1"] },
    { "id": 6, "tasks": ["10.1", "11.1"] },
    { "id": 7, "tasks": ["10.2", "11.2"] },
    { "id": 8, "tasks": ["11.3"] },
    { "id": 9, "tasks": ["12.1", "12.2", "12.3", "12.4", "12.5"] },
    { "id": 10, "tasks": ["14.1"] },
    { "id": 11, "tasks": ["14.2"] },
    { "id": 12, "tasks": ["14.3", "14.4"] },
    { "id": 13, "tasks": ["14.5"] }
  ]
}
```
