package server

import (
	"context"

	"github.com/Rhionin/pantry/internal/scan"
)

type ScanHistoryHandler struct {
	Repo interface {
		ListScanEntries(ctx context.Context, userID string, status scan.ScanStatus) ([]scan.ScanEntry, error)
	}
}

func (h *ScanHistoryHandler) Handle(req Request[struct{}, struct{}]) ([]scan.ScanEntry, error) {
	userIDParam := req.RawRequest.URL.Query().Get("userId")

	if userIDParam == "" {
		return nil, BadRequest("userId query parameter is required")
	}

	// Return only committed entries for history
	entries, err := h.Repo.ListScanEntries(req.Context, userIDParam, scan.Committed)
	if err != nil {
		return nil, InternalError(err)
	}

	return entries, nil
}
