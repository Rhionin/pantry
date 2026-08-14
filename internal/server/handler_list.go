package server

import (
	"context"

	"github.com/Rhionin/pantry/internal/product"
)

type ListHandler struct {
	Repo interface {
		ListProducts(ctx context.Context) ([]product.Product, error)
	}
}

func (h *ListHandler) Handle(req Request[struct{}, struct{}]) ([]product.Product, error) {
	return h.Repo.ListProducts(req.Context)
}
