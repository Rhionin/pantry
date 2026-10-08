package app

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"time"
)

// shoppingChangeMigration is the first migration that changes live shopping rows.
const shoppingChangeMigration = "012_shopping_list_group.sql"

// snapshotShoppingDB runs once, immediately before shoppingChangeMigration.
// Tests replace it to simulate a backup that cannot be written.
var snapshotShoppingDB = writePreGroupsBackup

func writePreGroupsBackup(db *sql.DB) error {
	path, err := sqliteMainPath(db)
	if err != nil {
		return err
	}
	if path == "" || path == ":memory:" {
		return nil
	}
	dir := filepath.Dir(path)
	matches, err := filepath.Glob(filepath.Join(dir, "pantry-pre-groups-*.db"))
	if err != nil {
		return fmt.Errorf("could not back up the pantry before updating shopping: %w", err)
	}
	if len(matches) > 0 {
		log.Printf("shopping backup already exists: %s", matches[0])
		return nil
	}
	dest := filepath.Join(dir, "pantry-pre-groups-"+time.Now().UTC().Format("20060102T150405Z")+".db")
	if _, err := db.Exec(`VACUUM INTO ?`, dest); err != nil {
		return fmt.Errorf("could not back up the pantry before updating shopping: %w", err)
	}
	log.Printf("wrote shopping backup: %s", dest)
	return nil
}

func sqliteMainPath(db *sql.DB) (string, error) {
	var seq int
	var name, file string
	if err := db.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &file); err != nil {
		return "", fmt.Errorf("could not back up the pantry before updating shopping: %w", err)
	}
	return file, nil
}
