# Implementation Plan

## Overview

This plan implements the two-concern fix from `design.md`. **Concern A** gets a small, real
code change: `RunMigrations` (`internal/app/migrate.go`) will log which migration files it
applies during a given call, with exact log line formats and unit test coverage using
`log.SetOutput` redirection, following the existing conventions in `internal/app/migrate_test.go`.
**Concern B**'s application-level fix was already implemented and tested in a prior session
(`persistExternalProduct` copying `ImageURL`, and the inventory/scan joins selecting `image_url`);
the only remaining work is operational — delete the disposable `pantry.db`, restart the server, and
re-scan a barcode so the already-fixed persistence path runs against a fresh DB. Per `design.md`,
no backfill mechanism (lazy refresh, admin endpoint, startup re-query task) is being built now — that
is explicitly out of scope and recorded only as a documented future consideration.

## Tasks

- [x] 1. Log applied migrations in `RunMigrations` (Concern A)
  - [x] 1.1 Add the `log` import and per-file/summary log lines to `internal/app/migrate.go`
    - Add `"log"` to the existing import block
    - Inside the per-file apply loop, after the migration is recorded as applied, log
      `applied migration: <filename>` (e.g. `applied migration: 003_add_product_image_url.sql`)
    - After the loop, if zero files were applied, log exactly one summary line:
      `schema up to date, no migrations applied`
    - Do NOT change the `RunMigrations(db *sql.DB) error` signature, return value, or error
      behavior — this is a pure logging addition
    - _Requirements: 2.1, 2.2, 3.4_

- [x] 2. Add unit test coverage for migration logging (Concern A)
  - [x] 2.1 Add `TestRunMigrations_LogsAppliedMigrationsOnFreshDB` to `internal/app/migrate_test.go`
    - Open a fresh `:memory:` DB, redirect `log` output to a `bytes.Buffer` via `log.SetOutput`,
      restoring the previous output with `defer log.SetOutput(previous)`
    - Call `RunMigrations` and assert the captured output contains one
      `applied migration: <filename>` line for each of the three current migration files
      (`001_initial_schema.sql`, `002_backfill_orphaned_products.sql`,
      `003_add_product_image_url.sql`) and does NOT contain the `schema up to date` summary line
    - _Requirements: 2.1_

  - [x] 2.2 Add `TestRunMigrations_LogsSummaryWhenAlreadyMigrated` to `internal/app/migrate_test.go`
    - Open a fresh `:memory:` DB, call `RunMigrations` once unredirected to set up the schema
    - Redirect `log` output to a new buffer, call `RunMigrations` a second time, and assert the
      captured output is exactly the summary line `schema up to date, no migrations applied` with
      no `applied migration:` lines
    - _Requirements: 2.2, 3.4_

- [x] 3. Checkpoint - run the full test suite and enforce coverage
  - Run the full Go test suite and confirm the two new tests from Task 2 pass alongside the
    existing `internal/app/migrate_test.go` tests (`TestMigrationApplies`,
    `TestMigrationIsIdempotent`, `TestMigration002BackfillsOrphanedProducts`)
  - Run `./scripts/test-coverage.sh` to enforce (and lock in) coverage thresholds; commit the
    updated script if the threshold increased, per AGENTS.md
  - Ask the user if any questions or ambiguities arise

- [x] 4. Perform the Concern B operational reset and confirm the fix
  - **This task is manual/operational, not automated code or a test.** It requires stopping and
    restarting the actual running server process and physically confirming the result — it is not
    satisfied by writing or running any automated test, and no automated test should be added for it
    (per `design.md`'s Testing Strategy, no new test is proposed for Concern B; the
    already-existing `TestExternalLookupPersistsImageURL`, `TestGetOrCreateItem_ProductImageURL`,
    and `TestGetScanEntry_ProductImageURL` already cover the application-level fix).
  - [x] 4.1 Stop the server, delete the disposable DB, and restart
    - Stop the running dev server process
    - Delete the disposable database file: `rm pantry.db` (from the repository root)
    - Restart the server: `go run ./cmd/server`, and confirm the startup log (per Task 1's fix)
      shows `applied migration: 001_initial_schema.sql`,
      `applied migration: 002_backfill_orphaned_products.sql`, and
      `applied migration: 003_add_product_image_url.sql`
    - Re-scan the sour cream barcode (`073420000158`) — or any barcode — through the normal scan
      flow so it resolves at Tier 3 and gets persisted with its `image_url` via the already-fixed
      `persistExternalProduct`
    - _Requirements: 2.3, 2.4_

  - [x] 4.2 Manually confirm `image_url` is populated and the UI renders the real thumbnail
    - Query the fresh DB directly:
      `sqlite3 pantry.db "SELECT id, image_url FROM products WHERE id = '073420000158';"` and
      confirm `image_url` is populated
    - And/or `curl localhost:8080/api/inventory` (or the relevant product-lookup endpoint) and
      confirm the returned `imageUrl` field is non-empty
    - Confirm in the UI that the Inventory and Scan Queue cards render the real thumbnail image
      instead of the initials-only placeholder Avatar
    - _Requirements: 2.4, 2.5_

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP — this plan has none,
  because the design has no Correctness Properties section requiring property-based tests, and the
  Concern A unit tests (Task 2) are core coverage for a real code change, not optional extras.
- Task 4 is intentionally excluded from the "coding agent" implementation loop for Tasks 1-3: it is
  an operational verification step against a running server, not code to write or an automated test
  to run, and is only valid because the current `pantry.db` is confirmed disposable test data.
- No backfill mechanism (lazy on-read refresh, admin re-sync endpoint, startup re-query task) is
  in scope for this plan — `design.md` explicitly defers that to a future bugfix/feature if
  non-disposable production data ever exists.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["2.1"] },
    { "id": 2, "tasks": ["2.2"] },
    { "id": 3, "tasks": ["4.1"] },
    { "id": 4, "tasks": ["4.2"] }
  ]
}
```
