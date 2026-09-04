# Bugfix Requirements Document

## Introduction

Product thumbnails never render anywhere in the UI: Inventory cards (`ItemRow.tsx`) and Scan Queue cards (`ScanEntryCard.tsx`) always fall back to the initials-only Mantine `Avatar` placeholder, even for a product the user confirms has a real thumbnail available from Open Food Facts. This is confirmed with the user's own "Light Sour Cream" product, barcode `073420000158`: already committed to inventory, a real image is available from Open Food Facts for this barcode, yet the `image_url` column for this product's row in the `products` table is `NULL`.

Investigation surfaced two distinct, independently-sufficient causes. Both are in scope for this bugfix because fixing only one leaves thumbnails broken for a real subset of products.

**Concern A — migration/deployment drift.** The `image_url` column and its supporting read/write code (`internal/product/product.go`, `internal/product/openfoodfacts.go`, `internal/product/lookup.go`, the hand-written joins in `internal/inventory/inventory.go` and `internal/scan/scan.go`) were added after migration `003_add_product_image_url.sql` was written, but the user's actual running dev server process was started before that migration existed in the embedded migration set. `RunMigrations` (`internal/app/migrate.go`) only applies migrations once, at process startup; a long-running process never re-checks for migrations added to the source tree after it started. Confirmed against the live `pantry.db`: `schema_migrations` contains only `001_initial_schema.sql` and `002_backfill_orphaned_products.sql`, and `.schema products` shows no `image_url` column at all. Nothing in the running app has ever populated or read `image_url` while in this state — this alone explains zero thumbnails rendering, independent of any application logic defect.

**Concern B — no backfill/refresh path for pre-existing products.** The three-tier lookup (`internal/product/lookup.go`) only calls the external Open Food Facts API (Tier 3) when a barcode is not already resolvable via a user override (Tier 1) or an existing global `products`/`barcodes` row (Tier 2). Once a barcode has been resolved once and persisted (as the sour cream barcode already was, prior to the `image_url` feature existing or while migration 003 was unapplied), every subsequent scan of that same barcode resolves at Tier 2 and never touches Open Food Facts again. There is no migration-time backfill, on-demand refresh, or manual re-sync path that populates `image_url` for a `products` row that predates the image feature or was created while it was broken. Restarting the server (fixing Concern A) makes the column exist and lets *newly* resolved products get an image, but it does not retroactively add one to the sour cream row, or any other already-persisted product missing an image.

## Bug Analysis

### Current Behavior (Defect)

1.1 WHILE a running server process was started before migration `003_add_product_image_url.sql` existed in the embedded migration set THEN the system continues serving requests against a `products` table with no `image_url` column, with no error or warning surfaced to the user or operator
1.2 WHILE the `products` table has no `image_url` column THEN the system returns `imageUrl` as empty for every product from every endpoint that exposes `ProductSummary`/`Product`, regardless of whether Open Food Facts has a real image for that barcode
1.3 WHEN a barcode is resolved at Tier 1 (user override) or Tier 2 (existing global `products`/`barcodes` row) THEN the system returns the persisted product without querying Open Food Facts, even if that product's `image_url` is `NULL`
1.4 WHEN a `products` row was created before the `image_url` column/feature existed, or while migration 003 was unapplied THEN the system leaves that row's `image_url` permanently `NULL`, because no code path re-queries Open Food Facts for a barcode that already resolves at Tier 1 or Tier 2
1.5 WHEN the sour cream product (barcode `073420000158`) is displayed on the Inventory page or Scan Queue THEN the system renders the initials-only placeholder Avatar instead of the real thumbnail Open Food Facts has for that barcode

### Expected Behavior (Correct)

2.1 WHEN the server process starts THEN the system SHALL apply every migration present in the embedded migration set that has not yet been recorded in `schema_migrations`, including migrations added after a previous instance of the process last started
2.2 IF an operator needs to confirm the running schema matches the embedded migration set THEN the system SHALL provide a way to verify this (e.g. a startup log line, a health/version endpoint, or equivalent), so that a schema/code mismatch is detectable without inspecting the database directly
2.3 WHEN a `products` row exists whose `image_url` is `NULL` and Open Food Facts has an image available for that product's barcode THEN the system SHALL provide a mechanism by which that row's `image_url` is eventually populated, without requiring the row to be deleted and re-created
2.4 WHEN the sour cream product (barcode `073420000158`) is displayed on the Inventory page or Scan Queue after the fix mechanism from 2.3 has run for it THEN the system SHALL render its real thumbnail image instead of the initials-only placeholder
2.5 WHERE a product's `image_url` cannot be resolved (Open Food Facts has no image, the lookup fails, or the backfill mechanism has not yet run for that row) THEN the system SHALL continue to render the initials-only placeholder rather than a broken image

### Unchanged Behavior (Regression Prevention)

3.1 WHEN a barcode is resolved by a user override (Tier 1) THEN the system SHALL CONTINUE TO return that override product without querying Open Food Facts
3.2 WHEN a barcode is resolved by an existing global database entry (Tier 2) whose `image_url` is already populated THEN the system SHALL CONTINUE TO return that product's existing `image_url` unchanged
3.3 WHEN a barcode is resolved only by the external Open Food Facts lookup (Tier 3) for a product that has never been persisted before THEN the system SHALL CONTINUE TO persist the resolved product, including its `image_url`, exactly as already implemented
3.4 WHEN `RunMigrations` is invoked against a database that already has every migration applied THEN the system SHALL CONTINUE TO apply zero migrations and make zero schema changes
3.5 WHEN a product has no `image_url` (empty or NULL) THEN the frontend Avatar SHALL CONTINUE TO fall back to initials exactly as already implemented, without a broken-image icon

## Open Questions for Design Phase

These are flagged for the user to weigh in on before design; requirements above are written so any of these resolutions can satisfy them.

- **Concern A fix scope:** Is a documented/asserted expectation ("restart the server to pick up new migrations, matching how `RunMigrations` has always worked") sufficient, or is a stronger startup check needed (e.g. logging the count/names of migrations applied on this startup, or a `/version`-style endpoint reporting the latest applied migration)? Building a full migration-drift-detection system (e.g. comparing a running process's schema against the source tree from outside the process) is likely out of scope unless explicitly wanted.
- **Concern B backfill mechanism:** which of the following (or a combination) should be designed:
  - (a) A startup migration/task that re-queries Open Food Facts for every product missing an `image_url`. Note this requires a live network call at startup, which is a new pattern for this codebase (migrations so far are pure SQL) and has rate-limit/latency implications.
  - (b) An on-demand lazy backfill: when a product is read and found to be missing an image, trigger a best-effort background refresh from Open Food Facts for next time.
  - (c) A manual/admin-triggered re-sync endpoint that an operator or user can call for a specific product or barcode.
  - (d) Accepting that only products resolved from now on get images, and documenting that pre-existing products without one will not retroactively gain one unless manually re-triggered.

## Bug Condition and Correctness Properties

### Bug Condition

```pascal
FUNCTION isBugConditionA(process)
  INPUT: process of type ServerProcess
  OUTPUT: boolean

  // Concern A triggers when a running process's applied migration set is
  // missing a migration that exists in the current embedded migration set.
  RETURN EXISTS m IN EmbeddedMigrations() WHERE m NOT IN process.AppliedMigrations()
END FUNCTION

FUNCTION isBugConditionB(row)
  INPUT: row of type ProductsRow
  OUTPUT: boolean

  // Concern B triggers for any persisted product with no image whose barcode
  // has a real image available from Open Food Facts, and that will never be
  // re-queried because it already resolves at Tier 1 or Tier 2.
  RETURN row.ImageURL = NULL AND OpenFoodFacts.HasImage(row.PrimaryBarcode) = true
END FUNCTION
```

### Property: Fix Checking — Migration Freshness (Concern A)

```pascal
// After a server process starts, its applied migration set must equal the
// embedded migration set, and this must be observable.
FOR ALL process WHERE process.JustStarted() = true DO
  ASSERT process.AppliedMigrations() = EmbeddedMigrations()               // 2.1
  ASSERT VerifiableFromOutside(process.AppliedMigrations())               // 2.2
END FOR
```

### Property: Fix Checking — Image Backfill (Concern B)

```pascal
// For every pre-existing products row missing an image where one is
// available, the chosen backfill mechanism must eventually populate it.
FOR ALL row WHERE isBugConditionB(row) DO
  runBackfillMechanism'(row)                                              // 2.3
  ASSERT row'.ImageURL != NULL
  ASSERT RenderedAvatar(row'.ImageURL) = RealThumbnail                    // 2.4
END FOR

// Rows with no available image, or not yet reached by the backfill
// mechanism, must keep degrading gracefully.
FOR ALL row WHERE row.ImageURL = NULL AND NOT isBugConditionB(row) DO
  ASSERT RenderedAvatar(row.ImageURL) = InitialsPlaceholder                // 2.5
END FOR
```

### Property: Preservation Checking

```pascal
// Tier 1, Tier 2 (already-imaged), and Tier 3 (newly-resolved) paths, plus
// idempotent re-migration and the existing Avatar fallback, behave exactly
// as before the fix.
FOR ALL X WHERE X.Tier = 1 OR X.Tier = 2 DO
  ASSERT Lookup(X) = Lookup'(X)                                           // 3.1, 3.2
END FOR

FOR ALL X WHERE X.Tier = 3 AND X.NeverPersistedBefore = true DO
  ASSERT persistExternalProduct(X) = persistExternalProduct'(X)           // 3.3
END FOR

FOR ALL process WHERE process.AppliedMigrations() = EmbeddedMigrations() DO
  ASSERT RunMigrations(process.DB) applies zero migrations                // 3.4
END FOR

FOR ALL row WHERE row.ImageURL = NULL OR row.ImageURL = "" DO
  ASSERT RenderedAvatar(row.ImageURL) = InitialsPlaceholder                // 3.5
END FOR
```

**Key definitions:**
- **Concern A** — migration/deployment drift: a running process serving requests against a schema older than its embedded migration set.
- **Concern B** — no backfill/refresh mechanism for `image_url` on `products` rows persisted before the image feature existed or while it was broken.
- **Counterexample** — the sour cream product, barcode `073420000158` ("Light Sour Cream"), already committed to inventory: `image_url` is `NULL` in the live `products` table, a real image is confirmed available from Open Food Facts for this barcode, and the running dev server's `schema_migrations` table shows migration `003_add_product_image_url.sql` was never applied.
