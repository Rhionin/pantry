package server

import (
	"context"
	"time"

	"github.com/Rhionin/pantry/internal/inventory"
)

type InventoryInstancesListHandler struct {
	Repo interface {
		ListItemInstances(ctx context.Context, itemID string) ([]inventory.ItemInstance, error)
	}
}

type inventoryInstancesListPathParams struct {
	ItemID string `json:"itemId"`
}

// ItemInstanceWithStatus extends ItemInstance with computed expiry status.
type ItemInstanceWithStatus struct {
	inventory.ItemInstance
	ExpiryStatus inventory.ExpiryStatus `json:"expiryStatus"`
}

func (h *InventoryInstancesListHandler) Handle(req Request[struct{}, inventoryInstancesListPathParams]) ([]ItemInstanceWithStatus, error) {
	instances, err := h.Repo.ListItemInstances(req.Context, req.PathParams.ItemID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	warningDays := 7

	result := make([]ItemInstanceWithStatus, len(instances))
	for i, inst := range instances {
		result[i] = ItemInstanceWithStatus{
			ItemInstance: inst,
			ExpiryStatus: inventory.ComputeExpiryStatus(inst.ExpiresAt, now, warningDays),
		}
	}

	return result, nil
}
