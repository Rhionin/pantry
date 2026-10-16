-- origin is the hostname that registered the browser, such as stockbot.rhionin.com.
-- Rows from before this column have an empty origin. They still receive alerts:
-- a live subscription must not be dropped just because it predates the column.
-- user_id and device_id name the household member and browser. A later subscribe
-- for that pair replaces older endpoints. A 404 or 410 still removes a row the
-- push service has forgotten. A row whose origin is the previous public host
-- is removed instead of contacted.

ALTER TABLE push_subscriptions ADD COLUMN origin TEXT NOT NULL DEFAULT '';
ALTER TABLE push_subscriptions ADD COLUMN user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE push_subscriptions ADD COLUMN device_id TEXT NOT NULL DEFAULT '';
