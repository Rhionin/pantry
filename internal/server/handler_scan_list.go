package server

import (
	"context"

	"github.com/Rhionin/pantry/internal/scan"
)

type ScanListHandler struct {
	Repo interface {
		ListScanEntries(ctx context.Context, userID string, status scan.ScanStatus) ([]scan.ScanEntry, error)
	}
}

func (h *ScanListHandler) Handle(req Request[struct{}, struct{}]) ([]scan.ScanEntry, error) {
	// Extract status from query params
	statusParam := req.RawRequest.URL.Query().Get("status")
	userIDParam := req.RawRequest.URL.Query().Get("userId")

	if userIDParam == "" {
		return nil, BadRequest("userId query parameter is required")
	}

	status := scan.ScanStatus(statusParam)
	if statusParam != "" && status != scan.Pending && status != scan.Flagged && status != scan.Committed && status != scan.Cancelled {
		return nil, BadRequest("invalid status value")
	}

	entries, err := h.Repo.ListScanEntries(req.Context, userIDParam, status)
	if err != nil {
		return nil, InternalError(err)
	}

	return entries, nil
}
