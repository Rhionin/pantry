package shopping_test

import (
	"context"
	"testing"

	"github.com/Rhionin/pantry/internal/shopping"
)

func TestSavePlannedLinesSnapshotsAutoRowsAndLeavesManuals(t *testing.T) {
	deps := newTestStore(t)
	ctx := context.Background()
	autoID := createTestItem(t, deps, ctx, "user-1", "auto-product", "Auto Product")
	manualID := createTestItem(t, deps, ctx, "user-1", "manual-product", "Manual Product")
	if _, err := deps.shopping.AddManualItem(ctx, "user-1", manualID, 5); err != nil {
		t.Fatalf("AddManualItem: %v", err)
	}

	lines := []shopping.PlannedLine{
		{ItemID: autoID, Quantity: 2, Note: "replacing 2 you used"},
		{ItemID: manualID, Quantity: 9, Note: "ignored", Manual: true},
	}
	if err := deps.shopping.SavePlannedLines(ctx, "user-1", lines, nil); err != nil {
		t.Fatalf("SavePlannedLines: %v", err)
	}
	active, err := deps.shopping.ListUnpurchased(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListUnpurchased: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("rows: got %#v", active)
	}
	var autoRow, manualRow shopping.ShoppingListItem
	for _, row := range active {
		switch row.ItemID {
		case autoID:
			autoRow = row
		case manualID:
			manualRow = row
		}
	}
	if autoRow.Quantity != 2 || autoRow.Note != "replacing 2 you used" || autoRow.ID == "" {
		t.Fatalf("auto row: %#v", autoRow)
	}
	if manualRow.Quantity != 5 || manualRow.Source != "manual" {
		t.Fatalf("manual row was resized: %#v", manualRow)
	}

	if err := deps.shopping.MarkPurchased(ctx, autoRow.ID); err != nil {
		t.Fatalf("MarkPurchased: %v", err)
	}
	if err := deps.shopping.SavePlannedLines(ctx, "user-1", lines, nil); err != nil {
		t.Fatalf("SavePlannedLines refresh: %v", err)
	}
	active, err = deps.shopping.ListUnpurchased(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListUnpurchased after purchase: %v", err)
	}
	if len(active) != 1 || active[0].ItemID != manualID {
		t.Fatalf("unchanged plan should keep the purchase hidden, got %#v", active)
	}

	changed := []shopping.PlannedLine{{ItemID: autoID, Quantity: 3, Note: "replacing 3 you used"}}
	if err := deps.shopping.SavePlannedLines(ctx, "user-1", changed, nil); err != nil {
		t.Fatalf("SavePlannedLines changed: %v", err)
	}
	active, err = deps.shopping.ListUnpurchased(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListUnpurchased after change: %v", err)
	}
	found := false
	for _, row := range active {
		if row.ItemID != autoID {
			continue
		}
		found = true
		if row.ID != autoRow.ID || row.Quantity != 3 || row.PurchasedAt != nil {
			t.Fatalf("changed plan should refresh the same row, got %#v", row)
		}
	}
	if !found {
		t.Fatalf("auto row missing after change: %#v", active)
	}

	if err := deps.shopping.SavePlannedLines(ctx, "user-1", nil, nil); err != nil {
		t.Fatalf("SavePlannedLines empty: %v", err)
	}
	active, err = deps.shopping.ListUnpurchased(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListUnpurchased after clear: %v", err)
	}
	if len(active) != 1 || active[0].ItemID != manualID || active[0].Quantity != 5 {
		t.Fatalf("clearing auto rows should leave the manual, got %#v", active)
	}
}
