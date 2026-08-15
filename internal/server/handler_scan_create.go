package server

import (
	"context"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
)

type ScanCreateHandler struct {
	Repo interface {
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

	unitCount := req.Body.UnitCount
	if unitCount == 0 {
		unitCount = 1
	}

	// Look up the product for this barcode
	lookupResult, err := h.LookupService.Lookup(req.Context, req.Body.Barcode, req.Body.UserID)
	if err != nil {
		return Created{}, InternalError(err)
	}

	// Determine status based on product lookup
	status := scan.Pending
	var prodID *string
	if lookupResult.IsFound() {
		prodID = &lookupResult.Product.ID
	} else {
		status = scan.Flagged
	}

	entry := scan.ScanEntry{
		UserID:    req.Body.UserID,
		Barcode:   req.Body.Barcode,
		ScannedAt: time.Now(),
		Direction: req.Body.Direction,
		UnitCount: unitCount,
		ExpiresAt: req.Body.ExpiresAt,
		Status:    status,
		ProductID: prodID,
	}

	created, err := h.Repo.CreateScanEntry(req.Context, entry)
	if err != nil {
		return Created{}, InternalError(err)
	}

	return Created{Value: created}, nil
}
