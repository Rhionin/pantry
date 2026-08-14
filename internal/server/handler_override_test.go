package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rhionin/pantry/internal/product"
)

func TestOverrideCreateHandler(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := product.NewRepo(db)

	prod := product.Product{ID: "prod-1", Name: "Test Product"}
	if err := repo.CreateProduct(context.Background(), prod); err != nil {
		t.Fatalf("failed to create product: %v", err)
	}

	handler := &OverrideCreateHandler{Repo: repo}

	reqBody := bytes.NewBufferString(`{"barcode":"999888","productId":"prod-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/products/overrides", reqBody)
	req.Pattern = "POST /api/products/overrides"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	HandleJSON(handler.Handle)(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	result, err := repo.LookupByBarcode(context.Background(), "999888", "default-user")
	if err != nil {
		t.Fatalf("failed to lookup barcode: %v", err)
	}
	if result == nil {
		t.Fatal("expected override to be found")
	}
	if result.Name != "Test Product" {
		t.Errorf("expected product name 'Test Product', got %q", result.Name)
	}
}
