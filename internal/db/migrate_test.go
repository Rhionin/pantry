package db_test

import (
	"database/sql"
	"sort"
	"testing"

	// Register the pure-Go SQLite driver.
	_ "modernc.org/sqlite"

	"github.com/Rhionin/pantry/internal/db"
)

// TestMigrationApplies verifies that RunMigrations applies the initial schema
// to an in-memory SQLite database and that all 8 expected tables are created.
func TestMigrationApplies(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory SQLite: %v", err)
	}
	defer conn.Close()

	if err := db.RunMigrations(conn); err != nil {
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

	if err := db.RunMigrations(conn); err != nil {
		t.Fatalf("first RunMigrations: %v", err)
	}
	if err := db.RunMigrations(conn); err != nil {
		t.Fatalf("second RunMigrations (idempotency check): %v", err)
	}

	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_migrations should have 1 row after two runs, got %d", count)
	}
}
