# Agent Guidelines

## File Editing

Use built-in file editing tools (read_file, write_file, str_replace) to read and modify code directly. Do not fall back to shell commands (cat, echo, heredocs, python -c, etc.) for file I/O — that requires unnecessary user approval and is more error-prone.

## Testing

After each task, run `./scripts/test-coverage.sh` to enforce coverage thresholds. The script auto-updates its threshold when coverage increases—commit the updated script to lock in the improvement.

Prefer API tests using the apitest framework in `internal/server/`. These exercise behavior from the customer's perspective and often eliminate the need for lower-level unit tests. Write unit tests only when API tests are insufficient (e.g., testing internal algorithms, edge cases in pure functions).

Test reproducible errors (bad requests, validation failures) but skip internal error paths (database errors, network timeouts) that API callers can't trigger. Focus coverage on customer-facing behavior.

Handler tests use a declarative table-driven framework with HTTP exchanges. Use `afterRequest: exchanges()` to verify behavior through HTTP requests rather than direct database queries. Test the HTTP contract, not implementation details.

## Database Setup in Tests

Use `app.RunMigrations(conn)` to initialize test databases—never duplicate the schema inline. The `newTestRepo` helper in `internal/inventory/inventory_test.go` is the canonical pattern for the inventory package. Other packages follow the same convention (see `internal/product/product_test.go`, `internal/scan/scan_test.go`).

## Code Style

Error messages should be user-friendly without function names. Avoid redundant comments that restate code—keep godoc and WHY comments, remove WHAT comments and numbered steps.
