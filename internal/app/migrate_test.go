package app_test

import (
	"database/sql"
	"sort"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/Rhionin/pantry/internal/app"
)

// TestMigrationApplies verifies that RunMigrations applies the initial schema
// to an in-memory SQLite database and that all 8 expected tables are created.
func TestMigrationApplies(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory SQLite: %v", err)
	}
	defer conn.Close()

	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Query sqlite_master for user-created tables (exclude the internal
	// schema_migrations tracking table).
	rows, err := conn.Query(`
		SELECT name FROM sqlite_master
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
		  AND name != 'schema_migrations'
		ORDER BY name`)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	want := []string{
		"barcodes",
		"cart_integrations",
		"consumption_events",
		"item_instances",
		"items",
		"products",
		"scan_entries",
		"shopping_list_items",
	}
	sort.Strings(want) // already sorted, but be explicit

	if len(got) != len(want) {
		t.Fatalf("expected %d tables, got %d: %v", len(want), len(got), got)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("table[%d]: want %q, got %q", i, name, got[i])
		}
	}
}

// TestMigrationIsIdempotent verifies that calling RunMigrations twice on the
// same database does not return an error or duplicate any tables.
func TestMigrationIsIdempotent(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory SQLite: %v", err)
	}
	defer conn.Close()

	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("first RunMigrations: %v", err)
	}
	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("second RunMigrations (idempotency check): %v", err)
	}

	// One schema_migrations row per applied .sql file; the second RunMigrations
	// must not re-apply any file, so the count equals the number of migration
	// files (currently 001_initial_schema.sql and 002_backfill_orphaned_products.sql).
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != 2 {
		t.Errorf("schema_migrations should have 2 rows after two runs, got %d", count)
	}
}

// Feature: external-product-persistence, Property 4: Data Repair — Pre-existing Orphans Become Visible
//
// For any items row whose product_id has no products row at the time the repair
// runs, migration 002 SHALL insert a backing products row (placeholder name) so
// the item references an existing product, and re-running RunMigrations SHALL stay
// idempotent (no duplicate products rows, unchanged schema_migrations count).
//
// This exercises logic not reachable through the HTTP API: the backfill is a
// one-time startup repair against orphans that already exist in the DB. The
// migration runs once at construction against an empty DB, so to simulate the
// repair against a pre-existing orphan we seed the orphan, clear the 002 record,
// and re-run — mirroring the pattern in
// internal/server/handler_external_persistence_test.go's orphan case.
func TestMigration002BackfillsOrphanedProducts(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory SQLite: %v", err)
	}
	defer conn.Close()

	// Create the schema (and record 002 as applied against the empty DB).
	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations (create schema): %v", err)
	}

	const orphanProductID = "011110728227" // barcode with no products row

	// Seed an orphaned items row whose product_id has no matching products row.
	if _, err := conn.Exec(
		`INSERT INTO items (id, user_id, product_id) VALUES ('orphan-item', 'user-1', ?)`,
		orphanProductID,
	); err != nil {
		t.Fatalf("seed orphan item: %v", err)
	}

	// Clear the 002 record so RunMigrations re-applies the backfill against the
	// now-seeded orphan (simulating startup repair with a pre-existing orphan).
	if _, err := conn.Exec(
		`DELETE FROM schema_migrations WHERE filename = '002_backfill_orphaned_products.sql'`,
	); err != nil {
		t.Fatalf("reset migration 002 record: %v", err)
	}

	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations (repair against orphan): %v", err)
	}

	// The backfilled products row exists with the placeholder name.
	var name string
	if err := conn.QueryRow(`SELECT name FROM products WHERE id = ?`, orphanProductID).Scan(&name); err != nil {
		t.Fatalf("expected backfilled products row for %q: %v", orphanProductID, err)
	}
	if want := "Product " + orphanProductID; name != want {
		t.Errorf("backfilled product name: want %q, got %q", want, name)
	}

	// Capture state before re-running to assert idempotency.
	migrationsBefore := countMigrations(t, conn)
	productsBefore := countProducts(t, conn, orphanProductID)

	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations (idempotent re-run): %v", err)
	}

	if got := countMigrations(t, conn); got != migrationsBefore {
		t.Errorf("schema_migrations count changed on re-run: before %d, after %d", migrationsBefore, got)
	}
	if got := countProducts(t, conn, orphanProductID); got != productsBefore {
		t.Errorf("duplicate products row created on re-run: before %d, after %d", productsBefore, got)
	}
	if got := countProducts(t, conn, orphanProductID); got != 1 {
		t.Errorf("products rows for %q: want 1, got %d", orphanProductID, got)
	}
}

func countMigrations(t *testing.T, conn *sql.DB) int {
	t.Helper()
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	return count
}

func countProducts(t *testing.T, conn *sql.DB, id string) int {
	t.Helper()
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM products WHERE id = ?`, id).Scan(&count); err != nil {
		t.Fatalf("count products: %v", err)
	}
	return count
}
