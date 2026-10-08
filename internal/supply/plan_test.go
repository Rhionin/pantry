package supply

import (
	"fmt"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
)

func TestPlanManyGroupsIsFast(t *testing.T) {
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	base := 100.0
	facts := make([]Fact, 200)
	for i := range facts {
		facts[i] = Fact{
			Product: ProductID(fmt.Sprintf("p%03d", i)),
			Name:    "Item",
			OnHand:  1,
			ItemID:  fmt.Sprintf("item-%03d", i),
		}
		if i < 160 {
			facts[i].Group = GroupID(fmt.Sprintf("g%02d", i/4))
			facts[i].Rule = "same_as_ran_out"
			facts[i].GroupHasQty = true
			facts[i].GroupBase = 1000
			facts[i].GroupDimension = product.DimensionMass
			facts[i].NetBase = &base
			facts[i].NetDimension = product.DimensionMass
			continue
		}
		override, err := Quantity(2)
		if err != nil {
			t.Fatal(err)
		}
		facts[i].Override = override
	}
	phase := Phase{started: now.Add(-24 * time.Hour)}
	start := time.Now()
	lines := plan(now, phase, 3, facts)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("planned 200 products in %s", elapsed)
	}
	if len(lines) != 80 {
		t.Fatalf("lines = %d, want 40 groups plus 40 ungrouped products", len(lines))
	}
}

func TestPlanGroupCountsItemsWhenSizesAreMissing(t *testing.T) {
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	started := now.Add(-60 * 24 * time.Hour)
	firstIn := started.Add(24 * time.Hour)
	facts := []Fact{
		{
			Product:   "small",
			Group:     "beans",
			Name:      "Small can",
			Rule:      "same_as_ran_out",
			OnHand:    1,
			ItemID:    "item-small",
			StockIns:  []time.Time{firstIn},
			StockOuts: []Withdrawal{{At: started.Add(20 * 24 * time.Hour), Qty: 1}, {At: started.Add(50 * 24 * time.Hour), Qty: 1}},
		},
		{
			Product:  "large",
			Group:    "beans",
			Name:     "Large can",
			Rule:     "same_as_ran_out",
			OnHand:   1,
			ItemID:   "item-large",
			StockIns: []time.Time{firstIn},
		},
	}
	lines := plan(now, Phase{started: started}, 3, facts)
	if len(lines) != 1 {
		t.Fatalf("lines: %#v", lines)
	}
	if lines[0].GroupID != "beans" {
		t.Fatalf("group = %q", lines[0].GroupID)
	}
	if lines[0].Note == "" || !contains(lines[0].Note, "Some sizes aren't known, so this counts items.") {
		t.Fatalf("note = %q", lines[0].Note)
	}
}

func contains(s, part string) bool {
	return len(s) >= len(part) && (s == part || len(part) == 0 || (len(s) > 0 && (stringIndex(s, part) >= 0)))
}

func stringIndex(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}
