package server

import (
	"context"

	"github.com/Rhionin/pantry/internal/group"
	"github.com/Rhionin/pantry/internal/product"
)

type LookupHandler struct {
	Service interface {
		Lookup(ctx context.Context, barcode, userID string) (product.LookupResult, error)
	}
	Groups *group.Groups
}

func (h *LookupHandler) Handle(req Request[struct{}, struct{}]) (product.LookupResult, error) {
	barcode := req.RawRequest.URL.Query().Get("barcode")
	if barcode == "" {
		return product.LookupResult{}, BadRequest("barcode query parameter is required")
	}

	userID := "user-1"

	result, err := h.Service.Lookup(req.Context, barcode, userID)
	if err != nil {
		return product.LookupResult{}, err
	}
	if result.Product != nil {
		considerGroupProduct(req.Context, h.Groups, result.Product.ID)
	}
	return result, nil
}
