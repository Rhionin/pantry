package server

import (
	"context"
	"sort"
	"time"

	"github.com/Rhionin/pantry/internal/cart"
	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/shopping"
	"github.com/Rhionin/pantry/internal/supply"
)

// ShoppingListGetHandler handles GET /api/shopping-list.
// It asks the supply service for lines, writes that snapshot, and returns it.
type ShoppingListGetHandler struct {
	ShoppingList shoppingListReader
	Pantry       pantryLister
	Ledger       *cart.Ledger
	Registry     *cart.Registry
	Supply       *supply.Service
	// GetAdjustment is separate so tests can satisfy shoppingListReader
	// without pulling adjustment methods onto every list reader.
	Adjustments interface {
		GetAdjustment(ctx context.Context, entryID string, providerID string) (*shopping.Adjustment, error)
	}
}

func (h *ShoppingListGetHandler) Handle(req Request[struct{}, struct{}]) ([]ShoppingListEntryResponse, error) {
	const userID = "user-1"

	provision, err := loadShoppingProvision(req.Context, userID, h.Pantry, h.ShoppingList, h.Supply, time.Now())
	if err != nil {
		return nil, InternalError(err)
	}

	providerID := targetProviderID(req, h.Registry)
	itemByID := make(map[string]inventory.Item, len(provision.Items))
	for _, item := range provision.Items {
		itemByID[item.ID] = item
	}

	rows := append([]shopping.ShoppingListItem(nil), provision.Rows...)
	sort.Slice(rows, func(i, j int) bool {
		left := itemByID[rows[i].ItemID].ProductID
		right := itemByID[rows[j].ItemID].ProductID
		if left != right {
			return left < right
		}
		return rows[i].ItemID < rows[j].ItemID
	})

	ledger := map[string]cart.LedgerEntry{}
	if h.Ledger != nil && providerID != "" {
		ledger, err = h.Ledger.ListForProvider(req.Context, cart.ProviderID(providerID))
		if err != nil {
			return nil, InternalError(err)
		}
	}

	resp := make([]ShoppingListEntryResponse, 0, len(rows))
	for _, row := range rows {
		computed := row.Quantity
		if requested := ledger[row.ItemID].Requested; requested > 0 {
			computed -= requested
			if computed < 0 {
				computed = 0
			}
		}
		provisionQty := computed
		var adjustment *int
		if providerID != "" && row.ID != "" && h.Adjustments != nil {
			adj, adjErr := h.Adjustments.GetAdjustment(req.Context, row.ID, providerID)
			if adjErr != nil {
				return nil, InternalError(adjErr)
			}
			if adj != nil {
				adjustment = &adj.Quantity
				provisionQty = adj.Quantity
			}
		}
		entry := ShoppingListEntryResponse{
			ID:               row.ID,
			ItemID:           row.ItemID,
			Quantity:         provisionQty,
			Source:           row.Source,
			Note:             row.Note,
			Provider:         providerID,
			ComputedQuantity: computed,
			Adjustment:       adjustment,
		}
		resp = append(resp, entry)
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
