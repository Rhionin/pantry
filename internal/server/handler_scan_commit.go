package server

import (
	"context"

	"github.com/Rhionin/pantry/internal/scan"
)

type ScanCommitHandler struct {
	Repo interface {
		GetScanEntry(ctx context.Context, id string) (*scan.ScanEntry, error)
		CommitStockIn(ctx context.Context, scanEntry *scan.ScanEntry) error
		CommitStockOut(ctx context.Context, scanEntry *scan.ScanEntry, instanceID *string) error
	}
}

type commitScanRequest struct {
	InstanceID *string `json:"instanceId,omitempty"`
}

type commitScanPathParams struct {
	ID string `json:"id"`
}

func (h *ScanCommitHandler) Handle(req Request[commitScanRequest, commitScanPathParams]) (*scan.ScanEntry, error) {
	if req.PathParams.ID == "" {
		return nil, BadRequest("scan entry id is required")
	}

	// Get the scan entry
	entry, err := h.Repo.GetScanEntry(req.Context, req.PathParams.ID)
	if err != nil {
		return nil, InternalError(err)
	}
	if entry == nil {
		return nil, NotFound("scan entry not found")
	}

	// Validate the entry can be committed
	if entry.Status != scan.Pending {
		return nil, BadRequest("only pending scan entries can be committed")
	}
	if entry.Direction == nil {
		return nil, BadRequest("scan direction must be set before committing")
	}
	if entry.ProductID == nil || *entry.ProductID == "" {
		return nil, BadRequest("product must be resolved before committing")
	}

	// Commit based on direction
	if *entry.Direction == scan.StockIn {
		if err := h.Repo.CommitStockIn(req.Context, entry); err != nil {
			return nil, InternalError(err)
		}
	} else {
		if err := h.Repo.CommitStockOut(req.Context, entry, req.Body.InstanceID); err != nil {
			return nil, InternalError(err)
		}
	}

	// Return the committed entry
	committed, err := h.Repo.GetScanEntry(req.Context, req.PathParams.ID)
	if err != nil {
		return nil, InternalError(err)
	}

	return committed, nil
}
