package group

import (
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/shopping"
)

func TestPickSameAsRanOut(t *testing.T) {
	in := Input{Members: []Member{
		{ProductID: "hunts", Name: "Hunt's 14.5 oz", LastConsumedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
		{ProductID: "gv", Name: "Great Value 14.5 oz", LastConsumedAt: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)},
	}}
	got, err := Pick(KindSameAsRanOut, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductID != "gv" || got.Because != "The last one used up was Great Value 14.5 oz." {
		t.Fatalf("%+v", got)
	}
}

func TestPickStockedWhenNothingRanOut(t *testing.T) {
	in := Input{Members: []Member{
		{ProductID: "a", Name: "A", LastStockedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ProductID: "b", Name: "B", LastStockedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
	}}
	got, err := Pick(KindSameAsRanOut, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductID != "b" || got.Because != "Nothing has run out yet. This is the one you stocked last." {
		t.Fatalf("%+v", got)
	}
}

func TestPickLowestIDWhenNothingHappened(t *testing.T) {
	got, err := Pick(KindSameAsRanOut, Input{Members: []Member{{ProductID: "b", Name: "B"}, {ProductID: "a", Name: "A"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductID != "a" || got.Because != "Nothing has run out yet." {
		t.Fatalf("%+v", got)
	}
}

func TestPickFavoriteAndMissingPin(t *testing.T) {
	in := Input{
		PinnedProductID: "a",
		Members: []Member{
			{ProductID: "a", Name: "Starred"},
			{ProductID: "b", Name: "Other", LastConsumedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
		},
	}
	got, err := Pick(KindFavorite, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Because != "This is the one with the star." || got.ProductID != "a" {
		t.Fatalf("%+v", got)
	}
	in.PinnedProductID = ""
	got, err = Pick(KindFavorite, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductID != "b" || got.Because != "No favorite is set. The last one used up was Other." {
		t.Fatalf("%+v", got)
	}
}

func TestPickBestDealSale(t *testing.T) {
	price, regular := 59, 89
	in := Input{
		Members: []Member{{ProductID: "gv", Name: "Great Value 14.5 oz"}},
		Deals: []shopping.Deal{{
			ItemID:            "gv",
			PriceCents:        &price,
			RegularPriceCents: &regular,
			NotedAt:           time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC),
		}},
	}
	got, err := Pick(KindBestDeal, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Because != "On sale for $0.59, usually $0.89. You noted this sale on Oct 5." {
		t.Fatalf("%+v", got)
	}
}

func TestPickBestDealPerOunce(t *testing.T) {
	small, _, err := product.BaseFromAmount(14.5, "oz")
	if err != nil {
		t.Fatal(err)
	}
	large, _, err := product.BaseFromAmount(29, "oz")
	if err != nil {
		t.Fatal(err)
	}
	cheapCan, dearCan := 80, 100
	in := Input{Members: []Member{
		{ProductID: "small", Name: "Small", NetBase: &small, Dimension: product.DimensionMass},
		{ProductID: "large", Name: "Large", NetBase: &large, Dimension: product.DimensionMass},
	}, Deals: []shopping.Deal{
		{ItemID: "small", PriceCents: &cheapCan, RegularPriceCents: intPtr(120)},
		{ItemID: "large", PriceCents: &dearCan, RegularPriceCents: intPtr(180)},
	}}
	got, err := Pick(KindBestDeal, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductID != "large" || got.ComparedPerItem {
		t.Fatalf("%+v", got)
	}
}

func TestPickBestDealPerItemWhenDimensionsDiffer(t *testing.T) {
	mass, _, err := product.BaseFromAmount(14.5, "oz")
	if err != nil {
		t.Fatal(err)
	}
	vol, _, err := product.BaseFromAmount(12, "fl oz")
	if err != nil {
		t.Fatal(err)
	}
	massPrice, volPrice := 50, 40
	in := Input{Members: []Member{
		{ProductID: "mass", Name: "Beans", NetBase: &mass, Dimension: product.DimensionMass},
		{ProductID: "vol", Name: "Seltzer", NetBase: &vol, Dimension: product.DimensionVolume},
	}, Deals: []shopping.Deal{
		{ItemID: "mass", PriceCents: &massPrice},
		{ItemID: "vol", PriceCents: &volPrice},
	}}
	got, err := Pick(KindBestDeal, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductID != "vol" || !got.ComparedPerItem {
		t.Fatalf("%+v", got)
	}
}

func TestPickBestDealLabelLosesToAPrice(t *testing.T) {
	price := 59
	in := Input{Members: []Member{
		{ProductID: "label", Name: "Label"},
		{ProductID: "priced", Name: "Priced"},
	}, Deals: []shopping.Deal{
		{ItemID: "label", Label: "Weekly ad"},
		{ItemID: "priced", PriceCents: &price},
	}}
	got, err := Pick(KindBestDeal, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductID != "priced" {
		t.Fatalf("%+v", got)
	}
}

func TestPickBestDealNothingOnSale(t *testing.T) {
	in := Input{
		PinnedProductID: "gv",
		Members:         []Member{{ProductID: "gv", Name: "Great Value 14.5 oz"}, {ProductID: "a", Name: "Other"}},
	}
	got, err := Pick(KindBestDeal, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Because != "Nothing is on sale. Otherwise buy Great Value 14.5 oz." {
		t.Fatalf("%+v", got)
	}
	in.PinnedProductID = ""
	got, err = Pick(KindBestDeal, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Because != "Nothing is on sale and no fallback is set. Nothing has run out yet." || got.ProductID != "a" {
		t.Fatalf("%+v", got)
	}
}

func TestPickUnknownRule(t *testing.T) {
	if _, err := Pick("nope", Input{}); err == nil {
		t.Fatal("expected an error")
	}
}

func intPtr(n int) *int { return &n }
