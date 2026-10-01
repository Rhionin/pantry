package server

import (
	"context"
	"fmt"

	"github.com/Rhionin/pantry/internal/cart"
	"github.com/Rhionin/pantry/internal/shopping"
)

// UnknownResolutionPathParams holds path parameters for the unknown resolution endpoint.
type UnknownResolutionPathParams struct {
	ID string `json:"id"`
}

// UnknownResolutionHandler handles POST /api/shopping-list/items/{id}/unknown-resolution.
// Handles the owner's declaration of whether an item reached the provider or not.
type UnknownResolutionHandler struct {
	ShoppingList interface {
		GetItemByID(ctx context.Context, entryID string) (*shopping.ShoppingListItem, error)
	}
	Ledger *cart.Ledger
}

// UnknownResolutionBody holds the request body.
type UnknownResolutionBody struct {
	ReachedProvider bool   `json:"reachedProvider"`
	Provider        string `json:"provider"`
}

// UnknownResolutionResponse holds the response.
type UnknownResolutionResponse struct {
	EntryID string `json:"entryId"`
}

// Handle handles unknown-outcome resolution.
func (h *UnknownResolutionHandler) Handle(req Request[UnknownResolutionBody, UnknownResolutionPathParams]) (*UnknownResolutionResponse, error) {
	entryID := req.PathParams.ID
	if entryID == "" {
		return nil, BadRequest("missing id path parameter")
	}

	// Get the entry to find its item ID
	entry, err := h.ShoppingList.GetItemByID(req.Context, entryID)
	if err != nil {
		return nil, InternalError(err)
	}
	if entry == nil {
		return nil, NotFound("entry not found")
	}

	if !req.Body.ReachedProvider {
		return &UnknownResolutionResponse{EntryID: entryID}, nil
	}
	if req.Body.Provider == "" {
		return nil, BadRequest("missing provider")
	}
	qty := entry.Quantity
	if qty < 1 {
		qty = 1
	}
	if h.Ledger == nil {
		return nil, InternalError(fmt.Errorf("ledger is not available"))
	}
	if err := h.Ledger.Advance(req.Context, cart.ProviderID(req.Body.Provider), entry.ItemID, qty); err != nil {
		return nil, InternalError(err)
	}
	return &UnknownResolutionResponse{EntryID: entryID}, nil
}
