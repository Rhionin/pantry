package server

import (
	"context"
	"time"

	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/shopping"
)

// ShoppingListGetHandler handles GET /api/shopping-list.
// It derives entries from inventory items with target quantities, merges with
// manual entries, and returns the combined list.
type ShoppingListGetHandler struct {
	ShoppingList interface {
		ListManualItems(ctx context.Context, userID string) ([]shopping.ShoppingListItem, error)
	}
	Pantry interface {
		ListItems(ctx context.Context, userID string) ([]inventory.Item, error)
		ListItemInstances(ctx context.Context, itemID string) ([]inventory.ItemInstance, error)
	}
}

func (h *ShoppingListGetHandler) Handle(req Request[struct{}, struct{}]) ([]ShoppingListEntryResponse, error) {
	const userID = "user-1"

	items, err := h.Pantry.ListItems(req.Context, userID)
	if err != nil {
		return nil, InternalError(err)
	}

	// Build DeriveInput for items that have a target quantity.
	deriveInputs := make([]shopping.DeriveInput, 0, len(items))
	for _, item := range items {
		if item.TargetQuantity == nil {
			continue
		}
		instances, err := h.Pantry.ListItemInstances(req.Context, item.ID)
		if err != nil {
			return nil, InternalError(err)
		}
		deriveInputs = append(deriveInputs, shopping.DeriveInput{
			ItemID:         item.ID,
			TargetQuantity: *item.TargetQuantity,
			CurrentCount:   len(instances),
		})
	}

	derived := shopping.DeriveShoppingList(deriveInputs)

	manualItems, err := h.ShoppingList.ListManualItems(req.Context, userID)
	if err != nil {
		return nil, InternalError(err)
	}

	// Convert manual DB rows to ManualEntry for merge.
	manualEntries := make([]shopping.ManualEntry, len(manualItems))
	for i, m := range manualItems {
		manualEntries[i] = shopping.ManualEntry{ItemID: m.ItemID, Quantity: m.Quantity}
	}

	merged := shopping.MergeEntries(derived, manualEntries)

	// Build ID lookup from manual items so merged entries can carry the DB row ID.
	manualByItemID := make(map[string]shopping.ShoppingListItem, len(manualItems))
	for _, m := range manualItems {
		manualByItemID[m.ItemID] = m
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
			e.ID = ""
			e.Source = "auto"
		}

		resp = append(resp, e)
	}

	if resp == nil {
		resp = []ShoppingListEntryResponse{}
	}
	return resp, nil
}
