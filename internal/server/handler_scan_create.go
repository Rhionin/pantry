package server

import (
	"context"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
)

type ScanCreateHandler struct {
	Queue interface {
		CreateScanEntry(ctx context.Context, entry scan.ScanEntry) (*scan.ScanEntry, error)
	}
	LookupService interface {
		Lookup(ctx context.Context, barcode, userID string) (product.LookupResult, error)
	}
}

type createScanRequest struct {
	Barcode   string              `json:"barcode"`
	Direction *scan.ScanDirection `json:"direction,omitempty"`
	UnitCount int                 `json:"unitCount,omitempty"`
	ExpiresAt *time.Time          `json:"expiresAt,omitempty"`
	UserID    string              `json:"userId"`
}

func (h *ScanCreateHandler) Handle(req Request[createScanRequest, struct{}]) (Created, error) {
	if req.Body.Barcode == "" {
		return Created{}, BadRequest("barcode is required")
	}
	if req.Body.UserID == "" {
		return Created{}, BadRequest("userId is required")
	}

	// Look up the product for this barcode
	lookupResult, err := h.LookupService.Lookup(req.Context, req.Body.Barcode, req.Body.UserID)
	if err != nil {
		return Created{}, InternalError(err)
	}

	entry := scan.NewEntryFromLookup(req.Body.UserID, req.Body.Barcode, lookupResult, req.Body.Direction, time.Now())
	if req.Body.ExpiresAt != nil {
		entry.ExpiresAt = req.Body.ExpiresAt
	}
	if req.Body.UnitCount != 0 {
		entry.UnitCount = req.Body.UnitCount // HTTP callers may override the default of 1
	}

	created, err := h.Queue.CreateScanEntry(req.Context, entry)
	if err != nil {
		return Created{}, InternalError(err)
	}

	return Created{Value: created}, nil
}
