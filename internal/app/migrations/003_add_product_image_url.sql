-- Adds a thumbnail image URL to products, sourced from Open Food Facts
-- (image_front_small_url) when a product is resolved externally. Existing
-- rows get NULL, which the UI treats as "no thumbnail available".
ALTER TABLE products ADD COLUMN image_url TEXT;
