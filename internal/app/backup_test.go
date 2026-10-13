package app

import (
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestShoppingBackupOnceBesideTheDatabase(t *testing.T) {
	dir := t.TempDir()
	conn := openFileDB(t, filepath.Join(dir, "pantry.db"))

	var buf bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(previous) })

	if err := RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	matches := backupFiles(t, dir)
	if len(matches) != 1 {
		t.Fatalf("backups: %v", matches)
	}
	if !strings.Contains(buf.String(), matches[0]) {
		t.Fatalf("log %q does not name %s", buf.String(), matches[0])
	}
	if columnExists(t, conn, "shopping_list_items", "group_id") == false {
		t.Fatal("live database is missing group_id")
	}
	backup := openFileDB(t, matches[0])
	if columnExists(t, backup, "shopping_list_items", "group_id") {
		t.Fatal("backup was taken after the shopping migration")
	}

	buf.Reset()
	if err := RunMigrations(conn); err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}
	if again := backupFiles(t, dir); len(again) != 1 || again[0] != matches[0] {
		t.Fatalf("second run wrote another backup: %v", again)
	}
}

func TestShoppingBackupSkipsWhenOneAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "pantry-pre-groups-existing.db")
	if err := os.WriteFile(existing, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	conn := openFileDB(t, filepath.Join(dir, "pantry.db"))

	var buf bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(previous) })

	if err := RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	matches := backupFiles(t, dir)
	if len(matches) != 1 || matches[0] != existing {
		t.Fatalf("backups: %v", matches)
	}
	if !strings.Contains(buf.String(), existing) {
		t.Fatalf("log %q does not name the existing backup", buf.String())
	}
	if !columnExists(t, conn, "shopping_list_items", "group_id") {
		t.Fatal("migration did not run")
	}
}

func TestRestockBackupOnceBeforeTheFlag(t *testing.T) {
	dir := t.TempDir()
	conn := openFileDB(t, filepath.Join(dir, "pantry.db"))
	if err := RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "pantry-pre-restock-*.db"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("backups: %v", matches)
	}
	if !columnExists(t, conn, "product_group_members", "no_restock") {
		t.Fatal("live database is missing no_restock")
	}
	backup := openFileDB(t, matches[0])
	if columnExists(t, backup, "product_group_members", "no_restock") {
		t.Fatal("backup was taken after the restock migration")
	}
	if !columnExists(t, backup, "shopping_list_items", "group_id") {
		t.Fatal("backup was taken before the shopping migration")
	}
	if err := RunMigrations(conn); err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}
	again, err := filepath.Glob(filepath.Join(dir, "pantry-pre-restock-*.db"))
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 || again[0] != matches[0] {
		t.Fatalf("second run wrote another backup: %v", again)
	}
}

func TestRestockBackupFailureStopsMigration(t *testing.T) {
	orig := snapshotRestockDB
	t.Cleanup(func() { snapshotRestockDB = orig })
	snapshotRestockDB = func(*sql.DB) error {
		return fmt.Errorf("disk full")
	}

	conn := openFileDB(t, filepath.Join(t.TempDir(), "pantry.db"))
	err := RunMigrations(conn)
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("RunMigrations err = %v", err)
	}
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE filename = ?`, restockChangeMigration).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("migration recorded after a failed backup: %d", count)
	}
	if columnExists(t, conn, "product_group_members", "no_restock") {
		t.Fatal("no_restock was added after the backup failed")
	}
	if !columnExists(t, conn, "shopping_list_items", "group_id") {
		t.Fatal("earlier shopping migration did not run")
	}
}

func TestShoppingBackupFailureStopsMigration(t *testing.T) {
	orig := snapshotShoppingDB
	t.Cleanup(func() { snapshotShoppingDB = orig })
	snapshotShoppingDB = func(*sql.DB) error {
		return fmt.Errorf("disk full")
	}

	conn := openFileDB(t, filepath.Join(t.TempDir(), "pantry.db"))
	err := RunMigrations(conn)
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("RunMigrations err = %v", err)
	}
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE filename = ?`, shoppingChangeMigration).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("migration recorded after a failed backup: %d", count)
	}
	if columnExists(t, conn, "shopping_list_items", "group_id") {
		t.Fatal("group_id was added after the backup failed")
	}
}

func openFileDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func backupFiles(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "pantry-pre-groups-*.db"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func columnExists(t *testing.T, conn *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := conn.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}
