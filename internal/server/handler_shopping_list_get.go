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
		SyncDerivedItems(ctx context.Context, userID string, derived []shopping.DerivedEntry) ([]shopping.ShoppingListItem, error)
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

	manualItems, err := h.ShoppingList.ListManualItems(req.Context, userID)
	if err != nil {
		return nil, InternalError(err)
	}

	counts, err := loadInstanceCounts(req.Context, h.Pantry.ListItemInstances, items)
	if err != nil {
		return nil, InternalError(err)
	}

	derived := replenishmentEntries(items, counts, manualItems)
	autoItems, err := h.ShoppingList.SyncDerivedItems(req.Context, userID, derived)
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
