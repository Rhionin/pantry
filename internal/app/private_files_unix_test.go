//go:build unix

package app

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSnapshotTempIsPrivateUnderAWorldUmask(t *testing.T) {
	old := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(old) })

	dir := t.TempDir()
	conn := openFileDB(t, filepath.Join(dir, "pantry.db"))
	if err := RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "pantry-pre-*-*.db"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 3 {
		t.Fatalf("backups: %v", matches)
	}
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode %o, want 0600", path, info.Mode().Perm())
		}
	}
}
