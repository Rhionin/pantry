package restock

import (
	"context"
	"database/sql"
	"testing"

	"github.com/Rhionin/stockbot/internal/app"
	_ "modernc.org/sqlite"
)

func newTestTargets(t *testing.T) (*Targets, *sql.DB) {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewTargets(conn), conn
}

func insertProduct(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO products (id, name, unit_of_measure) VALUES (?, ?, 'bags')`, id, name); err != nil {
		t.Fatal(err)
	}
}

func TestCopyLegacyKeepsCurrentBehavior(t *testing.T) {
	targets, db := newTestTargets(t)
	ctx := context.Background()
	insertProduct(t, db, "chips", "Tortilla chips")
	insertProduct(t, db, "beans", "Green beans")
	insertProduct(t, db, "milk", "Oat milk")
	if _, err := db.Exec(`
		INSERT INTO items (id, user_id, product_id, target_quantity, replenishment_mode) VALUES
			('item-chips', 'user-1', 'chips', 4, 'target'),
			('item-beans', 'user-1', 'beans', NULL, 'target'),
			('item-milk', 'user-1', 'milk', 9, 'replenish')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO supply_overrides (product_id, window_months) VALUES ('chips', 2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO product_groups (id, user_id, name, name_key) VALUES ('g1', 'user-1', 'Beans', 'beans')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO product_group_members (group_id, product_id, no_restock) VALUES ('g1', 'beans', 1)`); err != nil {
		t.Fatal(err)
	}

	if err := CopyLegacy(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CopyLegacy(ctx, db); err != nil {
		t.Fatal(err)
	}

	snap, err := targets.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]ItemView{}
	for _, item := range snap.Items {
		byName[item.Name] = item
	}
	chips := byName["Tortilla chips"]
	if chips.Level != LevelItem || chips.Target.Kind != KindMonths || chips.Target.Value != 2 {
		t.Fatalf("override wins over stored quantity: %+v", chips.Target)
	}
	beans := byName["Green beans"]
	if beans.Level != LevelItem || beans.Target.Value != 0 {
		t.Fatalf("don't restock member: %+v level %s", beans.Target, beans.Level)
	}
	milk := byName["Oat milk"]
	if milk.Level != LevelHousehold || milk.Target.Value != 3 {
		t.Fatalf("replenish mode is not an item target: %+v level %s", milk.Target, milk.Level)
	}
	if len(snap.Categories) != len(StarterCategories) {
		t.Fatalf("starters: %d", len(snap.Categories))
	}
}

func TestDontRestockAndCategoryPreview(t *testing.T) {
	targets, db := newTestTargets(t)
	ctx := context.Background()
	insertProduct(t, db, "bananas", "Bananas")
	insertProduct(t, db, "lemons", "Lemons")
	if _, err := db.Exec(`
		INSERT INTO items (id, user_id, product_id) VALUES
			('item-bananas', 'user-1', 'bananas'),
			('item-lemons', 'user-1', 'lemons')`); err != nil {
		t.Fatal(err)
	}
	created, err := targets.CreateCategory(ctx, "Produce")
	if err != nil {
		t.Fatal(err)
	}
	if err := targets.AddItemCategory(ctx, "item-bananas", created.ID); err != nil {
		t.Fatal(err)
	}
	if err := targets.AddItemCategory(ctx, "item-lemons", created.ID); err != nil {
		t.Fatal(err)
	}
	if err := targets.SetItem(ctx, "item-lemons", Target{Kind: KindAmount, Value: 6}); err != nil {
		t.Fatal(err)
	}

	preview, err := targets.PreviewCategory(ctx, created.ID, Target{Kind: KindMonths, Value: 0})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Title != "Stop restocking Produce?" || len(preview.Stop) != 1 || preview.Stop[0].Name != "Bananas" {
		t.Fatalf("preview stop: %+v", preview)
	}
	if len(preview.Keep) != 1 || preview.Keep[0].Name != "Lemons" || preview.Keep[0].Reason != "Set on this item" {
		t.Fatalf("preview keep: %+v", preview.Keep)
	}

	if err := targets.SetCategoryTarget(ctx, created.ID, &Target{Kind: KindMonths, Value: 0}); err != nil {
		t.Fatal(err)
	}
	snap, err := targets.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range snap.Items {
		if item.Name == "Bananas" && (item.Target.Value != 0 || item.Level != LevelCategory || item.QuietLine != "Not restocked · Produce") {
			t.Fatalf("bananas: %+v %s", item.Target, item.QuietLine)
		}
		if item.Name == "Lemons" && item.Level != LevelItem {
			t.Fatalf("lemons stayed on the item: %s", item.Level)
		}
	}

	if err := targets.RemoveItemCategory(ctx, "item-bananas", created.ID); err != nil {
		t.Fatal(err)
	}
	if err := targets.SaveOffTags(ctx, "bananas", []string{"en:fresh-fruits", "en:undefined"}, nil); err != nil {
		t.Fatal(err)
	}
	cards, err := targets.Suggestions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, card := range cards {
		if card.Category != "Produce" {
			continue
		}
		for _, item := range card.Items {
			if item.ItemID == "item-bananas" {
				t.Fatal("a removed membership must stay removed")
			}
		}
	}
}

func TestHouseholdDefaultMayBeZero(t *testing.T) {
	targets, db := newTestTargets(t)
	ctx := context.Background()
	insertProduct(t, db, "salt", "Salt")
	if _, err := db.Exec(`INSERT INTO items (id, user_id, product_id) VALUES ('item-salt', 'user-1', 'salt')`); err != nil {
		t.Fatal(err)
	}
	if err := targets.SetHousehold(ctx, Target{Kind: KindMonths, Value: 0}); err != nil {
		t.Fatal(err)
	}
	snap, err := targets.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Household.Value != 0 || snap.Items[0].Level != LevelHousehold || snap.Items[0].Target.Value != 0 {
		t.Fatalf("household 0: %+v item %+v", snap.Household, snap.Items[0].Target)
	}
	var months string
	if err := db.QueryRow(`SELECT value FROM app_settings WHERE key = 'supply_months'`).Scan(&months); err == nil && months == "0" {
		t.Fatal("a household default of 0 must not be written as supply_months")
	}
}
