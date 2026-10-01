-- Dead since 001: no Go file references this table. Its UNIQUE(user_id) permits
-- one row per user, so it can hold at most one provider's connection state,
-- while service_type shows multi-provider was the intent. SQLite cannot drop a
-- constraint in place, and rebuilding a table with zero rows and zero readers
-- to reach a shape that still would not fit Requirement 2 is worse than dropping
-- it. So migration 006 drops it.
DROP TABLE cart_integrations;

-- One connection per (user, provider). The UNIQUE is on the pair, not on
-- user_id alone — the single change that makes multiple providers possible.
CREATE TABLE provider_connections (
    id                      TEXT PRIMARY KEY,
    user_id                 TEXT NOT NULL,
    provider_id             TEXT NOT NULL,
    state                   TEXT NOT NULL DEFAULT 'disconnected',
    access_token            TEXT,
    refresh_token           TEXT,
    access_token_expires_at DATETIME,
    auth_state              TEXT,
    auth_state_at           DATETIME,
    updated_at              DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, provider_id)
);

-- Exactly one entry per (provider, item), per Requirement 10.12. The composite
-- primary key is ordered provider-first because the dominant read is "every
-- entry for one provider", which that order makes a range scan.
CREATE TABLE fulfillment_ledger (
    provider_id        TEXT NOT NULL,
    item_id            TEXT NOT NULL REFERENCES items(id),
    requested_quantity INTEGER NOT NULL DEFAULT 0,
    ledger_boundary    DATETIME NOT NULL,
    updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (provider_id, item_id)
);

-- Requirement 8 keys an adjustment by entry AND provider, so this cannot be a
-- column on shopping_list_items: one entry can carry a different adjustment for
-- each provider, and Requirement 8.2 requires setting one to leave the others
-- untouched. A single column could hold only one of them.
CREATE TABLE shopping_list_entry_adjustments (
    entry_id    TEXT NOT NULL REFERENCES shopping_list_items(id),
    provider_id TEXT NOT NULL,
    quantity    INTEGER NOT NULL,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (entry_id, provider_id)
);

-- Requirement 6.2: a product with no recorded mode behaves exactly as it did
-- before Replenishment_Mode existed, so every existing row must read 'target'.
-- A NOT NULL DEFAULT on ADD COLUMN gives that with no backfill UPDATE. The value
-- set is enforced in Go, not by CHECK, for the reason migration 005 records:
-- SQLite cannot add a CHECK constraint via ALTER TABLE.
ALTER TABLE items ADD COLUMN replenishment_mode TEXT NOT NULL DEFAULT 'target';

-- The one query this feature adds whose cost grows without bound: counting
-- consumption events after a per-item boundary. Note that no migration in this
-- repo creates an index today, so this is a deliberate first.
CREATE INDEX idx_consumption_events_item_consumed
    ON consumption_events (item_id, consumed_at);
