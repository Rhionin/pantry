package server

import (
	"context"

	"github.com/Rhionin/pantry/internal/product"
)

type CreateHandler struct {
	Repo interface {
		CreateProduct(ctx context.Context, product product.Product) error
	}
}

type createProductRequest struct {
	Name          string `json:"name"`
	Category      string `json:"category"`
	UnitOfMeasure string `json:"unitOfMeasure"`
}

func (h *CreateHandler) Handle(req Request[createProductRequest, struct{}]) (Created, error) {
	if req.Body.Name == "" {
		return Created{}, BadRequest("name is required")
	}

	prod := product.Product{
		Name:          req.Body.Name,
		Category:      req.Body.Category,
		UnitOfMeasure: req.Body.UnitOfMeasure,
	}

	if err := h.Repo.CreateProduct(req.Context, prod); err != nil {
		return Created{}, err
	}

	return Created{Value: prod}, nil
}
