package server

import (
	"context"
	"errors"

	"github.com/Rhionin/pantry/internal/inventory"
)

// SetTargetQuantityHandler handles POST /api/items/{itemId}/target-quantity.
// It validates the requested quantity and persists it on the item.
type SetTargetQuantityHandler struct {
	InventoryRepo interface {
		GetItem(ctx context.Context, itemID string) (*inventory.Item, error)
		UpdateTargetQuantity(ctx context.Context, itemID string, qty int) error
	}
}

type setTargetQuantityPathParams struct {
	ItemID string `json:"itemId"`
}

type setTargetQuantityRequest struct {
	TargetQuantity *int `json:"targetQuantity"`
}

type setTargetQuantityResponse struct {
	ItemID         string `json:"itemId"`
	TargetQuantity int    `json:"targetQuantity"`
}

func (h *SetTargetQuantityHandler) Handle(req Request[setTargetQuantityRequest, setTargetQuantityPathParams]) (*setTargetQuantityResponse, error) {
	itemID := req.PathParams.ItemID

	if req.Body.TargetQuantity == nil {
		return nil, &HTTPError{Code: 422, Message: "targetQuantity is required"}
	}
	qty := *req.Body.TargetQuantity
	if qty < 0 {
		return nil, &HTTPError{Code: 422, Message: "targetQuantity must be 0 or greater"}
	}

	item, err := h.InventoryRepo.GetItem(req.Context, itemID)
	if err != nil {
		return nil, InternalError(err)
	}
	if item == nil {
		return nil, NotFound("item not found")
	}

	if err := h.InventoryRepo.UpdateTargetQuantity(req.Context, itemID, qty); err != nil {
		if errors.Is(err, inventory.ErrInstanceNotFound) {
			return nil, NotFound("item not found")
		}
		return nil, InternalError(err)
	}

	return &setTargetQuantityResponse{
		ItemID:         itemID,
		TargetQuantity: qty,
	}, nil
}
