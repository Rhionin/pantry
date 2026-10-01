-- Household opt-in for sharing products a person typed in, plus one row per
-- share attempt. The setting is absent until someone turns it on, which is
-- off. Rows are written only after that opt-in and a per-product request;
-- nothing here is sent upstream by itself.
CREATE TABLE app_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE product_contributions (
    id              TEXT PRIMARY KEY,
    product_id      TEXT NOT NULL REFERENCES products(id),
    barcode         TEXT NOT NULL,
    external_source TEXT NOT NULL CHECK (external_source IN (
        'openfoodfacts', 'openproductsfacts', 'openbeautyfacts', 'openpetfoodfacts'
    )),
    status          TEXT NOT NULL CHECK (status IN ('not_configured', 'submitted', 'failed')),
    detail          TEXT,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_product_contributions_product
    ON product_contributions (product_id, created_at);
