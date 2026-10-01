-- Application credentials for a cart provider (client id, client secret, redirect
-- URI). These are the retailer's developer-app secrets, distinct from the OAuth
-- tokens in provider_connections. The HTTP API never selects client_secret into
-- a response. The database file is the secret store, the same way access tokens
-- are stored.

CREATE TABLE provider_app_credentials (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    client_id TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    redirect_uri TEXT NOT NULL,
    modality TEXT NOT NULL DEFAULT 'PICKUP',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, provider_id)
);
