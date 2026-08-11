// Package app provides application initialization and database setup.
package app

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations applies all numbered .sql files embedded in the migrations
// directory, in lexicographic order, skipping any that have already been
// recorded in the schema_migrations table. It is idempotent.
func RunMigrations(db *sql.DB) error {
	// Ensure the tracking table exists.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		filename TEXT PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}

	// Collect .sql files; fs.ReadDir already returns entries sorted by name.
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

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
	}

	return nil
}
