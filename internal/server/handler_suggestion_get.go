package server

import (
	"context"
	"errors"

	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/suggestion"
)

// SuggestionGetHandler handles GET /api/suggestions/{itemId}.
// It verifies the item exists, fetches its consumption events, and returns
// a target-quantity suggestion produced by the suggestion engine.
type SuggestionGetHandler struct {
	SuggestionRepo interface {
		ListConsumptionEvents(ctx context.Context, itemID string) ([]suggestion.ConsumptionEvent, error)
	}
	InventoryRepo interface {
		GetItem(ctx context.Context, itemID string) (*inventory.Item, error)
	}
}

type suggestionGetPathParams struct {
	ItemID string `json:"itemId"`
}

func (h *SuggestionGetHandler) Handle(req Request[struct{}, suggestionGetPathParams]) (*suggestion.TargetQuantitySuggestion, error) {
	itemID := req.PathParams.ItemID

	item, err := h.InventoryRepo.GetItem(req.Context, itemID)
	if err != nil {
		return nil, InternalError(err)
	}
	if item == nil {
		return nil, NotFound("item not found")
	}

	events, err := h.SuggestionRepo.ListConsumptionEvents(req.Context, itemID)
	if err != nil {
		return nil, InternalError(err)
	}

	result := suggestion.SuggestTargetQuantity(itemID, events)
	return &result, nil
}

// ErrItemNotFound is returned by InventoryRepo when the item ID does not exist.
var ErrItemNotFound = errors.New("item not found")
