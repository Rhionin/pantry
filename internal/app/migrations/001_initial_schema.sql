CREATE TABLE products (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    category    TEXT,
    unit_of_measure TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Barcode-to-product mappings with two-tier precedence: user overrides and global entries.
-- The UNIQUE constraint ensures one product per barcode within each scope (global or per-user override).
-- Barcode conflicts are resolved via the flagged entry workflow where users create overrides.
CREATE TABLE barcodes (
    barcode         TEXT NOT NULL,
    product_id      TEXT NOT NULL REFERENCES products(id),
    source          TEXT NOT NULL CHECK (source IN ('global', 'user_override')),
    user_id         TEXT NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (barcode, source, user_id)
);

CREATE TABLE items (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL,
    product_id      TEXT NOT NULL REFERENCES products(id),
    target_quantity INTEGER,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, product_id)
);

CREATE TABLE item_instances (
    id              TEXT PRIMARY KEY,
    item_id         TEXT NOT NULL REFERENCES items(id),
    stock_in_at     DATETIME NOT NULL,
    expires_at      DATETIME,
    removed_at      DATETIME,
    removal_reason  TEXT CHECK (removal_reason IN ('consumed', 'expired', 'manual')),
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE scan_entries (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL,
    barcode         TEXT NOT NULL,
    scanned_at      DATETIME NOT NULL,
    direction       TEXT CHECK (direction IN ('stock_in', 'stock_out')),
    unit_count      INTEGER NOT NULL DEFAULT 1,
    expires_at      DATETIME,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'flagged', 'committed', 'cancelled')),
    product_id      TEXT REFERENCES products(id),
    committed_at    DATETIME,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE consumption_events (
    id              TEXT PRIMARY KEY,
    item_id         TEXT NOT NULL REFERENCES items(id),
    consumed_at     DATETIME NOT NULL,
    scan_entry_id   TEXT REFERENCES scan_entries(id)
);

CREATE TABLE shopping_list_items (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL,
    item_id         TEXT NOT NULL REFERENCES items(id),
    quantity        INTEGER NOT NULL,
    source          TEXT NOT NULL CHECK (source IN ('manual', 'auto')),
    purchased_at    DATETIME,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE cart_integrations (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL UNIQUE,
    service_type    TEXT NOT NULL,
    config_json     TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
