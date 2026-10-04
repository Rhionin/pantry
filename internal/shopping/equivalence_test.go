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
