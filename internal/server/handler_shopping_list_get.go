package server

import (
	"context"
	"time"

	"github.com/Rhionin/pantry/internal/cart"
	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/shopping"
	"github.com/Rhionin/pantry/internal/suggestion"
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
		GetAdjustment(ctx context.Context, entryID string, providerID string) (*shopping.Adjustment, error)
	}
	Pantry interface {
		ListItems(ctx context.Context, userID string) ([]inventory.Item, error)
		ListItemInstances(ctx context.Context, itemID string) ([]inventory.ItemInstance, error)
	}
	Ledger         *cart.Ledger
	Registry       *cart.Registry
	ConsumptionLog *suggestion.ConsumptionLog
}

func (h *ShoppingListGetHandler) Handle(req Request[struct{}, struct{}]) ([]ShoppingListEntryResponse, error) {
	const userID = "user-1"

	provision, err := loadShoppingProvision(req.Context, userID, h.Pantry, h.ShoppingList)
	if err != nil {
		return nil, InternalError(err)
	}

	providerID := targetProviderID(req, h.Registry)
	itemByID := make(map[string]inventory.Item, len(provision.Items))
	for _, item := range provision.Items {
		itemByID[item.ID] = item
	}
	instanceCount := provision.Counts

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

	ledger := map[string]cart.LedgerEntry{}
	if h.Ledger != nil && providerID != "" {
		ledger, err = h.Ledger.ListForProvider(req.Context, cart.ProviderID(providerID))
		if err != nil {
			return nil, InternalError(err)
		}
	}
	consumedAt := map[string][]time.Time{}
	if h.ConsumptionLog != nil && len(resp) > 0 {
		ids := make([]string, len(resp))
		for i, entry := range resp {
			ids[i] = entry.ItemID
		}
		consumedAt, err = h.ConsumptionLog.ListConsumedAtByItems(req.Context, ids)
		if err != nil {
			return nil, InternalError(err)
		}
	}

	for i := range resp {
		entry := &resp[i]
		item := itemByID[entry.ItemID]
		mode := item.ReplenishmentMode
		if mode == "" {
			mode = inventory.TargetMode
		}
		_, hasLedger := ledger[entry.ItemID]
		requested := ledger[entry.ItemID].Requested
		consumed := countConsumedSince(consumedAt[entry.ItemID], ledger[entry.ItemID].Boundary, hasLedger)
		computed := entry.Quantity
		if requested > 0 {
			computed -= requested
			if computed < 0 {
				computed = 0
			}
		}
		if mode == inventory.ReplenishMode {
			computed = consumed - requested
			if computed < 0 {
				computed = 0
			}
		}
		provision := computed
		var adjustment *int
		if providerID != "" && entry.ID != "" {
			adj, adjErr := h.ShoppingList.GetAdjustment(req.Context, entry.ID, providerID)
			if adjErr != nil {
				return nil, InternalError(adjErr)
			}
			if adj != nil {
				adjustment = &adj.Quantity
				provision = adj.Quantity
			}
		}
		entry.Quantity = provision
		entry.ComputedQuantity = computed
		entry.ReplenishmentMode = string(mode)
		entry.Provider = providerID
		entry.Adjustment = adjustment
		entry.Basis = &ShoppingListBasis{
			Mode:           string(mode),
			TargetQuantity: item.TargetQuantity,
			InstanceCount:  instanceCount[entry.ItemID],
			ConsumedUnits:  consumed,
			Requested:      requested,
		}
	}

	if resp == nil {
		resp = []ShoppingListEntryResponse{}
	}
	return resp, nil
}

func targetProviderID(req Request[struct{}, struct{}], registry *cart.Registry) string {
	if req.RawRequest != nil {
		if provider := req.RawRequest.URL.Query().Get("provider"); provider != "" {
			return provider
		}
	}
	if registry != nil {
		if id, ok := registry.SoleConfigured(); ok {
			return string(id)
		}
	}
	return ""
}

func countConsumedSince(times []time.Time, boundary time.Time, hasBoundary bool) int {
	if !hasBoundary {
		return len(times)
	}
	count := 0
	for _, at := range times {
		if at.After(boundary) {
			count++
		}
	}
	return count
}
