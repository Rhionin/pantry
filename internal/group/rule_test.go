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

func TestPickFavorVarietyLeastRecentlyBought(t *testing.T) {
	early := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	in := Input{Members: []Member{
		{ProductID: "late", Name: "Blueberry", LastStockedAt: late},
		{ProductID: "early", Name: "Bran", LastStockedAt: early},
		{ProductID: "never", Name: "Corn"},
	}}
	got, err := Pick(KindFavorVariety, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductID != "never" || got.Because != "Next up: Corn · rotates through 3" {
		t.Fatalf("%+v", got)
	}
	in.Members = in.Members[:2]
	got, err = Pick(KindFavorVariety, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductID != "early" || got.Because != "Next up: Bran · rotates through 2" {
		t.Fatalf("%+v", got)
	}
}

func TestPickFavorVarietyTieBreaksByNameThenID(t *testing.T) {
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got, err := Pick(KindFavorVariety, Input{Members: []Member{
		{ProductID: "b", Name: "Zucchini"},
		{ProductID: "a", Name: "Apple"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductID != "a" {
		t.Fatalf("%+v", got)
	}
	got, err = Pick(KindFavorVariety, Input{Members: []Member{
		{ProductID: "b", Name: "Same", LastStockedAt: when},
		{ProductID: "a", Name: "Same", LastStockedAt: when},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductID != "a" || got.Because != "Next up: Same · rotates through 2" {
		t.Fatalf("%+v", got)
	}
}

func TestPickSkipsNoRestockForEveryRule(t *testing.T) {
	early := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	members := []Member{
		{ProductID: "fancy", Name: "Fancy", NoRestock: true, LastConsumedAt: late, LastStockedAt: early},
		{ProductID: "plain", Name: "Plain", LastConsumedAt: early, LastStockedAt: late},
	}
	price := 40
	deals := []shopping.Deal{{ItemID: "fancy", PriceCents: &price, RegularPriceCents: intPtr(80)}}

	same, err := Pick(KindSameAsRanOut, Input{Members: members})
	if err != nil {
		t.Fatal(err)
	}
	if same.ProductID != "plain" || same.Because != "Fancy isn't restocked. The last one used up was Plain." {
		t.Fatalf("same %+v", same)
	}

	favorite, err := Pick(KindFavorite, Input{Members: members, PinnedProductID: "fancy"})
	if err != nil {
		t.Fatal(err)
	}
	if favorite.ProductID != "plain" || favorite.Because != "Fancy isn't restocked. The last one used up was Plain." {
		t.Fatalf("favorite %+v", favorite)
	}

	deal, err := Pick(KindBestDeal, Input{Members: members, PinnedProductID: "fancy", Deals: deals})
	if err != nil {
		t.Fatal(err)
	}
	if deal.ProductID != "plain" || deal.Because != "Fancy isn't restocked. Nothing is on sale and no fallback is set. The last one used up was Plain." {
		t.Fatalf("deal %+v", deal)
	}

	variety, err := Pick(KindFavorVariety, Input{Members: members})
	if err != nil {
		t.Fatal(err)
	}
	if variety.ProductID != "plain" || variety.Because != "Next up: Plain · rotates through 1" {
		t.Fatalf("variety %+v", variety)
	}
}

func TestPickBestDealSaleSkipsNoRestock(t *testing.T) {
	fancy, plain := 30, 80
	in := Input{
		Members: []Member{
			{ProductID: "fancy", Name: "Fancy", NoRestock: true},
			{ProductID: "plain", Name: "Plain"},
		},
		Deals: []shopping.Deal{
			{ItemID: "fancy", PriceCents: &fancy, RegularPriceCents: intPtr(90)},
			{ItemID: "plain", PriceCents: &plain, RegularPriceCents: intPtr(100)},
		},
	}
	got, err := Pick(KindBestDeal, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductID != "plain" || got.Because != "On sale for $0.80, usually $1.00." {
		t.Fatalf("%+v", got)
	}
}

func TestPickAllNoRestockBuysNothing(t *testing.T) {
	members := []Member{
		{ProductID: "a", Name: "A", NoRestock: true, LastStockedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ProductID: "b", Name: "B", NoRestock: true},
	}
	for _, kind := range []Kind{KindSameAsRanOut, KindFavorite, KindBestDeal, KindFavorVariety} {
		got, err := Pick(kind, Input{Members: members, PinnedProductID: "a"})
		if err != nil {
			t.Fatal(err)
		}
		if got.ProductID != "" || got.Because != everyNoRestock {
			t.Fatalf("%s %+v", kind, got)
		}
	}
}

func TestPickUnknownRule(t *testing.T) {
	if _, err := Pick("nope", Input{}); err == nil {
		t.Fatal("expected an error")
	}
}

func intPtr(n int) *int { return &n }
