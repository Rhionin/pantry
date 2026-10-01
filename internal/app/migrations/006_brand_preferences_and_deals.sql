-- Brand preferences remember which product a household wants for a shared
-- replenishment need (the same generic name and unit). ignore_price means
-- the usual brand stays even when another brand is cheaper.
CREATE TABLE brand_preferences (
    user_id      TEXT NOT NULL,
    need_key     TEXT NOT NULL,
    item_id      TEXT NOT NULL REFERENCES items(id),
    ignore_price INTEGER NOT NULL DEFAULT 0 CHECK (ignore_price IN (0, 1)),
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, need_key)
);

-- Household-noted sales. A future retailer adapter can write source = 'retailer'.
-- Recorded rows are enough to offer a deal at cart time before any store API exists.
CREATE TABLE item_deals (
    user_id              TEXT NOT NULL,
    item_id              TEXT NOT NULL REFERENCES items(id),
    price_cents          INTEGER CHECK (price_cents IS NULL OR price_cents >= 0),
    regular_price_cents  INTEGER CHECK (regular_price_cents IS NULL OR regular_price_cents >= 0),
    label                TEXT NOT NULL DEFAULT '',
    source               TEXT NOT NULL DEFAULT 'recorded' CHECK (source IN ('recorded', 'retailer')),
    updated_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, item_id)
);
