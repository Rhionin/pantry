# Requirements Document

## Introduction

The local `products` table is a write-once cache of Open Food Facts data. Once a barcode resolves at Tier 2 of the three-tier lookup, the cached row is never revalidated, so the cache drifts away from upstream: corrected names and categories never reach the user, rotated `image_front_small_url` values leave thumbnails returning 404, and a product first cached from a thin upstream record keeps its empty category forever.

This feature adds time-to-live based freshness to externally-sourced product rows using a stale-while-revalidate strategy: a stale cached row is served immediately and revalidated behind the response, so the scan path keeps its current latency. A manual refresh endpoint gives the user an escape hatch and gives tests a deterministic seam. User-authored data is protected: rows the user created are never revalidated, and a name the user edited survives every refresh.

## Glossary

- **Product_Catalog**: The `product.Catalog` data-access type, which owns reads and writes of the `products` and `barcodes` tables.
- **Lookup_Service**: The `product.LookupService` type, which performs the three-tier barcode lookup (Tier 1 user override, Tier 2 local catalog, Tier 3 Open Food Facts).
- **OFF_Client**: The `product.OpenFoodFactsClient` type, which queries the Open Food Facts API for a single barcode.
- **Refresh_Service**: The component that revalidates a single externally-sourced product row against Open Food Facts and merges the result into the local row.
- **Refresh_API**: The HTTP handler serving `POST /api/products/{id}/refresh`.
- **Migration_Runner**: The `app.RunMigrations` function, which applies numbered SQL migrations in `internal/app/migrations`.
- **Pantry_Server**: The single Go server binary in `cmd/server`, which reads configuration from environment variables.
- **External_Row**: A `products` row whose `source` column equals `external`, meaning its field values originated from Open Food Facts.
- **User_Row**: A `products` row whose `source` column equals `user`, meaning a user authored its field values.
- **Cache_TTL**: The maximum age at which an External_Row is considered fresh, measured as the elapsed time since the row's `refreshed_at` timestamp.
- **Stale_Row**: An External_Row whose `refreshed_at` is NULL or older than Cache_TTL.
- **Fresh_Row**: An External_Row whose `refreshed_at` is within Cache_TTL of the current time.
- **Refresh_Outcome**: The classification of a completed refresh attempt, one of `updated`, `unchanged`, or `not_found_upstream`.
- **Clock**: The injectable time source used by Refresh_Service and Lookup_Service to read the current time.

## Requirements

### Requirement 1

**User Story:** As a pantry user, I want the products table to record where each product's data came from and when it was last checked against Open Food Facts, so that the system can tell fresh cached data from stale cached data.

#### Acceptance Criteria

1. THE Migration_Runner SHALL add a `source` column to the `products` table that accepts the values `external` and `user`.
2. THE Migration_Runner SHALL add a nullable `refreshed_at` timestamp column to the `products` table.
3. THE Migration_Runner SHALL add a `name_overridden` boolean column to the `products` table with a default value of false.
4. WHEN the migration that adds the freshness columns is applied, THE Migration_Runner SHALL set `source` to `external`, `refreshed_at` to NULL, and `name_overridden` to false for every pre-existing `products` row.
5. WHERE a `products` row has `refreshed_at` equal to NULL, THE Product_Catalog SHALL report that row as a Stale_Row.
6. WHEN Lookup_Service persists a product resolved from Open Food Facts, THE Product_Catalog SHALL store that row with `source` set to `external`, `name_overridden` set to false, and `refreshed_at` set to the current Clock time.
7. WHEN a user creates a product through the product API, THE Product_Catalog SHALL store that row with `source` set to `user`.

### Requirement 2

**User Story:** As a pantry user scanning items, I want scans to stay as fast as they are today, so that adding freshness checks does not slow down my workflow.

#### Acceptance Criteria

1. WHEN Lookup_Service resolves a barcode at Tier 2, THE Lookup_Service SHALL return the cached product without waiting for any Open Food Facts request.
2. WHEN Lookup_Service resolves a barcode to a Stale_Row, THE Lookup_Service SHALL return the currently cached field values of that row.
3. WHEN Lookup_Service resolves a barcode to a Stale_Row, THE Lookup_Service SHALL schedule a Refresh_Service revalidation of that row that runs after the lookup response is returned.
4. WHEN Lookup_Service resolves a barcode to a Fresh_Row, THE Lookup_Service SHALL schedule no revalidation.
5. WHILE a revalidation of a given barcode is in flight, THE Refresh_Service SHALL make no additional Open Food Facts request for that barcode, so that N concurrent stale lookups of one barcode produce exactly one Open Food Facts request.
6. WHEN Lookup_Service resolves a barcode to a User_Row, THE Lookup_Service SHALL schedule no revalidation.

### Requirement 3

**User Story:** As a pantry user, I want a refresh to pick up corrected upstream facts while keeping any product name I typed myself, so that upstream improvements reach me without overwriting my edits.

#### Acceptance Criteria

1. WHEN Refresh_Service receives product data from Open Food Facts for an External_Row, THE Refresh_Service SHALL overwrite each of the row's `category`, `unit_of_measure`, and `image_url` values for which Open Food Facts returned a non-empty value, and SHALL retain the row's existing value for each of those fields for which Open Food Facts returned an empty value.
2. WHERE an External_Row has `name_overridden` equal to false AND Open Food Facts returned a non-empty name, WHEN Refresh_Service receives product data from Open Food Facts for that row, THE Refresh_Service SHALL overwrite the row's `name` with the name returned by Open Food Facts, and SHALL otherwise retain the row's existing `name`.
3. WHERE an External_Row has `name_overridden` equal to true, WHEN Refresh_Service receives product data from Open Food Facts for that row, THE Refresh_Service SHALL retain the row's existing `name`.
4. WHEN a user updates the `name` of an External_Row through the product API, THE Product_Catalog SHALL set that row's `name_overridden` to true.
5. WHEN a user updates fields other than `name` on an External_Row through the product API, THE Product_Catalog SHALL retain that row's existing `name_overridden` value.
6. WHEN Refresh_Service completes a merge for a row, THE Refresh_Service SHALL set that row's `refreshed_at` to the current Clock time.
7. WHEN Refresh_Service completes a merge for a row, THE Refresh_Service SHALL retain that row's existing `id`, `source`, and `created_at` values.

### Requirement 4

**User Story:** As a pantry user, I want a stale product refreshed on demand and told what changed, so that I have a way to fix a wrong name or broken thumbnail immediately.

#### Acceptance Criteria

1. WHEN Refresh_API receives `POST /api/products/{id}/refresh` for an existing External_Row, THE Refresh_API SHALL run the Refresh_Service revalidation for that row synchronously and respond after the revalidation completes.
2. WHEN Refresh_API runs a revalidation, THE Refresh_API SHALL run that revalidation regardless of whether the row is a Fresh_Row or a Stale_Row.
3. WHEN Refresh_API completes a revalidation, THE Refresh_API SHALL respond with HTTP 200, the refreshed product summary, and a Refresh_Outcome value.
4. WHEN a revalidation changes at least one of the row's `name`, `category`, `unit_of_measure`, or `image_url` values, THE Refresh_API SHALL report the Refresh_Outcome `updated`.
5. WHEN a revalidation leaves all of the row's `name`, `category`, `unit_of_measure`, and `image_url` values equal to their prior values, THE Refresh_API SHALL report the Refresh_Outcome `unchanged`.
6. WHEN Open Food Facts reports that the barcode is unknown during a revalidation, THE Refresh_API SHALL report the Refresh_Outcome `not_found_upstream`.
7. IF Refresh_API receives a refresh request for a product `id` that has no `products` row, THEN THE Refresh_API SHALL respond with HTTP 404 and a message stating that the product was not found.
8. IF Refresh_API receives a refresh request for a User_Row, THEN THE Refresh_API SHALL leave that row's field values unchanged and respond with the Refresh_Outcome `unchanged`.
9. IF the Open Food Facts request fails during a synchronous refresh, THEN THE Refresh_API SHALL leave the row's field values unchanged and respond with HTTP 502 and a message stating that the product could not be refreshed.

### Requirement 5

**User Story:** As a pantry user, I want a failed refresh to leave my cached product intact, so that an upstream outage never empties or hides a product I own.

#### Acceptance Criteria

1. WHEN Open Food Facts reports that the barcode is unknown during a revalidation, THE Refresh_Service SHALL retain the row's existing `name`, `category`, `unit_of_measure`, and `image_url` values.
2. WHEN Open Food Facts reports that the barcode is unknown during a revalidation, THE Refresh_Service SHALL set the row's `refreshed_at` to the current Clock time, so that the next lookup within Cache_TTL issues no further Open Food Facts request.
3. IF an Open Food Facts request fails with a transport, timeout, or non-success status error during a revalidation, THEN THE Refresh_Service SHALL retain the row's existing field values and set the row's `refreshed_at` to the current Clock time.
4. WHEN a revalidation fails, THE Refresh_Service SHALL retain the row's `products` row and its `barcodes` mappings.
5. WHEN a background revalidation fails, THE Refresh_Service SHALL write one log entry naming the barcode and the failure reason.
6. WHEN a background revalidation fails, THE Lookup_Service SHALL return the same lookup response it would have returned had the revalidation not been scheduled.

### Requirement 6

**User Story:** As the pantry operator, I want the freshness window configurable through the environment, so that I can tune revalidation frequency without rebuilding the binary.

#### Acceptance Criteria

1. WHERE the `PRODUCT_CACHE_TTL` environment variable is unset or empty, THE Pantry_Server SHALL use a Cache_TTL of 30 days.
2. WHERE the `PRODUCT_CACHE_TTL` environment variable holds a valid Go duration string, THE Pantry_Server SHALL use that duration as the Cache_TTL.
3. IF the `PRODUCT_CACHE_TTL` environment variable holds a value that is not a valid Go duration, THEN THE Pantry_Server SHALL log a message naming the invalid value and use a Cache_TTL of 30 days.
4. WHERE the `DISABLE_EXTERNAL_PRODUCT_LOOKUP` environment variable equals `true`, THE Refresh_Service SHALL leave every row's field values unchanged and make no Open Food Facts request.

### Requirement 7

**User Story:** As a pantry user, I want my own product data left alone by the refresh machinery, so that overrides I created keep winning.

#### Acceptance Criteria

1. THE Refresh_Service SHALL restrict field-value writes to rows whose `source` equals `external`.
2. WHEN Lookup_Service resolves a barcode that has a `user_override` mapping for the requesting user, THE Lookup_Service SHALL return the overriding product, regardless of the freshness of any `global` mapping for that barcode.
3. WHEN a revalidation completes for an External_Row that a `user_override` mapping shadows, THE Lookup_Service SHALL continue to return the overriding product for the user who owns that override.
4. THE Product_Catalog SHALL retain the `barcodes.source` value of every existing mapping when a revalidation writes to a `products` row.

### Requirement 8

**User Story:** As a developer, I want the freshness behavior verifiable without new infrastructure or timing flakiness, so that the suite stays fast and deterministic.

#### Acceptance Criteria

1. THE Pantry_Server SHALL provide the refresh behavior using the existing single Go binary and SQLite database, with no additional process, scheduler, or network service.
2. THE Refresh_Service SHALL read the current time from an injectable Clock, so that tests can advance time without waiting.
3. THE Refresh_Service SHALL expose a mechanism that lets a test await the completion of all scheduled background revalidations.
4. WHEN a test completes, THE Refresh_Service SHALL leave no running goroutine that it started.
5. THE Refresh_Service SHALL determine staleness without calling any sleep or wall-clock wait function.
6. WHEN Lookup_Service resolves a barcode at Tier 1, at Tier 2 for a Fresh_Row, or finds no product at any tier, THE Lookup_Service SHALL return the same result shape and `Source` value it returns before this feature is added.
