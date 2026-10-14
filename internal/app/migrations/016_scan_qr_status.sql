-- A scanned QR payload is not a product barcode. The status check has to
-- name that row so it can sit in the queue without a product lookup.
-- foreign_keys stays off for this connection: the rest of the app does not
-- enable them, and turning them on here would reject rows the old table allowed.

PRAGMA foreign_keys=OFF;

CREATE TABLE scan_entries_qr (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL,
    barcode         TEXT NOT NULL,
    scanned_at      DATETIME NOT NULL,
    direction       TEXT CHECK (direction IN ('stock_in', 'stock_out')),
    unit_count      INTEGER NOT NULL DEFAULT 1,
    expires_at      DATETIME,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'flagged', 'committed', 'cancelled', 'qr')),
    product_id      TEXT REFERENCES products(id),
    committed_at    DATETIME,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO scan_entries_qr (
    id, user_id, barcode, scanned_at, direction, unit_count, expires_at,
    status, product_id, committed_at, created_at
)
SELECT
    id, user_id, barcode, scanned_at, direction, unit_count, expires_at,
    status, product_id, committed_at, created_at
FROM scan_entries;

DROP TABLE scan_entries;

ALTER TABLE scan_entries_qr RENAME TO scan_entries;
