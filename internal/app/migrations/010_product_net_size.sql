-- Per-unit net size. net_base_value is grams or milliliters.
-- net_dimension is mass or volume. net_size_origin is off, manual, or backfill.
-- The three stay null together when the size is unknown. A cleared size keeps
-- origin manual and nulls the value, which the Go backfill will not refill.
-- pack_count is how many individual units one scan of this barcode adds.
-- Null means that answer has not been remembered yet.
ALTER TABLE products ADD COLUMN net_base_value REAL;
ALTER TABLE products ADD COLUMN net_dimension TEXT;
ALTER TABLE products ADD COLUMN net_size_origin TEXT;
ALTER TABLE products ADD COLUMN pack_count INTEGER;
