package server

import (
	"context"

	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/shopping"
)

// replenishmentEntries is the auto shopping list: equivalent store-brand
// products are one need, then each remaining need contributes its gap.
func replenishmentEntries(items []inventory.Item, counts map[string]int, manual []shopping.ShoppingListItem) []shopping.DerivedEntry {
	manualIDs := make(map[string]struct{}, len(manual))
	for _, item := range manual {
		manualIDs[item.ItemID] = struct{}{}
	}

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
	return shopping.DeriveShoppingList(shopping.CollapseEquivalentNeeds(needs, manualIDs))
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
