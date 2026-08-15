package inventory

import (
	"context"
	"time"
)

// ListHandler handles GET /api/inventory requests.
type ListHandler struct {
	Repo interface {
		GetInventoryList(ctx context.Context, userID string, now time.Time, warningDays int) ([]InventoryItem, error)
	}
	// In a real app, userID would come from auth middleware
	UserID      string
	Now         func() time.Time
	WarningDays int
}

func (h *ListHandler) Handle(req struct {
	Context context.Context
}) ([]InventoryItem, error) {
	now := time.Now()
	if h.Now != nil {
		now = h.Now()
	}

	warningDays := 7
	if h.WarningDays != 0 {
		warningDays = h.WarningDays
	}

	return h.Repo.GetInventoryList(req.Context, h.UserID, now, warningDays)
}

// InstancesListHandler handles GET /api/inventory/{itemId}/instances requests.
type InstancesListHandler struct {
	Repo interface {
		ListItemInstances(ctx context.Context, itemID string) ([]ItemInstance, error)
	}
	Now         func() time.Time
	WarningDays int
}

type instancesListPathParams struct {
	ItemID string `json:"itemId"`
}

// ItemInstanceWithStatus extends ItemInstance with computed expiry status.
type ItemInstanceWithStatus struct {
	ItemInstance
	ExpiryStatus ExpiryStatus `json:"expiryStatus"`
}

func (h *InstancesListHandler) Handle(req struct {
	Context    context.Context
	PathParams instancesListPathParams
}) ([]ItemInstanceWithStatus, error) {
	instances, err := h.Repo.ListItemInstances(req.Context, req.PathParams.ItemID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if h.Now != nil {
		now = h.Now()
	}

	warningDays := 7
	if h.WarningDays != 0 {
		warningDays = h.WarningDays
	}

	result := make([]ItemInstanceWithStatus, len(instances))
	for i, inst := range instances {
		result[i] = ItemInstanceWithStatus{
			ItemInstance: inst,
			ExpiryStatus: ComputeExpiryStatus(inst.ExpiresAt, now, warningDays),
		}
	}

	return result, nil
}

// InstanceCreateHandler handles POST /api/inventory/{itemId}/instances requests.
type InstanceCreateHandler struct {
	Repo interface {
		AddInstance(ctx context.Context, instance ItemInstance) (*ItemInstance, error)
	}
}

type instanceCreatePathParams struct {
	ItemID string `json:"itemId"`
}

type instanceCreateRequest struct {
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

func (h *InstanceCreateHandler) Handle(req struct {
	Context    context.Context
	PathParams instanceCreatePathParams
	Body       instanceCreateRequest
}) (*ItemInstance, error) {
	instance := ItemInstance{
		ItemID:    req.PathParams.ItemID,
		StockInAt: time.Now(),
		ExpiresAt: req.Body.ExpiresAt,
	}

	return h.Repo.AddInstance(req.Context, instance)
}

// InstanceDeleteHandler handles DELETE /api/inventory/instances/{instanceId} requests.
type InstanceDeleteHandler struct {
	Repo interface {
		RemoveInstance(ctx context.Context, instanceID string, reason string) error
	}
}

type instanceDeletePathParams struct {
	InstanceID string `json:"instanceId"`
}

func (h *InstanceDeleteHandler) Handle(req struct {
	Context    context.Context
	PathParams instanceDeletePathParams
}) (struct{}, error) {
	err := h.Repo.RemoveInstance(req.Context, req.PathParams.InstanceID, "manual")
	if err != nil {
		return struct{}{}, err
	}
	return struct{}{}, nil
}
