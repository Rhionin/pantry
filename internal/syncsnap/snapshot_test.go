package syncsnap

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Rhionin/stockbot/internal/app"
	_ "modernc.org/sqlite"
)

func TestBuildEmptyHousehold(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := app.RunMigrations(conn); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 10, 10, 1, 2, 3, 0, time.UTC)
	snap, err := Build(context.Background(), conn, when)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.GeneratedAt.Equal(when) || snap.Products == nil || snap.Groups == nil || snap.ShoppingList == nil {
		t.Fatalf("snap = %+v", snap)
	}
	if len(snap.Products) != 0 || len(snap.Moves) != 0 {
		t.Fatalf("expected an empty pantry, got %+v", snap)
	}
}

func TestBuildReadsBrandFromTheProductName(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := app.RunMigrations(conn); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`
		INSERT INTO products (id, name) VALUES ('p1', 'Hunt''s Tomato Paste'), ('p2', 'Tomato Paste');
		INSERT INTO product_groups (id, user_id, name, name_key, rule) VALUES ('g1', 'user-1', 'Tomato Paste', 'tomato paste', 'favorite');
		INSERT INTO product_group_members (product_id, group_id) VALUES ('p1', 'g1'), ('p2', 'g1');
	`); err != nil {
		t.Fatal(err)
	}
	snap, err := Build(context.Background(), conn, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, p := range snap.Products {
		got[p.ID] = p.Brand
	}
	if got["p1"] != "Hunt's" || got["p2"] != "" {
		t.Fatalf("brands = %#v", got)
	}
}
