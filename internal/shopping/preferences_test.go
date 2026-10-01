package shopping_test

import (
	"context"
	"testing"

	"github.com/Rhionin/pantry/internal/shopping"
)

func TestBrandPreferenceRoundTrip(t *testing.T) {
	deps := newTestStore(t)
	ctx := context.Background()
	gv := createTestItem(t, deps, ctx, "user-1", "prod-gv", "Great Value Cut Green Beans")
	kr := createTestItem(t, deps, ctx, "user-1", "prod-kr", "Kroger Cut Green Beans")

	if err := deps.shopping.SavePreference(ctx, "user-1", shopping.Preference{
		NeedKey: "cut green beans\x00can", ItemID: gv,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := deps.shopping.SavePreference(ctx, "user-1", shopping.Preference{
		NeedKey: "cut green beans\x00can", ItemID: kr, IgnorePrice: true,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	prefs, err := deps.shopping.ListPreferences(ctx, "user-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(prefs) != 1 || prefs[0].ItemID != kr || !prefs[0].IgnorePrice {
		t.Fatalf("prefs = %+v", prefs)
	}
	if err := deps.shopping.DeletePreference(ctx, "user-1", "cut green beans\x00can"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	prefs, err = deps.shopping.ListPreferences(ctx, "user-1")
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(prefs) != 0 {
		t.Fatalf("prefs after delete = %+v", prefs)
	}
}

func TestDealRoundTrip(t *testing.T) {
	deps := newTestStore(t)
	ctx := context.Background()
	kr := createTestItem(t, deps, ctx, "user-1", "prod-kr-deal", "Kroger Cut Green Beans")
	price := 79
	regular := 125
	if err := deps.shopping.SaveDeal(ctx, "user-1", shopping.Deal{
		ItemID: kr, PriceCents: &price, RegularPriceCents: &regular, Label: "Weekly ad",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	price = 69
	if err := deps.shopping.SaveDeal(ctx, "user-1", shopping.Deal{
		ItemID: kr, PriceCents: &price, Label: "Weekly ad",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	deals, err := deps.shopping.ListDeals(ctx, "user-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(deals) != 1 || deals[0].PriceCents == nil || *deals[0].PriceCents != 69 || deals[0].RegularPriceCents != nil {
		t.Fatalf("deals = %+v", deals)
	}
	if deals[0].Source != shopping.DealSourceRecorded {
		t.Fatalf("source = %q", deals[0].Source)
	}
	if err := deps.shopping.DeleteDeal(ctx, "user-1", kr); err != nil {
		t.Fatalf("delete: %v", err)
	}
	deals, err = deps.shopping.ListDeals(ctx, "user-1")
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(deals) != 0 {
		t.Fatalf("deals after delete = %+v", deals)
	}
}
