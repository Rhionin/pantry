package server

import (
	"context"

	"github.com/Rhionin/pantry/internal/cart"
	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/shopping"
)

// ShoppingListExportHandler handles POST /api/shopping-list/export.
// It fetches the current shopping list and submits it to the cart exporter.
type ShoppingListExportHandler struct {
	ShoppingList interface {
		ListManualItems(ctx context.Context, userID string) ([]shopping.ShoppingListItem, error)
	}
	Pantry interface {
		ListItems(ctx context.Context, userID string) ([]inventory.Item, error)
		ListItemInstances(ctx context.Context, itemID string) ([]inventory.ItemInstance, error)
	}
	Provisioner cart.Provisioner
}

type shoppingListExportResponse struct {
	Exported int `json:"exported"`
}

func (h *ShoppingListExportHandler) Handle(req Request[struct{}, struct{}]) (*shoppingListExportResponse, error) {
	const userID = "user-1"
	const providerID = "kroger"

	// For now, use the no-op provisioner
	// TODO: wire up real provider when available
	report, err := h.Provisioner.Provision(req.Context, userID, cart.ProviderID(providerID))
	if err != nil {
		return nil, InternalError(err)
	}

	// The no-op provisioner returns no entries, so exported is 0
	// When a real provider is wired, this should use the report.Confirmed count
	return &shoppingListExportResponse{Exported: report.Confirmed}, nil
}
