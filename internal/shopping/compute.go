// Package shopping provides shopping list derivation and manual item management.
package shopping

// DerivedEntryWithLedger extends DerivedEntry with ledger information.
type DerivedEntryWithLedger struct {
	DerivedEntry
	LedgerRequested int
}

// ComputeQuantity computes the quantity to provision for one item.
// It's a pure function with no database access.
// For replenish mode: max(0, consumed - requested)
// For target mode: max(0, target - instances - requested)
// Returns 0 for target mode when no target is recorded.
func ComputeQuantity(consumed int, requested int, targetQuantity *int, instances int, mode ReplenishmentMode) int {
	switch mode {
	case ReplenishMode:
		// replenish: max(0, consumed - requested)
		qty := consumed - requested
		if qty < 0 {
			return 0
		}
		return qty
	case TargetMode:
		// target: max(0, target - instances - requested)
		if targetQuantity == nil {
			return 0
		}
		qty := *targetQuantity - instances - requested
		if qty < 0 {
			return 0
		}
		return qty
	default:
		// Fallback to target mode
		if targetQuantity == nil {
			return 0
		}
		qty := *targetQuantity - instances - requested
		if qty < 0 {
			return 0
		}
		return qty
	}
}

// ReplenishmentMode identifies the shopping list calculation mode.
type ReplenishmentMode string

const (
	ReplenishMode ReplenishmentMode = "replenish"
	TargetMode    ReplenishmentMode = "target"
)

// Valid returns true if the mode is a valid value.
func (m ReplenishmentMode) Valid() bool {
	return m == ReplenishMode || m == TargetMode
}

// ComputeEntries computes all shopping list entries with ledger-net quantities.
// It wraps DeriveShoppingList for the target basis then subtracts Requested
// and clamps; builds replenish entries directly; and calls MergeEntries to
// decide which entry wins.
func ComputeEntries(
	derived []DerivedEntry,
	manual []ManualEntry,
	ledger map[string]LedgerEntry,
	instances map[string]int,
	consumed map[string]int,
	modes map[string]ReplenishmentMode,
	targets map[string]*int,
) []ManualEntry {
	// Step 1: Compute target basis entries with ledger-net quantities
	// (DeriveShoppingList computes target - instances, we then subtract requested)
	targetEntries := make(map[string]int) // itemID -> quantity
	for _, d := range derived {
		targetEntries[d.ItemID] = d.Quantity
	}

	// Step 2: Subtract requested from each entry
	for itemID, qty := range targetEntries {
		ledgerQty := ledger[itemID].Requested
		newQty := qty - ledgerQty
		if newQty < 0 {
			newQty = 0
		}
		targetEntries[itemID] = newQty
	}

	// Step 3: Build replenish entries directly (including products with no target)
	replenishEntries := make(map[string]int)
	for itemID, cons := range consumed {
		ledgerQty := ledger[itemID].Requested
		// replenish: max(0, consumed - requested)
		qty := cons - ledgerQty
		if qty < 0 {
			qty = 0
		}
		replenishEntries[itemID] = qty
	}

	// Step 4: Merge entries using the existing MergeEntries logic
	// but adapted to work with our ledger-net values
	// Create derived entries from targetEntries
	derivedWithLedger := make([]DerivedEntry, 0, len(targetEntries))
	for itemID, qty := range targetEntries {
		if qty > 0 {
			derivedWithLedger = append(derivedWithLedger, DerivedEntry{
				ItemID:   itemID,
				Quantity: qty,
				Source:   "auto",
			})
		}
	}

	// Create manual entries from manual
	manualEntries := make([]ManualEntry, len(manual))
	for i, m := range manual {
		// Apply ledger-net for manual entries too
		ledgerQty := ledger[m.ItemID].Requested
		qty := m.Quantity - ledgerQty
		if qty < 0 {
			qty = 0
		}
		manualEntries[i] = ManualEntry{
			ItemID:   m.ItemID,
			Quantity: qty,
		}
	}

	// Use MergeEntries to combine them
	return MergeEntries(derivedWithLedger, manualEntries)
}

// LedgerEntry holds the ledger state for one item.
type LedgerEntry struct {
	Requested int
}
