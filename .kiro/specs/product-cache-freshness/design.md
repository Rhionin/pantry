# Design Document

## Overview

The `products` table becomes a TTL'd cache of Open Food Facts data. Three new columns record provenance and freshness (`source`, `refreshed_at`, `name_overridden`). A new `product.Refresher` owns revalidation: it decides whether a row is eligible, dedupes concurrent work, calls Open Food Facts, merges the answer into the row, and stamps `refreshed_at`.

Two entry points drive it:

- **Background (stale-while-revalidate).** `LookupService.Lookup` resolves Tier 2 exactly as it does today and returns immediately, then hands the resolved product ID to `Refresher.ScheduleRefresh`. Scan latency is unchanged; the revalidation runs on its own goroutine with its own context.
- **Synchronous.** `POST /api/products/{id}/refresh` calls the same `Refresher.Refresh` inline and reports the outcome (`updated`, `unchanged`, `not_found_upstream`).

Everything stays in the single Go binary against the single SQLite file. No scheduler, no new process, no new network dependency (Requirement 8.1).

Language: Go. Code below is Go, matching existing package conventions.

## Architecture

```
HTTP: GET /api/products/lookup            HTTP: POST /api/products/{id}/refresh
            │                                            │
            ▼                                            ▼
   product.LookupService                        server.RefreshHandler
     Tier 1/2: Catalog.LookupByBarcode                   │
     Tier 3:   OpenFoodFacts.LookupBarcode               │
            │                                            │
            │ ScheduleRefresh(ctx, id)   ─── async ───┐   │ Refresh(ctx, id)  ─── sync ───┐
            │ (returns immediately)                  ▼   ▼                               ▼
            │                                   product.Refresher ──────────────► OpenFoodFacts
            ▼                                    gate → dedupe → merge → stamp          client
     LookupResult (unchanged shape)                     │
                                                        ▼
                                                 product.Catalog
                                                (products, barcodes)
```

`Refresher` is the only component that writes freshness-driven changes. `LookupService` gains one field and one call; its return shape is untouched (Requirement 8.6).

### Why this shape

- The freshness gate (is the row external? is it stale?) runs **synchronously** inside `ScheduleRefresh` using the request context, because it is a single primary-key read against local SQLite. That satisfies Requirement 2.4 literally (a fresh row schedules *nothing* — no goroutine, no `WaitGroup` entry) and keeps `LookupService` free of a clock and a TTL.
- `LookupService` does not need to know what "stale" means. It reports "I served this product from cache" and the `Refresher` decides. This keeps the TTL and the clock in one place (Requirement 8.2).

## Components and Interfaces

### `product.Refresher`

Named for its domain role, not its storage mechanism, per the AGENTS.md naming rule. Lives in a new file `internal/product/refresh.go`.

```go
// Source values for products.source.
const (
    SourceExternal = "external"
    SourceUser     = "user"
)

// RefreshOutcome classifies a completed revalidation attempt.
type RefreshOutcome string

const (
    OutcomeUpdated         RefreshOutcome = "updated"
    OutcomeUnchanged       RefreshOutcome = "unchanged"
    OutcomeNotFoundUpstream RefreshOutcome = "not_found_upstream"
)

// Refresher revalidates externally-sourced product rows against Open Food Facts.
type Refresher struct {
    Catalog interface {
        GetProductByID(ctx context.Context, id string) (*Product, error)
        SaveRefresh(ctx context.Context, p Product, refreshedAt time.Time) error
        MarkRefreshed(ctx context.Context, id string, refreshedAt time.Time) error
    }
    OpenFoodFacts interface {
        LookupBarcode(ctx context.Context, barcode string) (*ProductSummary, error)
    }

    // TTL is the age at which an external row becomes stale.
    TTL time.Duration

    // Now supplies the current time. Defaults to time.Now when nil.
    Now func() time.Time

    // ExternalLookupEnabled reports whether upstream requests are permitted.
    // False disables all revalidation, including refreshed_at stamping.
    ExternalLookupEnabled bool

    // BackgroundTimeout bounds a background revalidation. Defaults to 30s when zero.
    BackgroundTimeout time.Duration

    mu       sync.Mutex
    inFlight map[string]struct{}
    wg       sync.WaitGroup
}
```

Methods:

| Method | Purpose |
|---|---|
| `Refresh(ctx, productID) (RefreshOutcome, error)` | The single core path. Loads the row, calls upstream, merges, stamps `refreshed_at`. Both entry points call this. |
| `ScheduleRefresh(ctx, productID)` | Gates synchronously, then runs `Refresh` on a goroutine with a detached context. Never returns an error — background failures are logged. |
| `Wait()` | Blocks until every goroutine started by `ScheduleRefresh` has returned. |

`Refresh` ignores the TTL entirely (Requirement 4.2). Staleness is only consulted by `ScheduleRefresh`.

`Refresh` control flow:

1. `if !r.ExternalLookupEnabled` → return `OutcomeUnchanged, nil` with no read and no write (Requirement 6.4).
2. Load the row. Missing row → return `ErrProductNotFound`-wrapping sentinel `ErrRefreshTargetMissing` so the handler can map it to 404.
3. `if row.Source != SourceExternal` → return `OutcomeUnchanged, nil` without writing anything (Requirements 7.1, 4.8).
4. Call `OpenFoodFacts.LookupBarcode(ctx, row.ID)`.
   - `ErrProductNotFound` → `MarkRefreshed(row.ID, now)`, return `OutcomeNotFoundUpstream, nil` (Requirements 5.1, 5.2).
   - Any other error → `MarkRefreshed(row.ID, now)`, then return `OutcomeUnchanged, err` (Requirement 5.3). The stamp happens **before** returning the error so an upstream outage does not cause every subsequent lookup to retry immediately.
   - Success → `merged := mergeRefresh(*row, *upstream)`, `SaveRefresh(merged, now)`, return `classify(*row, merged), nil`.

**The barcode used for the upstream call is the product ID.** `persistExternalProduct` sets `Product.ID = barcode` for every externally-resolved product, and migration 002's placeholder rows have `id = product_id = barcode` as well, so every `source = 'external'` row's ID is its barcode by construction. This avoids a second query against `barcodes` and avoids ambiguity when a product carries several barcodes. The invariant is documented on `Refresh`.

`ScheduleRefresh` control flow (all steps 1–4 synchronous, on the caller's context):

1. `if !r.ExternalLookupEnabled` → return.
2. Load the row via `GetProductByID`. Nil row, or `row.Source != SourceExternal` → return (Requirement 2.6).
3. `if !r.isStale(row)` → return (Requirement 2.4).
4. Under `r.mu`: if `productID` is already in `inFlight`, return; otherwise insert it and `r.wg.Add(1)` (Requirement 2.5).
5. `go func()`: `defer` removing the ID from `inFlight` and `r.wg.Done()`; build `ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), r.backgroundTimeout())`; call `r.Refresh`; log failures.

```go
func (r *Refresher) isStale(p *Product) bool {
    if p.RefreshedAt == nil {
        return true // Requirement 1.5
    }
    return r.now().Sub(*p.RefreshedAt) > r.TTL
}

func (r *Refresher) now() time.Time {
    if r.Now != nil {
        return r.Now()
    }
    return time.Now()
}
```

No sleep, no timer, no wall-clock wait (Requirement 8.5).

### Single-flight mechanism: hand-rolled map + mutex

**`golang.org/x/sync` is not in `go.mod`.** I checked: `go.mod` lists `go-json-experiment/json`, `google/uuid`, `justinrixx/retryhttp`, `steinfletcher/apitest`, `apitest-jsonpath`, and `modernc.org/sqlite` as direct requires, with `golang.org/x/sys` as the only `golang.org/x/*` entry (indirect). Adopting `singleflight` means a new direct dependency.

Decision: hand-rolled `map[string]struct{}` guarded by a `sync.Mutex`.

Rationale:
- `singleflight.Group.Do` is built to *share a result* among concurrent callers. Here the callers do not want a result — `ScheduleRefresh` is fire-and-forget, and the second caller should drop its request, not block waiting on the first. `singleflight.DoChan` plus discarding the channel would work, but it is more machinery than the semantics need.
- The `Refresher` already needs a `sync.WaitGroup` for Requirement 8.3, so it already carries synchronization state and a mutex is not a new concept in the type.
- The implementation is roughly fifteen lines, fully covered by a concurrency property test, and adds no supply-chain surface to a single-binary hobby-scale service.

The map is keyed by product ID (which equals the barcode for external rows), so "one request per barcode in flight" is exactly what Requirement 2.5 asks for.

### Background goroutine lifecycle and test-awaitable completion

Requirement 8.3 asks for a mechanism a test can await. The design uses the `WaitGroup` the service already needs, exposed as `Refresher.Wait()`.

The critical ordering property: `wg.Add(1)` happens in step 4 of `ScheduleRefresh`, **synchronously, before `Lookup` returns**. So by the time an HTTP response reaches the test, the counter is already incremented, and `Wait()` cannot return early because the goroutine has not started yet. That is what makes API tests deterministic without a sleep or a poll loop.

`Wait()` is a legitimate production lifecycle API (drain-before-exit), not a test-only hook, so nothing test-shaped leaks into the production path. Alternatives considered and rejected: an injectable dispatcher (`func(func())`) that runs inline in tests would make tests deterministic but would mean the code path under test is not the code path that runs in production — the exact thing a concurrency bug would hide behind.

Context: the goroutine gets `context.WithTimeout(context.WithoutCancel(context.Background()), 30s)`. The request context is unusable — `net/http` cancels it when the handler returns, which is precisely when the revalidation starts. The timeout bounds the goroutine's lifetime so `Wait()` always terminates (Requirement 8.4); the OFF client's own 10s HTTP timeout sits comfortably inside it.

Shutdown: `cmd/server/main.go` currently calls `http.ListenAndServe` with no graceful-shutdown path, so there is no place to drain from and this design does not invent one. Goroutines are bounded by the per-refresh timeout and the process exits by signal, as it does today. `Wait()` is the hook a future graceful-shutdown change would call.

### Injectable clock

A `Now func() time.Time` field defaulting to `time.Now` when nil, not an interface.

Rationale: one method, one call site pattern, zero-value-usable, and it matches how the codebase already treats optional collaborators (`LookupService`'s inline interfaces, `shopping.NoOpExporter`). A `Clock` interface would need a `realClock` implementation and constructor plumbing for no gain.

Owners:
- `product.Refresher.Now` — used for `isStale` and for every `refreshed_at` stamp.
- `product.LookupService.Now` — used only by `persistExternalProduct` to stamp `refreshed_at` on a newly cached external product (Requirement 1.6).

Both default to `time.Now`, so existing construction sites compile and behave unchanged.

### `product.Catalog` changes

`Product` gains three fields:

```go
type Product struct {
    ID             string     `json:"id"`
    Name           string     `json:"name"`
    Category       string     `json:"category"`
    UnitOfMeasure  string     `json:"unitOfMeasure"`
    ImageURL       string     `json:"imageUrl,omitempty"`
    CreatedAt      time.Time  `json:"createdAt"`
    Source         string     `json:"source,omitempty"`
    RefreshedAt    *time.Time `json:"refreshedAt,omitempty"`
    NameOverridden bool       `json:"nameOverridden,omitempty"`
}
```

`RefreshedAt` is `*time.Time` because the column is nullable and NULL is semantically distinct from the zero time — it means "never checked", which drives Requirement 1.5. Reads scan into `sql.NullTime` and convert.

All three carry `omitempty` so that existing responses are byte-identical when the fields are unset. In particular `PUT /api/products/{id}` returns a `Product` the handler assembles in memory (it never re-reads the row), so without `omitempty` that response would sprout a misleading `"source": ""`.

New and changed methods:

| Method | Change |
|---|---|
| `CreateProduct` | INSERT now includes `source`, `refreshed_at`, `name_overridden`. `Source` empty → `SourceUser`; any value other than `external`/`user` → error (the CHECK constraint SQLite will not let us add). `RefreshedAt` nil → NULL. |
| `GetProductByID` | Column list gains `source`, `refreshed_at`, `COALESCE(name_overridden, 0)`. Required: the `Refresher` gates on all three. |
| `ListProducts` | Same three columns added, so the UI can show provenance and last-checked time. |
| `UpdateProduct` | Conditionally sets `name_overridden` (below). Signature unchanged. |
| `SaveRefresh(ctx, p Product, at time.Time) error` | **New.** Writes `name`, `category`, `unit_of_measure`, `image_url`, `refreshed_at = at`. Touches neither `id`, `source`, `created_at`, nor `name_overridden` (Requirement 3.7). |
| `MarkRefreshed(ctx, id string, at time.Time) error` | **New.** Writes only `refreshed_at = at`. Used by both failure paths, where no upstream field values exist to write. |
| `LookupByBarcode` | **Unchanged.** |

`LookupByBarcode` deliberately keeps its signature and its `*ProductSummary` return. It is the Tier-2 hot path whose result shape Requirement 8.6 freezes, and it is called directly by `internal/product/product_test.go` and `lookup_properties_test.go`. The `Refresher` re-reads the row by primary key anyway, so widening the barcode join would buy nothing.

`ProductSummary` is **not** extended. It is the wire type the scanner and queue components consume, freshness is a server-side concern on that path, and Requirement 8.6 requires its shape to be stable. The refresh endpoint returns the fuller `Product` instead, which is where a "last checked" display belongs.

### `product.LookupService` changes

```go
type LookupService struct {
    Catalog interface {
        LookupByBarcode(ctx context.Context, barcode, userID string) (*ProductSummary, error)
        GetProductByID(ctx context.Context, id string) (*Product, error)
        CreateProduct(ctx context.Context, product Product) error
        UpsertBarcodeMapping(ctx context.Context, barcode, productID, source, userID string) error
    }
    OpenFoodFacts interface {
        LookupBarcode(ctx context.Context, barcode string) (*ProductSummary, error)
    }

    // Refresher schedules background revalidation of stale cached rows.
    // Nil disables revalidation.
    Refresher interface {
        ScheduleRefresh(ctx context.Context, productID string)
    }

    // Now supplies the current time. Defaults to time.Now when nil.
    Now func() time.Time
}
```

The inline `Catalog` interface needs **no widening** — `GetProductByID` and `CreateProduct` are already listed and their signatures are unchanged. `*product.Catalog` continues to satisfy it structurally, so `cmd/server/main.go` and `internal/server/test_runner_test.go` need no change on that account.

Two edits to `Lookup`/`persistExternalProduct`:

1. In the Tier-2 branch, after resolving `product` and before returning, add
   `if s.Refresher != nil { s.Refresher.ScheduleRefresh(ctx, product.ID) }`.
   The returned `LookupResult` is built from the values read before the call, so the response is identical whether a revalidation was scheduled or not (Requirements 2.1, 2.2, 5.6).
2. In `persistExternalProduct`, the `CreateProduct` call sets `Source: SourceExternal` and `RefreshedAt: &now` where `now = s.now()` (Requirement 1.6). `NameOverridden` stays false by zero value.

The `Refresher` field is nil-tolerant so that any existing `LookupService` literal — notably the one `exchanges()` builds — keeps compiling and behaving as it does today.

### Merge semantics

```go
// mergeRefresh returns cached with upstream values applied. A field is taken
// from upstream only when upstream actually reported a value for it; an empty
// upstream field means "no opinion", not "clear this".
func mergeRefresh(cached Product, upstream ProductSummary) Product {
    merged := cached
    if upstream.Name != "" && !cached.NameOverridden {
        merged.Name = upstream.Name
    }
    if upstream.Category != "" {
        merged.Category = upstream.Category
    }
    if upstream.UnitOfMeasure != "" {
        merged.UnitOfMeasure = upstream.UnitOfMeasure
    }
    if upstream.ImageURL != "" {
        merged.ImageURL = upstream.ImageURL
    }
    return merged
}
```

| Field | Overwritten when |
|---|---|
| `name` | Upstream name is non-empty **and** `name_overridden` is false |
| `category` | Upstream category is non-empty |
| `unit_of_measure` | Upstream unit is non-empty |
| `image_url` | Upstream image URL is non-empty |
| `source`, `created_at`, `id`, `name_overridden` | Never |
| `refreshed_at` | Always, on every completed attempt including both failure paths |

**Why the non-empty guard.** Requirements 3.1 and 3.2 overwrite a field only where Open Food Facts returned a non-empty value, and retain the cached value otherwise. An unconditional overwrite would be unsafe: `OpenFoodFactsClient.LookupBarcode` hardcodes `UnitOfMeasure: ""` — Open Food Facts is never asked for a unit and never supplies one — so it would blank the unit on **every** product the first time it is refreshed. Requirement 5 states the governing intent plainly: a refresh must never degrade a cached product. So an empty upstream field means "no opinion", not "clear this". With today's client that makes `unit_of_measure` effectively immutable under refresh, which is the correct outcome; if the client later parses `quantity` or `product_quantity_unit`, the same rule starts propagating real units with no further change.

The same guard covers the "upstream record is thinner than what we cached" case for `category` and `image_url`, so a rotated-but-missing thumbnail never wipes a working one.

Note on `ProductSummary` ownership: the fake OFF client returns a pointer into its own store, so `mergeRefresh` takes `upstream` by value and never mutates it.

### How `name_overridden` gets set

`UpdateProduct` today writes all four fields with no idea which changed. Rather than read-then-compare (two statements, a race window, and an extra round trip on a single-writer SQLite connection), the flag is set inside the same UPDATE. SQLite evaluates expressions in a `SET` clause against the **pre-update** row, so the old name is available to compare against:

```sql
UPDATE products
SET name = ?,
    category = ?,
    unit_of_measure = ?,
    image_url = ?,
    name_overridden = CASE
        WHEN source = 'external' AND name <> ? THEN 1
        ELSE name_overridden
    END
WHERE id = ?
```

The new name is bound twice: once as the assignment, once for the comparison. Effects:

- External row, submitted name differs from stored name → flag becomes true (Requirement 3.4).
- External row, submitted name equals stored name → flag keeps its prior value, so an edit to category alone does not claim the name (Requirement 3.5).
- User row → flag is never touched; it is meaningless there because the `Refresher` never writes user rows.
- The flag is sticky. Once a user has named a product, renaming it back to the upstream string does not surrender ownership. That is the conservative reading of Requirement 3.3 and needs no extra state.

**Caller impact: none.** `UpdateProduct`'s only caller is `server.UpdateHandler`, and the signature does not change.

One pre-existing behavior worth recording because it interacts with refresh: `UpdateHandler` builds its `product.Product` from a request body that has no `imageUrl` field, so a PUT already blanks `image_url`. This design does not change that, and a subsequent refresh restores the thumbnail from upstream. Fixing the PUT is out of scope.

### Refresh endpoint

Registered alongside the other product routes in `internal/server/server.go`, in the same `HandleJSON` style:

```go
refreshHandler := &RefreshHandler{Refresher: refresher, Catalog: catalog}
mux.HandleFunc("POST /api/products/{id}/refresh", HandleJSON(refreshHandler.Handle))
```

`NewHandler` gains a `refresher *product.Refresher` parameter (constructed in `main.go`, where the OFF client and the TTL config live). New file `internal/server/handler_refresh.go`:

```go
type RefreshHandler struct {
    Refresher interface {
        Refresh(ctx context.Context, productID string) (product.RefreshOutcome, error)
    }
    Catalog interface {
        GetProductByID(ctx context.Context, id string) (*product.Product, error)
    }
}

type refreshProductPathParams struct {
    ID string `json:"id"`
}

type refreshProductResponse struct {
    Product product.Product       `json:"product"`
    Outcome product.RefreshOutcome `json:"outcome"`
}
```

Request: no body. Response on success:

```json
{
  "product": {
    "id": "011110728227",
    "name": "Whole Milk",
    "category": "Dairy",
    "unitOfMeasure": "gallon",
    "imageUrl": "https://images.openfoodfacts.org/…/front_en.4.200.jpg",
    "createdAt": "2025-01-04T18:22:10Z",
    "source": "external",
    "refreshedAt": "2025-03-11T09:00:00Z",
    "nameOverridden": false
  },
  "outcome": "updated"
}
```

Handler flow:

1. Empty `ID` → `BadRequest("product id is required")`, matching `UpdateHandler`. (The route pattern makes this unreachable in practice.)
2. `Refresher.Refresh(ctx, id)`.
   - `errors.Is(err, product.ErrRefreshTargetMissing)` → `NotFound("product not found")` → **404** (Requirement 4.7).
   - Any other error → `BadGateway("could not refresh product from Open Food Facts")` → **502** (Requirement 4.9). The row is already intact because `Refresh` writes no field values on the error path.
   - Otherwise → re-read via `GetProductByID` and return `refreshProductResponse` → **200** (Requirement 4.3).
3. A `source = 'user'` row returns 200 with `"outcome": "unchanged"` and its values untouched (Requirement 4.8), because `Refresh` short-circuits at step 3 of its control flow.

The re-read in step 2 is what makes Requirement 4.1's "responds after the revalidation completes" observable: the body carries post-merge values, including the new `refreshedAt`.

`handler_errors.go` gains one constructor, since 502 has no existing helper:

```go
func BadGateway(message string) error {
    return &HTTPError{Code: http.StatusBadGateway, Message: message}
}
```

### Outcome classification

```go
func classify(before, after Product) RefreshOutcome {
    if before.Name != after.Name ||
        before.Category != after.Category ||
        before.UnitOfMeasure != after.UnitOfMeasure ||
        before.ImageURL != after.ImageURL {
        return OutcomeUpdated
    }
    return OutcomeUnchanged
}
```

The comparison is on the four field values only, evaluated on the merge result before it is written — so `refreshed_at` moving does not by itself count as an update (Requirements 4.4, 4.5). `not_found_upstream` is set on the upstream-not-found branch and never reaches `classify` (Requirement 4.6). A transport failure never produces an outcome the client sees; it produces a 502.

### Configuration in `cmd/server/main.go`

```go
const defaultProductCacheTTL = 30 * 24 * time.Hour

func productCacheTTL() time.Duration {
    raw := os.Getenv("PRODUCT_CACHE_TTL")
    if raw == "" {
        return defaultProductCacheTTL
    }
    ttl, err := time.ParseDuration(raw)
    if err != nil {
        log.Printf("invalid PRODUCT_CACHE_TTL %q, using default of %s", raw, defaultProductCacheTTL)
        return defaultProductCacheTTL
    }
    return ttl
}
```

Unset or empty → 30 days (Requirement 6.1). Valid Go duration → used as given (Requirement 6.2). Unparseable → logged with the offending value and defaulted; startup continues (Requirement 6.3). A value that parses is used as-is even if non-positive: `0s` is a legitimate "always revalidate" debugging setting, and the single-flight guard keeps it from stampeding upstream.

Wiring:

```go
externalLookupEnabled := os.Getenv("DISABLE_EXTERNAL_PRODUCT_LOOKUP") != "true"

var externalProducts interface {
    LookupBarcode(context.Context, string) (*product.ProductSummary, error)
} = product.NewOpenFoodFactsClient()
if !externalLookupEnabled {
    externalProducts = disabledProductLookup{}
}

refresher := &product.Refresher{
    Catalog:               catalog,
    OpenFoodFacts:         externalProducts,
    TTL:                   productCacheTTL(),
    ExternalLookupEnabled: externalLookupEnabled,
}
lookupService := &product.LookupService{
    Catalog:       catalog,
    OpenFoodFacts: externalProducts,
    Refresher:     refresher,
}
handler := server.NewHandler(catalog, lookupService, refresher, sqlDB)
```

### Interaction with `DISABLE_EXTERNAL_PRODUCT_LOOKUP`

This needs an explicit resolution, because routing refresh through the existing stub produces a silent data-integrity bug.

`disabledProductLookup.LookupBarcode` returns `product.ErrProductNotFound`. If the `Refresher` treated that like any other upstream not-found, Requirement 5.2 would fire and stamp `refreshed_at = now` — marking every row it touched as fresh for a full TTL even though nothing was ever checked. Turning external lookup off for an afternoon would silently suppress real revalidation for the next 30 days, and Requirement 6.4's "leave every row's field values unchanged and make no Open Food Facts request" would be violated in spirit while appearing to be satisfied.

Resolution: the `Refresher` is told directly, via `ExternalLookupEnabled`, and never infers disabled-ness from an error value. When the flag is false:

- `ScheduleRefresh` returns immediately — no row read, no goroutine, no `WaitGroup` entry.
- `Refresh` returns `(OutcomeUnchanged, nil)` before reading the row, so `refreshed_at` is untouched and the endpoint answers 200 `unchanged`.

`refreshed_at` therefore only ever advances as the result of a genuine attempt to reach Open Food Facts (Requirements 5.2, 6.4). The stub stays where it is, serving the Tier-3 lookup path unchanged.

## Data Models

### Migration `004_add_product_freshness.sql`

```sql
-- Freshness and provenance for the products cache. SQLite cannot add a CHECK
-- constraint via ALTER TABLE, so the source value set {'external','user'} is
-- enforced in Go (product.Catalog.CreateProduct).
ALTER TABLE products ADD COLUMN source TEXT NOT NULL DEFAULT 'external';
ALTER TABLE products ADD COLUMN refreshed_at DATETIME;
ALTER TABLE products ADD COLUMN name_overridden INTEGER NOT NULL DEFAULT 0;

-- Every pre-existing row is treated as externally-sourced and never verified.
-- No override has ever been entered, so no classification heuristic is needed.
-- The placeholder rows migration 002 created as 'Product ' || id are left in
-- place deliberately: a real Open Food Facts fetch will overwrite them.
UPDATE products
SET source = 'external',
    refreshed_at = NULL,
    name_overridden = 0;
```

SQLite `ALTER TABLE ADD COLUMN` constraints observed: both `NOT NULL` columns carry constant defaults (`'external'`, `0`) as required, no CHECK is attached to an existing table, and no default uses a non-constant expression such as `CURRENT_TIMESTAMP`. The `Migration_Runner` applies files in lexicographic order after `003_add_product_image_url.sql` and records `004_add_product_freshness.sql` in `schema_migrations`, so a second run is a no-op (Requirement 1.4 holds under re-application).

The blanket `UPDATE` is redundant with the `source` default and with `refreshed_at`'s NULL-ability, but it states Requirement 1.4 explicitly in the file rather than leaving it as a consequence of a default, and it makes the migration correct regardless of the defaults chosen. New rows never rely on the column default: `CreateProduct` always binds `source` explicitly.

### Resulting `products` schema

| Column | Type | Notes |
|---|---|---|
| `id` | TEXT PK | Equals the barcode for external rows |
| `name` | TEXT NOT NULL | |
| `category` | TEXT | NULL when unknown |
| `unit_of_measure` | TEXT | NULL when unknown |
| `image_url` | TEXT | NULL when unknown |
| `created_at` | DATETIME NOT NULL | Never written by refresh |
| `source` | TEXT NOT NULL DEFAULT 'external' | `external` \| `user`, enforced in Go |
| `refreshed_at` | DATETIME NULL | NULL = never checked = stale |
| `name_overridden` | INTEGER NOT NULL DEFAULT 0 | Sticky once true |

## Error Handling

| Situation | `Refresher` behavior | Client-visible result |
|---|---|---|
| Product ID has no row | Return `ErrRefreshTargetMissing`, write nothing | 404, `{"error":"product not found"}` |
| Row is `source = 'user'` | Return `OutcomeUnchanged`, write nothing | 200, `"outcome":"unchanged"` |
| Upstream reports the barcode unknown | Retain the four fields, `MarkRefreshed` | 200, `"outcome":"not_found_upstream"` |
| Upstream transport/timeout/non-2xx | Retain the four fields, `MarkRefreshed`, return the error | 502, `{"error":"could not refresh product from Open Food Facts"}` |
| External lookup disabled | Return `OutcomeUnchanged` before any read or write | 200, `"outcome":"unchanged"` |
| Background revalidation fails | One `log.Printf` naming the barcode and the reason | Nothing — the lookup response already went out unchanged |
| Catalog write fails | Propagate the error | 502 from the endpoint; logged on the background path |

Background failures never surface to the scan path (Requirements 5.5, 5.6). A single log line per failure, formatted as `refresh %s failed: %v`, keeps an outage from flooding the log — the single-flight guard caps concurrent attempts at one per barcode.

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system — essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

### Property 1: Migration 004 backfills provenance without disturbing data

For any set of `products` rows present in a database migrated through `003`, after `RunMigrations` applies `004` every row has `source = 'external'`, `refreshed_at` NULL, and `name_overridden` false, while its `id`, `name`, `category`, `unit_of_measure`, `image_url`, and `created_at` values are unchanged; applying the migrations a second time changes nothing.

**Validates: Requirements 1.1, 1.2, 1.3, 1.4**

### Property 2: Writes record provenance

For any product persisted from an Open Food Facts resolution, the stored row has `source = 'external'`, `name_overridden` false, and `refreshed_at` equal to the Clock time at persist time; and for any product created through the product API, the stored row has `source = 'user'`.

**Validates: Requirements 1.6, 1.7**

### Property 3: Staleness is exactly "never checked, or older than the TTL"

For any external row and any Clock time, the row is treated as stale if and only if its `refreshed_at` is NULL or the elapsed time since `refreshed_at` exceeds the Cache_TTL; and for any lookup resolving to a fresh row or to a `source = 'user'` row, no Open Food Facts request is made and `refreshed_at` does not change.

**Validates: Requirements 1.5, 2.4, 2.6**

### Property 4: A lookup response is unaffected by revalidation

For any barcode resolving at Tier 1, at Tier 2 to a fresh row, at Tier 2 to a stale row, or to nothing at any tier, the returned `LookupResult` product fields equal the cached row values read at lookup time and the `Source` value is the same one returned before this feature existed — no matter whether a revalidation was scheduled, and no matter whether that revalidation later succeeded, found nothing upstream, or failed.

**Validates: Requirements 2.1, 2.2, 5.6, 8.6**

### Property 5: A stale lookup revalidates behind the response

For any Tier-2 lookup resolving to a stale external row, no Open Food Facts request has been made at the moment the response is returned, and after awaiting the Refresh_Service exactly one Open Food Facts request has been made for that barcode and the row's `refreshed_at` has advanced.

**Validates: Requirements 2.3, 3.6**

### Property 6: Concurrent revalidations of one barcode collapse to one upstream request

For any number N of concurrent stale lookups of a single barcode, exactly one Open Food Facts request is made for that barcode.

**Validates: Requirements 2.5**

### Property 7: A merge takes an upstream field only when upstream supplied one

For any cached external row and any Open Food Facts response, the merged row's `category`, `unit_of_measure`, and `image_url` each equal the upstream value when that upstream value is non-empty and equal the prior cached value otherwise; and the merged `name` equals the upstream name when the upstream name is non-empty and `name_overridden` is false, and equals the prior cached name otherwise — including when Open Food Facts reports the barcode unknown or the request fails, in which case all four retain their prior values.

**Validates: Requirements 3.1, 3.2, 3.3, 5.1, 5.3**

### Property 8: Every attempt stamps `refreshed_at`

For any refresh attempt on an external row that reaches Open Food Facts — whether upstream returns a product, reports the barcode unknown, or fails with a transport, timeout, or non-success error — the row's `refreshed_at` afterwards equals the Clock time of the attempt.

**Validates: Requirements 3.6, 5.2, 5.3**

### Property 9: A refresh preserves identity and mappings

For any refresh attempt and any outcome, the row's `id`, `source`, and `created_at` are unchanged, the row still exists, and every `barcodes` mapping for that product retains its `barcode`, `source`, and `user_id` values.

**Validates: Requirements 3.7, 5.4, 7.4**

### Property 10: A name edit on an external row claims the name

For any external row and any product-API update, `name_overridden` becomes true when the submitted name differs from the stored name and otherwise retains its prior value.

**Validates: Requirements 3.4, 3.5**

### Property 11: The endpoint refreshes regardless of freshness and reports the stored result

For any existing external row and any `refreshed_at` value inside or outside the Cache_TTL, `POST /api/products/{id}/refresh` makes an Open Food Facts request, responds 200, and returns a product body whose field values equal those a subsequent read of that row returns.

**Validates: Requirements 4.1, 4.2, 4.3**

### Property 12: The outcome classifies the field delta

For any refresh, the reported outcome is `updated` when at least one of `name`, `category`, `unit_of_measure`, `image_url` differs from its pre-merge value, `unchanged` when all four are equal to their pre-merge values, and `not_found_upstream` when Open Food Facts reported the barcode unknown.

**Validates: Requirements 4.4, 4.5, 4.6**

### Property 13: Refresh never writes a row it does not own

For any `source = 'user'` row, a refresh through either entry point leaves `name`, `category`, `unit_of_measure`, `image_url`, and `refreshed_at` unchanged, and the endpoint responds 200 with the outcome `unchanged`.

**Validates: Requirements 4.8, 7.1**

### Property 14: The endpoint's failure statuses are total

For any product id with no `products` row, the refresh endpoint responds 404 with a message stating the product was not found; and for any Open Food Facts failure that is not an unknown-barcode report, the endpoint responds 502 with a message stating the product could not be refreshed and leaves the row's four field values unchanged.

**Validates: Requirements 4.7, 4.9**

### Property 15: A user override outranks freshness

For any barcode carrying both a `user_override` mapping for a user and a `global` mapping, a lookup by that user returns the overriding product regardless of the global row's `refreshed_at`, and continues to return it after a revalidation of the shadowed global row completes.

**Validates: Requirements 7.2, 7.3**

### Property 16: TTL configuration is a total function

For any string value of `PRODUCT_CACHE_TTL`, the effective Cache_TTL is the parsed duration when the string is a valid Go duration and 30 days otherwise, including when the variable is unset or empty; startup never fails on this value.

**Validates: Requirements 6.1, 6.2, 6.3**

### Property 17: Disabling external lookup makes refresh inert

For any row, when external product lookup is disabled, a refresh through either entry point makes no Open Food Facts request and leaves every column of that row unchanged, `refreshed_at` included.

**Validates: Requirements 6.4**

Criteria with no property: 5.5 (a log side effect with no return value to quantify over), 8.1, 8.2, 8.3, 8.5 (architectural and testability constraints satisfied by construction), and 8.4 (a process-level guarantee checked once with a goroutine count rather than per input).

## Testing Strategy

Per AGENTS.md, API tests come first: `handlerTestCase` / `runHandlerTests` in `internal/server/`, `afterRequest: exchanges(...)` to verify through the HTTP contract, `app.RunMigrations(conn)` for schema. Unit tests only where the HTTP surface cannot reach the logic.

### Test infrastructure changes

**`fakeOpenFoodFacts` (`internal/server/open_food_facts_fake_test.go`)** currently supports only `Seed` and has no mutex. Refresh tests need three additions:

1. **Changing the answer between calls.** `Seed` already overwrites its map entry, and `LookupBarcode` reads at call time, so re-seeding a barcode between the cache fill and the refresh already changes what upstream reports. What it needs is a documented commitment to that behavior, since every merge test depends on the upstream answer differing from what was cached.
2. **`SeedError(barcode string, err error)`** — records an error to return for a barcode instead of a product, so tests can exercise the 502 path and Requirement 5.3. Distinct from an absent barcode, which yields `ErrProductNotFound`.
3. **`CallCount(barcode string) int`** — the observable for Properties 3, 5, 6, 11, and 17 ("no upstream request was made" / "exactly one was made"). Requires a `sync.Mutex` guarding `store`, `errs`, and `calls`, because background revalidation goroutines call `LookupBarcode` concurrently with the test.

A fourth addition for Property 6: **`SeedBlocking(barcode string, release <-chan struct{})`** or equivalent, so N concurrent `ScheduleRefresh` calls can be held inside `LookupBarcode` long enough to prove the in-flight guard rejected the duplicates rather than merely serializing them.

**`testEnv` (`internal/server/test_runner_test.go`)** gains two fields:

```go
type testEnv struct {
    T             *testing.T
    DB            *sql.DB
    ProductStore  *product.Catalog
    OpenFoodFacts *fakeOpenFoodFacts
    Refresher     *product.Refresher // await background revalidation, control TTL
    Clock         *fakeClock         // advance time without waiting
    Res           *http.Response
}
```

`fakeClock` is a `sync.Mutex`-guarded `time.Time` with `Now() time.Time` and `Advance(time.Duration)`, wired in as `Refresher.Now` and `LookupService.Now`. Requirement 8.5 falls out: tests move time, they never wait for it.

`setupTestWithDB` builds the `Refresher` and passes it to both `LookupService` and `NewHandler`.

**`exchanges()` must reuse `env.Refresher`, not construct a new one.** It currently rebuilds a `LookupService` and calls `NewHandler` from scratch. A second `Refresher` would have its own `WaitGroup` and in-flight map, so `env.Refresher.Wait()` would return without awaiting goroutines the exchange started — a silent flake. The fix is to thread `env.Refresher` through both.

A `setupExternalProduct(id, name, category string, refreshedAt *time.Time)` helper joins the existing `setupProduct*` family. The existing helpers keep creating `source = 'user'` rows (`Source` empty defaults to user), which is correct for them and means no existing test starts revalidating.

### API tests (`internal/server/handler_refresh_test.go` and additions to the lookup tests)

- Refresh an external row whose upstream answer changed → 200, `"outcome":"updated"`, body carries the new values; `afterRequest: exchanges(GET /api/products)` confirms the row was persisted, not just echoed.
- Refresh an external row whose upstream answer is identical → 200, `"outcome":"unchanged"`.
- Refresh a fresh external row → still calls upstream (Property 11).
- Refresh where the barcode is absent from the fake → 200, `"outcome":"not_found_upstream"`; `exchanges(GET /api/products)` confirms the cached name and image survived.
- Refresh where `SeedError` returns a transport error → 502; `exchanges(GET /api/products)` confirms the four values are untouched.
- Refresh an unknown id → 404.
- Refresh a `source = 'user'` row → 200, `"outcome":"unchanged"`, values untouched.
- Refresh with the `Refresher` disabled → 200, `"outcome":"unchanged"`, `CallCount` zero.
- Stale Tier-2 lookup: `GET /api/products/lookup` returns the **cached** name, `CallCount` is 0 at that point, then `Refresher.Wait()` and `exchanges(GET /api/products)` shows the merged name.
- Fresh Tier-2 lookup: `Clock` set inside the TTL → response as before, `Wait()`, `CallCount` still 0.
- Name-override flow: PUT a new name on an external row → refresh → the user's name survives while category and image update.
- PUT a category-only change (same name) → refresh → the name **does** update, proving Requirement 3.5.
- User-override precedence: seed a `user_override` and a stale `global` mapping for one barcode, lookup, refresh the global row, lookup again → the override product both times (Property 15).

`refreshed_at` precision is the one place a direct `env.DB` query in `afterRequest` is warranted — asserting an exact stamp against the fake clock is not expressible through the API, since `refreshedAt` is serialized at second-or-better granularity but the comparison is what Property 8 needs. Everything else goes through `exchanges()`.

### Unit tests (only where the API cannot reach)

| Target | File | Why not an API test |
|---|---|---|
| `mergeRefresh` per-field table + the empty-upstream guard | `internal/product/refresh_test.go` | A pure function; enumerating 16 field-presence combinations through HTTP would be slow and obscure. Property 7. |
| `classify` | `internal/product/refresh_test.go` | Pure function over a field tuple. Property 12. |
| Single-flight under N concurrent `ScheduleRefresh` | `internal/product/refresh_test.go` | Needs a blocking upstream and goroutine control that the HTTP layer cannot express. Property 6. |
| `isStale` across NULL / inside / outside / exactly-at TTL | `internal/product/refresh_test.go` | Pure predicate; boundary at exactly TTL is only reachable directly. Property 3. |
| `productCacheTTL` | `cmd/server/main_test.go` | Reads the environment before any server exists. Property 16. |
| Migration 004 backfill and idempotence | `internal/app/migrate_test.go` | Schema-level; needs a database migrated only through 003. Property 1. |
| `CreateProduct` source validation | `internal/product/product_test.go` | The API never submits an invalid source; this guards the constraint SQLite cannot hold. |
| No leaked goroutine after `Wait()` | `internal/product/refresh_test.go` | Process-level. Requirement 8.4. |

Property tests use `pgregory.net/rapid`, already present in `go.mod` and already used in `internal/inventory/aggregate_properties_test.go` and `internal/product/lookup_properties_test.go`. Minimum 100 iterations each, tagged `Feature: product-cache-freshness, Property N: <property text>`.

Run `go test -race ./...` for the concurrency properties.

### Coverage checkpoint

After the implementation tasks and again after the test tasks, run `./scripts/test-coverage.sh` and commit the script if it raises its own threshold.

## Preserved Behavior

These must not regress. The existing test files that encode them should keep passing **unmodified** — that is the regression witness, and any edit to them is a signal that the design drifted.

1. **Tier 1 override resolution.** `LookupByBarcode`'s `CASE`-based precedence is untouched. A `user_override` row for the requesting user still wins over a `global` row for the same barcode, and refresh writes only the `products` table — never `barcodes` (Requirements 7.2, 7.3, 7.4).
2. **Tier 2 fresh hits.** A cached hit returns the same `ProductSummary` fields and `Source: "global"` as today, with no upstream request and no added latency beyond one primary-key read for the freshness gate (Requirements 2.1, 8.6).
3. **Not-found at all tiers.** An unknown barcode still returns a zero-valued `LookupResult` with `IsFound() == false` and an empty `Source`, and upstream transport errors are still swallowed into not-found for the flagged-entry workflow. `Lookup`'s error handling is unchanged (Requirement 8.6).
4. **`LookupResult.Source` contract.** The three values `user_override`, `global`, `external` and the empty string keep their current meanings, and the Tier-2 branch keeps returning the conservative `"global"`. Freshness is never exposed through `Source`.
5. **`ProductSummary` wire shape.** Unchanged, so `frontend/src/api/client.ts` and every scanner and queue component need no edits.
6. **Existing `Product` responses.** `GET /api/products` and `PUT /api/products/{id}` gain three `omitempty` fields that are absent whenever unset, so current JSONPath assertions and the frontend's parsing are unaffected.
7. **Existing `CreateProduct` callers.** Every current caller omits `Source`, which defaults to `user` — so no pre-existing product, seeded in a test or created through the API, becomes a revalidation target by accident.
