-- Groups, suggestions, and dismissals. Quantity is an ounce-equivalent stored
-- as grams or milliliters (quantity_base_value + quantity_dimension), not a
-- count of cans. Both quantity columns stay null together. A window and a
-- quantity cannot both be set. Null target means the account supply window.

CREATE TABLE IF NOT EXISTS product_groups (
    id                  TEXT PRIMARY KEY,
    user_id             TEXT NOT NULL DEFAULT 'user-1',
    name                TEXT NOT NULL,
    name_key            TEXT NOT NULL,
    rule                TEXT NOT NULL DEFAULT 'same_as_ran_out',
    rule_confirmed      INTEGER NOT NULL DEFAULT 0,
    pinned_product_id   TEXT,
    window_months       INTEGER,
    quantity_base_value REAL,
    quantity_dimension  TEXT,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, name_key),
    CHECK (rule IN ('same_as_ran_out', 'favorite', 'best_deal')),
    CHECK (rule_confirmed IN (0, 1)),
    CHECK (window_months IS NULL OR (quantity_base_value IS NULL AND quantity_dimension IS NULL)),
    CHECK (
        (quantity_base_value IS NULL AND quantity_dimension IS NULL)
        OR (quantity_base_value IS NOT NULL AND quantity_dimension IS NOT NULL)
    ),
    CHECK (quantity_dimension IS NULL OR quantity_dimension IN ('mass', 'volume')),
    CHECK (window_months IS NULL OR window_months BETWEEN 1 AND 12),
    CHECK (quantity_base_value IS NULL OR quantity_base_value > 0)
);

CREATE TABLE IF NOT EXISTS product_group_members (
    product_id  TEXT PRIMARY KEY,
    group_id    TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_product_group_members_group ON product_group_members (group_id);

CREATE TABLE IF NOT EXISTS group_suggestions (
    id                 TEXT PRIMARY KEY,
    user_id            TEXT NOT NULL DEFAULT 'user-1',
    kind               TEXT NOT NULL,
    title              TEXT NOT NULL,
    proposed_rule      TEXT,
    pinned_product_id  TEXT,
    existing_group_id  TEXT,
    status             TEXT NOT NULL DEFAULT 'open',
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (kind IN ('looks_alike', 'from_scan', 'from_old_plan')),
    CHECK (status IN ('open', 'accepted', 'dismissed')),
    CHECK (proposed_rule IS NULL OR proposed_rule IN ('same_as_ran_out', 'favorite', 'best_deal'))
);

CREATE TABLE IF NOT EXISTS group_suggestion_members (
    suggestion_id TEXT NOT NULL,
    product_id    TEXT NOT NULL,
    included      INTEGER NOT NULL DEFAULT 1,
    caution       TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (suggestion_id, product_id),
    CHECK (included IN (0, 1))
);

CREATE TABLE IF NOT EXISTS group_suggestion_dismissals (
    user_id      TEXT NOT NULL,
    product_id_a TEXT NOT NULL,
    product_id_b TEXT NOT NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, product_id_a, product_id_b),
    CHECK (product_id_a < product_id_b)
);
