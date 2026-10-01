package server

import (
	"context"
)

// inventoryWipeConfirmation is the exact text a caller must send to empty the
// pantry. It is a deliberate speed bump so one request cannot wipe stock.
const inventoryWipeConfirmation = "WIPE INVENTORY"

type InventoryWipeHandler struct {
	Pantry interface {
		Wipe(ctx context.Context, userID string) error
	}
	// In a real app, userID would come from auth middleware.
	UserID string
}

type inventoryWipeRequest struct {
	Confirmation string `json:"confirmation"`
}

func (h *InventoryWipeHandler) Handle(req Request[inventoryWipeRequest, struct{}]) (struct{}, error) {
	if req.Body.Confirmation != inventoryWipeConfirmation {
		return struct{}{}, BadRequest("Type WIPE INVENTORY to confirm wiping the inventory.")
	}

	userID := "user-1"
	if h.UserID != "" {
		userID = h.UserID
	}

	if err := h.Pantry.Wipe(req.Context, userID); err != nil {
		return struct{}{}, InternalError(err)
	}
	return struct{}{}, nil
}
