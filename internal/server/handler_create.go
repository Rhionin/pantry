package server

import (
	"context"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/google/uuid"
)

type CreateHandler struct {
	Catalog     *product.Catalog
	Contributor product.UpstreamContributor
}

type productWriteBody struct {
	Name          string `json:"name"`
	Category      string `json:"category"`
	UnitOfMeasure string `json:"unitOfMeasure"`
	Contribute    bool   `json:"contribute"`
	ContributeTo  string `json:"contributeTo"`
	Barcode       string `json:"barcode"`
}

type productWriteResponse struct {
	product.Product
	Contribution *product.ShareOutcome `json:"contribution,omitempty"`
}

func (h *CreateHandler) Handle(req Request[productWriteBody, struct{}]) (Created, error) {
	if req.Body.Name == "" {
		return Created{}, BadRequest("name is required")
	}
	if err := validateWriteContribution(req.Body); err != nil {
		return Created{}, err
	}

	prod := product.Product{
		ID:            uuid.NewString(),
		Name:          req.Body.Name,
		Category:      req.Body.Category,
		UnitOfMeasure: req.Body.UnitOfMeasure,
	}
	if err := h.Catalog.CreateProduct(req.Context, prod); err != nil {
		return Created{}, err
	}

	stored, err := h.Catalog.GetProductByID(req.Context, prod.ID)
	if err != nil {
		return Created{}, err
	}
	if stored == nil {
		stored = &prod
	}

	outcome, err := recordWriteContribution(req.Context, h.Catalog, h.Contributor, req.Body, *stored)
	if err != nil {
		return Created{}, err
	}
	return Created{Value: productWriteResponse{Product: *stored, Contribution: outcome}}, nil
}

func validateWriteContribution(body productWriteBody) error {
	if err := product.ValidateContribution(body.Contribute, body.Barcode, product.ExternalSource(body.ContributeTo)); err != nil {
		return BadRequest(err.Error())
	}
	return nil
}

func recordWriteContribution(ctx context.Context, catalog *product.Catalog, contributor product.UpstreamContributor, body productWriteBody, prod product.Product) (*product.ShareOutcome, error) {
	return product.RecordShare(ctx, catalog, contributor, product.ShareRequest{
		Contribute:            body.Contribute,
		ProductID:             prod.ID,
		Name:                  prod.Name,
		Category:              prod.Category,
		UnitOfMeasure:         prod.UnitOfMeasure,
		Barcode:               body.Barcode,
		Database:              product.ExternalSource(body.ContributeTo),
		ProductSource:         prod.Source,
		ProductExternalSource: prod.ExternalSource,
	})
}
