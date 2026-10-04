-- One stock-in moment per restock after opening. Snapshot scans do not write here.
CREATE TABLE stock_in_events (
    id         INTEGER PRIMARY KEY,
    product_id TEXT NOT NULL,
    at         TIMESTAMP NOT NULL
);

CREATE INDEX stock_in_events_product_at ON stock_in_events (product_id, at);

-- A product keeps either a time window or a raw quantity, never both.
-- What that means for a shopping line lives in the supply plan.
CREATE TABLE supply_overrides (
    product_id    TEXT PRIMARY KEY,
    window_months INTEGER,
    quantity      INTEGER,
    CHECK ((window_months IS NOT NULL) <> (quantity IS NOT NULL)),
    CHECK (window_months IS NULL OR window_months BETWEEN 1 AND 12),
    CHECK (quantity IS NULL OR quantity BETWEEN 1 AND 999)
);

ALTER TABLE shopping_list_items ADD COLUMN note TEXT NOT NULL DEFAULT '';
