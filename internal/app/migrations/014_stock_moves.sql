-- One row is one stock in or stock out the household can see and correct.
-- Instances and consumption rows point back at the move that caused them,
-- so changing the quantity puts the shelf and the pace back together.

CREATE TABLE stock_moves (
    id              TEXT PRIMARY KEY,
    item_id         TEXT NOT NULL,
    product_id      TEXT NOT NULL,
    direction       TEXT NOT NULL CHECK (direction IN ('in', 'out')),
    quantity        INTEGER NOT NULL CHECK (quantity >= 1),
    at              DATETIME NOT NULL,
    source          TEXT NOT NULL CHECK (source IN ('scan', 'manual')),
    scan_entry_id   TEXT,
    tracked         INTEGER NOT NULL DEFAULT 0 CHECK (tracked IN (0, 1)),
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX stock_moves_item_at ON stock_moves (item_id, at);
CREATE INDEX stock_moves_product_at ON stock_moves (product_id, at);

ALTER TABLE item_instances ADD COLUMN stock_move_id TEXT;
ALTER TABLE item_instances ADD COLUMN removed_by_move_id TEXT;
ALTER TABLE consumption_events ADD COLUMN stock_move_id TEXT;
ALTER TABLE stock_in_events ADD COLUMN stock_move_id TEXT;

CREATE INDEX item_instances_stock_move ON item_instances (stock_move_id);
CREATE INDEX item_instances_removed_by ON item_instances (removed_by_move_id);
CREATE INDEX consumption_events_stock_move ON consumption_events (stock_move_id);

-- Serializes the one-time repair that groups older rows into moves.
CREATE TABLE history_repair (
    id INTEGER PRIMARY KEY CHECK (id = 1)
);

INSERT INTO history_repair (id) VALUES (1);
