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

	existing, err := h.Catalog.GetProductByID(req.Context, req.PathParams.ID)
	if err != nil {
		return productWriteResponse{}, err
	}

	var prod product.Product
	if existing != nil {
		prod = *existing
	}
	prod.ID = req.PathParams.ID
	prod.Name = req.Body.Name
	prod.Category = req.Body.Category
	prod.UnitOfMeasure = req.Body.UnitOfMeasure
	if err := applyWrittenSize(&prod, req.Body, false); err != nil {
		return productWriteResponse{}, err
	}
	if err := h.Catalog.UpdateProduct(req.Context, prod); err != nil {
		return productWriteResponse{}, inputOrInternal(err)
	}

	stored, err := h.Catalog.GetProductByID(req.Context, prod.ID)
	if err != nil {
		return productWriteResponse{}, err
	}
	if stored == nil {
		return productWriteResponse{}, NotFound("product not found")
	}

	// Provenance comes from the stored row so a product already loaded from
	// upstream is not sent again.
	outcome, err := recordWriteContribution(req.Context, h.Catalog, h.Contributor, req.Body, *stored)
	if err != nil {
		return productWriteResponse{}, err
	}
	return productWriteResponse{Product: *stored, Contribution: outcome}, nil
}

func applyWrittenSize(prod *product.Product, body productWriteBody, creating bool) error {
	if err := prod.ApplyTypedSize(body.NetAmount, body.NetUnit, body.NetSizeSet, creating); err != nil {
		return BadRequest(err.Error())
	}
	if err := prod.ApplyTypedPack(body.PackCount, body.PackCountSet); err != nil {
		return BadRequest(err.Error())
	}
	return nil
}

func inputOrInternal(err error) error {
	if product.IsInputError(err) {
		return BadRequest(err.Error())
	}
	return err
}
