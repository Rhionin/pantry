package server

import (
	"context"

	"github.com/Rhionin/pantry/internal/group"
	"github.com/Rhionin/pantry/internal/scan"
)

type ScanListHandler struct {
	Queue interface {
		ListScanEntries(ctx context.Context, userID string, status scan.ScanStatus) ([]scan.ScanEntry, error)
	}
	Groups *group.Groups
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

	entries, err := h.Queue.ListScanEntries(req.Context, userIDParam, status)
	if err != nil {
		return nil, InternalError(err)
	}
	if err := h.attachGroupHints(req.Context, entries); err != nil {
		return nil, InternalError(err)
	}

	return entries, nil
}

func (h *ScanListHandler) attachGroupHints(ctx context.Context, entries []scan.ScanEntry) error {
	if h.Groups == nil || len(entries) == 0 {
		return nil
	}
	names := map[string]string{}
	for _, entry := range entries {
		if entry.ProductID == nil || entry.Product == nil || entry.Product.Name == "" {
			continue
		}
		names[*entry.ProductID] = entry.Product.Name
	}
	hints, err := h.Groups.Hints(ctx, names)
	if err != nil {
		return err
	}
	for i := range entries {
		if entries[i].ProductID == nil {
			continue
		}
		hint, ok := hints[*entries[i].ProductID]
		if !ok || hint.Name == "" {
			continue
		}
		entries[i].GroupHint = &scan.GroupHint{GroupID: hint.GroupID, Name: hint.Name}
	}
	return nil
}
