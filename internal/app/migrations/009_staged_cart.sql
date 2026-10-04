-- A staged cart is a snapshot of the supply plan. touched marks a line the
-- owner changed, so a later fill refreshes the rest and leaves that line.
-- Removing an auto line records a skip so the next fill does not put it back.
ALTER TABLE shopping_list_items ADD COLUMN touched INTEGER NOT NULL DEFAULT 0;

CREATE TABLE staged_cart_skips (
    user_id TEXT NOT NULL,
    item_id TEXT NOT NULL,
    PRIMARY KEY (user_id, item_id)
);
