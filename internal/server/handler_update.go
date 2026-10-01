package server

import (
	"github.com/Rhionin/pantry/internal/product"
)

type UpdateHandler struct {
	Catalog     *product.Catalog
	Contributor product.UpstreamContributor
}

type updateProductPathParams struct {
	ID string `json:"id"`
}

func (h *UpdateHandler) Handle(req Request[productWriteBody, updateProductPathParams]) (productWriteResponse, error) {
	if req.PathParams.ID == "" {
		return productWriteResponse{}, BadRequest("product id is required")
	}
	if req.Body.Name == "" {
		return productWriteResponse{}, BadRequest("name is required")
	}
	if err := validateWriteContribution(req.Body); err != nil {
		return productWriteResponse{}, err
	}

	prod := product.Product{
		ID:            req.PathParams.ID,
		Name:          req.Body.Name,
		Category:      req.Body.Category,
		UnitOfMeasure: req.Body.UnitOfMeasure,
	}
	if err := h.Catalog.UpdateProduct(req.Context, prod); err != nil {
		return productWriteResponse{}, err
	}

	stored, err := h.Catalog.GetProductByID(req.Context, prod.ID)
	if err != nil {
		return productWriteResponse{}, err
	}
	if stored == nil {
		return productWriteResponse{}, NotFound("product not found")
	}

	// The response keeps the fields the request set. Provenance comes from the
	// stored row so a product already loaded from upstream is not sent again.
	outcome, err := recordWriteContribution(req.Context, h.Catalog, h.Contributor, req.Body, product.Product{
		ID:             stored.ID,
		Name:           prod.Name,
		Category:       prod.Category,
		UnitOfMeasure:  prod.UnitOfMeasure,
		Source:         stored.Source,
		ExternalSource: stored.ExternalSource,
	})
	if err != nil {
		return productWriteResponse{}, err
	}
	return productWriteResponse{Product: prod, Contribution: outcome}, nil
}
