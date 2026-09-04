# Product Thumbnail Persistence Bugfix Design

## Overview

Product thumbnails never rendered because two independent defects combined to keep `image_url`
empty for every product. **Concern A**: `RunMigrations` (`internal/app/migrate.go`) only applies
migrations once, at process startup, and gives no observable signal of *which* migrations it
applied. A long-running dev server process started before migration `003_add_product_image_url.sql`
existed in the embedded set kept serving requests against a `products` table with no `image_url`
column, with nothing in the logs to reveal that the running schema was stale. **Concern B**: the
three-tier lookup (`internal/product/lookup.go`) never re-queries Open Food Facts for a barcode that
already resolves at Tier 1 or Tier 2, so a `products` row created before the image feature existed
(or while migration 003 was unapplied) keeps a permanently `NULL` `image_url` — restarting the server
fixes new resolutions but does nothing for rows already persisted.

Both concerns are now resolved, but with very different weight:

- **Concern B's actual code fix was already implemented and tested in a prior session.**
  `persistExternalProduct` in `internal/product/lookup.go` now copies `ImageURL` when persisting a
  newly-resolved Tier-3 product, and the join queries in `internal/inventory/inventory.go` and
  `internal/scan/scan.go` now select `image_url`. `TestExternalLookupPersistsImageURL`,
  `TestGetOrCreateItem_ProductImageURL`, and `TestGetScanEntry_ProductImageURL` cover this. The only
  remaining work for Concern B is **operational, not code**: since the current `pantry.db` holds
  only disposable test data (confirmed by the user — nothing in it needs to be preserved), the fix is
  to delete that file so every barcode re-resolves through Tier 3 on next scan and picks up
  `image_url` naturally through the already-fixed persistence path. No backfill mechanism (lazy
  refresh, admin endpoint, startup re-query task) is being built now. **This approach is valid only
  because the data is confirmed disposable.** If or when real, non-disposable production data exists,
  deleting the database is no longer an option and a backfill mechanism (option (b) or (c) from
  `bugfix.md`'s Open Questions) would need to be designed at that time. That is out of scope here and
  is recorded as a documented future consideration, not built.
- **Concern A gets a small, real code change**: `RunMigrations` will log which migration files it
  applies during a given call, so a future "the fix isn't taking effect" symptom is diagnosable from
  server startup logs alone, without shelling into the DB to inspect `schema_migrations`.

## Glossary

- **Bug_Condition_A**: A running server process whose applied-migrations set is missing a migration
  that exists in the current embedded migration set (`isBugConditionA` in `bugfix.md`). Concern A's
  fix does not make drift impossible (only a restart applies new migrations, as `RunMigrations`
  already worked) — it makes drift **observable** the moment `RunMigrations` next runs.
- **Bug_Condition_B**: A persisted `products` row with `image_url = NULL` whose barcode has a real
  image available from Open Food Facts, and that will never be re-queried because it already
  resolves at Tier 1 or Tier 2 (`isBugConditionB` in `bugfix.md`).
- **RunMigrations**: `internal/app/migrate.go`. Applies embedded `.sql` migration files in
  lexicographic order, skipping files already recorded in `schema_migrations`. Called from
  `cmd/server/main.go` and from ~9 test files' DB-setup helpers (`newTestPantry`, `newTestCatalog`,
  `newTestQueue`, `newTestConsumptionLog`, and the `internal/server` apitest setup).
- **applied migration (this call)**: A migration file for which `RunMigrations` executed the SQL and
  inserted the `schema_migrations` row *during the current invocation* — as opposed to one already
  recorded from a prior invocation, which is skipped and not logged.
- **disposable dev DB**: The `pantry.db` file used in local development, confirmed by the user to
  contain only test data with no retention requirement.
- **LookupService / persistExternalProduct**: The Tier-3 persistence path in
  `internal/product/lookup.go`, already fixed and tested prior to this design (see Overview).

## Bug Details

### Bug Condition

**Concern A** (from `bugfix.md`):
```
FUNCTION isBugConditionA(process)
  INPUT: process of type ServerProcess
  OUTPUT: boolean

  RETURN EXISTS m IN EmbeddedMigrations() WHERE m NOT IN process.AppliedMigrations()
END FUNCTION
```

**Concern B** (from `bugfix.md`):
```
FUNCTION isBugConditionB(row)
  INPUT: row of type ProductsRow
  OUTPUT: boolean

  RETURN row.ImageURL = NULL AND OpenFoodFacts.HasImage(row.PrimaryBarcode) = true
END FUNCTION
```

### Examples

- **Sour cream counterexample (Concern A, historical)**: The user's dev server process was started
  before migration `003_add_product_image_url.sql` existed. `schema_migrations` contained only
  `001_initial_schema.sql` and `002_backfill_orphaned_products.sql`; `.schema products` showed no
  `image_url` column. Nothing in the running app's logs indicated the schema was stale — the operator
  had to inspect the SQLite file directly to discover this. After this design's fix, the next time
  `RunMigrations` runs against that same DB, `log.Printf("applied migration: %s", "003_add_product_image_url.sql")`
  is emitted, making the drift visible in startup logs without a DB inspection.
- **Sour cream counterexample (Concern B, historical)**: Barcode `073420000158` ("Light Sour Cream")
  was already committed to inventory before `image_url` existed as a column/feature. It resolves at
  Tier 2 on every subsequent scan, so it never gets a chance to pick up an image via Tier 3. Because
  the DB holding this row is disposable test data, the resolution is to delete the DB file rather
  than build a backfill for a row that isn't worth preserving.
- **Already-up-to-date DB (Concern A, unaffected case)**: `RunMigrations` runs against a DB where
  every embedded migration is already recorded. Zero files are applied. Expected: a single summary
  log line confirming this, no per-file log lines, and — per Req 3.4 — zero schema changes.
- **Fresh external resolution (Concern B, already fixed)**: A barcode never seen before resolves at
  Tier 3; `persistExternalProduct` copies `ImageURL` into the persisted row. Already covered by
  `TestExternalLookupPersistsImageURL`.

## Expected Behavior

### Preservation Requirements

**Unchanged Behaviors:**
- Tier 1 (user override) resolutions continue to return the override product without querying Open
  Food Facts (Req 3.1).
- Tier 2 (global) resolutions whose `image_url` is already populated continue to return that
  `image_url` unchanged (Req 3.2).
- Tier 3 (never-persisted-before) resolutions continue to persist the resolved product, including
  `image_url`, exactly as already implemented and tested (Req 3.3).
- `RunMigrations` against an already-fully-migrated DB continues to apply zero migrations and make
  zero schema changes (Req 3.4) — the new logging is purely observational and does not change this.
- A product with no `image_url` (empty or NULL) continues to render the initials-only Avatar
  fallback, without a broken-image icon (Req 3.5).

**Scope:**
The logging change to `RunMigrations` MUST NOT alter its return value, error behavior, or the set of
migrations it applies — it only adds log output describing what it already does. The operational
Concern B fix (deleting `pantry.db`) touches no code path and cannot regress Tier 1/Tier 2/Tier 3
behavior; those are exercised identically against a fresh, empty DB as against any other DB.

## Hypothesized Root Cause

1. **Concern A root cause — silent migration application**: `RunMigrations` has always applied
   pending migrations correctly; the defect is purely a lack of observability. Nothing in
   `RunMigrations` or `cmd/server/main.go` reports *which* migrations ran on a given startup — only
   `log.Println("migrations applied")` in `main.go`, which is uninformative when zero, one, or many
   files were actually applied. This is why the schema drift was only discoverable by opening the
   SQLite file directly.
2. **Concern B root cause — no re-resolution path (already fixed)**: `LookupService.Lookup`'s
   Tier 1/Tier 2 branches correctly avoid an external call for already-resolvable barcodes, but this
   means a `products` row's `image_url` is fixed forever at whatever it was when first persisted. The
   application-level half of this (Tier 3 dropping `ImageURL` on persist) was the actual code bug and
   is already fixed. The remaining symptom — old rows stuck at `NULL` — is a data problem, not a code
   problem, and the disposable nature of the current DB makes a delete-and-reresolve strictly
   simpler and just as correct as a backfill mechanism for this dataset.
3. **Rejected alternative for Concern A — `/version` or `/health` endpoint reporting applied
   migrations**: `bugfix.md`'s Open Question 2.2 floated this. Rejected because it requires a new
   HTTP surface, new handler, and routing wiring for a diagnostic that a log line already satisfies;
   it also only helps if someone remembers to call it, whereas a startup log line is unconditionally
   present in every `main.go` run.
4. **Rejected alternative for Concern A — cross-process schema-drift detection**: comparing a running
   process's schema against the current source tree from outside the process (e.g. a CI check or a
   separate drift-monitoring job). Rejected as disproportionate to the actual failure mode: the
   drift only happens because a long-running process was never restarted after a migration was added;
   a restart (which already re-runs `RunMigrations` and, with this fix, logs what it applied) is the
   correct and sufficient remedy. `bugfix.md` itself flags full drift-detection as likely out of
   scope.
5. **Rejected alternative for Concern B — building a backfill mechanism now**: options (a) startup
   re-query task, (b) lazy on-read refresh, and (c) an admin re-sync endpoint were all considered in
   `bugfix.md`'s Open Questions. Rejected for now because the only data affected is confirmed
   disposable; building any of these adds a new pattern (live network calls at startup, background
   refresh triggers, or a new authenticated endpoint) to solve a problem that a one-line `rm pantry.db`
   already solves for the current dataset. Documented as a future consideration below, not built.

## Correctness Properties

Property 1: Migration Application Is Logged (Concern A)

_For any_ call to `RunMigrations`, and for any migration file that call applies (i.e. executes and
records because it was not already in `schema_migrations`), the call SHALL emit a log line naming
that file; and if the call applies zero files, it SHALL emit exactly one summary log line stating the
schema is already up to date. In both cases, the set of applied migrations and all other behavior of
`RunMigrations` SHALL be unchanged from before this fix.

**Validates: Requirements 2.1, 2.2**

Property 2: Bug Condition B No Longer Arises After The Operational Reset

_For any_ `products` row created after the disposable `pantry.db` file is deleted and the server is
restarted, `isBugConditionB` (as defined in `bugfix.md`) no longer applies to that row: if the row was
persisted via Tier 3, its `image_url` is populated from Open Food Facts by the already-fixed
`persistExternalProduct` (or is legitimately absent if Open Food Facts has no image for that
barcode, which is not a bug condition). No new code is required for this property to hold — it holds
because the previously-fixed persistence path is exercised for every barcode once the DB no longer
contains a stale, pre-fix Tier-2-resolvable row.

**Validates: Requirements 2.3, 2.4**

Property 3: Preservation — Migrations, Tiers, and Fallback Rendering Unchanged

_For all_ resolutions at Tier 1 or Tier 2 with an already-populated `image_url`, the fixed code SHALL
return the same result as before (Req 3.1, 3.2). _For all_ Tier-3 resolutions of a barcode never
persisted before, the already-implemented persistence behavior SHALL be unchanged (Req 3.3). _For
any_ `RunMigrations` call against a DB where every embedded migration is already applied, zero
migrations SHALL be applied and zero schema changes SHALL occur (Req 3.4). _For any_ product whose
`image_url` is empty or NULL, the frontend Avatar SHALL continue to render the initials-only
placeholder (Req 3.5).

**Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5**

## Fix Implementation

### Concern A: log applied migrations in `RunMigrations`

**File**: `internal/app/migrate.go`
**Function**: `RunMigrations`

Add the standard `log` package as a direct import (see "Why `log` directly" below) and log one line
per migration file actually applied, plus a single summary line when none were applied:

```go
package app

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"
)

// ... RunMigrations signature and schema_migrations bootstrap unchanged ...

	applied := 0
	for _, name := range files {
		// Check if already applied.
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE filename = ?`, name).Scan(&count); err != nil {
			return fmt.Errorf("check migration %q: %w", name, err)
		}
		if count > 0 {
			continue
		}

		// Read the embedded SQL file.
		sqlBytes, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read embedded migration %q: %w", name, err)
		}

		if _, err := db.Exec(string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %q: %w", name, err)
		}

		// Record it as applied.
		if _, err := db.Exec(`INSERT INTO schema_migrations (filename) VALUES (?)`, name); err != nil {
			return fmt.Errorf("record migration %q: %w", name, err)
		}

		log.Printf("applied migration: %s", name)
		applied++
	}

	if applied == 0 {
		log.Println("schema up to date, no migrations applied")
	}

	return nil
}
```

**Exact log line formats** (these are the literal strings, not paraphrased):
- Per applied file: `applied migration: <filename>` — e.g. `applied migration: 003_add_product_image_url.sql`
- Zero-applied summary: `schema up to date, no migrations applied`

No other line is added. The existing `log.Println("migrations applied")` in `cmd/server/main.go`
(logged unconditionally after `RunMigrations` returns successfully) is left as-is — it confirms the
call succeeded; the new per-file lines say what it actually did.

**Signature is unchanged.** `RunMigrations(db *sql.DB) error` keeps its exact signature so that
`cmd/server/main.go` and every test helper that calls it (`newTestPantry` in
`internal/inventory/inventory_test.go`, `newTestCatalog` in `internal/product/product_test.go`,
`newTestQueue` in `internal/scan/scan_test.go`, `newTestConsumptionLog` in
`internal/suggestion/suggestion_test.go`, the `internal/shopping` equivalent, the `internal/server`
apitest setup, and `internal/app/migrate_test.go` itself) need zero changes.

**Why `log` directly, not an injected logger**: `internal/app` has no existing logging
abstraction, and a grep of `internal/**/*.go` for `"log"` found zero existing imports of the standard
`log` package anywhere under `internal/` — so there is no established injected-logger convention to
follow either way. `cmd/server/main.go` already imports and calls `log.Fatalf`/`log.Println`/
`log.Printf` directly for exactly this kind of operational message. Given the explicit constraint of
not changing `RunMigrations`'s signature (which rules out passing in a `*log.Logger` or an interface
parameter without touching every call site), calling `log` directly inside `internal/app` is the
minimal-diff option consistent with the one direct precedent this codebase has (`main.go`). This is
recorded here as a deliberate choice, not a default.

**Test-run log noise is expected and accepted.** Most of the ~9 test helpers call `RunMigrations`
against a fresh `:memory:` DB, so a typical test run will log every migration file every time (test
databases start with an empty `schema_migrations` table). This is harmless: Go's `testing` package
only surfaces `log` package output (which writes to `os.Stderr` by default, not `t.Log`) when a test
fails or `-v` is passed, and even then it is just informational stderr noise, not a test failure. The
logging is intentionally **unconditional** — it does not check "is this the real server" — per the
design goal of keeping the change simple; gating it behind an environment flag or a "production mode"
check would add complexity this bugfix does not need.

### Concern B: operational reset (no code change)

Concern B requires no source change. The already-completed application-level fix
(`persistExternalProduct` copying `ImageURL`, and the inventory/scan joins selecting `image_url`) is
exercised by simply ensuring every barcode re-resolves through Tier 3 instead of hitting a stale
Tier-2 row. Steps:

1. Stop the running dev server process.
2. Delete the disposable database file: `rm pantry.db` (run from the repository root, where
   `cmd/server/main.go`'s default `DB_PATH` of `pantry.db` resolves).
3. Restart the server: `go run ./cmd/server`. On startup, `RunMigrations` runs against the now-empty
   file and (per the Concern A fix above) logs `applied migration: 001_initial_schema.sql`,
   `applied migration: 002_backfill_orphaned_products.sql`, and
   `applied migration: 003_add_product_image_url.sql` — confirming the fresh schema includes
   `image_url` from the first startup.
4. Re-scan the sour cream barcode (`073420000158`) — or any barcode — through the normal scan flow
   so it resolves at Tier 3 and gets persisted with its `image_url` via the already-fixed
   `persistExternalProduct`.
5. Verify, the same way verification was already done earlier in this session: query the fresh DB
   directly, e.g. `sqlite3 pantry.db "SELECT id, image_url FROM products WHERE id = '073420000158';"`
   and confirm `image_url` is populated; and/or `curl localhost:8080/api/inventory` (or the relevant
   product-lookup endpoint) and confirm the returned `imageUrl` field is non-empty. Then confirm in
   the UI that Inventory/Scan Queue cards render the real thumbnail instead of the initials
   placeholder.

**Documented future consideration (not built now):** if or when real, non-disposable data exists in
this database, deleting the file to force re-resolution is no longer acceptable, and a backfill
mechanism will need to be designed — most likely option (b) (lazy on-read refresh: when a product is
read and found to be missing an `image_url`, trigger a best-effort background Open Food Facts
refresh) or option (c) (a manual/admin-triggered re-sync endpoint), per the choices enumerated in
`bugfix.md`'s Open Questions. This is explicitly out of scope for this bugfix.

## Testing Strategy

### Concern A: unit test for migration logging

**File**: `internal/app/migrate_test.go` (existing file; follow its existing conventions — plain
`*testing.T`, `sql.Open("sqlite", ":memory:")`, table-free discrete test functions per the two
existing tests in this file).

Go's `log` package writes to a package-level `*log.Logger` backed by `os.Stderr` by default;
`log.SetOutput` redirects it for the duration of a test. Add two new test functions:

1. **`TestRunMigrations_LogsAppliedMigrationsOnFreshDB`**: open a fresh `:memory:` DB, redirect
   `log` output to a `bytes.Buffer` via `log.SetOutput(&buf)` (restoring the previous output with
   `defer log.SetOutput(previous)` — the default is `os.Stderr`, retrievable by capturing it before
   redirecting, since `log` does not expose a getter), call `RunMigrations`, then assert the captured
   output contains one `applied migration: <filename>` line for each of the three current migration
   files (`001_initial_schema.sql`, `002_backfill_orphaned_products.sql`,
   `003_add_product_image_url.sql`) and does NOT contain the `schema up to date` summary line.
2. **`TestRunMigrations_LogsSummaryWhenAlreadyMigrated`**: open a fresh `:memory:` DB, call
   `RunMigrations` once (unredirected, to set up the schema), then redirect `log` output to a new
   buffer, call `RunMigrations` a second time, and assert the second call's captured output is
   exactly the summary line `schema up to date, no migrations applied` with no `applied migration:`
   lines.

Both tests assert on `strings.Contains`/`strings.Count` against the buffer contents rather than exact
byte-for-byte output, so line ordering or an incidental trailing newline from `log`'s default flags
does not make the test brittle. These are unit tests (not property-based tests) because the input
space here is a fixed, small set of embedded migration files, not an open-ended domain — there is no
meaningful "for all N migrations" generator; the two fixed scenarios (fresh DB, already-migrated DB)
are the entire behavior space Property 1 needs to cover.

### Concern B: no new test needed

The application-level fix for Concern B was already implemented and is already covered end-to-end by
existing regression tests:

- `TestExternalLookupPersistsImageURL` (`internal/product/lookup_properties_test.go`) — asserts
  `persistExternalProduct` copies `ImageURL` into the persisted `Product` row on Tier-3 resolution.
- `TestGetOrCreateItem_ProductImageURL` (`internal/inventory/inventory_test.go`) — asserts
  `Item.Product.ImageURL` is projected through the items-products join.
- `TestGetScanEntry_ProductImageURL` (`internal/scan/scan_test.go`) — asserts
  `ScanEntry.Product.ImageURL` is projected through the scan_entries-products join.

Property 2 above is a statement that the bug condition itself stops arising once the operational
reset (delete `pantry.db`, restart, re-scan) is performed, given code these three tests already lock
in. No new automated test is proposed for Concern B; the operational verification steps in Fix
Implementation (step 5) are the manual confirmation for this specific bugfix instance.

After implementation, run `./scripts/test-coverage.sh` to enforce (and lock in) coverage thresholds
per AGENTS.md.
