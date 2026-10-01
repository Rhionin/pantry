package server

import (
	"context"
	"time"

	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/shopping"
)

// ShoppingListGetHandler handles GET /api/shopping-list.
// It derives entries from inventory targets, pooling recognized store-brand
// equivalents into one need, merges with manual entries, and returns the combined list.
type ShoppingListGetHandler struct {
	ShoppingList interface {
		ListManualItems(ctx context.Context, userID string) ([]shopping.ShoppingListItem, error)
		ListPreferences(ctx context.Context, userID string) ([]shopping.Preference, error)
		ListDeals(ctx context.Context, userID string) ([]shopping.Deal, error)
		SyncDerivedItems(ctx context.Context, userID string, derived []shopping.DerivedEntry) ([]shopping.ShoppingListItem, error)
	}
	Pantry interface {
		ListItems(ctx context.Context, userID string) ([]inventory.Item, error)
		ListItemInstances(ctx context.Context, itemID string) ([]inventory.ItemInstance, error)
	}
}

func (h *ShoppingListGetHandler) Handle(req Request[struct{}, struct{}]) ([]ShoppingListEntryResponse, error) {
	const userID = "user-1"

	provision, err := loadShoppingProvision(req.Context, userID, h.Pantry, h.ShoppingList)
	if err != nil {
		return nil, InternalError(err)
	}

	autoItems, err := h.ShoppingList.SyncDerivedItems(req.Context, userID, provision.Derived)
	if err != nil {
		return nil, InternalError(err)
	}

	manualItems := provision.Manual
	merged := provision.Merged

	// Build ID lookup from manual items so merged entries can carry the DB row ID.
	manualByItemID := make(map[string]shopping.ShoppingListItem, len(manualItems))
	for _, m := range manualItems {
		manualByItemID[m.ItemID] = m
	}
	autoByItemID := make(map[string]shopping.ShoppingListItem, len(autoItems))
	for _, item := range autoItems {
		autoByItemID[item.ItemID] = item
	}

	resp := make([]ShoppingListEntryResponse, 0, len(merged))
	for _, entry := range merged {
		dbRow, isManual := manualByItemID[entry.ItemID]

		e := ShoppingListEntryResponse{
			ItemID:   entry.ItemID,
			Quantity: entry.Quantity,
		}

		if isManual {
			e.ID = dbRow.ID
			e.Source = "manual"
			if dbRow.PurchasedAt != nil {
				ts := dbRow.PurchasedAt.Format(time.RFC3339)
				e.PurchasedAt = &ts
			}
		} else {
			autoRow, active := autoByItemID[entry.ItemID]
			if !active {
				continue
			}
			e.ID = autoRow.ID
			e.Source = "auto"
		}

		resp = append(resp, e)
	}

	if resp == nil {
		resp = []ShoppingListEntryResponse{}
	}
	return resp, nil
}
