-- One row is one browser that asked to hear about scans while Pantry is closed.
-- The endpoint is the push service URL. p256dh and auth are the encryption
-- keys that browser handed us. A 404 or 410 from the push service means the
-- browser dropped the subscription, and that row is removed.

CREATE TABLE push_subscriptions (
    endpoint   TEXT PRIMARY KEY,
    p256dh     TEXT NOT NULL,
    auth       TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
