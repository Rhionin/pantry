// Package shopping provides shopping list derivation and manual item management.
package shopping

// DeriveInput holds the data needed to determine whether an item needs restocking.
type DeriveInput struct {
	ItemID       string
	TargetQuantity int
	CurrentCount int
}

// DerivedEntry represents an automatically derived shopping list entry.
type DerivedEntry struct {
	ItemID   string
	Quantity int
	Source   string // always "auto" for derived entries
}

// ManualEntry represents a manually added shopping list entry.
type ManualEntry struct {
	ItemID   string
	Quantity int
}

// DeriveShoppingList computes a derived shopping list from the given items.
// For each item where CurrentCount < TargetQuantity, it produces a DerivedEntry
// with Quantity = TargetQuantity - CurrentCount. Items at or above their target
// are omitted.
//
// Validates: Requirements 4.1, 4.2, 4.8
func DeriveShoppingList(items []DeriveInput) []DerivedEntry {
	result := make([]DerivedEntry, 0, len(items))
	for _, item := range items {
		if item.CurrentCount < item.TargetQuantity {
			result = append(result, DerivedEntry{
				ItemID:   item.ItemID,
				Quantity: item.TargetQuantity - item.CurrentCount,
				Source:   "auto",
			})
		}
	}
	return result
}

// MergeEntries combines derived and manual entries into a single list.
// When both a derived and a manual entry exist for the same ItemID, the manual
// entry's quantity takes precedence (overrides the derived quantity). Derived
// entries with no manual counterpart are included as-is.
//
// Validates: Requirements 4.4, 4.8
func MergeEntries(derived []DerivedEntry, manual []ManualEntry) []ManualEntry {
	// Index manual entries by ItemID for O(1) lookup.
	manualByItem := make(map[string]ManualEntry, len(manual))
	for _, m := range manual {
		manualByItem[m.ItemID] = m
	}

	// Start with all manual entries.
	result := make([]ManualEntry, len(manual))
	copy(result, manual)

	// Add derived entries only when no manual entry overrides them.
	for _, d := range derived {
		if _, hasManual := manualByItem[d.ItemID]; !hasManual {
			result = append(result, ManualEntry{
				ItemID:   d.ItemID,
				Quantity: d.Quantity,
			})
		}
	}

	return result
}
