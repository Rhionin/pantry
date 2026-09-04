# External Product Persistence Bugfix Design

## Overview

When a barcode is resolved through the external Open Food Facts lookup (Tier 3 of the three-tier
lookup in `internal/product/lookup.go`), the resolved product summary is returned to the caller but
is never written to the `products` table. `ScanCreateHandler` stores the returned `Product.ID`
(which for external results is the barcode itself) on the scan entry, and committing a stock-in
creates an `items` row referencing that `product_id`. Because no matching `products` row exists,
every inventory read — all of which use an inner `JOIN products p ON p.id = i.product_id` in
`internal/inventory/inventory.go` — silently drops the item. The result: an externally-sourced
product is invisible in `GET /api/inventory` even though the `items` and `item_instances` rows
exist, and the scan card renders "Unknown product".

The fix persists the externally-resolved product at resolution time, inside the `LookupService`
Tier-3 path, so that the `Product.ID` returned to the handler already references a real `products`
row (and a `barcodes` mapping). Persistence is idempotent: repeated lookups of the same barcode
must not create duplicate rows and must respect the `UNIQUE (barcode, source, user_id)` constraint
on `barcodes`. A one-time repair migration backfills the pre-existing orphaned item. The cosmetic
"Unknown product" display is addressed by fixing persistence at the source; a LEFT-JOIN fallback in
inventory queries is evaluated below and recommended *against* as the primary fix.

The strategy keeps the blast radius small: Tier 1 (user override) and Tier 2 (global) resolutions
already reference persisted products and MUST behave identically after the fix.

## Glossary

- **Bug_Condition (C)**: A barcode resolution whose source is `"external"` (Tier 3). This is the
  only source that returns a product ID without a backing `products` row.
- **Property (P)**: For an external resolution, the returned product ID references an existing
  `products` row, its barcode mapping exists, and any item committed from it is visible in inventory.
- **Preservation**: Behavior for Tier 1 (user override), Tier 2 (global), and not-found resolutions,
  and for already-persisted products, must remain byte-for-byte identical.
- **LookupService**: The three-tier orchestrator in `internal/product/lookup.go`. Its `Lookup`
  method returns a `LookupResult{Product, Source}`.
- **Catalog**: The product data-access type (`product.Catalog`, `product.NewCatalog`) in
  `internal/product/product.go`. Owns `products` and `barcodes` writes (`CreateProduct`,
  `UpsertBarcodeMapping`, `LookupByBarcode`, `GetProductByID`).
- **external source**: Open Food Facts. The external client sets `ProductSummary.ID = barcode`.
- **orphaned item**: An `items` row whose `product_id` has no corresponding `products` row.

## Bug Details

### Bug Condition

The bug manifests when the three-tier lookup falls through to Tier 3 and Open Food Facts returns a
product. `LookupService.Lookup` builds a `LookupResult` with `Source = "external"` and a
`ProductSummary` whose `ID` is the barcode, then returns it without persisting anything. The scan
create handler records that ID on the scan entry; on commit, an `items` row is created referencing a
`product_id` that has no `products` row, so the inner-join inventory queries exclude it.

**Formal Specification:**
```
FUNCTION isBugCondition(X)
  INPUT: X of type BarcodeResolution   // a barcode resolved during scan/commit
  OUTPUT: boolean

  RETURN X.source = "external"
         AND productRowExists(X.product.ID) = false
END FUNCTION
```

The `productRowExists(...) = false` clause captures why Tier 1/Tier 2 are exempt: those sources
resolve *through* the `barcodes`→`products` join, so a `products` row is guaranteed to already exist.

### Examples

- **Kroger lemon juice (confirmed counterexample)**: Scan barcode `011110728227`. Open Food Facts
  returns a product named e.g. "Lemon Juice". Scan entry is stored `status=pending,
  product_id=011110728227`. A stock-in is committed, creating an `items` row and an `item_instances`
  row. `products` is empty. `GET /api/inventory` returns `[]` (item dropped by inner join), while
  `GET /api/inventory/{itemId}/instances` returns the live instance — inventory is internally
  inconsistent. Expected: the product is persisted and the item appears in `GET /api/inventory`.
- **Repeated external scan (idempotency)**: Scan `011110728227` twice. Expected: exactly one
  `products` row and one `global` `barcodes` mapping — no duplicate rows and no
  `UNIQUE (barcode, source, user_id)` violation.
- **Tier 2 global product (must be unaffected)**: Barcode already mapped to a global product. Lookup
  returns the existing product; no new `products` or `barcodes` row is created; item is visible in
  inventory exactly as before.
- **Not-found (edge case, must be unaffected)**: Barcode unknown to all tiers. Lookup returns
  not-found; nothing is persisted; scan entry is flagged as before.

## Expected Behavior

### Preservation Requirements

**Unchanged Behaviors:**
- Tier 1 user-override resolutions return the override product and create no duplicate `products`
  or `barcodes` rows (Req 3.1).
- Tier 2 global resolutions return the global product and create no duplicate rows (Req 3.2).
- Not-found and external-error resolutions return a not-found result and persist nothing (Req 3.3).
- Stock-in commits for already-persisted products create the item and show it in
  `GET /api/inventory` exactly as before (Req 3.4).
- `GET /api/inventory` continues to return already-persisted items ordered by product name (Req 3.5).

**Scope:**
All resolutions where `isBugCondition` is false MUST be completely unaffected. This includes:
- User-override (Tier 1) lookups
- Global-DB (Tier 2) lookups
- Not-found results (external API returns not-found or errors)
- Any inventory read for items whose products already exist

The actual expected correct behavior for external resolutions (persistence and visibility) is
defined in the Correctness Properties section (Property 1) and the data-repair behavior in
Property 3.

## Hypothesized Root Cause

Based on the bug analysis and confirmed reproduction, the root cause is precisely located:

1. **Missing persistence on Tier-3 resolution**: In `LookupService.Lookup`, the external branch
   returns `LookupResult{Product: product, Source: "external"}` without ever calling
   `Catalog.CreateProduct` or `Catalog.UpsertBarcodeMapping`. This is the single root cause. The
   scan-create and commit paths are correct given their contract — they trust that a returned
   `Product.ID` references a real row, which holds for Tier 1/2 but not Tier 3.

2. **Inner join in inventory queries (amplifier, not cause)**: `getItemByUserAndProduct`,
   `getItemByID`, and `ListItems` in `internal/inventory/inventory.go` use
   `JOIN products p ON p.id = i.product_id`. This correctly hides orphaned items but turns the
   missing-persistence defect into a *silent* disappearance. Fixing persistence removes the orphan
   condition; the join behavior is otherwise correct and should not be relaxed as the primary fix
   (see Fix Implementation, decision 4).

3. **`UpsertBarcodeMapping` source CHECK (constraint to respect)**: The `barcodes.source` column has
   `CHECK (source IN ('global', 'user_override'))`. Persisting the external mapping as source
   `"external"` (as suggested in Req 2.1's phrasing) would violate this CHECK. The mapping must be
   stored with an allowed source. See Fix Implementation, decision 2.

4. **Pre-existing orphan (separate, in-scope concern)**: The Kroger lemon juice row was created
   before the fix and will remain orphaned after the code fix, because the fix only applies to new
   resolutions. A data-repair migration is required (Req 2.6).

## Correctness Properties

Property 1: Bug Condition - External Product Is Persisted And Visible

_For any_ resolution where the bug condition holds (source is `external`, i.e. `isBugCondition`
returns true), the fixed `LookupService.Lookup` SHALL persist the resolved product into the
`products` table and its barcode mapping into the `barcodes` table before returning, so that the
returned `Product.ID` references an existing `products` row; and any `items` row committed from that
resolution SHALL be visible in `GET /api/inventory` and retrievable by product ID.

**Validates: Requirements 2.1, 2.2, 2.3, 2.4, 2.5**

Property 2: Preservation - Non-External Resolutions Unchanged

_For any_ resolution where the bug condition does NOT hold (user override, global, or not-found),
the fixed code SHALL produce the same `LookupResult` as the original, SHALL NOT create any duplicate
`products` or `barcodes` rows, and SHALL leave the inventory view identical to the original.

**Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5**

Property 3: Idempotency - Repeated External Resolution Creates No Duplicates

_For any_ barcode resolved externally two or more times, the fixed persistence path SHALL create at
most one `products` row and one `barcodes` mapping for that barcode/source/user scope, respecting
the `UNIQUE (barcode, source, user_id)` constraint, and SHALL return the same product ID each time.

**Validates: Requirements 2.1**

Property 4: Data Repair - Pre-existing Orphans Become Visible

_For any_ `items` row whose `product_id` has no `products` row at the time the repair runs, the
repair SHALL insert a backing `products` row so the item references an existing product and becomes
visible in `GET /api/inventory`.

**Validates: Requirements 2.6, 2.7**

## Fix Implementation

### Changes Required

Assuming the root cause analysis is correct, the fix has four parts.

**1. Persist the external product inside the Tier-3 path.**

**File**: `internal/product/lookup.go`
**Function**: `LookupService.Lookup`

The `LookupService` already depends on `Catalog` through a narrow interface. Widen that interface so
the service can persist on external resolution:

```go
Catalog interface {
    LookupByBarcode(ctx context.Context, barcode, userID string) (*ProductSummary, error)
    GetProductByID(ctx context.Context, id string) (*Product, error)
    CreateProduct(ctx context.Context, product Product) error
    UpsertBarcodeMapping(ctx context.Context, barcode, productID, source, userID string) error
}
```

`*product.Catalog` already implements all four methods, so `cmd/server/main.go` and the apitest
wiring in `test_runner_test.go` need no signature changes at the call site.

In the Tier-3 branch, after Open Food Facts returns a product, persist it idempotently before
returning:

```go
// External API returned a product. Persist it so the returned ID references a real products row.
if err := s.persistExternalProduct(ctx, product); err != nil {
    return LookupResult{}, err
}
return LookupResult{Product: product, Source: "external"}, nil
```

`persistExternalProduct` performs an idempotent upsert:
- Check `GetProductByID(product.ID)`. If a row already exists (e.g. a prior external scan created
  it), skip the `CreateProduct` insert to avoid a primary-key conflict.
- Otherwise `CreateProduct(Product{ID: product.ID (=barcode), Name, Category, UnitOfMeasure})`.
- `UpsertBarcodeMapping(barcode, product.ID, "global", "")` — this uses `INSERT OR REPLACE` and is
  naturally idempotent under `UNIQUE (barcode, source, user_id)`.

Because `product.ID == barcode` for external results, the `products.id` and the `barcodes.barcode`
are the same value; the barcode mapping links the two consistently.

**2. Store the external barcode mapping as source `"global"`, not `"external"`.**

Req 2.1 mentions persisting the mapping, and the bug condition names the *resolution* source
`"external"`. However, the `barcodes.source` CHECK constraint only permits `'global'` and
`'user_override'`. Introducing a new `'external'` source would require a schema change and would
change Tier-2 lookup semantics (`LookupByBarcode` only reads `global`/`user_override`). The correct,
minimal choice is to persist externally-discovered products as **global** catalog entries: a product
that Open Food Facts knows about is legitimately a global product, and storing it as `global` means
a subsequent scan of the same barcode by any user resolves at Tier 2 without another external call.
The `LookupResult.Source` returned to callers remains `"external"` on first discovery (that is the
tier that produced it), preserving the existing `Source` contract for that call. This decision is
documented here as the resolution of the "persist as external" phrasing in Req 2.1.

**3. Add a data-repair migration for pre-existing orphans.**

**File**: `internal/app/migrations/002_backfill_orphaned_products.sql`

Add a new numbered migration (the migration runner applies `.sql` files in lexicographic order and
is idempotent per-file via `schema_migrations`). It backfills a placeholder `products` row for every
orphaned item so the inner join matches:

```sql
-- Backfill products rows for items whose product_id has no products row.
-- Such orphans were created before external-product persistence existed
-- (e.g. barcode 011110728227). The product name is unknown at repair time,
-- so we seed a stable placeholder keyed on the product_id (which for external
-- resolutions equals the barcode). A later external re-scan upserts the real name.
INSERT INTO products (id, name)
SELECT DISTINCT i.product_id, 'Product ' || i.product_id
FROM items i
LEFT JOIN products p ON p.id = i.product_id
WHERE p.id IS NULL;
```

Notes:
- The migration is expressed purely in SQL and runs inside `RunMigrations`, so tests initialize it
  via `app.RunMigrations(conn)` with no inline schema duplication (per AGENTS.md).
- It targets any orphan generically, not just the one known barcode, satisfying "_for any_ item" in
  Property 4.
- The placeholder name is a defensible best-effort: the original external name was never persisted,
  so it is unrecoverable from the DB alone. Re-scanning the barcode after the fix will upsert the
  real Open Food Facts name via the `UpsertBarcodeMapping`/name-refresh path.

**4. Decision: keep inventory queries as inner joins; do NOT add a LEFT JOIN fallback as the fix.**

The "Unknown product" display and the dropped-item behavior share one root cause: a missing
`products` row. Two options were considered:

- **(a) Fix persistence only (recommended).** Once every item references a real product, the inner
  joins return the item and its real name, and the repair migration covers historical orphans. This
  keeps inventory queries simple and preserves the existing ordering-by-name contract (Req 3.5).
- **(b) Relax inventory queries to `LEFT JOIN products` with a `COALESCE(p.name, 'Unknown product')`
  fallback as defense-in-depth.** This would surface orphaned items even if some future path forgets
  to persist. But it masks data-integrity bugs (an orphan would silently render as "Unknown
  product" forever instead of being caught), changes `ORDER BY p.name` semantics for null names, and
  broadens the diff beyond the root cause.

**Recommendation: (a).** Fix persistence at the source and repair existing data. Do not change the
inventory join. Defense-in-depth via LEFT JOIN is explicitly rejected here because the invariant
"every item references a real product" is now guaranteed by the persistence fix plus the
`items.product_id REFERENCES products(id)` foreign key, and masking violations would hide
regressions that the tests below are designed to catch. Req 2.7 ("display resolved name rather than
'Unknown product'") is satisfied because the item now always has a real (or backfilled) product row.

## Testing Strategy

### Validation Approach

Two phases: first surface counterexamples that demonstrate the bug on the UNFIXED code (confirming
the root cause), then verify the fix persists external products, repairs orphans, and preserves all
non-external behavior. Per AGENTS.md, prefer API tests in `internal/server/` using the
`handlerTestCase` / `runHandlerTests` framework with `exchanges()` for `afterRequest` verification,
so behavior is checked through the HTTP contract rather than direct DB queries. Property-based tests
(rapid, following `internal/scan/scan_properties_test.go`) cover the quantified properties.

### Exploratory Bug Condition Checking

**Goal**: Surface counterexamples that demonstrate the bug BEFORE implementing the fix, and confirm
the root cause is missing persistence. If refuted, re-hypothesize.

**Test Plan**: Using the server apitest framework with the `fakeOpenFoodFacts` seam, simulate an
external-only resolution end to end: create a scan entry for a barcode the fake resolves, set it to
stock-in, commit it, then read `GET /api/inventory`. Run against the UNFIXED code to observe the
item missing from inventory.

**Test Cases**:
1. **External scan then inventory (primary counterexample)**: `fakeOpenFoodFacts` returns a product
   for barcode `011110728227`; create scan → commit stock-in → `GET /api/inventory`. Expected on
   unfixed code: item absent (empty list). Confirms Req 1.1–1.3.
2. **External scan then product-by-ID**: after the external scan create, assert the product is NOT
   retrievable by ID on unfixed code. Confirms Req 1.1 / 2.5 gap.
3. **Inventory vs. instances inconsistency**: after commit, `GET /api/inventory/{itemId}/instances`
   returns the instance while `GET /api/inventory` omits the parent. Confirms Req 1.4.
4. **Pre-existing orphan (edge case)**: seed an `items` + `item_instances` row whose `product_id`
   has no `products` row; `GET /api/inventory` omits it on unfixed code. Confirms Req 1.5 / 2.6.

**Expected Counterexamples**:
- `GET /api/inventory` returns `[]` (or omits the item) after committing an externally-resolved
  stock-in, while the instance endpoint returns the live instance.
- Cause: no `products` row was written on Tier-3 resolution.

### Fix Checking

**Goal**: For all inputs where the bug condition holds, the fixed lookup persists the product and
the committed item is visible.

**Pseudocode:**
```
FOR ALL X WHERE isBugCondition(X) DO
  result := Lookup_fixed(X.barcode, X.userID)
  ASSERT productExistsInCatalog(result.Product.ID)          // 2.1, 2.5
  itemID := commitStockIn_fixed(X)
  ASSERT itemReferencesExistingProduct(itemID)              // 2.2
  ASSERT itemAppearsIn(GET "/api/inventory")                // 2.3, 2.4
END FOR
```

**Data-repair fix checking:**
```
FOR ALL item WHERE itemHasNoProductsRow(item) DO
  runRepair()   // migration 002
  ASSERT itemReferencesExistingProduct(item.id)             // 2.6
  ASSERT itemAppearsIn(GET "/api/inventory")                // 2.6, 2.7
END FOR
```

### Preservation Checking

**Goal**: For all inputs where the bug condition does NOT hold, the fixed system behaves identically
to the original.

**Pseudocode:**
```
FOR ALL X WHERE NOT isBugCondition(X) DO
  ASSERT Lookup_original(X) = Lookup_fixed(X)               // 3.1, 3.2, 3.3
  ASSERT productRowCount_before = productRowCount_after     // no duplicate rows
  ASSERT inventoryView_original(X) = inventoryView_fixed(X) // 3.4, 3.5
END FOR
```

**Testing Approach**: Property-based testing is recommended for preservation because it generates
many resolutions across the input domain (override / global / not-found) and asserts row counts are
unchanged and results are stable. Observe behavior on non-external inputs first, then encode it.

**Test Cases**:
1. **User-override preservation**: seed an override; lookup returns it; assert `products`/`barcodes`
   row counts unchanged after lookup (Req 3.1).
2. **Global preservation**: seed a global product; lookup returns it; assert no new rows (Req 3.2).
3. **Not-found preservation**: fake returns not-found; lookup returns not-found; nothing persisted;
   scan entry flagged (Req 3.3).
4. **Already-persisted inventory preservation**: commit a stock-in for a pre-seeded product; item
   appears in `GET /api/inventory` ordered by name (Req 3.4, 3.5).

### Unit Tests

Reserved for logic not reachable through the HTTP API (per AGENTS.md, prefer API tests):
- `LookupService.persistExternalProduct` idempotency at the package level in
  `internal/product/lookup_test.go`: calling `Lookup` twice for the same external barcode yields one
  `products` row and one `global` barcode mapping (Property 3), extending the existing table-driven
  `TestLookupService` with a case that asserts persistence side effects via `Catalog.GetProductByID`
  and `LookupByBarcode` (a second lookup now resolves at Tier 2 as `global`).
- Migration `002` behavior in `internal/app/migrate_test.go`: seed an orphan, run `RunMigrations`,
  assert the backfilled `products` row exists and re-running migrations stays idempotent.

### Property-Based Tests

Following the rapid style in `internal/scan/scan_properties_test.go`, placed in
`internal/product/lookup_properties_test.go`:
- **Property 1 (Bug Condition)**: for generated barcodes resolved only externally, assert a
  `products` row exists after `Lookup` and the returned ID matches.
- **Property 2 (Preservation)**: for generated override/global/not-found scenarios, assert
  `Lookup_fixed` equals the original result and `products`/`barcodes` row counts are unchanged.
- **Property 3 (Idempotency)**: for a generated barcode looked up N≥2 times externally, assert
  exactly one `products` row and one `global` `barcodes` row and a stable returned ID.
- **Property 4 (Data Repair)**: for a generated set of orphaned items, assert every item references
  a real product after `RunMigrations`.

### Integration Tests

Full-flow API tests in `internal/server/` (`handler_scan_create_test.go` and/or a dedicated
`handler_external_persistence_test.go`) using `handlerTestCase` + `exchanges()`:
- **External scan → commit → visible**: `fakeOpenFoodFacts` resolves the barcode; POST create scan,
  PATCH to stock-in and commit, then `afterRequest: exchanges(GET /api/inventory)` asserts the item
  is present with its resolved name (Req 2.2–2.5, 2.7).
- **Repeated external scan idempotency**: two create-scan exchanges for the same barcode; assert via
  a later `GET` that the product resolves and only one item/product results (Req 2.1, Property 3).
- **Orphan repair via migration**: seed an orphaned item in `setup` (writing `items`/`item_instances`
  with no `products` row through `env.DB`), and because `RunMigrations` runs in test setup, assert
  through `exchanges(GET /api/inventory)` that the repaired item appears (Req 2.6, 2.7).
- **Preservation**: global and override barcodes still resolve and their items still appear in
  `GET /api/inventory` ordered by name, unchanged from before (Req 3.1, 3.2, 3.4, 3.5).

After implementation, run `./scripts/test-coverage.sh` to enforce (and lock in) coverage thresholds
per AGENTS.md.
