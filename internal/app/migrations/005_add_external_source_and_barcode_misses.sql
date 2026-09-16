-- Provenance for the products cache: which Product Opener database supplied a
-- row's field values. SQLite cannot add a CHECK constraint via ALTER TABLE, so
-- the value set {'openfoodfacts','openproductsfacts','openbeautyfacts',
-- 'openpetfoodfacts'} is enforced in Go (product.Catalog.CreateProduct),
-- exactly as products.source is.
--
-- No backfill UPDATE follows, unlike migration 004: the column is nullable with
-- no default, so every pre-existing row is already NULL, which is the correct
-- value. A row cached before this feature existed genuinely has unknown
-- provenance, and Refresher treats NULL as "assume Open Food Facts and stamp it
-- on the next successful revalidation". Guessing a value here would be
-- indistinguishable from a verified one.
ALTER TABLE products ADD COLUMN external_source TEXT;

-- Barcodes every Product Opener database reported unknown, with no upstream
-- error anywhere in the fan-out. Kept in its own table rather than as a
-- sentinel products row so that no existing query has to learn to exclude it.
CREATE TABLE barcode_misses (
    barcode    TEXT PRIMARY KEY,
    checked_at DATETIME NOT NULL
);
