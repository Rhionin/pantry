# Implementation Plan

## Overview

This plan fixes the missing persistence of externally-resolved products on Tier-3
lookups. It follows the exploratory bugfix workflow: surface counterexamples on the
UNFIXED code first, lock in the behavior that must not change, then apply the fix and
re-run the same tests to confirm the bug is resolved with no regressions. Behavior is
verified through the HTTP contract (`handlerTestCase` / `runHandlerTests` +
`exchanges()`) wherever the effect is API-observable, with property-based and unit
tests covering logic not reachable through the API.

## Tasks

This plan follows the exploratory bugfix workflow: surface counterexamples on the
UNFIXED code first (Task 1), lock in the behavior that must not change (Task 2),
then apply the fix and re-run the same tests to confirm the bug is resolved with no
regressions (Task 3). Per AGENTS.md, behavior is verified through the HTTP contract
using `handlerTestCase` / `runHandlerTests` + `exchanges()` wherever the effect is
API-observable; property-based tests (rapid) cover the quantified properties in
`internal/product/lookup_properties_test.go` (modeled on
`internal/scan/scan_properties_test.go`); unit tests cover only logic not reachable
through the API (idempotency side effects, migration `002`).

The **Bug Condition** is `X.source == "external" AND productRowExists(X.product.ID) == false`
(design "Bug Details"). The fix persists the externally-resolved product idempotently
inside the Tier-3 branch of `internal/product/lookup.go`, stores the barcode mapping as
source `"global"` (respecting the `barcodes.source` CHECK), and adds a data-repair
migration `internal/app/migrations/002_backfill_orphaned_products.sql`. Inventory queries
stay inner joins (design Fix Implementation, decision 4).

---

- [x] 1. Write bug condition exploration tests (BEFORE implementing the fix)
  - **Property 1: Bug Condition** - External Product Is Persisted And Visible
  - **CRITICAL**: These tests MUST FAIL on the UNFIXED code - failure confirms the bug exists (missing Tier-3 persistence)
  - **DO NOT attempt to fix the test or the code when it fails at this stage**
  - **NOTE**: These tests encode the expected behavior - they will validate the fix when they pass after implementation
  - **GOAL**: Surface counterexamples that demonstrate the item disappears from inventory after an external-only resolution
  - **Scoped PBT Approach**: This is a deterministic, reproducible bug — scope the exploration to the confirmed counterexample barcode `011110728227` plus a small set of fake-resolved barcodes rather than the full string domain
  - Add API tests in a new `internal/server/handler_external_persistence_test.go` using `handlerTestCase` / `runHandlerTests`; drive the external seam via `env.OpenFoodFacts` (`fakeOpenFoodFacts`) so the barcode resolves only at Tier 3:
    - **Case: external scan → commit → inventory (primary counterexample)** — fake resolves `011110728227` to a named product; POST create scan (stock-in), PATCH/commit, then `afterRequest: exchanges(GET /api/inventory)`. On UNFIXED code the item is absent (empty list / omitted). Confirms Req 1.1, 1.2, 1.3.
    - **Case: external scan → product not retrievable by ID** — after the external scan create, assert the resolved product ID is NOT retrievable through the product-by-ID / catalog path on UNFIXED code. Confirms Req 1.1, 2.5 gap.
    - **Case: inventory vs. instances inconsistency** — after commit, `exchanges(GET /api/inventory/{itemId}/instances, GET /api/inventory)`: the instances endpoint returns the live instance while `GET /api/inventory` omits the parent. Confirms Req 1.4.
    - **Case: pre-existing orphan (edge case)** — in `setup`, seed `items` + `item_instances` rows via `env.DB` whose `product_id` has no `products` row; `exchanges(GET /api/inventory)` omits it on UNFIXED code. Confirms Req 1.5, 2.6.
  - **EXPECTED OUTCOME**: All cases FAIL on the UNFIXED code (this is correct — it proves the bug exists)
  - Document the counterexamples found (e.g., "committing an external scan of `011110728227` yields `GET /api/inventory` == [] while `/instances` returns the instance; no `products` row was written on Tier-3 resolution")
  - Mark this task complete when the tests are written, run, and their failure is documented
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5_

- [x] 2. Write preservation property tests (BEFORE implementing the fix)
  - **Property 2: Preservation** - Non-External Resolutions Unchanged
  - **IMPORTANT**: Follow the observation-first methodology — run the UNFIXED code for non-bug-condition inputs, record the actual outputs, then encode those observed outputs as assertions
  - **GOAL**: Lock in the behavior for Tier 1 (user override), Tier 2 (global), not-found, and already-persisted products so the fix cannot regress it
  - Add property-based tests in a new `internal/product/lookup_properties_test.go` (rapid, modeled on `internal/scan/scan_properties_test.go`) initialized via `app.RunMigrations(conn)` per AGENTS.md:
    - **User-override preservation** — seed a user override; assert `Lookup` returns it and that `products` / `barcodes` row counts are unchanged after the lookup. Observe on UNFIXED code first. Confirms Req 3.1.
    - **Global preservation** — seed a global product + mapping; assert `Lookup` returns the global product and creates no new `products` / `barcodes` rows. Confirms Req 3.2.
    - **Not-found preservation** — fake returns not-found (or errors); assert `Lookup` returns a not-found result (`IsFound() == false`) and persists nothing. Confirms Req 3.3.
    - **Result equality** — for generated override / global / not-found scenarios, assert the returned `LookupResult` (Product + Source) matches the observed original result.
  - Add an API preservation test in `internal/server/handler_external_persistence_test.go` using `handlerTestCase` + `exchanges()`:
    - **Already-persisted inventory preservation** — `setup` seeds products via `env.ProductStore`; commit a stock-in for a pre-seeded product; `exchanges(GET /api/inventory)` shows the item present and ordered by product name. Confirms Req 3.4, 3.5.
  - **EXPECTED OUTCOME**: All preservation tests PASS on the UNFIXED code (this confirms the baseline behavior to preserve)
  - Mark this task complete when the tests are written, run, and passing on unfixed code
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5_

- [x] 3. Fix for missing external-product persistence on Tier-3 resolution

  - [x] 3.1 Widen the LookupService Catalog interface and persist the external product
    - In `internal/product/lookup.go`, widen the `LookupService.Catalog` interface to add `GetProductByID(ctx, id) (*Product, error)`, `CreateProduct(ctx, Product) error`, and `UpsertBarcodeMapping(ctx, barcode, productID, source, userID string) error` (`*product.Catalog` already implements all four, so `cmd/server/main.go` and the apitest wiring in `test_runner_test.go` need no call-site changes)
    - In the Tier-3 branch, after Open Food Facts returns a product, call a new `persistExternalProduct(ctx, product)` helper before returning `LookupResult{Product: product, Source: "external"}`
    - `persistExternalProduct` MUST be idempotent: check `GetProductByID(product.ID)` first and skip `CreateProduct` when a row already exists (avoids a primary-key conflict); otherwise `CreateProduct(Product{ID: product.ID, Name, Category, UnitOfMeasure})` where `product.ID == barcode` for external results
    - _Bug_Condition: isBugCondition(X) == (X.source == "external" AND productRowExists(X.product.ID) == false)_
    - _Expected_Behavior: after Lookup, productExistsInCatalog(result.Product.ID) is true and the returned ID references a real products row (Property 1)_
    - _Preservation: Tier 1/Tier 2/not-found branches are untouched; only the Tier-3 return path changes (Property 2)_
    - _Requirements: 2.1, 2.2, 2.5_

  - [x] 3.2 Persist the external barcode mapping as source "global"
    - In `persistExternalProduct`, call `UpsertBarcodeMapping(barcode, product.ID, "global", "")` — NOT `"external"` — because `barcodes.source` has `CHECK (source IN ('global', 'user_override'))`
    - `UpsertBarcodeMapping` uses `INSERT OR REPLACE` and is naturally idempotent under `UNIQUE (barcode, source, user_id)`; a subsequent scan of the same barcode then resolves at Tier 2 as `global`
    - Keep `LookupResult.Source == "external"` on first discovery (the tier that produced it) to preserve the existing `Source` contract for that call
    - _Bug_Condition: isBugCondition(X) == (X.source == "external" AND productRowExists(X.product.ID) == false)_
    - _Expected_Behavior: the external mapping exists in barcodes with source "global"; no CHECK violation (design Fix Implementation, decision 2)_
    - _Preservation: does not alter Tier-2 read semantics for pre-existing global/override rows (Property 2)_
    - _Requirements: 2.1_

  - [x] 3.3 Add the data-repair migration for pre-existing orphans
    - Create `internal/app/migrations/002_backfill_orphaned_products.sql` (the runner applies `.sql` files in lexicographic order and is idempotent per-file via `schema_migrations`)
    - Backfill a placeholder `products` row for every orphaned item so the inner join matches, generically (not just barcode `011110728227`):
      `INSERT INTO products (id, name) SELECT DISTINCT i.product_id, 'Product ' || i.product_id FROM items i LEFT JOIN products p ON p.id = i.product_id WHERE p.id IS NULL;`
    - Do NOT relax inventory queries to a LEFT JOIN — keep the inner joins in `internal/inventory/inventory.go` (design Fix Implementation, decision 4)
    - _Bug_Condition: itemHasNoProductsRow(item) — an items row whose product_id has no products row_
    - _Expected_Behavior: after RunMigrations, every such item references an existing products row and appears in GET /api/inventory (Property 4)_
    - _Preservation: items whose products already exist are untouched by the WHERE p.id IS NULL guard (Property 2)_
    - _Requirements: 2.6, 2.7_

  - [x] 3.4 Verify bug condition exploration tests now pass
    - **Property 1: Expected Behavior** - External Product Is Persisted And Visible
    - **IMPORTANT**: Re-run the SAME tests from Task 1 — do NOT write new tests
    - The Task 1 tests encode the expected behavior; when they pass they confirm the external product is persisted, retrievable by ID, and its committed item is visible in `GET /api/inventory`, and the inventory/instances views are consistent
    - Also confirm the pre-existing-orphan exploration case now passes because `RunMigrations` (which runs in test setup) applies migration `002`
    - **EXPECTED OUTCOME**: Tests PASS (confirms the bug is fixed)
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7_

  - [x] 3.5 Verify preservation tests still pass
    - **Property 2: Preservation** - Non-External Resolutions Unchanged
    - **IMPORTANT**: Re-run the SAME tests from Task 2 — do NOT write new tests
    - Confirm Tier 1 / Tier 2 / not-found lookups return identical `LookupResult`s, create no duplicate `products` / `barcodes` rows, and that already-persisted items still appear in `GET /api/inventory` ordered by name
    - **EXPECTED OUTCOME**: Tests PASS (confirms no regressions)
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5_

- [x] 4. Add idempotency and migration coverage (logic not reachable through the API)
  - Per AGENTS.md, reserve unit tests for logic the HTTP API cannot trigger directly:
    - **Idempotency (Property 3)** — in `internal/product/lookup_properties_test.go`, add a property-based test: for a generated barcode looked up externally N≥2 times, assert exactly one `products` row and one `global` `barcodes` mapping for that barcode/source/user scope, respecting `UNIQUE (barcode, source, user_id)`, and a stable returned product ID each time. Verify side effects via `Catalog.GetProductByID` / `LookupByBarcode` (a second lookup now resolves at Tier 2 as `global`). Optionally extend the table-driven `TestLookupService` in `internal/product/lookup_test.go` with a persistence-side-effect case.
    - **Migration 002 behavior (Property 4)** — in `internal/app/migrate_test.go`, seed an orphaned `items` row, run `app.RunMigrations(conn)`, assert the backfilled `products` row exists, and assert re-running `RunMigrations` stays idempotent (`schema_migrations` count unchanged, no duplicate `products` rows). Use `app.RunMigrations` — never duplicate schema inline.
  - _Requirements: 2.1, 2.6, 2.7_
  - _Properties: Property 3 (Idempotency), Property 4 (Data Repair)_

- [x] 5. Checkpoint - Ensure all tests pass and coverage is enforced
  - Run the full suite and confirm every test from Tasks 1–4 passes (exploration tests now green, preservation tests still green, idempotency and migration tests green)
  - Run `./scripts/test-coverage.sh` to enforce (and lock in) coverage thresholds; commit the updated script if the threshold increased, per AGENTS.md
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
   3.1 Widen Catalog interface + persistExternalProduct (persist product)
        ▼
   3.2 Persist barcode mapping as "global"      (depends on 3.1)
        ▼
   3.3 Data-repair migration 002                (independent of 3.1/3.2; groups here)
        ▼
   3.4 Re-run Task 1 tests → now PASS           (depends on 3.1, 3.2, 3.3)
   3.5 Re-run Task 2 tests → still PASS         (depends on 3.1, 3.2, 3.3)
          ▼
Task 4 (Idempotency [Property 3] + migration [Property 4] unit/property coverage)
          ▼                                     (depends on 3.1–3.3)
Task 5 (Checkpoint: all tests pass + ./scripts/test-coverage.sh)
          ▲
          └── depends on Tasks 1–4
```

```json
{"waves": [["1", "2"], ["3.1"], ["3.2", "3.3"], ["3.4", "3.5"], ["4"], ["5"]]}
```

## Notes

**Ordering notes:**
- Tasks 1 and 2 are written FIRST, against UNFIXED code (test-first): Task 1 must fail, Task 2 must pass.
- Task 3 subtasks are ordered so persistence (3.1) precedes the mapping decision (3.2); the migration (3.3) is independent but grouped so all fix code lands before the verification subtasks (3.4, 3.5).
- Task 4 depends on the fix (3.1–3.3) being in place.
- Task 5 gates completion on everything above and enforces coverage.
