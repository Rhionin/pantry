package server

import (
	"context"

	"github.com/Rhionin/pantry/internal/product"
)

type LookupHandler struct {
	Service interface {
		Lookup(ctx context.Context, barcode, userID string) (product.LookupResult, error)
	}
}

func (h *LookupHandler) Handle(req Request[struct{}, struct{}]) (product.LookupResult, error) {
	barcode := req.RawRequest.URL.Query().Get("barcode")
	if barcode == "" {
		return product.LookupResult{}, BadRequest("barcode query parameter is required")
	}

	userID := "default-user"

	return h.Service.Lookup(req.Context, barcode, userID)
}
