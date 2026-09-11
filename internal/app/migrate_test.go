package app_test

import (
	"bytes"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

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
	// files (currently 001_initial_schema.sql, 002_backfill_orphaned_products.sql,
	// 003_add_product_image_url.sql, and 004_add_product_freshness.sql).
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != 4 {
		t.Errorf("schema_migrations should have 4 rows after two runs, got %d", count)
	}
}

// TestRunMigrations_LogsAppliedMigrationsOnFreshDB verifies that RunMigrations
// logs one "applied migration: <filename>" line per migration file it applies
// against a fresh database, and does not log the "schema up to date" summary
// line (since at least one file was applied).
func TestRunMigrations_LogsAppliedMigrationsOnFreshDB(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory SQLite: %v", err)
	}
	defer conn.Close()

	previous := log.Writer()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(previous)

	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	output := buf.String()

	for _, name := range []string{
		"001_initial_schema.sql",
		"002_backfill_orphaned_products.sql",
		"003_add_product_image_url.sql",
	} {
		want := "applied migration: " + name
		if !strings.Contains(output, want) {
			t.Errorf("log output missing %q; got: %s", want, output)
		}
	}

	if strings.Contains(output, "schema up to date") {
		t.Errorf("log output should not contain the summary line when migrations were applied; got: %s", output)
	}
}

// TestRunMigrations_LogsSummaryWhenAlreadyMigrated verifies that RunMigrations
// logs exactly the "schema up to date, no migrations applied" summary line
// (and no "applied migration:" lines) when called against a database that has
// already been fully migrated.
func TestRunMigrations_LogsSummaryWhenAlreadyMigrated(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory SQLite: %v", err)
	}
	defer conn.Close()

	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("first RunMigrations (set up schema): %v", err)
	}

	previous := log.Writer()
	previousFlags := log.Flags()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer log.SetOutput(previous)
	defer log.SetFlags(previousFlags)

	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}

	want := "schema up to date, no migrations applied\n"
	if got := buf.String(); got != want {
		t.Errorf("log output: want %q, got %q", want, got)
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

// Feature: product-cache-freshness, Property 1: Migration 004 backfills provenance without disturbing data
//
// For any products row that exists before migration 004 runs, THE Migration_Runner
// SHALL set source to 'external', refreshed_at to NULL, and name_overridden to false
// for that row, and SHALL leave that row's id, name, category, unit_of_measure,
// image_url, and created_at unchanged. Re-running RunMigrations SHALL be a no-op.
//
// Validates: Requirements 1.1, 1.2, 1.3, 1.4
func TestMigration004BackfillsProvenanceAndIsIdempotent(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory SQLite: %v", err)
	}
	defer conn.Close()

	applyMigrationsThrough003(t, conn)

	// Seed products rows directly against the pre-004 schema (no source,
	// refreshed_at, or name_overridden columns yet).
	type seedRow struct {
		id, name, category, unitOfMeasure, imageURL string
		createdAt                                   time.Time
	}
	seeds := []seedRow{
		{
			id: "011110728227", name: "Whole Milk", category: "Dairy", unitOfMeasure: "gallon",
			imageURL: "https://images.openfoodfacts.org/milk.jpg", createdAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		},
		{
			id: "some-uuid", name: "Homemade Jam", category: "", unitOfMeasure: "",
			imageURL: "", createdAt: time.Date(2023, 6, 7, 8, 9, 10, 0, time.UTC),
		},
	}
	for _, s := range seeds {
		if _, err := conn.Exec(
			`INSERT INTO products (id, name, category, unit_of_measure, image_url, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			s.id, s.name, nullableSeed(s.category), nullableSeed(s.unitOfMeasure), nullableSeed(s.imageURL), s.createdAt,
		); err != nil {
			t.Fatalf("seed product %q: %v", s.id, err)
		}
	}

	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations (apply 004): %v", err)
	}

	for _, s := range seeds {
		var (
			gotName, gotCategory, gotUnit, gotImage, gotSource string
			gotCreatedAt                                       time.Time
			gotRefreshedAt                                     sql.NullTime
			gotNameOverridden                                  bool
		)
		if err := conn.QueryRow(
			`SELECT name, COALESCE(category, ''), COALESCE(unit_of_measure, ''), COALESCE(image_url, ''),
			        created_at, source, refreshed_at, name_overridden
			 FROM products WHERE id = ?`, s.id,
		).Scan(&gotName, &gotCategory, &gotUnit, &gotImage, &gotCreatedAt, &gotSource, &gotRefreshedAt, &gotNameOverridden); err != nil {
			t.Fatalf("read back product %q: %v", s.id, err)
		}

		if gotName != s.name {
			t.Errorf("product %q name: want %q, got %q", s.id, s.name, gotName)
		}
		if gotCategory != s.category {
			t.Errorf("product %q category: want %q, got %q", s.id, s.category, gotCategory)
		}
		if gotUnit != s.unitOfMeasure {
			t.Errorf("product %q unit_of_measure: want %q, got %q", s.id, s.unitOfMeasure, gotUnit)
		}
		if gotImage != s.imageURL {
			t.Errorf("product %q image_url: want %q, got %q", s.id, s.imageURL, gotImage)
		}
		if !gotCreatedAt.Equal(s.createdAt) {
			t.Errorf("product %q created_at: want %v, got %v", s.id, s.createdAt, gotCreatedAt)
		}
		if gotSource != "external" {
			t.Errorf("product %q source: want %q, got %q", s.id, "external", gotSource)
		}
		if gotRefreshedAt.Valid {
			t.Errorf("product %q refreshed_at: want NULL, got %v", s.id, gotRefreshedAt.Time)
		}
		if gotNameOverridden {
			t.Errorf("product %q name_overridden: want false, got true", s.id)
		}
	}

	// A second RunMigrations must be a no-op: schema_migrations count is stable.
	migrationsBefore := countMigrations(t, conn)
	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations (idempotent re-run): %v", err)
	}
	if got := countMigrations(t, conn); got != migrationsBefore {
		t.Errorf("schema_migrations count changed on re-run: before %d, after %d", migrationsBefore, got)
	}
}

// applyMigrationsThrough003 builds a database migrated only through
// 003_add_product_image_url.sql, by applying migrations 001-003 directly from
// disk and recording each in schema_migrations. This simulates a pre-004
// database so TestMigration004BackfillsProvenanceAndIsIdempotent can seed rows
// against the old schema before RunMigrations applies 004.
func applyMigrationsThrough003(t *testing.T, conn *sql.DB) {
	t.Helper()

	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		filename TEXT PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatalf("create schema_migrations table: %v", err)
	}

	for _, name := range []string{
		"001_initial_schema.sql",
		"002_backfill_orphaned_products.sql",
		"003_add_product_image_url.sql",
	} {
		sqlBytes, err := os.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			t.Fatalf("read migration %q: %v", name, err)
		}
		if _, err := conn.Exec(string(sqlBytes)); err != nil {
			t.Fatalf("apply migration %q: %v", name, err)
		}
		if _, err := conn.Exec(`INSERT INTO schema_migrations (filename) VALUES (?)`, name); err != nil {
			t.Fatalf("record migration %q: %v", name, err)
		}
	}
}

// nullableSeed converts an empty string to nil so seeded optional columns are
// stored as NULL rather than empty string, matching product.Catalog's convention.
func nullableSeed(s string) any {
	if s == "" {
		return nil
	}
	return s
}
