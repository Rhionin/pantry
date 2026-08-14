package server

import (
	"context"

	"github.com/Rhionin/pantry/internal/product"
)

type UpdateHandler struct {
	Repo interface {
		UpdateProduct(ctx context.Context, product product.Product) error
	}
}

type updateProductRequest struct {
	Name          string `json:"name"`
	Category      string `json:"category"`
	UnitOfMeasure string `json:"unitOfMeasure"`
}

type updateProductPathParams struct {
	ID string `json:"id"`
}

func (h *UpdateHandler) Handle(req Request[updateProductRequest, updateProductPathParams]) (product.Product, error) {
	if req.PathParams.ID == "" {
		return product.Product{}, BadRequest("product id is required")
	}

	if req.Body.Name == "" {
		return product.Product{}, BadRequest("name is required")
	}

	prod := product.Product{
		ID:            req.PathParams.ID,
		Name:          req.Body.Name,
		Category:      req.Body.Category,
		UnitOfMeasure: req.Body.UnitOfMeasure,
	}

	if err := h.Repo.UpdateProduct(req.Context, prod); err != nil {
		return product.Product{}, err
	}

	return prod, nil
}
