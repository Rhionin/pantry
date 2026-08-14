package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/go-json-experiment/json"
)

func TestLookupHandler(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := product.NewRepo(db)

	prod := product.Product{ID: "prod-1", Name: "Test Product", Category: "Test"}
	if err := repo.CreateProduct(context.Background(), prod); err != nil {
		t.Fatalf("failed to create test product: %v", err)
	}
	if err := repo.UpsertBarcodeMapping(context.Background(), "123456", "prod-1", "global", ""); err != nil {
		t.Fatalf("failed to create barcode mapping: %v", err)
	}

	lookupService := &product.LookupService{
		Repo: repo,
		OpenFoodFacts: mockOpenFoodFacts{
			lookupFn: func(ctx context.Context, barcode string) (*product.ProductSummary, error) {
				return nil, product.ErrProductNotFound
			},
		},
	}

	handler := &LookupHandler{Service: lookupService}

	req := httptest.NewRequest(http.MethodGet, "/api/products/lookup?barcode=123456", nil)
	req.Pattern = "GET /api/products/lookup"
	w := httptest.NewRecorder()

	HandleJSON(handler.Handle)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var result product.LookupResult
	if err := json.UnmarshalRead(w.Body, &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !result.IsFound() {
		t.Error("expected product to be found")
	}
	if result.Product.Name != "Test Product" {
		t.Errorf("expected product name 'Test Product', got %q", result.Product.Name)
	}
}

func TestLookupHandler_MissingBarcode(t *testing.T) {
	handler := &LookupHandler{Service: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/products/lookup", nil)
	req.Pattern = "GET /api/products/lookup"
	w := httptest.NewRecorder()

	HandleJSON(handler.Handle)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

type mockOpenFoodFacts struct {
	lookupFn func(ctx context.Context, barcode string) (*product.ProductSummary, error)
}

func (m mockOpenFoodFacts) LookupBarcode(ctx context.Context, barcode string) (*product.ProductSummary, error) {
	return m.lookupFn(ctx, barcode)
}
