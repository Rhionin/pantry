-- Favor variety joins the rule list, and a member can stay in the group
-- without being bought again. SQLite cannot change a CHECK with ALTER TABLE,
-- so the two tables that name the rules are rebuilt. The migrator writes a
-- pantry-pre-restock backup before this file runs. no_restock 0 means the
-- product is still in the rotation.

CREATE TABLE product_groups_next (
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
    CHECK (rule IN ('same_as_ran_out', 'favorite', 'best_deal', 'favor_variety')),
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

INSERT INTO product_groups_next (
    id, user_id, name, name_key, rule, rule_confirmed, pinned_product_id,
    window_months, quantity_base_value, quantity_dimension, created_at
)
SELECT
    id, user_id, name, name_key, rule, rule_confirmed, pinned_product_id,
    window_months, quantity_base_value, quantity_dimension, created_at
FROM product_groups;

DROP TABLE product_groups;
ALTER TABLE product_groups_next RENAME TO product_groups;

CREATE TABLE product_group_members_next (
    product_id  TEXT PRIMARY KEY,
    group_id    TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    no_restock  INTEGER NOT NULL DEFAULT 0,
    CHECK (no_restock IN (0, 1))
);

INSERT INTO product_group_members_next (product_id, group_id, created_at, no_restock)
SELECT product_id, group_id, created_at, 0 FROM product_group_members;

DROP TABLE product_group_members;
ALTER TABLE product_group_members_next RENAME TO product_group_members;

CREATE INDEX IF NOT EXISTS idx_product_group_members_group ON product_group_members (group_id);

CREATE TABLE group_suggestions_next (
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
    CHECK (proposed_rule IS NULL OR proposed_rule IN ('same_as_ran_out', 'favorite', 'best_deal', 'favor_variety'))
);

INSERT INTO group_suggestions_next (
    id, user_id, kind, title, proposed_rule, pinned_product_id, existing_group_id, status, created_at
)
SELECT
    id, user_id, kind, title, proposed_rule, pinned_product_id, existing_group_id, status, created_at
FROM group_suggestions;

DROP TABLE group_suggestions;
ALTER TABLE group_suggestions_next RENAME TO group_suggestions;
