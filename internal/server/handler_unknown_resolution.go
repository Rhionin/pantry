package server

import (
	"context"

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
	Provisioner cart.Provisioner
}

// UnknownResolutionBody holds the request body.
type UnknownResolutionBody struct {
	ReachedProvider bool `json:"reachedProvider"`
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

	// If reachedProvider is true, the ledger should be advanced
	// This would require access to the Ledger to call Advance

	return &UnknownResolutionResponse{EntryID: entryID}, nil
}
