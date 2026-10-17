package history

import (
	"testing"
	"time"
)

// A committed scan written by time.Now().String() keeps the monotonic " m="
// suffix. The repair reads the shelf instant back through the driver, which
// drops that suffix, so a SQL equality check never sees the row.
func TestRepairMatchesSuffixedScannedAt(t *testing.T) {
	const suffixed = "2026-10-17 18:55:19.032067103 +0000 UTC m=+12.3"
	db := openHistoryDB(t)
	execHistory(t, db, `INSERT INTO products (id, name) VALUES ('prod', 'Oats')`)
	execHistory(t, db, `INSERT INTO items (id, user_id, product_id) VALUES ('item', 'user-1', 'prod')`)
	execHistory(t, db, `
		INSERT INTO scan_entries (id, user_id, barcode, scanned_at, direction, unit_count, status, product_id) VALUES
			('scan-decoy', 'user-1', 'oats', '2026-10-17 19:00:00 +0000 UTC', 'stock_in', 1, 'committed', 'prod'),
			('scan-suffix', 'user-1', 'oats', ?, 'stock_in', 1, 'committed', 'prod'),
			('scan-out-suffix', 'user-1', 'oats', ?, 'stock_out', 1, 'committed', 'prod')`,
		suffixed, suffixed)
	execHistory(t, db, `
		INSERT INTO item_instances (id, item_id, stock_in_at, removed_at, removal_reason) VALUES
			('inst-in', 'item', ?, NULL, NULL),
			('inst-out', 'item', '2026-10-17 12:00:00 +0000 UTC', ?, 'manual')`,
		suffixed, suffixed)

	view, err := NewJournal(db).Product(t.Context(), "item", time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	inAt := time.Date(2026, 10, 17, 18, 55, 19, 32067103, time.UTC)
	if got := findMove(t, view, inAt, "in"); got.Source != "scan" {
		t.Fatalf("suffixed stock-in source = %q", got.Source)
	}
	if got := findMove(t, view, inAt, "out"); got.Source != "scan" {
		t.Fatalf("suffixed stock-out source = %q", got.Source)
	}
	plain := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	if got := findMove(t, view, plain, "in"); got.Source != "manual" {
		t.Fatalf("unmatched stock-in source = %q", got.Source)
	}

	rows, err := db.Query(`SELECT direction, source, COALESCE(scan_entry_id, '') FROM stock_moves ORDER BY direction, at`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var direction, source, scanID string
		if err := rows.Scan(&direction, &source, &scanID); err != nil {
			t.Fatal(err)
		}
		got = append(got, direction+" "+source+" "+scanID)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"in manual ",
		"in scan scan-suffix",
		"out scan scan-out-suffix",
	}
	if len(got) != len(want) {
		t.Fatalf("moves %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("moves %#v", got)
		}
	}
}
