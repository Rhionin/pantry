# Implementation Plan: Pantry Management

## Overview

Implement the Pantry Management app as a Go REST API backend with a React + TypeScript frontend (Mantine UI), backed by a SQLite event-log database. Work proceeds layer-by-layer: project scaffolding and schema first, then the scan queue, then inventory, then suggestions, then shopping list, then frontend, with property-based tests and integration tests woven in throughout.

---

## Tasks

- [x] 1. Project scaffolding and database schema
  - [x] 1.1 Initialize Go module, directory layout, and SQLite migration runner
    - Create `go.mod` (`module github.com/Rhionin/pantry`), `cmd/server/main.go`, and `internal/` subdirectories (`db`, `routes`, `service`, `model`)
    - Add dependencies: `modernc.org/sqlite` (pure-Go SQLite driver), `pgregory.net/rapid` (property tests); use Go 1.22+ standard library `net/http` `ServeMux` for routing (no third-party router needed — `ServeMux` supports method+path patterns like `GET /api/scans/{id}`)
    - Implement a simple migration runner that applies numbered `.sql` files from `internal/db/migrations/` in order
    - _Requirements: all_

  - [x] 1.2 Write and apply the initial SQLite schema migration
    - Create `internal/db/migrations/001_initial_schema.sql` with all tables from the design: `products`, `barcodes`, `items`, `item_instances`, `scan_entries`, `consumption_events`, `shopping_list_items`, `cart_integrations`
    - Include all constraints, CHECK clauses, and the `UNIQUE (barcode, source, user_id)` constraint on `barcodes`
    - Write a `TestMigrationApplies` integration test using an in-memory SQLite database
    - _Requirements: 1.1, 1.2, 2.1_

  - [x] 1.3 Initialize React + TypeScript frontend project
    - Bootstrap with Vite (`npm create vite@latest`) and create the top-level routing shell with React Router
    - Install Mantine v7: `@mantine/core`, `@mantine/hooks`, `@mantine/dates`, `@mantine/notifications`; import `@mantine/core/styles.css` in `main.tsx` and wrap the app in `<MantineProvider>`
    - Add `fast-check` as a dev dependency for frontend property tests
    - Add Playwright as a dev dependency for end-to-end tests
    - _Requirements: all_

- [x] 2. Product lookup and override layer
  - [x] 2.1 Implement product repository and types (colocated in feature package)
    - Write `internal/product/product.go` with types: `Product` and `ProductSummary`
    - Implement repository methods: `CreateProduct`, `GetProductByID`, `ListProducts`, `UpdateProduct`, `UpsertBarcodeMapping`, `LookupByBarcode` (checks `user_override` rows before `global` rows)
    - Types and repository colocated in the same package — no separate `internal/model/` or `internal/db/` files for products
    - _Requirements: 1.15, 1.16, 1.17, 1.18, 1.19_

  - [x] 2.2 Implement Open Food Facts API client
    - Write `internal/product/openfoodfacts.go` with `LookupBarcode(ctx, barcode) (*ProductSummary, error)` using the Open Food Facts REST API (`https://world.openfoodfacts.org/api/v2/product/{barcode}`)
    - Implement retry with exponential backoff (max 3 attempts) for rate-limit and transient errors
    - Expose a `ProductLookupClient` interface so tests can inject a mock
    - _Requirements: 1.15_

  - [x] 2.3 Implement product lookup service (override → local DB → external API)
    - Write business logic in `internal/product/lookup.go` implementing the three-tier lookup: user overrides first, global DB second, Open Food Facts third
    - Return a disambiguation list when multiple products match a barcode
    - _Requirements: 1.15, 1.18, 1.19_

  - [x] 2.4 Write integration tests for product lookup service
    - Test override precedence: user override beats global DB beats external API
    - Test disambiguation: multiple matches returns list, not single product
    - Test external fallback with a mock HTTP server
    - _Requirements: 1.15, 1.18, 1.19_

  - [x] 2.5 Wire product API endpoints
    - Create handler functions in feature packages (e.g., `internal/product/handlers.go`), each implementing `http.Handler`:
      - `LookupHandler` — `GET /api/products/lookup?barcode={barcode}`
      - `ListHandler` — `GET /api/products`
      - `CreateHandler` — `POST /api/products`
      - `UpdateHandler` — `PUT /api/products/{id}`
      - `OverrideCreateHandler` — `POST /api/products/overrides`
    - Register all routes in `main.go` using `mux.Handle("METHOD /path", &product.HandlerStruct{...})`
    - _Requirements: 1.16, 1.17, 1.18, 1.19_

  - [x] 2.6 Rewrite handler tests using apitest framework
    - Replace manual `httptest.NewRecorder()` and `httptest.NewRequest()` test setup with `github.com/steinfletcher/apitest` fluent API
    - Tests should invoke the full `server.NewHandler()` to test the complete request/response cycle through the `http.ServeMux`
    - Identify and extract reusable setup function logic into `internal/server/setup_test.go`
    - Use `apitest.New().Handler(handler).Get("/api/products")...` pattern for all handler tests
    - Each test should use apitest's `.Expect().Status(200).Body(...)` assertions instead of manual status checks and JSON unmarshaling
    - Migrate all existing handler tests in `internal/server/handler_*_test.go`: lookup, list, create, update, override
    - _Requirements: 1.16, 1.17, 1.18, 1.19_

- [ ] 3. Scan queue backend
  - [ ] 3.1 Implement scan entry repository and types (colocated in feature package)
    - Write `internal/scan/scan.go` with types: `ScanEntry` and related models
    - Implement repository methods: `CreateScanEntry`, `GetScanEntry`, `ListScanEntries` (filter by status), `UpdateScanEntry` (direction, unit count, expiry, product_id, status), `CommitScanEntry`, `BatchUpdateScanEntries`
    - Enforce append-only semantics on `scan_entries`: repository functions MUST NOT issue UPDATE or DELETE SQL; status transitions are encoded as new column values via a single allowed UPDATE path (PATCH endpoint only)
    - Types and repository colocated in the feature package
    - _Requirements: 1.1, 1.2, 1.6, 1.7, 1.10, 1.14_

  - [ ] 3.2 Implement scan commit service (stock-in path)
    - Write business logic in `internal/scan/commit.go` with `CommitStockIn(scanEntry)` — creates exactly N `item_instances` rows (N = `unit_count`) with the scan entry's `stock_in_at` and `expires_at`, then marks the scan entry `committed`
    - _Requirements: 1.11_

  - [ ]* 3.3 Write property test for Property 5 (stock-in creates exactly N instances)
    - **Property 5: Stock-in commit creates exactly N instances**
    - For any scan entry with direction `stock_in` and unit count N ≥ 1, committing creates exactly N new item instances and increases total instance count by N
    - Use `rapid` to generate arbitrary (barcode, unit count 1–20, optional expiry) combinations
    - **Validates: Requirements 1.11**

  - [ ] 3.4 Implement scan commit service (stock-out path)
    - Write `CommitStockOut(scanEntry, instanceID *string)` in `internal/scan/commit.go` — if `instanceID` is nil, select the use-oldest-first instance via `SELECT … ORDER BY expires_at ASC NULLS LAST LIMIT 1`; set `removed_at` and `removal_reason = 'consumed'`; insert a `consumption_events` row; mark scan entry `committed`
    - _Requirements: 1.12, 1.9, 2.3_

  - [ ]* 3.5 Write property test for Property 6 (stock-out removes use-oldest-first)
    - **Property 6: Stock-out commit removes the use-oldest-first instance**
    - For any item with a generated set of instances (varying expiry dates including nulls), committing a stock-out with no instance selected removes exactly the instance with the earliest non-null expiry date, or the lexicographically smallest id when all are null
    - **Validates: Requirements 1.12, 1.9, 2.3**

  - [ ] 3.6 Implement scan commit service (flagged entry resolution)
    - Write `ResolveFlaggedEntry(scanEntryID, productID)` in `internal/scan/commit.go` — creates a `barcodes` row with `source='user_override'`, sets `product_id` on the scan entry, transitions status `flagged → pending`
    - _Requirements: 1.17, 1.19_

  - [ ]* 3.7 Write property test for Property 8 (flagged entry resolution)
    - **Property 8: Flagged entry resolution creates override and transitions to pending**
    - For any flagged scan entry and any product, resolving it creates exactly one `user_override` barcode row and leaves the entry in `pending` status
    - **Validates: Requirements 1.17, 1.19**

  - [ ] 3.8 Wire scan queue API endpoints
    - Create handler functions in `internal/scan/handlers.go`, each implementing `http.Handler`:
      - `CreateHandler` — `POST /api/scans`
      - `ListHandler` — `GET /api/scans?status=`
      - `HistoryHandler` — `GET /api/scans/history`
      - `UpdateHandler` — `PATCH /api/scans/{id}`
      - `CommitHandler` — `POST /api/scans/{id}/commit`
      - `BatchCommitHandler` — `POST /api/scans/batch-commit`
    - Register all routes in `main.go` using Go 1.22+ `ServeMux` method+path patterns
    - _Requirements: 1.1, 1.2, 1.6, 1.7, 1.8, 1.9, 1.10, 1.11, 1.12, 1.13, 1.14_

  - [ ]* 3.9 Write property tests for Properties 1, 2, 3, 4 (scan entry integrity, direction propagation, ordering, batch update)
    - **Property 1: Scan entry data integrity** — for any barcode and timestamp, the stored entry preserves them exactly with `status=pending`
    - **Property 2: Scan direction propagation** — scans within 5 min carry the pre-selected direction; scans after 5 min do not
    - **Property 3: Scan queue chronological ordering** — list result is always ascending by `scanned_at`
    - **Property 4: Batch update applies to all selected entries** — every selected entry is updated; no unselected entry is modified
    - Use `rapid` with generated barcode strings, timestamps, and direction values
    - **Validates: Requirements 1.1, 1.2, 1.4, 1.5, 1.7, 1.10**

  - [ ]* 3.10 Write property test for Property 7 (committed entry in history)
    - **Property 7: Committed scan entry is preserved in history**
    - For any committed scan entry, it is absent from `pending` list and present in history with all original fields unchanged
    - **Validates: Requirements 1.14**

- [ ] 4. Checkpoint — Ensure all scan queue tests pass
  - Run `go test ./...` and confirm all scan queue unit and property tests pass. Ask the user if any questions arise before continuing.

- [ ] 5. Inventory backend
  - [ ] 5.1 Implement item and instance repository and types (colocated in feature package)
    - Write `internal/inventory/inventory.go` with types: `Item`, `ItemInstance` and related models
    - Implement repository methods: `GetOrCreateItem`, `ListItems`, `ListItemInstances`, `AddInstance`, `RemoveInstance`, `GetInstance`
    - `RemoveInstance` must check existence and return a typed `ErrInstanceNotFound` if missing
    - Types and repository colocated in the feature package
    - _Requirements: 2.1, 2.2, 2.3, 2.6, 2.7_

  - [ ] 5.2 Implement expiry status calculation
    - Write `internal/inventory/expiry.go`: pure function `ComputeExpiryStatus(expiresAt *time.Time, now time.Time, warningDays int) ExpiryStatus` returning `ok`, `near_expiry`, or `expired`
    - _Requirements: 2.8, 2.9_

  - [ ]* 5.3 Write property test for Property 9 (expiry status consistency)
    - **Property 9: Expiry status is consistent with dates and warning period**
    - For any expiry date, current time, and warning period, the returned status satisfies all three branches of the algorithm; instances with no expiry always return `ok`
    - Use `rapid` with generated time pairs and warning periods 1–30 days
    - **Validates: Requirements 2.8, 2.9**

  - [ ] 5.4 Implement inventory aggregation service
    - Write `internal/inventory/aggregate.go`: `GetInventoryList(userID)` — returns `InventoryItem` slice with `instanceCount`, `nearExpiryCount`, `expiredCount`, and `needsAttention` derived by calling `ComputeExpiryStatus` on each instance; applies use-oldest-first sort for instance lists
    - _Requirements: 2.2, 2.3, 2.10, 2.11_

  - [ ]* 5.5 Write property tests for Properties 10 and 11 (Needs Attention, search filter)
    - **Property 10: Needs Attention section contains exactly the right items** — generated inventory state produces Needs Attention list iff item has ≥1 non-ok instance
    - **Property 11: Inventory search filter returns exactly matching items** — for any query string and inventory, filtered result is exactly the case-insensitive subset
    - **Validates: Requirements 2.4, 2.10, 2.11**

  - [ ] 5.6 Wire inventory API endpoints
    - Create handler functions in `internal/inventory/handlers.go`, each implementing `http.Handler`:
      - `ListHandler` — `GET /api/inventory`
      - `InstancesListHandler` — `GET /api/inventory/{itemId}/instances`
      - `InstanceCreateHandler` — `POST /api/inventory/{itemId}/instances`
      - `InstanceDeleteHandler` — `DELETE /api/inventory/instances/{instanceId}`
    - Return HTTP 404 with a structured error body when `ErrInstanceNotFound`
    - Register all routes in `main.go`
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7_

- [ ] 6. Suggestion engine backend
  - [ ] 6.1 Implement consumption event repository and types (colocated in feature package)
    - Write `internal/suggestion/suggestion.go` with types: `ConsumptionEvent`, `TargetQuantitySuggestion` and related models
    - Implement repository methods: `InsertConsumptionEvent` and `ListConsumptionEvents(itemID)` (sorted ascending by `consumed_at`)
    - Types and repository colocated in the feature package
    - _Requirements: 3.1_

  - [ ] 6.2 Implement suggestion service
    - Write `internal/suggestion/engine.go`: `SuggestTargetQuantity(itemID, events []ConsumptionEvent) TargetQuantitySuggestion`
    - Return `dataInsufficient: true` when `len(events) < 3`; otherwise compute median inter-consumption interval, derive `ceil(restockHorizon / medianInterval) + 1`, and populate `reasoning` string
    - _Requirements: 3.1, 3.2, 3.5_

  - [ ]* 6.3 Write property test for Property 14 (data-insufficient threshold)
    - **Property 14: Suggestion returns data-insufficient for fewer than 3 consumption events**
    - For any item with 0, 1, or 2 events: `dataInsufficient = true`, no numeric suggestion. For any item with ≥3 events: numeric suggestion and non-empty reasoning.
    - Use `rapid` to generate event lists of length 0–10 with varying timestamps
    - **Validates: Requirements 3.1, 3.2, 3.5**

  - [ ] 6.4 Wire suggestion and target-quantity API endpoints
    - Create handler functions in `internal/suggestion/handlers.go`, each implementing `http.Handler`:
      - `GetHandler` — `GET /api/suggestions/{itemId}`
      - `SetTargetQuantityHandler` — `POST /api/items/{itemId}/target-quantity`
    - Register all routes in `main.go`
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6_

- [ ] 7. Shopping list backend
  - [ ] 7.1 Implement shopping list derivation service
    - Write `internal/shopping/derive.go`: `DeriveShoppingList(userID)` — for each item with a non-null `target_quantity` where `currentInstanceCount < target_quantity`, add a derived entry with `quantity = target_quantity − currentInstanceCount`; merge with manual `shopping_list_items` rows (manual overrides derived quantity for the same item)
    - _Requirements: 4.1, 4.2, 4.8_

  - [ ]* 7.2 Write property test for Property 12 (shopping list gap quantity)
    - **Property 12: Shopping list gap quantity is always correct**
    - For any item with target quantity T and instance count C: if C < T → item present with quantity T−C; if C ≥ T → item absent from auto-generated list
    - Use `rapid` to generate (T, C) pairs across a wide range
    - **Validates: Requirements 4.1, 4.2, 4.8**

  - [ ] 7.3 Implement shopping list repository and types (colocated in feature package)
    - Write `internal/shopping/shopping.go` with types: `ShoppingListItem` and related models
    - Implement repository methods: `AddManualItem`, `RemoveItem`, `MarkPurchased`, `ListManualItems`
    - `MarkPurchased` sets `purchased_at` and does NOT modify any `item_instances` row
    - Types and repository colocated in the feature package
    - _Requirements: 4.4, 4.5, 4.6_

  - [ ]* 7.4 Write property test for Property 13 (marking purchased does not modify inventory)
    - **Property 13: Purchasing a shopping list item does not modify inventory**
    - For any inventory state and shopping list item marked as purchased, all `item_instances` row counts are identical before and after the mark-as-purchased call
    - **Validates: Requirements 4.6**

  - [ ] 7.5 Implement cart export service stub
    - Write `internal/shopping/export.go` with a `CartExporter` interface and a no-op implementation; return structured errors for partial failures
    - _Requirements: 4.9, 4.10_

  - [ ] 7.6 Wire shopping list API endpoints
    - Create handler functions in `internal/shopping/handlers.go`, each implementing `http.Handler`:
      - `GetHandler` — `GET /api/shopping-list`
      - `ItemCreateHandler` — `POST /api/shopping-list/items`
      - `ItemDeleteHandler` — `DELETE /api/shopping-list/items/{id}`
      - `ItemUpdateHandler` — `PATCH /api/shopping-list/items/{id}`
      - `ExportHandler` — `POST /api/shopping-list/export`
    - Register all routes in `main.go`
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8, 4.9, 4.10_

- [ ] 8. Checkpoint — Ensure all backend tests pass
  - Run `go test ./...` and confirm all backend tests pass. Ask the user if any questions arise before continuing.

- [ ] 9. Frontend — shared utilities and TypeScript types
  - [ ] 9.1 Define shared TypeScript interfaces and API client
    - Create `src/types/index.ts` with all DTOs from the design: `ScanEntry`, `ProductSummary`, `InventoryItem`, `ItemInstance`, `ShoppingListEntry`, `TargetQuantitySuggestion`
    - Create `src/api/client.ts` with typed `fetch` wrappers for every backend endpoint
    - _Requirements: all_

  - [ ] 9.2 Implement expiry status utility (TypeScript)
    - Create `src/utils/expiry.ts`: `computeExpiryStatus(expiresAt: string | null, now: Date, warningDays = 7): 'ok' | 'near_expiry' | 'expired'`
    - _Requirements: 2.8, 2.9_

  - [ ]* 9.3 Write fast-check property test for expiry status utility (Property 9)
    - **Property 9: Expiry status is consistent with dates and warning period** (frontend mirror)
    - Use `fast-check` arbitraries for dates and warning periods; verify all three status branches
    - **Validates: Requirements 2.8, 2.9**

  - [ ] 9.4 Implement shopping list derivation utility (TypeScript)
    - Create `src/utils/shoppingList.ts`: `deriveShoppingListEntries(items: InventoryItem[]): ShoppingListEntry[]` — pure function computing gap quantities
    - _Requirements: 4.1, 4.2_

  - [ ]* 9.5 Write fast-check property test for shopping list derivation utility (Property 12)
    - **Property 12: Shopping list gap quantity is always correct** (frontend mirror)
    - Use `fast-check` to generate arbitrary `InventoryItem` arrays with varying `targetQuantity` and `instanceCount`
    - **Validates: Requirements 4.1, 4.2, 4.8**

- [ ] 10. Frontend — barcode scanning components
  - [ ] 10.1 Implement `BarcodeInputField` component
    - Keyboard-capture `<input>` that auto-submits when a barcode terminator character (e.g. Enter) is detected; calls `onScan(barcode: string)` callback; renders as visually hidden when embedded in scanner page
    - _Requirements: 1.1_

  - [ ] 10.2 Implement `CameraScanner` component
    - Uses `BarcodeDetector` API (`new BarcodeDetector({ formats: [...] })`); requests camera permission; shows live video feed with scan overlay; calls `onScan(barcode)` on detection; gracefully falls back when `BarcodeDetector` is unavailable
    - _Requirements: 1.1, 1.3_

  - [ ] 10.3 Implement `ScanDirectionToggle` component and 5-minute auto-clear
    - Toggle between `stock_in` / `stock_out` / unset; records `lastScanAt` in component state; sets a `setTimeout` for 5 minutes; clears direction when timer fires; resets timer on each new scan
    - _Requirements: 1.4, 1.5_

  - [ ]* 10.4 Write unit tests for `ScanDirectionToggle` auto-clear with mocked timers
    - Use `vi.useFakeTimers()` (Vitest); confirm direction clears after 5 min idle; confirm direction is preserved if a scan occurs within 5 min
    - _Requirements: 1.4, 1.5_

- [ ] 11. Frontend — scan queue pages
  - [ ] 11.1 Implement `ScanQueuePage` and `ScanEntryCard`
    - Fetches `GET /api/scans?status=pending` and `?status=flagged`; renders entries in chronological order using `ScanEntryCard`; shows flagged badge for flagged entries
    - _Requirements: 1.6, 1.7, 1.15_

  - [ ] 11.2 Implement `BatchReviewPanel`
    - Multi-select checkboxes on `ScanEntryCard`; batch direction + expiry form; calls `POST /api/scans/batch-commit` on confirm
    - _Requirements: 1.10_

  - [ ] 11.3 Implement `FlaggedEntryResolver`
    - Product search autocomplete calling `GET /api/products`; "Create new product" form; on selection calls `POST /api/products/overrides` then `PATCH /api/scans/{id}` to attach product and transition to pending
    - _Requirements: 1.16, 1.17_

  - [ ] 11.4 Implement `DisambiguationModal`
    - Shown when `GET /api/products/lookup` returns multiple products; lists matching products; on selection optionally saves override via `POST /api/products/overrides`
    - _Requirements: 1.18, 1.19_

  - [ ] 11.5 Implement stock-out instance selection view
    - When reviewing a pending stock-out entry, fetches `GET /api/inventory/{itemId}/instances`; renders instances sorted use-oldest-first; allows selecting a specific instance before committing
    - _Requirements: 1.9, 2.3_

- [ ] 12. Frontend — inventory pages
  - [ ] 12.1 Implement `InventoryPage` and `ItemRow`
    - Fetches `GET /api/inventory`; renders grouped list; places `needsAttention` items in "Needs Attention" section at top; includes search input that filters by name/category using `src/utils/inventoryFilter.ts` (local filter — no extra API call)
    - _Requirements: 2.2, 2.4, 2.10, 2.11_

  - [ ] 12.2 Implement `ItemInstanceList` and expiry badges
    - Fetches `GET /api/inventory/{itemId}/instances` on item selection; renders instances sorted use-oldest-first; shows `near_expiry` (yellow) and `expired` (red) badges using `computeExpiryStatus`
    - _Requirements: 2.3, 2.8, 2.9_

  - [ ] 12.3 Implement `AddInstanceModal`
    - Form with expiry date picker; on submit calls `POST /api/inventory/{itemId}/instances`; closes and refreshes instance list on success
    - _Requirements: 2.5_

- [ ] 13. Frontend — suggestions and shopping list pages
  - [ ] 13.1 Implement `SuggestionPanel`
    - Fetches `GET /api/suggestions/{itemId}` on demand; displays suggested quantity and reasoning; "Accept" button calls `POST /api/items/{itemId}/target-quantity`; "Set manually" shows numeric input
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5_

  - [ ] 13.2 Implement `ShoppingListPage` and `CartExportButton`
    - Fetches `GET /api/shopping-list`; renders derived and manual entries; marks items purchased via `PATCH /api/shopping-list/items/{id}`; manual add calls `POST /api/shopping-list/items`; export calls `POST /api/shopping-list/export`; shows partial-failure notification toast
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8, 4.9, 4.10_

- [ ] 14. End-to-end tests (Playwright)
  - [ ] 14.1 Write Playwright test: scan → review → commit (stock-in) flow
    - Simulate HID barcode input into `BarcodeInputField`; verify scan entry appears in queue; set direction and expiry; commit; verify inventory count increases
    - _Requirements: 1.1, 1.2, 1.8, 1.11, 2.2_

  - [ ]* 14.2 Write Playwright test: flagged barcode resolution flow
    - Scan unknown barcode; verify flagged badge; resolve via `FlaggedEntryResolver`; verify entry transitions to pending; commit
    - _Requirements: 1.15, 1.16, 1.17_

  - [ ]* 14.3 Write Playwright test: stock-out use-oldest-first flow
    - Stock in two instances with different expiry dates; stock out once with no instance selected; verify the earlier-expiry instance is removed
    - _Requirements: 1.9, 1.12, 2.3_

  - [ ]* 14.4 Write Playwright test: shopping list derivation and purchase flow
    - Set target quantity; reduce inventory below target; verify item appears in shopping list with correct gap quantity; mark purchased; verify item leaves active list; verify inventory unchanged
    - _Requirements: 4.1, 4.2, 4.6, 4.8_

- [ ] 15. Final checkpoint — Ensure all tests pass
  - Run `go test ./...` (backend) and `npm test -- --run` (frontend). Confirm all unit, property, and integration tests pass. Run `npx playwright test` for E2E. Ask the user if any questions arise.

---

## Notes

- Tasks marked with `*` are optional and can be skipped for a faster MVP.
- Each task references specific requirements for traceability.
- Backend property tests use `pgregory.net/rapid`; frontend property tests use `fast-check`.
- All property tests must include a comment: `// Feature: pantry-management, Property N: <property text>`.
- Database integration tests use in-memory SQLite (`:memory:`) — no Docker needed.
- The scan direction auto-clear (5-minute idle) is implemented entirely client-side; the timer is reset on every successful scan.
- Append-only semantics on `scan_entries` are enforced at the application layer; repository functions must not issue bare UPDATE/DELETE SQL on that table.
- **Routing**: Uses Go 1.22+ `net/http.ServeMux` with method+path patterns (e.g. `GET /api/scans/{id}`). No third-party router.
- **Route structure**: Handlers are defined within feature packages (e.g., `internal/product/handlers.go`, `internal/scan/handlers.go`). Each handler or handler struct declares its own anonymous interface(s) for dependencies — no shared interface types are reused across handler structs.
- **Feature-based organization**: All code for a domain (types, repository, business logic, tests) lives in a single feature package under `internal/`. This maximizes discoverability and reduces coupling. Shared infrastructure (`app/`, migrations) lives in `internal/app/`.
- **Only third-party dependencies**: `modernc.org/sqlite` and `pgregory.net/rapid` for backend; `fast-check` for frontend property tests.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.2", "1.3"] },
    { "id": 1, "tasks": ["2.1", "2.2", "9.1"] },
    { "id": 2, "tasks": ["2.3", "3.1", "6.1"] },
    { "id": 3, "tasks": ["2.4", "2.5", "3.2", "3.4", "3.6", "9.2", "9.4"] },
    { "id": 4, "tasks": ["2.6", "3.3", "3.5", "3.7", "3.8", "9.3", "9.5"] },
    { "id": 5, "tasks": ["3.9", "3.10", "5.1", "6.2"] },
    { "id": 6, "tasks": ["5.2", "5.4", "6.3", "6.4", "7.1", "7.3"] },
    { "id": 7, "tasks": ["5.3", "5.5", "5.6", "7.2", "7.4", "7.5", "10.1", "10.2", "10.3"] },
    { "id": 8, "tasks": ["7.6", "10.4", "11.1", "11.2", "11.3", "11.4", "11.5", "12.1"] },
    { "id": 9, "tasks": ["12.2", "12.3", "13.1", "13.2"] },
    { "id": 10, "tasks": ["14.1", "14.2", "14.3", "14.4"] }
  ]
}
```
