package shopping_test

import (
	"context"
	"testing"

	"github.com/Rhionin/pantry/internal/shopping"
)

func TestSyncDerivedItemsPreservesPurchaseUntilInventoryChanges(t *testing.T) {
	deps := newTestStore(t)
	ctx := context.Background()
	itemID := createTestItem(t, deps, ctx, "user-1", "auto-product", "Auto Product")
	gap := []shopping.DerivedEntry{{ItemID: itemID, Quantity: 2, Source: "auto"}}

	active, err := deps.shopping.SyncDerivedItems(ctx, "user-1", gap)
	if err != nil {
		t.Fatalf("SyncDerivedItems: %v", err)
	}
	if len(active) != 1 || active[0].ID == "" {
		t.Fatalf("active derived items: expected one item with an ID, got %#v", active)
	}
	purchasedID := active[0].ID
	active, err = deps.shopping.SyncDerivedItems(ctx, "user-1", gap)
	if err != nil {
		t.Fatalf("SyncDerivedItems refresh: %v", err)
	}
	if len(active) != 1 || active[0].ID != purchasedID {
		t.Fatalf("refreshed derived items: expected existing item %q, got %#v", purchasedID, active)
	}
	if err := deps.shopping.MarkPurchased(ctx, purchasedID); err != nil {
		t.Fatalf("MarkPurchased: %v", err)
	}

	active, err = deps.shopping.SyncDerivedItems(ctx, "user-1", gap)
	if err != nil {
		t.Fatalf("SyncDerivedItems after purchase: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("active derived items after purchase: want none, got %#v", active)
	}

	changedGap := []shopping.DerivedEntry{{ItemID: itemID, Quantity: 3, Source: "auto"}}
	active, err = deps.shopping.SyncDerivedItems(ctx, "user-1", changedGap)
	if err != nil {
		t.Fatalf("SyncDerivedItems after gap change: %v", err)
	}
	if len(active) != 1 || active[0].ID == purchasedID {
		t.Fatalf("active derived items after gap change: expected a new item, got %#v", active)
	}

	if _, err := deps.shopping.SyncDerivedItems(ctx, "user-1", nil); err != nil {
		t.Fatalf("SyncDerivedItems at target: %v", err)
	}
	active, err = deps.shopping.SyncDerivedItems(ctx, "user-1", gap)
	if err != nil {
		t.Fatalf("SyncDerivedItems after inventory change: %v", err)
	}
	if len(active) != 1 || active[0].ID == purchasedID {
		t.Fatalf("active derived items after inventory change: expected a new item, got %#v", active)
	}
}
