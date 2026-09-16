# Requirements Document

## Introduction

Tier 3 of the barcode lookup queries exactly one upstream database: Open Food Facts. A scanned barcode that belongs to a sibling Product Opener database — Open Products Facts (household and general merchandise), Open Beauty Facts, or Open Pet Food Facts — is reported as unknown, lands in the scan queue as a flagged entry, and forces the user to type the product name by hand. Every non-grocery item in a pantry hits this path.

This feature widens Tier 3 to all four Product Opener databases. A barcode that reaches Tier 3 is queried against all four concurrently, so the added coverage costs roughly one request's latency rather than four. The winning database is recorded on the local product row, so a later revalidation re-queries the database the data actually came from instead of assuming Open Food Facts. A barcode confirmed unknown in all four databases is cached as a miss with its own expiry, so a repeatedly scanned unknown barcode stops fanning out four requests per scan — but only when every database gave a clean answer. If any database errored, nothing is cached and the next scan retries, so a transient outage can never harden into a permanent "unknown".

Provenance is also surfaced to the user on the scan card and the inventory row, so the origin of a product's name and thumbnail is visible.

**Open decision flagged for review:** the precedence ordering in Requirement 2 places Open Food Facts above Open Products Facts (confirmed), then Open Beauty Facts, then Open Pet Food Facts (recommended, ordered by expected pantry-scan frequency). Only the first pair was confirmed during clarification. The remaining ordering matters only for barcodes present in more than one upstream database, which is an upstream data-entry overlap rather than genuine ambiguity.

## Glossary

- **Product_Opener_Database**: One of the four upstream databases running the Product Opener platform and serving the same read API shape: Open Food Facts, Open Products Facts, Open Beauty Facts, and Open Pet Food Facts.
- **External_Source**: The identifier of the Product_Opener_Database a product's field values came from, one of `openfoodfacts`, `openproductsfacts`, `openbeautyfacts`, or `openpetfoodfacts`.
- **Database_Precedence**: The fixed ordering used to pick a single winner when more than one Product_Opener_Database returns a hit for the same barcode: Open Food Facts, then Open Products Facts, then Open Beauty Facts, then Open Pet Food Facts.
- **Product_Opener_Client**: The component that queries a single Product_Opener_Database for one barcode.
- **External_Lookup**: The component that queries all enabled Product_Opener_Databases for one barcode and returns a single outcome.
- **Lookup_Service**: The `product.LookupService` type, which performs the three-tier barcode lookup (Tier 1 user override, Tier 2 local catalog, Tier 3 External_Lookup).
- **Product_Catalog**: The `product.Catalog` data-access type, which owns reads and writes of the `products` and `barcodes` tables.
- **Refresh_Service**: The `product.Refresher` type, which revalidates an External_Row against its upstream database and merges the result into the local row.
- **Migration_Runner**: The `app.RunMigrations` function, which applies numbered SQL migrations in `internal/app/migrations`.
- **Pantry_Server**: The single Go server binary in `cmd/server`, which reads configuration from environment variables.
- **External_Row**: A `products` row whose `source` column equals `external`.
- **User_Row**: A `products` row whose `source` column equals `user`.
- **Legacy_External_Row**: An External_Row that has no External_Source value, meaning the row was cached before this feature existed.
- **Upstream_Hit**: A response from a Product_Opener_Database that supplies a usable product record for the requested barcode.
- **Upstream_Miss**: A response from a Product_Opener_Database that reports the requested barcode as unknown to that database.
- **Upstream_Error**: A transport failure, timeout, malformed response, or non-success status other than a barcode-unknown response from a Product_Opener_Database.
- **Confirmed_Miss**: A recorded outcome stating that a barcode returned an Upstream_Miss from every Product_Opener_Database with no Upstream_Error.
- **Miss_TTL**: The age at which a Confirmed_Miss expires and the barcode becomes eligible for another External_Lookup.
- **Cache_TTL**: The existing product cache time-to-live governing when an External_Row becomes stale, configured by `PRODUCT_CACHE_TTL`.
- **Clock**: The injectable time source used by Lookup_Service and Refresh_Service to read the current time.
- **Scan_Card**: The scan queue entry card and flagged-entry resolver in the frontend (`ScanEntryCard`, `FlaggedEntryResolver`).
- **Inventory_Row**: The inventory item row in the frontend (`ItemRow`).

## Requirements

### Requirement 1

**User Story:** As a pantry user scanning a non-grocery item, I want the barcode looked up in every Product Opener database at once, so that household, beauty, and pet products resolve automatically without adding scan latency.

#### Acceptance Criteria

1. WHEN Lookup_Service reaches Tier 3 for a barcode, THE External_Lookup SHALL query Open Food Facts, Open Products Facts, Open Beauty Facts, and Open Pet Food Facts for that barcode.
2. WHEN External_Lookup queries the Product_Opener_Databases for a barcode, THE External_Lookup SHALL issue those requests concurrently, so that the elapsed time of the fan-out is bounded by the slowest single request rather than the sum of all requests.
3. WHEN External_Lookup queries the Product_Opener_Databases for a barcode, THE External_Lookup SHALL apply the same per-request timeout to every request.
4. WHEN exactly one Product_Opener_Database returns an Upstream_Hit for a barcode, THE Lookup_Service SHALL return that product with the `Source` value `external`.
5. WHEN External_Lookup completes a fan-out, THE External_Lookup SHALL perform at most one database write at a time, so that the single-writer SQLite connection is never contended by the fan-out.
6. WHEN Lookup_Service persists a product resolved by External_Lookup, THE Product_Catalog SHALL store the `products` row with `source` set to `external` and the `barcodes` mapping with source `global`, matching the values stored for an Open Food Facts result before this feature is added.

### Requirement 2

**User Story:** As a pantry user, I want one deterministic product returned when a barcode exists in more than one upstream database, so that the same scan always produces the same result and I am never asked to choose.

#### Acceptance Criteria

1. WHEN more than one Product_Opener_Database returns an Upstream_Hit for the same barcode, THE External_Lookup SHALL select the Upstream_Hit from the database that ranks highest in Database_Precedence and discard the remaining Upstream_Hits.
2. THE External_Lookup SHALL rank Open Food Facts above Open Products Facts in Database_Precedence.
3. THE External_Lookup SHALL rank Open Products Facts above Open Beauty Facts, and Open Beauty Facts above Open Pet Food Facts, in Database_Precedence.
4. WHEN External_Lookup selects an Upstream_Hit, THE External_Lookup SHALL return the same selection for every repeated fan-out that receives the same set of upstream responses.
5. WHEN more than one Product_Opener_Database returns an Upstream_Hit for the same barcode, THE Lookup_Service SHALL return a single product and SHALL create no flagged scan entry for the overlap.
6. WHEN at least one Product_Opener_Database returns an Upstream_Hit for a barcode AND at least one other Product_Opener_Database returns an Upstream_Error for that barcode, THE Lookup_Service SHALL return the selected Upstream_Hit and persist the resolved product.

### Requirement 3

**User Story:** As a pantry user, I want each cached product to record which upstream database supplied the product data, so that a later refresh asks that same database instead of guessing.

#### Acceptance Criteria

1. THE Migration_Runner SHALL add a nullable `external_source` column to the `products` table that accepts the values `openfoodfacts`, `openproductsfacts`, `openbeautyfacts`, and `openpetfoodfacts`.
2. WHEN the migration that adds the `external_source` column is applied, THE Migration_Runner SHALL retain the existing `source`, `refreshed_at`, and `name_overridden` values of every pre-existing `products` row.
3. WHEN the migration that adds the `external_source` column is applied, THE Migration_Runner SHALL set `external_source` to NULL for every pre-existing `products` row.
4. WHEN Lookup_Service persists a product resolved by External_Lookup, THE Product_Catalog SHALL store that row's `external_source` as the External_Source of the Product_Opener_Database that supplied the Upstream_Hit.
5. WHEN a user creates a product through the product API, THE Product_Catalog SHALL store that row with `external_source` set to NULL.
6. WHEN Refresh_Service revalidates an External_Row, THE Product_Catalog SHALL retain that row's existing `external_source` value.
7. THE Product_Catalog SHALL reject a `products` row whose `external_source` value is outside the four defined External_Source values.

### Requirement 4

**User Story:** As a pantry user, I want a refresh of a household or beauty product to re-check the database that product came from, so that a refresh cannot blank or contradict a product just because Open Food Facts does not know the barcode.

#### Acceptance Criteria

1. WHEN Refresh_Service revalidates an External_Row that has an External_Source value, THE Refresh_Service SHALL query the Product_Opener_Database identified by that External_Source value.
2. WHEN Refresh_Service revalidates an External_Row that has an External_Source value, THE Refresh_Service SHALL query no other Product_Opener_Database.
3. WHERE an External_Row is a Legacy_External_Row, WHEN Refresh_Service revalidates that row, THE Refresh_Service SHALL query Open Food Facts.
4. WHEN Refresh_Service receives an Upstream_Hit while revalidating a Legacy_External_Row, THE Product_Catalog SHALL set that row's `external_source` to `openfoodfacts`.
5. WHEN Refresh_Service revalidates a User_Row, THE Refresh_Service SHALL query no Product_Opener_Database and SHALL leave that row's field values unchanged.
6. WHEN Refresh_Service receives an Upstream_Hit for an External_Row, THE Refresh_Service SHALL merge the upstream values, report the outcome, and stamp `refreshed_at` exactly as it does for an Open Food Facts result before this feature is added.
7. WHEN Refresh_Service receives an Upstream_Miss for an External_Row, THE Refresh_Service SHALL retain that row's existing `name`, `category`, `unit_of_measure`, `image_url`, and `external_source` values.

### Requirement 5

**User Story:** As a pantry user who rescans the same unrecognized barcode, I want the app to remember that no database knows it, so that repeated scans stop paying for four upstream requests each time.

#### Acceptance Criteria

1. WHEN every Product_Opener_Database returns an Upstream_Miss for a barcode AND no Product_Opener_Database returns an Upstream_Error for that barcode, THE Lookup_Service SHALL record a Confirmed_Miss for that barcode stamped with the current Clock time.
2. WHILE a barcode has a Confirmed_Miss whose age is within Miss_TTL, THE Lookup_Service SHALL return a not-found result for that barcode and SHALL query no Product_Opener_Database.
3. WHEN Lookup_Service receives a barcode whose Confirmed_Miss is older than Miss_TTL, THE External_Lookup SHALL query the Product_Opener_Databases for that barcode again.
4. WHEN External_Lookup returns an Upstream_Hit for a barcode that has an expired Confirmed_Miss, THE Lookup_Service SHALL persist the resolved product and SHALL return that product on subsequent lookups of that barcode.
5. WHEN every Product_Opener_Database returns an Upstream_Miss for a barcode that already has a Confirmed_Miss, THE Lookup_Service SHALL stamp that Confirmed_Miss with the current Clock time, so that the next re-check happens one Miss_TTL later.
6. WHEN Lookup_Service returns a not-found result for a barcode with a Confirmed_Miss, THE Lookup_Service SHALL return the same result shape and `Source` value it returns for a barcode that is unknown upstream before this feature is added, so that the flagged-entry workflow behaves unchanged.
7. THE Product_Catalog SHALL exclude Confirmed_Miss records from product list responses, barcode lookup results, and inventory responses.
8. WHEN a user resolves a flagged scan entry by creating a product for a barcode that has a Confirmed_Miss, THE Lookup_Service SHALL return the user-created product for subsequent lookups of that barcode.

### Requirement 6

**User Story:** As a pantry user, I want an upstream outage to leave an unrecognized barcode retryable, so that a network blip never permanently marks a real product as unknown.

#### Acceptance Criteria

1. IF at least one Product_Opener_Database returns an Upstream_Error for a barcode AND no Product_Opener_Database returns an Upstream_Hit for that barcode, THEN THE Lookup_Service SHALL record no Confirmed_Miss for that barcode.
2. IF at least one Product_Opener_Database returns an Upstream_Error for a barcode AND no Product_Opener_Database returns an Upstream_Hit for that barcode, THEN THE Lookup_Service SHALL return a not-found result and SHALL create no `products` row for that barcode.
3. WHEN a fan-out for a barcode ends with at least one Upstream_Error and no Upstream_Hit, THE External_Lookup SHALL query the Product_Opener_Databases again on the next lookup of that barcode.
4. THE External_Lookup SHALL report an Upstream_Miss and an Upstream_Error as distinct outcomes to Lookup_Service, so that Lookup_Service can decide whether to record a Confirmed_Miss.
5. WHEN External_Lookup receives an Upstream_Error from a Product_Opener_Database, THE External_Lookup SHALL write one log entry naming the barcode, the database, and the failure reason.
6. WHEN External_Lookup receives an Upstream_Error from a Product_Opener_Database, THE Lookup_Service SHALL return a response to the caller without exposing the upstream failure detail in that response.

### Requirement 7

**User Story:** As a developer adding upstream databases, I want one client implementation shared by all four databases, so that retry, timeout, and response handling stay identical and a fifth database is a configuration change.

#### Acceptance Criteria

1. THE Product_Opener_Client SHALL accept the base URL of the Product_Opener_Database it queries as a construction parameter.
2. THE Product_Opener_Client SHALL apply the same retry behavior, including rate-limit retry, to every Product_Opener_Database.
3. THE Product_Opener_Client SHALL apply a 10 second request timeout to every Product_Opener_Database.
4. THE Product_Opener_Client SHALL decode the Product Opener response fields `product_name`, `categories`, `code`, `image_front_small_url`, and `status` for every Product_Opener_Database.
5. WHEN a Product_Opener_Database responds with HTTP 404, or with a status value other than 1, or with an empty product name, THE Product_Opener_Client SHALL report an Upstream_Miss.
6. WHEN a Product_Opener_Client resolves a barcode, THE Product_Opener_Client SHALL return a product whose `id` equals the requested barcode, so that the invariant that an External_Row's product ID is its barcode continues to hold.
7. THE Product_Opener_Client SHALL expose the queried base URL to tests, so that a fake upstream server can stand in for each of the four Product_Opener_Databases.

### Requirement 8

**User Story:** As the pantry operator, I want the existing external-lookup switch and cache settings to keep governing all four databases, so that disabling upstream access still disables upstream access.

#### Acceptance Criteria

1. WHERE the `DISABLE_EXTERNAL_PRODUCT_LOOKUP` environment variable equals `true`, THE Pantry_Server SHALL query no Product_Opener_Database for any barcode.
2. WHERE the `DISABLE_EXTERNAL_PRODUCT_LOOKUP` environment variable equals `true`, WHEN Lookup_Service reaches Tier 3 for a barcode, THE Lookup_Service SHALL return a not-found result and SHALL record no Confirmed_Miss.
3. WHERE the `DISABLE_EXTERNAL_PRODUCT_LOOKUP` environment variable equals `true`, THE Refresh_Service SHALL leave every row's field values and `refreshed_at` value unchanged.
4. WHERE the `PRODUCT_CACHE_TTL` environment variable holds a valid Go duration string, THE Pantry_Server SHALL use that duration as the Cache_TTL for External_Rows of every External_Source value.
5. WHERE the Miss_TTL configuration value is unset or empty, THE Pantry_Server SHALL use a Miss_TTL of 7 days.
6. WHERE the Miss_TTL configuration value holds a valid Go duration string, THE Pantry_Server SHALL use that duration as the Miss_TTL.
7. IF the Miss_TTL configuration value holds a value that is not a valid Go duration, THEN THE Pantry_Server SHALL write one log entry naming the invalid value and use a Miss_TTL of 7 days.
8. THE Pantry_Server SHALL provide the multi-database lookup using the existing single Go binary and SQLite database, with no additional process, scheduler, or network service.

### Requirement 9

**User Story:** As a pantry user reviewing a scanned item, I want to see which database supplied the product data, so that I can judge a wrong name or thumbnail and know where the data came from.

#### Acceptance Criteria

1. WHEN a barcode lookup response includes a product resolved from a Product_Opener_Database, THE Pantry_Server SHALL include that product's External_Source value in the response.
2. WHEN a scan queue response includes an entry whose product has an External_Source value, THE Pantry_Server SHALL include that External_Source value in the response.
3. WHEN an inventory response includes an item whose product has an External_Source value, THE Pantry_Server SHALL include that External_Source value in the response.
4. WHEN a Scan_Card displays an entry whose product carries an External_Source value, THE Scan_Card SHALL display a label naming the Product_Opener_Database that value identifies.
5. WHEN an Inventory_Row displays an item whose product carries an External_Source value, THE Inventory_Row SHALL display a label naming the Product_Opener_Database that value identifies.
6. WHERE a product has no External_Source value, THE Scan_Card and THE Inventory_Row SHALL display no provenance label.
7. WHEN the Scan_Card or the Inventory_Row displays a provenance label, THE label SHALL carry an accessible text alternative naming the Product_Opener_Database.

### Requirement 10

**User Story:** As a developer, I want the multi-database behavior verifiable without real network calls or timing flakiness, so that the suite stays fast and deterministic.

#### Acceptance Criteria

1. THE Pantry_Server SHALL allow a test to substitute a fake upstream for each of the four Product_Opener_Databases independently, so that a test can express a hit from one database and a miss or error from another.
2. THE External_Lookup SHALL read the current time from an injectable Clock, so that a test can expire a Confirmed_Miss without waiting.
3. THE External_Lookup SHALL determine Confirmed_Miss expiry without calling any sleep or wall-clock wait function.
4. WHEN a fan-out completes, THE External_Lookup SHALL leave no running goroutine that the fan-out started.
5. WHEN Lookup_Service resolves a barcode at Tier 1 or Tier 2, THE Lookup_Service SHALL return the same result shape and `Source` value it returns before this feature is added.
6. WHEN a barcode resolves from Open Food Facts, THE Lookup_Service SHALL persist the same `products` and `barcodes` values it persists before this feature is added, apart from the added `external_source` value.
