package group_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/group"
	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/supply"
)

func TestBuyCountOunceTargetCountsOunces(t *testing.T) {
	small, _, err := product.BaseFromAmount(14.5, "oz")
	if err != nil {
		t.Fatal(err)
	}
	large, _, err := product.BaseFromAmount(29, "oz")
	if err != nil {
		t.Fatal(err)
	}
	goal, _, err := product.BaseFromAmount(24, "oz")
	if err != nil {
		t.Fatal(err)
	}
	members := []group.Member{
		{ProductID: "small", Name: "14.5 oz", NetBase: &small, Dimension: product.DimensionMass, OnHand: 1},
		{ProductID: "large", Name: "29 oz", NetBase: &large, Dimension: product.DimensionMass, OnHand: 0},
	}
	target := group.BuyTarget{HasQuantity: true, Base: goal, Dimension: product.DimensionMass}
	buy, explain := group.BuyCount(target, members, "large", group.UsageRate{})
	if buy != 1 {
		t.Fatalf("buy %d (%s)", buy, explain)
	}
	if explain != "14.5 oz toward 24 oz." {
		t.Fatalf("explain %q", explain)
	}
}

func TestBuyCountMissingSizeDoesNotCountItems(t *testing.T) {
	goal, _, err := product.BaseFromAmount(24, "oz")
	if err != nil {
		t.Fatal(err)
	}
	members := []group.Member{
		{ProductID: "bare", Name: "Can", OnHand: 1},
		{ProductID: "other", Name: "Other", OnHand: 0},
	}
	buy, explain := group.BuyCount(group.BuyTarget{HasQuantity: true, Base: goal, Dimension: product.DimensionMass}, members, "bare", group.UsageRate{})
	if buy != 0 {
		t.Fatalf("buy %d", buy)
	}
	if !strings.Contains(explain, "can't be counted") {
		t.Fatalf("explain %q", explain)
	}
}

func TestBuyCountMonthWindowUsesDeriveGates(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	base, _, err := product.BaseFromAmount(14.5, "oz")
	if err != nil {
		t.Fatal(err)
	}
	members := []group.Member{{ProductID: "small", NetBase: &base, Dimension: product.DimensionMass}}
	startedSoon := now.Add(-10 * 24 * time.Hour)
	if _, ok := supply.DeriveAmount(now, startedSoon, []time.Time{startedSoon}, nil); ok {
		t.Fatal("expected the opening gate to fail")
	}
	buy, _ := group.BuyCount(group.BuyTarget{WindowMonths: 3}, members, "small", group.UsageRate{})
	if buy != 0 {
		t.Fatalf("opening buy %d", buy)
	}

	started := now.Add(-100 * 24 * time.Hour)
	first := started.Add(24 * time.Hour)
	outs := []supply.AmountWithdrawal{
		{At: first.Add(10 * 24 * time.Hour), Qty: base},
		{At: first.Add(40 * 24 * time.Hour), Qty: base},
	}
	per, ok := supply.DeriveAmount(now, started, []time.Time{first}, outs)
	if !ok {
		t.Fatal("expected a rate")
	}
	buy, explain := group.BuyCount(group.BuyTarget{WindowMonths: 3}, members, "small", group.UsageRate{PerDay: per, OK: true})
	if buy != 3 {
		t.Fatalf("buy %d explain %q per %v", buy, explain, per)
	}

	itemPer, ok := supply.DeriveAmount(now, started, []time.Time{first}, []supply.AmountWithdrawal{
		{At: first.Add(10 * 24 * time.Hour), Qty: 1},
		{At: first.Add(40 * 24 * time.Hour), Qty: 1},
	})
	if !ok {
		t.Fatal("expected an item rate")
	}
	unsized := []group.Member{{ProductID: "small", OnHand: 0}}
	buy, explain = group.BuyCount(group.BuyTarget{WindowMonths: 3}, unsized, "small", group.UsageRate{PerDay: itemPer, OK: true, ItemCount: true})
	if buy != 3 || !strings.Contains(explain, "counts items") {
		t.Fatalf("item buy %d explain %q", buy, explain)
	}
}
