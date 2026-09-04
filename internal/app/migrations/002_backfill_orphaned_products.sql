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
