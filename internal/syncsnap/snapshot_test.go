package syncsnap

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/app"
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
