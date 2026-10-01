package shopping

import "testing"

func TestGenericProductName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "store brand prefix", in: "Kroger Cut Green Beans", want: "cut green beans"},
		{name: "store brand prefix different brand", in: "Great Value Cut Green Beans", want: "cut green beans"},
		{name: "store brand with trademark", in: "Kroger® Cut Green Beans", want: "cut green beans"},
		{name: "case and extra space", in: "  GREAT   VALUE cut green beans ", want: "cut green beans"},
		{name: "brand suffix", in: "Cut Green Beans, Western Family", want: "cut green beans"},
		{name: "ampersand brand", in: "Good & Gather Cut Green Beans", want: "cut green beans"},
		{name: "apostrophe brand", in: "Member's Mark Cut Green Beans", want: "cut green beans"},
		{name: "longest private label wins", in: "Kirkland Signature Cut Green Beans", want: "cut green beans"},
		{name: "stacked store brands", in: "Kroger Simple Truth Cut Green Beans", want: "cut green beans"},
		{name: "whole foods private label", in: "365 by Whole Foods Market Cut Green Beans", want: "cut green beans"},
		{name: "already generic", in: "Cut Green Beans", want: "cut green beans"},
		{name: "national brand stays", in: "Del Monte Cut Green Beans", want: "del monte cut green beans"},
		{name: "organic stays", in: "Kroger Organic Cut Green Beans", want: "organic cut green beans"},
		{name: "style qualifier stays", in: "Kroger French Style Green Beans", want: "french style green beans"},
		{name: "pack size stays", in: "Great Value Cut Green Beans 14.5 oz", want: "cut green beans 14 5 oz"},
		{name: "brand only", in: "Kroger", want: ""},
		{name: "empty", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GenericProductName(tt.in); got != tt.want {
				t.Errorf("GenericProductName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGenericProductName_EveryStoreBrandStripsFromCommodity(t *testing.T) {
	if len(storeBrandNames) == 0 {
		t.Fatal("store brand list is empty")
	}
	for _, brand := range storeBrandNames {
		name := brand + " Cut Green Beans"
		got := GenericProductName(name)
		if got != "cut green beans" {
			t.Errorf("GenericProductName(%q) = %q, want %q", name, got, "cut green beans")
		}
	}
}

func TestCollapseEquivalentNeeds(t *testing.T) {
	green := func(id, name string, target, count int) ReplenishmentItem {
		return ReplenishmentItem{
			ItemID: id, Name: name, UnitOfMeasure: "can",
			HasTarget: true, TargetQuantity: target, CurrentCount: count,
		}
	}

	tests := []struct {
		name   string
		items  []ReplenishmentItem
		manual map[string]struct{}
		want   []DeriveInput
	}{
		{
			name: "three store brands are one supply",
			items: []ReplenishmentItem{
				green("gv", "Great Value Cut Green Beans", 4, 1),
				green("kr", "Kroger Cut Green Beans", 4, 0),
				green("wf", "Western Family Cut Green Beans", 4, 2),
			},
			want: []DeriveInput{{ItemID: "gv", TargetQuantity: 4, CurrentCount: 3}},
		},
		{
			name: "highest target names the line",
			items: []ReplenishmentItem{
				green("gv", "Great Value Cut Green Beans", 4, 1),
				green("kr", "Kroger Cut Green Beans", 6, 1),
			},
			want: []DeriveInput{{ItemID: "kr", TargetQuantity: 6, CurrentCount: 2}},
		},
		{
			name: "different commodities stay separate",
			items: []ReplenishmentItem{
				green("beans", "Kroger Cut Green Beans", 4, 1),
				green("corn", "Kroger Whole Kernel Corn", 2, 0),
			},
			want: []DeriveInput{
				{ItemID: "beans", TargetQuantity: 4, CurrentCount: 1},
				{ItemID: "corn", TargetQuantity: 2, CurrentCount: 0},
			},
		},
		{
			name: "unlisted national brand stays separate",
			items: []ReplenishmentItem{
				green("dm", "Del Monte Cut Green Beans", 4, 0),
				green("kr", "Kroger Cut Green Beans", 4, 0),
			},
			want: []DeriveInput{
				{ItemID: "dm", TargetQuantity: 4, CurrentCount: 0},
				{ItemID: "kr", TargetQuantity: 4, CurrentCount: 0},
			},
		},
		{
			name: "organic does not match conventional",
			items: []ReplenishmentItem{
				green("org", "Kroger Organic Cut Green Beans", 4, 0),
				green("reg", "Great Value Cut Green Beans", 4, 0),
			},
			want: []DeriveInput{
				{ItemID: "org", TargetQuantity: 4, CurrentCount: 0},
				{ItemID: "reg", TargetQuantity: 4, CurrentCount: 0},
			},
		},
		{
			name: "pack size stays distinct",
			items: []ReplenishmentItem{
				green("small", "Kroger Cut Green Beans 14.5 oz", 4, 0),
				green("large", "Great Value Cut Green Beans 28 oz", 4, 0),
			},
			want: []DeriveInput{
				{ItemID: "small", TargetQuantity: 4, CurrentCount: 0},
				{ItemID: "large", TargetQuantity: 4, CurrentCount: 0},
			},
		},
		{
			name: "same pack size text matches across brands",
			items: []ReplenishmentItem{
				green("kr", "Kroger Cut Green Beans 14.5 oz", 4, 1),
				green("gv", "Great Value Cut Green Beans 14.5 oz", 4, 1),
			},
			want: []DeriveInput{{ItemID: "gv", TargetQuantity: 4, CurrentCount: 2}},
		},
		{
			name: "unbranded name matches a store brand",
			items: []ReplenishmentItem{
				green("plain", "Cut Green Beans", 3, 1),
				green("kr", "Kroger Cut Green Beans", 3, 2),
			},
			want: []DeriveInput{{ItemID: "plain", TargetQuantity: 3, CurrentCount: 3}},
		},
		{
			name: "stock without a target still counts",
			items: []ReplenishmentItem{
				{ItemID: "gv", Name: "Great Value Cut Green Beans", UnitOfMeasure: "can", CurrentCount: 3},
				green("kr", "Kroger Cut Green Beans", 4, 0),
			},
			want: []DeriveInput{{ItemID: "kr", TargetQuantity: 4, CurrentCount: 3}},
		},
		{
			name: "no targets produces nothing",
			items: []ReplenishmentItem{
				{ItemID: "gv", Name: "Great Value Cut Green Beans", UnitOfMeasure: "can", CurrentCount: 2},
				{ItemID: "kr", Name: "Kroger Cut Green Beans", UnitOfMeasure: "can", CurrentCount: 1},
			},
			want: []DeriveInput{},
		},
		{
			name: "on hand covers the shared target",
			items: []ReplenishmentItem{
				green("gv", "Great Value Cut Green Beans", 4, 2),
				green("kr", "Kroger Cut Green Beans", 4, 2),
			},
			want: []DeriveInput{{ItemID: "gv", TargetQuantity: 4, CurrentCount: 4}},
		},
		{
			name: "different units stay separate",
			items: []ReplenishmentItem{
				green("can", "Kroger Cut Green Beans", 4, 0),
				{ItemID: "lb", Name: "Great Value Cut Green Beans", UnitOfMeasure: "lb", HasTarget: true, TargetQuantity: 4},
			},
			want: []DeriveInput{
				{ItemID: "can", TargetQuantity: 4, CurrentCount: 0},
				{ItemID: "lb", TargetQuantity: 4, CurrentCount: 0},
			},
		},
		{
			name: "unit capitalization still matches",
			items: []ReplenishmentItem{
				green("kr", "Kroger Cut Green Beans", 4, 1),
				{ItemID: "gv", Name: "Great Value Cut Green Beans", UnitOfMeasure: "Can", HasTarget: true, TargetQuantity: 4, CurrentCount: 1},
			},
			want: []DeriveInput{{ItemID: "gv", TargetQuantity: 4, CurrentCount: 2}},
		},
		{
			name: "blank unit does not match a specified unit",
			items: []ReplenishmentItem{
				{ItemID: "blank", Name: "Kroger Cut Green Beans", HasTarget: true, TargetQuantity: 4},
				green("can", "Great Value Cut Green Beans", 4, 0),
			},
			want: []DeriveInput{
				{ItemID: "blank", TargetQuantity: 4, CurrentCount: 0},
				{ItemID: "can", TargetQuantity: 4, CurrentCount: 0},
			},
		},
		{
			name: "brand-only names are not grouped",
			items: []ReplenishmentItem{
				green("kr", "Kroger", 2, 0),
				green("gv", "Great Value", 2, 0),
			},
			want: []DeriveInput{
				{ItemID: "kr", TargetQuantity: 2, CurrentCount: 0},
				{ItemID: "gv", TargetQuantity: 2, CurrentCount: 0},
			},
		},
		{
			name: "empty names are not grouped",
			items: []ReplenishmentItem{
				{ItemID: "a", HasTarget: true, TargetQuantity: 2},
				{ItemID: "b", HasTarget: true, TargetQuantity: 2},
			},
			want: []DeriveInput{
				{ItemID: "a", TargetQuantity: 2, CurrentCount: 0},
				{ItemID: "b", TargetQuantity: 2, CurrentCount: 0},
			},
		},
		{
			name: "manual line for any brand suppresses the shared auto gap",
			items: []ReplenishmentItem{
				green("gv", "Great Value Cut Green Beans", 4, 0),
				green("kr", "Kroger Cut Green Beans", 4, 0),
				green("corn", "Kroger Whole Kernel Corn", 2, 0),
			},
			manual: map[string]struct{}{"kr": {}},
			want:   []DeriveInput{{ItemID: "corn", TargetQuantity: 2, CurrentCount: 0}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CollapseEquivalentNeeds(tt.items, tt.manual)
			if len(got) != len(tt.want) {
				t.Fatalf("len=%d, want %d\ngot %#v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %#v, want %#v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCollapseEquivalentNeeds_SharedGapFeedsDerive(t *testing.T) {
	collapsed := CollapseEquivalentNeeds([]ReplenishmentItem{
		{ItemID: "gv", Name: "Great Value Cut Green Beans", UnitOfMeasure: "can", HasTarget: true, TargetQuantity: 4, CurrentCount: 1},
		{ItemID: "kr", Name: "Kroger Cut Green Beans", UnitOfMeasure: "can", HasTarget: true, TargetQuantity: 4, CurrentCount: 1},
		{ItemID: "wf", Name: "Western Family Cut Green Beans", UnitOfMeasure: "can", HasTarget: true, TargetQuantity: 4, CurrentCount: 1},
	}, nil)
	got := DeriveShoppingList(collapsed)
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1\ngot %#v", len(got), got)
	}
	if got[0].ItemID != "gv" || got[0].Quantity != 1 || got[0].Source != "auto" {
		t.Errorf("got %#v, want Great Value gap of 1", got[0])
	}
}
