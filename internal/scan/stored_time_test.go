package scan_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Rhionin/stockbot/internal/scan"
)

func TestScanWritesStoreCleanUTC(t *testing.T) {
	queue, db := newTestQueue(t)
	ctx := context.Background()
	created, err := queue.CreateScanEntry(ctx, scan.ScanEntry{
		UserID:  "user-1",
		Barcode: "000111222333",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.CommitScanEntry(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	var scanned, committed string
	if err := db.QueryRow(`
		SELECT CAST(scanned_at AS TEXT), CAST(committed_at AS TEXT)
		FROM scan_entries WHERE id = ?`, created.ID).Scan(&scanned, &committed); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{scanned, committed} {
		if !strings.Contains(raw, "+0000 UTC") || strings.Contains(raw, "m=") {
			t.Fatalf("stored %q", raw)
		}
	}
}

func TestSuffixedScanTimesReadBackInOrder(t *testing.T) {
	queue, db := newTestQueue(t)
	const suffix = "2026-10-17 18:00:00.5 +0000 UTC m=+9"
	const later = "2026-10-17 18:00:01 +0000 UTC"
	want := time.Date(2026, 10, 17, 18, 0, 0, 500000000, time.UTC)
	if _, err := db.Exec(`
		INSERT INTO scan_entries (id, user_id, barcode, scanned_at, unit_count, status, committed_at) VALUES
			('later', 'user-1', '111', ?, 1, 'pending', NULL),
			('suffix', 'user-1', '222', ?, 1, 'committed', ?)`,
		later, suffix, suffix); err != nil {
		t.Fatal(err)
	}
	got, err := queue.GetScanEntry(context.Background(), "suffix")
	if err != nil {
		t.Fatal(err)
	}
	if !got.ScannedAt.Equal(want) || got.CommittedAt == nil || !got.CommittedAt.Equal(want) {
		t.Fatalf("scanned %s committed %v", got.ScannedAt, got.CommittedAt)
	}
	listed, err := queue.ListScanEntries(context.Background(), "user-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].ID != "suffix" || listed[1].ID != "later" {
		t.Fatalf("order = %s, %s", listed[0].ID, listed[1].ID)
	}
}
