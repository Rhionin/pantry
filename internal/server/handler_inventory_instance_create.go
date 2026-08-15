package server

import (
	"context"
	"time"

	"github.com/Rhionin/pantry/internal/inventory"
)

type InventoryInstanceCreateHandler struct {
	Repo interface {
		AddInstance(ctx context.Context, instance inventory.ItemInstance) (*inventory.ItemInstance, error)
	}
}

type inventoryInstanceCreatePathParams struct {
	ItemID string `json:"itemId"`
}

type inventoryInstanceCreateRequest struct {
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

func (h *InventoryInstanceCreateHandler) Handle(req Request[inventoryInstanceCreateRequest, inventoryInstanceCreatePathParams]) (Created, error) {
	instance := inventory.ItemInstance{
		ItemID:    req.PathParams.ItemID,
		StockInAt: time.Now(),
		ExpiresAt: req.Body.ExpiresAt,
	}

	created, err := h.Repo.AddInstance(req.Context, instance)
	if err != nil {
		return Created{}, InternalError(err)
	}

	return Created{Value: created}, nil
}
