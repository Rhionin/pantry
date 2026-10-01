package server

import (
	"context"

	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/shopping"
)

// planReplenishment builds the shared needs and the auto lines for them.
// A saved brand preference replaces the representative product on a line.
// The gap quantity is unchanged.
func planReplenishment(items []inventory.Item, counts map[string]int, manual []shopping.ShoppingListItem, prefs []shopping.Preference) ([]shopping.ReplenishmentItem, []shopping.DerivedEntry) {
	manualIDs := make(map[string]struct{}, len(manual))
	for _, item := range manual {
		manualIDs[item.ItemID] = struct{}{}
	}
	needs := replenishmentNeeds(items, counts)
	derived := shopping.DeriveShoppingList(shopping.ApplyPreferences(shopping.CollapseEquivalentNeeds(needs, manualIDs), needs, prefs))
	return needs, derived
}

func replenishmentNeeds(items []inventory.Item, counts map[string]int) []shopping.ReplenishmentItem {
	needs := make([]shopping.ReplenishmentItem, 0, len(items))
	for _, item := range items {
		need := shopping.ReplenishmentItem{
			ItemID:       item.ID,
			CurrentCount: counts[item.ID],
		}
		if item.Product != nil {
			need.Name = item.Product.Name
			need.UnitOfMeasure = item.Product.UnitOfMeasure
		}
		if item.TargetQuantity != nil {
			need.HasTarget = true
			need.TargetQuantity = *item.TargetQuantity
		}
		needs = append(needs, need)
	}
	return needs
}

type shoppingProvision struct {
	Manual  []shopping.ShoppingListItem
	Derived []shopping.DerivedEntry
	Merged  []shopping.ManualEntry
	Needs   []shopping.ReplenishmentItem
	Deals   []shopping.Deal
	Prefs   []shopping.Preference
}

type shoppingListReader interface {
	ListManualItems(ctx context.Context, userID string) ([]shopping.ShoppingListItem, error)
	ListPreferences(ctx context.Context, userID string) ([]shopping.Preference, error)
	ListDeals(ctx context.Context, userID string) ([]shopping.Deal, error)
}

type pantryLister interface {
	ListItems(ctx context.Context, userID string) ([]inventory.Item, error)
	ListItemInstances(ctx context.Context, itemID string) ([]inventory.ItemInstance, error)
}

func loadShoppingProvision(ctx context.Context, userID string, pantry pantryLister, list shoppingListReader) (shoppingProvision, error) {
	items, err := pantry.ListItems(ctx, userID)
	if err != nil {
		return shoppingProvision{}, err
	}
	manual, err := list.ListManualItems(ctx, userID)
	if err != nil {
		return shoppingProvision{}, err
	}
	prefs, err := list.ListPreferences(ctx, userID)
	if err != nil {
		return shoppingProvision{}, err
	}
	deals, err := list.ListDeals(ctx, userID)
	if err != nil {
		return shoppingProvision{}, err
	}
	counts, err := loadInstanceCounts(ctx, pantry.ListItemInstances, items)
	if err != nil {
		return shoppingProvision{}, err
	}
	needs, derived := planReplenishment(items, counts, manual, prefs)
	manualEntries := make([]shopping.ManualEntry, len(manual))
	for i, item := range manual {
		manualEntries[i] = shopping.ManualEntry{ItemID: item.ItemID, Quantity: item.Quantity}
	}
	return shoppingProvision{
		Manual:  manual,
		Derived: derived,
		Merged:  shopping.MergeEntries(derived, manualEntries),
		Needs:   needs,
		Deals:   deals,
		Prefs:   prefs,
	}, nil
}

func itemName(needs []shopping.ReplenishmentItem, itemID string) string {
	for _, item := range needs {
		if item.ItemID == itemID {
			return item.Name
		}
	}
	return ""
}

func loadInstanceCounts(ctx context.Context, list func(context.Context, string) ([]inventory.ItemInstance, error), items []inventory.Item) (map[string]int, error) {
	counts := make(map[string]int, len(items))
	for _, item := range items {
		instances, err := list(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		counts[item.ID] = len(instances)
	}
	return counts, nil
}
