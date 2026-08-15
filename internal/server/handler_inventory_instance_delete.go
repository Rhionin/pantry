package server

import (
	"context"
	"errors"

	"github.com/Rhionin/pantry/internal/inventory"
)

type InventoryInstanceDeleteHandler struct {
	Repo interface {
		RemoveInstance(ctx context.Context, instanceID string, reason string) error
	}
}

type inventoryInstanceDeletePathParams struct {
	InstanceID string `json:"instanceId"`
}

func (h *InventoryInstanceDeleteHandler) Handle(req Request[struct{}, inventoryInstanceDeletePathParams]) (struct{}, error) {
	err := h.Repo.RemoveInstance(req.Context, req.PathParams.InstanceID, "manual")
	if err != nil {
		if errors.Is(err, inventory.ErrInstanceNotFound) {
			return struct{}{}, NotFound("item instance not found")
		}
		return struct{}{}, InternalError(err)
	}
	return struct{}{}, nil
}
