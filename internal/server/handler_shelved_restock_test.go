package server

import (
	"net/http"
	"testing"

	"github.com/Rhionin/stockbot/internal/restock"
	"github.com/Rhionin/stockbot/internal/scan"
)

// A wipe hides the item from restock until it is stocked in again. The saved
// item target is still stored and shows up on that later stock-in.
func TestWipeHidesRestockItemsUntilStockIn(t *testing.T) {
	handler, env := setupTestWithDB(t)
	createItemViaStockIn(t, env.DB, env.ProductStore, "user-1", "prod-milk", "Milk", "item-milk")
	exchangeJSON(t, handler, http.MethodPut, "/api/restock/items/item-milk", `{"kind":"amount","value":4}`, http.StatusOK)
	dairy := decodeID(t, exchangeJSON(t, handler, http.MethodPost, "/api/restock/categories", `{"name":"Dairy"}`, http.StatusOK))
	exchangeJSON(t, handler, http.MethodPut, "/api/restock/categories/"+dairy+"/target", `{"kind":"amount","value":2}`, http.StatusOK)
	exchangeJSON(t, handler, http.MethodPost, "/api/restock/items/item-milk/categories", `{"categoryId":"`+dairy+`"}`, http.StatusOK)

	before := mustRestock(t, handler)
	if restockByProduct(before, "prod-milk") == nil || before.ItemCount != 1 {
		t.Fatalf("setup restock = itemCount %d items %d", before.ItemCount, len(before.Items))
	}
	if got := categoryNamed(before, "Dairy"); got == nil || got.Items != 1 || got.OwnTargets != 1 || got.Target == nil || got.Target.Value != 2 {
		t.Fatalf("setup Dairy = %+v", got)
	}

	exchangeJSON(t, handler, http.MethodPost, "/api/inventory/wipe", `{"confirmation":"WIPE INVENTORY"}`, http.StatusOK)

	wiped := mustRestock(t, handler)
	if restockByProduct(wiped, "prod-milk") != nil || wiped.ItemCount != 0 || len(wiped.Items) != 0 {
		t.Fatalf("wiped restock still lists items: itemCount %d %+v", wiped.ItemCount, wiped.Items)
	}
	if got := categoryNamed(wiped, "Dairy"); got == nil || got.Items != 0 || got.OwnTargets != 0 {
		t.Fatalf("wiped Dairy count = %+v", got)
	}
	if got := categoryTarget(wiped, "Dairy"); got == nil || got.Kind != restock.KindAmount || got.Value != 2 {
		t.Fatalf("wiped Dairy target = %+v", got)
	}

	insertPendingScan(env, "scan-milk-again", "prod-milk", scan.StockIn)
	exchangeJSON(t, handler, http.MethodPost, "/api/scans/scan-milk-again/commit", ``, http.StatusOK)

	back := mustRestock(t, handler)
	view := restockByProduct(back, "prod-milk")
	if view == nil || view.Target.Kind != restock.KindAmount || view.Target.Value != 4 || view.Level != restock.LevelItem {
		t.Fatalf("stocking Milk in again did not bring back its saved target of 4: %+v", view)
	}
	if back.ItemCount != 1 {
		t.Fatalf("itemCount after restock = %d", back.ItemCount)
	}
	if got := categoryNamed(back, "Dairy"); got == nil || got.Items != 1 || got.OwnTargets != 1 {
		t.Fatalf("Dairy count after restock = %+v", got)
	}
}

func categoryNamed(snap restock.Snapshot, name string) *restock.CategorySummary {
	for i := range snap.Categories {
		if snap.Categories[i].Name == name {
			return &snap.Categories[i]
		}
	}
	return nil
}
