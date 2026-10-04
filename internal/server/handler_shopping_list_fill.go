package server

import (
	"context"
	"time"

	"github.com/Rhionin/pantry/internal/cart"
	"github.com/Rhionin/pantry/internal/shopping"
	"github.com/Rhionin/pantry/internal/supply"
)

// ShoppingListFillHandler handles POST /api/shopping-list/fill.
// One call snapshots the current supply plan into the staged cart.
// Lines the owner already changed stay. The response is that staged cart.
type ShoppingListFillHandler struct {
	ShoppingList shoppingPlanWriter
	Pantry       pantryLister
	Supply       *supply.Service
	Ledger       *cart.Ledger
	Registry     *cart.Registry
	Adjustments  interface {
		GetAdjustment(ctx context.Context, entryID string, providerID string) (*shopping.Adjustment, error)
	}
}

func (h *ShoppingListFillHandler) Handle(req Request[struct{}, struct{}]) ([]ShoppingListEntryResponse, error) {
	const userID = "user-1"

	provision, err := stageShoppingPlan(req.Context, userID, h.Pantry, h.ShoppingList, h.Supply, time.Now())
	if err != nil {
		return nil, InternalError(err)
	}
	return presentShoppingList(req, provision, h.Ledger, h.Registry, h.Adjustments)
}
