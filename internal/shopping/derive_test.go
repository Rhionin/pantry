package shopping

import (
	"testing"
)

// TestDeriveShoppingList covers the core derivation logic: gap computation,
// at-target exclusion, over-target exclusion, and empty input.
func TestDeriveShoppingList(t *testing.T) {
	tests := []struct {
		name  string
		input []DeriveInput
		want  []DerivedEntry
	}{
		{
			name:  "empty input",
			input: nil,
			want:  []DerivedEntry{},
		},
		{
			name: "all items at or above target are excluded",
			input: []DeriveInput{
				{ItemID: "a", TargetQuantity: 3, CurrentCount: 3},
				{ItemID: "b", TargetQuantity: 2, CurrentCount: 5},
			},
			want: []DerivedEntry{},
		},
		{
			name: "item below target produces correct gap quantity",
			input: []DeriveInput{
				{ItemID: "a", TargetQuantity: 5, CurrentCount: 2},
			},
			want: []DerivedEntry{
				{ItemID: "a", Quantity: 3, Source: "auto"},
			},
		},
		{
			name: "zero current count produces full target quantity",
			input: []DeriveInput{
				{ItemID: "a", TargetQuantity: 4, CurrentCount: 0},
			},
			want: []DerivedEntry{
				{ItemID: "a", Quantity: 4, Source: "auto"},
			},
		},
		{
			name: "mixed: below and at target",
			input: []DeriveInput{
				{ItemID: "needs", TargetQuantity: 6, CurrentCount: 4},
				{ItemID: "full",  TargetQuantity: 3, CurrentCount: 3},
				{ItemID: "over",  TargetQuantity: 2, CurrentCount: 7},
			},
			want: []DerivedEntry{
				{ItemID: "needs", Quantity: 2, Source: "auto"},
			},
		},
		{
			name: "source field is always auto",
			input: []DeriveInput{
				{ItemID: "x", TargetQuantity: 1, CurrentCount: 0},
			},
			want: []DerivedEntry{
				{ItemID: "x", Quantity: 1, Source: "auto"},
			},
		},
		{
			name: "target quantity of 1 with zero stock",
			input: []DeriveInput{
				{ItemID: "single", TargetQuantity: 1, CurrentCount: 0},
			},
			want: []DerivedEntry{
				{ItemID: "single", Quantity: 1, Source: "auto"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveShoppingList(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len(got)=%d, len(want)=%d\ngot:  %+v\nwant: %+v",
					len(got), len(tt.want), got, tt.want)
			}
			// Build a map for order-independent comparison.
			byID := make(map[string]DerivedEntry, len(got))
			for _, e := range got {
				byID[e.ItemID] = e
			}
			for _, w := range tt.want {
				g, ok := byID[w.ItemID]
				if !ok {
					t.Errorf("missing entry for ItemID %q", w.ItemID)
					continue
				}
				if g.Quantity != w.Quantity {
					t.Errorf("ItemID %q: Quantity got %d, want %d", w.ItemID, g.Quantity, w.Quantity)
				}
				if g.Source != w.Source {
					t.Errorf("ItemID %q: Source got %q, want %q", w.ItemID, g.Source, w.Source)
				}
			}
		})
	}
}

// TestMergeEntries covers the merge logic: manual overrides derived for the
// same ItemID; derived entries without a manual counterpart are included.
func TestMergeEntries(t *testing.T) {
	tests := []struct {
		name    string
		derived []DerivedEntry
		manual  []ManualEntry
		wantLen int
		// wantByID maps ItemID → expected quantity in merged result
		wantByID map[string]int
	}{
		{
			name:     "empty derived and manual",
			derived:  nil,
			manual:   nil,
			wantLen:  0,
			wantByID: map[string]int{},
		},
		{
			name: "derived only — all included",
			derived: []DerivedEntry{
				{ItemID: "a", Quantity: 2, Source: "auto"},
				{ItemID: "b", Quantity: 1, Source: "auto"},
			},
			manual:  nil,
			wantLen: 2,
			wantByID: map[string]int{"a": 2, "b": 1},
		},
		{
			name:    "manual only — all included",
			derived: nil,
			manual: []ManualEntry{
				{ItemID: "a", Quantity: 5},
			},
			wantLen: 1,
			wantByID: map[string]int{"a": 5},
		},
		{
			name: "manual overrides derived for same item",
			derived: []DerivedEntry{
				{ItemID: "a", Quantity: 2, Source: "auto"},
			},
			manual: []ManualEntry{
				{ItemID: "a", Quantity: 10},
			},
			wantLen:  1,
			wantByID: map[string]int{"a": 10},
		},
		{
			name: "non-overlapping derived and manual are both included",
			derived: []DerivedEntry{
				{ItemID: "auto-item", Quantity: 3, Source: "auto"},
			},
			manual: []ManualEntry{
				{ItemID: "manual-item", Quantity: 7},
			},
			wantLen:  2,
			wantByID: map[string]int{"auto-item": 3, "manual-item": 7},
		},
		{
			name: "partial overlap: one shared item, one unique each",
			derived: []DerivedEntry{
				{ItemID: "shared", Quantity: 2, Source: "auto"},
				{ItemID: "only-derived", Quantity: 4, Source: "auto"},
			},
			manual: []ManualEntry{
				{ItemID: "shared", Quantity: 9},
				{ItemID: "only-manual", Quantity: 1},
			},
			wantLen:  3,
			wantByID: map[string]int{"shared": 9, "only-derived": 4, "only-manual": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeEntries(tt.derived, tt.manual)
			if len(got) != tt.wantLen {
				t.Fatalf("len(got)=%d, want %d\ngot: %+v", len(got), tt.wantLen, got)
			}
			byID := make(map[string]int, len(got))
			for _, e := range got {
				byID[e.ItemID] = e.Quantity
			}
			for itemID, wantQty := range tt.wantByID {
				gotQty, ok := byID[itemID]
				if !ok {
					t.Errorf("missing entry for ItemID %q", itemID)
					continue
				}
				if gotQty != wantQty {
					t.Errorf("ItemID %q: Quantity got %d, want %d", itemID, gotQty, wantQty)
				}
			}
		})
	}
}
