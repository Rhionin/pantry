package supply

import (
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
)

func TestUsageFromMembers(t *testing.T) {
	base, dimension, err := product.BaseFromAmount(16, "oz")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	started := now.Add(-90 * 24 * time.Hour)
	firstIn := started.Add(time.Second)
	sized := []Fact{{
		Product:      "powder",
		NetBase:      &base,
		NetDimension: dimension,
		StockIns:     []time.Time{firstIn},
		StockOuts: []Withdrawal{
			{At: firstIn.Add(30 * 24 * time.Hour), Qty: 1},
			{At: firstIn.Add(60 * 24 * time.Hour), Qty: 1},
		},
	}}
	phase := Phase{started: started}

	got := usageFromMembers(now, phase, sized)
	if got == nil || got.PerMonth != 16 || got.Unit != "oz" {
		t.Fatalf("usage = %+v, want 16 oz per month", got)
	}

	counted := []Fact{{
		Product:   "powder",
		StockIns:  sized[0].StockIns,
		StockOuts: sized[0].StockOuts,
	}}
	if usageFromMembers(now, phase, counted) != nil {
		t.Fatal("an item-count rate is not an amount")
	}
	if usageFromMembers(now, Phase{}, sized) != nil {
		t.Fatal("opening has no measured rate")
	}
	if usageFromMembers(now, phase, nil) != nil {
		t.Fatal("a group with no members has no rate")
	}
}
