package server

import (
	"context"
)

type OverrideCreateHandler struct {
	Repo interface {
		UpsertBarcodeMapping(ctx context.Context, barcode, productID, source, userID string) error
	}
}

type createOverrideRequest struct {
	Barcode   string `json:"barcode"`
	ProductID string `json:"productId"`
}

type createOverrideResponse struct {
	Barcode   string `json:"barcode"`
	ProductID string `json:"productId"`
	Source    string `json:"source"`
}

func (h *OverrideCreateHandler) Handle(req Request[createOverrideRequest, struct{}]) (Created, error) {
	if req.Body.Barcode == "" {
		return Created{}, BadRequest("barcode is required")
	}
	if req.Body.ProductID == "" {
		return Created{}, BadRequest("productId is required")
	}

	userID := "default-user"

	if err := h.Repo.UpsertBarcodeMapping(req.Context, req.Body.Barcode, req.Body.ProductID, "user_override", userID); err != nil {
		return Created{}, err
	}

	resp := createOverrideResponse{
		Barcode:   req.Body.Barcode,
		ProductID: req.Body.ProductID,
		Source:    "user_override",
	}

	return Created{Value: resp}, nil
}
