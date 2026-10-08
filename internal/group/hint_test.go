package group

import (
	"context"
	"testing"
)

func TestHints_MatchesGroupAndSkipsMembers(t *testing.T) {
	groups, catalog, _ := newTestGroups(t)
	ctx := context.Background()
	mustProduct(t, catalog, "kr", "Kroger Cut Green Beans")
	mustProduct(t, catalog, "dm", "Del Monte Cut Green Beans")
	mustProduct(t, catalog, "milk", "Whole Milk")
	mustProduct(t, catalog, "french", "Del Monte French Style Green Beans")
	if _, err := groups.Create(ctx, "Cut green beans", []string{"kr"}, nil); err != nil {
		t.Fatalf("create group: %v", err)
	}

	hints, err := groups.Hints(ctx, map[string]string{
		"kr":     "Kroger Cut Green Beans",
		"dm":     "Del Monte Cut Green Beans",
		"milk":   "Whole Milk",
		"french": "Del Monte French Style Green Beans",
	})
	if err != nil {
		t.Fatalf("hints: %v", err)
	}
	if _, ok := hints["kr"]; ok {
		t.Fatalf("a member should not be hinted, got %+v", hints["kr"])
	}
	if _, ok := hints["milk"]; ok {
		t.Fatalf("milk should not match cut green beans, got %+v", hints["milk"])
	}
	if _, ok := hints["french"]; ok {
		t.Fatalf("french style should not match cut green beans, got %+v", hints["french"])
	}
	hint, ok := hints["dm"]
	if !ok || hint.Name != "Cut green beans" || hint.GroupID == "" {
		t.Fatalf("del monte hint = %+v, ok %v", hint, ok)
	}
}

func TestNoteFromScan_Once(t *testing.T) {
	groups, catalog, conn := newTestGroups(t)
	ctx := context.Background()
	mustProduct(t, catalog, "dm", "Del Monte Cut Green Beans")
	created, err := groups.Create(ctx, "Cut green beans", nil, nil)
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := groups.NoteFromScan(ctx, "dm"); err != nil {
		t.Fatalf("note: %v", err)
	}
	if err := groups.NoteFromScan(ctx, "dm"); err != nil {
		t.Fatalf("second note: %v", err)
	}
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM group_suggestions WHERE kind = 'from_scan'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("from_scan rows = %d, want 1", n)
	}
	var existing string
	if err := conn.QueryRow(`SELECT COALESCE(existing_group_id, '') FROM group_suggestions WHERE kind = 'from_scan'`).Scan(&existing); err != nil {
		t.Fatal(err)
	}
	if existing != created.ID {
		t.Fatalf("existing group = %q, want %q", existing, created.ID)
	}
	hints, err := groups.Hints(ctx, map[string]string{"dm": "Del Monte Cut Green Beans"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := hints["dm"]; ok {
		t.Fatalf("a noted product should not be hinted again, got %+v", hints["dm"])
	}
}
