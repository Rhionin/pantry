# Bugfix Requirements Document

## Introduction

When a barcode is resolved through the external Open Food Facts lookup (Tier 3 of the three-tier lookup in `internal/product/lookup.go`), the returned product is never written to the `products` table. The scan entry records `product_id = <barcode>`, and committing a stock-in creates an `items` row referencing that `product_id`. Because no matching `products` row exists, the item is orphaned: every inventory query in `internal/inventory/inventory.go` uses an inner `JOIN products p ON p.id = i.product_id`, so the item silently disappears from all inventory views and the scan card shows "Unknown product".

This defect makes any product sourced exclusively from the external API invisible after stock-in, even though the item and its live instances exist in the database. Confirmed against the live dev DB with Kroger lemon juice (barcode `011110728227`): scan entry `status=committed, product_id=011110728227`, an `items` row and a live `item_instances` row both present, the `products` table empty, `GET /api/inventory` returns `[]`, and `GET /api/inventory/{itemId}/instances` returns the instance.

The core defect is the missing product persistence on external resolution. Two secondary concerns are in scope for regression coverage but are distinct from the root cause: repairing the already-orphaned row created before the fix, and the cosmetic "Unknown product" display when an item's product cannot be named.

## Bug Analysis

### Current Behavior (Defect)

When an external (Tier 3) lookup resolves a barcode, the resolved product is not persisted, and any item created from it is dropped by inventory queries.

1.1 WHEN a barcode is resolved only by the external Open Food Facts lookup (Tier 3) THEN the system returns the product summary to the caller without inserting a row into the `products` table
1.2 WHEN a stock-in scan entry whose `product_id` was set from an external-only resolution is committed THEN the system creates an `items` row referencing a `product_id` that has no corresponding `products` row
1.3 WHEN `GET /api/inventory` is called after committing such an externally-resolved item THEN the system omits that item from the response because the inner `JOIN products` matches no product row
1.4 WHEN `GET /api/inventory/{itemId}/instances` is called for such an item THEN the system returns the live instances even though the parent item is invisible in the inventory list, leaving inventory state internally inconsistent
1.5 WHEN a scan card references an externally-resolved product that was never persisted THEN the system displays "Unknown product" because no `products` row exists to supply the name

### Expected Behavior (Correct)

When an external lookup resolves a product, that product must be persisted so that any item created from it references a real `products` row and is visible in inventory.

2.1 WHEN a barcode is resolved only by the external Open Food Facts lookup (Tier 3) THEN the system SHALL persist the resolved product into the `products` table (and its barcode mapping) before returning it, so the returned product ID references an existing `products` row
2.2 WHEN a stock-in scan entry whose `product_id` came from an external resolution is committed THEN the system SHALL create an `items` row whose `product_id` references an existing `products` row
2.3 WHEN `GET /api/inventory` is called after committing an externally-resolved item THEN the system SHALL include that item in the response
2.4 WHEN `GET /api/inventory/{itemId}/instances` returns live instances for an item THEN the system SHALL also expose that item's parent through `GET /api/inventory`, keeping the two views consistent
2.5 WHEN a product was resolved and persisted from the external lookup THEN the system SHALL make that product retrievable by its ID (e.g. via the catalog / product-by-ID path) with its resolved name
2.6 WHEN an orphaned item created before this fix exists (an `items` row whose `product_id` has no `products` row) THEN the system SHALL repair it so the referenced product exists and the item becomes visible in inventory
2.7 WHEN an item's product record exists THEN the system SHALL display the product's resolved name rather than "Unknown product"

### Unchanged Behavior (Regression Prevention)

Behavior for barcodes resolved by the earlier tiers, and for products already persisted, must not change.

3.1 WHEN a barcode is resolved by a user override (Tier 1) THEN the system SHALL CONTINUE TO return that override product and SHALL NOT create a duplicate `products` row
3.2 WHEN a barcode is resolved by an existing global database entry (Tier 2) THEN the system SHALL CONTINUE TO return that global product and SHALL NOT create a duplicate `products` row
3.3 WHEN a barcode is not found by any tier (external API returns not-found or errors) THEN the system SHALL CONTINUE TO return a not-found result and SHALL NOT persist any product
3.4 WHEN a stock-in is committed for an item whose product already exists in the `products` table THEN the system SHALL CONTINUE TO create the item and show it in `GET /api/inventory` exactly as before
3.5 WHEN `GET /api/inventory` is called for items whose products already exist THEN the system SHALL CONTINUE TO return them ordered by product name as before

## Bug Condition and Correctness Properties

### Bug Condition

```pascal
FUNCTION isBugCondition(X)
  INPUT: X of type BarcodeResolution   // a barcode resolved during scan/commit
  OUTPUT: boolean

  // The bug triggers only when a resolution comes from the external (Tier 3)
  // source, because Tier 1 and Tier 2 resolutions already reference a
  // persisted products row.
  RETURN X.source = "external"
END FUNCTION
```

### Property: Fix Checking

```pascal
// For every externally-resolved product, a products row must exist and any
// item created from it must be visible in inventory.
FOR ALL X WHERE isBugCondition(X) DO
  result ← Lookup'(X.barcode, X.userID)          // F' = fixed lookup
  ASSERT productExistsInCatalog(result.Product.ID)          // 2.1, 2.5

  itemID ← commitStockIn'(X)                       // fixed commit path
  ASSERT itemReferencesExistingProduct(itemID)              // 2.2
  ASSERT itemAppearsIn(GET "/api/inventory")                // 2.3, 2.4
END FOR
```

### Property: Fix Checking — Data Repair (pre-existing orphan)

```pascal
// The already-stuck row must be repaired.
FOR ALL item WHERE itemHasNoProductsRow(item) DO
  runRepair'()
  ASSERT itemReferencesExistingProduct(item.id)             // 2.6
  ASSERT itemAppearsIn(GET "/api/inventory")                // 2.6
END FOR
```

### Property: Preservation Checking

```pascal
// For every non-external resolution (user override, global, or not-found),
// the fixed system behaves identically to the original.
FOR ALL X WHERE NOT isBugCondition(X) DO
  ASSERT Lookup(X) = Lookup'(X)                    // 3.1, 3.2, 3.3
  ASSERT inventoryView(X) = inventoryView'(X)      // 3.4, 3.5
END FOR
```

**Key definitions:**
- **F** — the original (unfixed) lookup/commit path, before the fix.
- **F'** — the fixed path that persists the externally-resolved product.
- **Counterexample** — scanning Kroger lemon juice (barcode `011110728227`), committing a stock-in, then calling `GET /api/inventory` returns `[]` while the item and its instances exist in the database.
