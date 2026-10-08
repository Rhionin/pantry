package group

import (
	"context"
	"testing"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/shopping"
)

func TestSeed_OldPlanPinsFavoriteAndIncludesDelMonte(t *testing.T) {
	groups, catalog, conn := newTestGroups(t)
	ctx := context.Background()
	for _, row := range []struct{ id, name string }{
		{"gv", "Great Value Cut Green Beans"},
		{"kr", "Kroger Cut Green Beans"},
		{"dm", "Del Monte Cut Green Beans"},
	} {
		if err := catalog.CreateProduct(ctx, product.Product{ID: row.id, Name: row.name, UnitOfMeasure: "can"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := conn.Exec(`INSERT INTO items (id, user_id, product_id) VALUES ('item-gv', 'user-1', 'gv')`); err != nil {
		t.Fatal(err)
	}
	key, ok := shopping.NeedKey("Great Value Cut Green Beans", "can")
	if !ok {
		t.Fatal("need key")
	}
	if _, err := conn.Exec(`INSERT INTO brand_preferences (user_id, need_key, item_id, ignore_price) VALUES ('user-1', ?, 'item-gv', 1)`, key); err != nil {
		t.Fatal(err)
	}

	if err := groups.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if err := groups.Seed(ctx); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM group_suggestions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("suggestions = %d, want 1", n)
	}
	var kind, rule, pin, title string
	if err := conn.QueryRow(`SELECT kind, proposed_rule, pinned_product_id, title FROM group_suggestions`).Scan(&kind, &rule, &pin, &title); err != nil {
		t.Fatal(err)
	}
	if kind != "from_old_plan" || rule != "favorite" || pin != "gv" {
		t.Fatalf("card = %s %s pin %s title %s", kind, rule, pin, title)
	}
	members := map[string]struct{}{}
	rows, err := conn.Query(`SELECT product_id FROM group_suggestion_members`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		members[id] = struct{}{}
	}
	for _, id := range []string{"gv", "kr", "dm"} {
		if _, ok := members[id]; !ok {
			t.Fatalf("missing %s in %+v", id, members)
		}
	}
}

func TestSeed_SkipsGoneAndSinglePreferences(t *testing.T) {
	groups, catalog, conn := newTestGroups(t)
	ctx := context.Background()
	if err := catalog.CreateProduct(ctx, product.Product{ID: "milk", Name: "Whole Milk", UnitOfMeasure: "carton"}); err != nil {
		t.Fatal(err)
	}
	key, ok := shopping.NeedKey("Whole Milk", "carton")
	if !ok {
		t.Fatal("need key")
	}
	if _, err := conn.Exec(`INSERT INTO brand_preferences (user_id, need_key, item_id, ignore_price) VALUES ('user-1', ?, 'missing-item', 1)`, key); err != nil {
		t.Fatal(err)
	}
	if err := groups.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM group_suggestions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("suggestions = %d, want 0", n)
	}
}
