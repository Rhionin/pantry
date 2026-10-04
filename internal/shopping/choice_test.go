package shopping

import (
	"errors"
	"testing"
)

func cents(n int) *int { return &n }

func beans(id, name string, target, count int) ReplenishmentItem {
	return ReplenishmentItem{
		ItemID: id, Name: name, UnitOfMeasure: "can",
		HasTarget: true, TargetQuantity: target, CurrentCount: count,
	}
}

func TestConsiderationsForLines(t *testing.T) {
	items := []ReplenishmentItem{
		beans("gv", "Great Value Cut Green Beans", 4, 1),
		beans("kr", "Kroger Cut Green Beans", 4, 1),
		beans("wf", "Western Family Cut Green Beans", 4, 1),
		beans("dm", "Del Monte Cut Green Beans", 4, 1),
	}
	key, ok := NeedKey("Great Value Cut Green Beans", "can")
	if !ok {
		t.Fatal("expected a need key")
	}

	t.Run("cheaper sale is offered and a higher sale is not", func(t *testing.T) {
		deals := []Deal{
			{ItemID: "kr", PriceCents: cents(79), Label: "Weekly ad", Source: DealSourceRecorded},
			{ItemID: "wf", PriceCents: cents(200), RegularPriceCents: cents(210), Label: "Not cheaper", Source: DealSourceRecorded},
		}
		// Usual brand also has a price so the higher sale can be compared.
		deals = append(deals, Deal{ItemID: "gv", PriceCents: cents(125), Source: DealSourceRecorded})
		notes := ConsiderationsForLines([]string{"gv", "dm"}, items, deals, nil)
		if len(notes) != 1 {
			t.Fatalf("notes = %d, want 1 (Del Monte is its own product)", len(notes))
		}
		if notes[0].Offer == nil || notes[0].Offer.ItemID != "kr" || notes[0].Offer.Label != "Weekly ad" {
			t.Fatalf("offer = %+v, want kroger weekly ad", notes[0].Offer)
		}
		if notes[0].Offer.UsualPriceCents == nil || *notes[0].Offer.UsualPriceCents != 125 {
			t.Fatalf("usual price = %+v", notes[0].Offer.UsualPriceCents)
		}
	})

	t.Run("label-only sale is offered when the usual brand is not on sale", func(t *testing.T) {
		notes := ConsiderationsForLines([]string{"gv"}, items, []Deal{
			{ItemID: "kr", Label: "Manager special", Source: DealSourceRecorded},
		}, nil)
		if len(notes) != 1 || notes[0].Offer == nil || notes[0].Offer.ItemID != "kr" {
			t.Fatalf("offer = %+v", notes[0].Offer)
		}
	})

	t.Run("ignore price suppresses the cheaper brand", func(t *testing.T) {
		notes := ConsiderationsForLines([]string{"kr"}, items, []Deal{
			{ItemID: "gv", PriceCents: cents(50), Label: "Sale", Source: DealSourceRecorded},
		}, []Preference{{NeedKey: key, ItemID: "kr", IgnorePrice: true}})
		if len(notes) != 1 {
			t.Fatalf("notes = %d", len(notes))
		}
		if !notes[0].IgnorePrice || notes[0].PreferredItemID != "kr" || notes[0].Offer != nil {
			t.Fatalf("note = %+v", notes[0])
		}
	})

	t.Run("two sales pick the lower price", func(t *testing.T) {
		notes := ConsiderationsForLines([]string{"gv"}, items, []Deal{
			{ItemID: "kr", PriceCents: cents(90), Label: "Sale", Source: DealSourceRecorded},
			{ItemID: "wf", PriceCents: cents(70), Label: "Sale", Source: DealSourceRecorded},
		}, nil)
		if notes[0].Offer == nil || notes[0].Offer.ItemID != "wf" {
			t.Fatalf("offer = %+v, want western family", notes[0].Offer)
		}
	})
}

func TestSubstituteBrand(t *testing.T) {
	items := []ReplenishmentItem{
		beans("gv", "Great Value Cut Green Beans", 4, 1),
		beans("kr", "Kroger Cut Green Beans", 4, 1),
		beans("corn", "Kroger Whole Kernel Corn", 2, 0),
	}
	got, err := SubstituteBrand("gv", "kr", items)
	if err != nil || got != "kr" {
		t.Fatalf("SubstituteBrand same need = %q, %v", got, err)
	}
	if _, err := SubstituteBrand("gv", "corn", items); !errors.Is(err, ErrDifferentProduct) {
		t.Fatalf("different product err = %v", err)
	}
	got, err = SubstituteBrand("gv", "", items)
	if err != nil || got != "gv" {
		t.Fatalf("empty substitution = %q, %v", got, err)
	}
}

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
}
