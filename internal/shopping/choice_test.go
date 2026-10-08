package shopping

import (
	"errors"
	"testing"
)

func cents(n int) *int { return &n }

func TestCombineDeals(t *testing.T) {
	recorded := []Deal{{ItemID: "gv", PriceCents: cents(80), Source: DealSourceRecorded}}
	live := []Deal{
		{ItemID: "gv", PriceCents: cents(10), Source: DealSourceRetailer},
		{ItemID: "kr", PriceCents: cents(70), Source: DealSourceRetailer},
	}
	merged := MergeDeals(recorded, live)
	if len(merged) != 2 || merged[0].ItemID != "gv" || *merged[0].PriceCents != 80 {
		t.Fatalf("merge = %+v, recorded price should win", merged)
	}
	if merged[1].ItemID != "kr" {
		t.Fatalf("live-only deal missing: %+v", merged)
	}
	if MergeDeals([]Deal{{ItemID: ""}}, []Deal{{ItemID: ""}}) != nil {
		t.Fatal("deals without an item are ignored")
	}
	fellBack := CombineDeals(recorded, live, errors.New("store down"))
	if len(fellBack) != 1 || fellBack[0].ItemID != "gv" {
		t.Fatalf("outage = %+v, want recorded only", fellBack)
	}
}

func TestDealOnSale(t *testing.T) {
	labeled := Deal{Label: "Weekly ad"}
	if !labeled.OnSale() {
		t.Fatal("label should count as a sale")
	}
	priced := Deal{PriceCents: cents(79)}
	if !priced.OnSale() {
		t.Fatal("a noted price should count as a sale")
	}
	higher := Deal{PriceCents: cents(100), RegularPriceCents: cents(90)}
	if higher.OnSale() {
		t.Fatal("a higher price without a label is not a sale")
	}
	if (Deal{}).OnSale() {
		t.Fatal("an empty note is not a sale")
	}
	lower := Deal{PriceCents: cents(70), RegularPriceCents: cents(90)}
	if !lower.OnSale() {
		t.Fatal("a price below the regular price is a sale")
	}
}
