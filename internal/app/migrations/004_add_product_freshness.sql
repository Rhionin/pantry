-- Freshness and provenance for the products cache. SQLite cannot add a CHECK
-- constraint via ALTER TABLE, so the source value set {'external','user'} is
-- enforced in Go (product.Catalog.CreateProduct).
ALTER TABLE products ADD COLUMN source TEXT NOT NULL DEFAULT 'external';
ALTER TABLE products ADD COLUMN refreshed_at DATETIME;
ALTER TABLE products ADD COLUMN name_overridden INTEGER NOT NULL DEFAULT 0;

-- Every pre-existing row is treated as externally-sourced and never verified.
-- No override has ever been entered, so no classification heuristic is needed.
-- The placeholder rows migration 002 created as 'Product ' || id are left in
-- place deliberately: a real Open Food Facts fetch will overwrite them.
UPDATE products
SET source = 'external',
    refreshed_at = NULL,
    name_overridden = 0;
