package server

import (
	"context"
	"time"

	"github.com/Rhionin/pantry/internal/inventory"
)

type InventoryListHandler struct {
	Repo interface {
		GetInventoryList(ctx context.Context, userID string, now time.Time, warningDays int, query string) ([]inventory.InventoryItem, error)
	}
	// In a real app, userID would come from auth middleware
	UserID string
}

func (h *InventoryListHandler) Handle(req Request[struct{}, struct{}]) ([]inventory.InventoryItem, error) {
	userID := "user-1" // Hardcoded for now; would come from auth middleware in production
	if h.UserID != "" {
		userID = h.UserID
	}

	now := time.Now()
	warningDays := 7

	// Extract query parameter from raw request
	query := req.RawRequest.URL.Query().Get("q")

	return h.Repo.GetInventoryList(req.Context, userID, now, warningDays, query)
}
