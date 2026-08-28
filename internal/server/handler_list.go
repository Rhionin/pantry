package server

import (
	"context"

	"github.com/Rhionin/pantry/internal/product"
)

type ListHandler struct {
	Catalog interface {
		ListProducts(ctx context.Context) ([]product.Product, error)
	}
}

func (h *ListHandler) Handle(req Request[struct{}, struct{}]) ([]product.Product, error) {
	return h.Catalog.ListProducts(req.Context)
}
