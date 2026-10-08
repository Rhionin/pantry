-- A shopping line can name the product group it was planned for.
-- A skip can name that same group so the next fill does not put the line
-- back under a different member. Both columns stay null for older rows.
ALTER TABLE shopping_list_items ADD COLUMN group_id TEXT;
ALTER TABLE staged_cart_skips ADD COLUMN group_id TEXT;
