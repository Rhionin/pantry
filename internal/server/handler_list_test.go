package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/go-json-experiment/json"
)

func TestListHandler(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := product.NewRepo(db)

	products := []product.Product{
		{ID: "prod-1", Name: "Apple", Category: "Fruit"},
		{ID: "prod-2", Name: "Banana", Category: "Fruit"},
	}
	for _, p := range products {
		if err := repo.CreateProduct(context.Background(), p); err != nil {
			t.Fatalf("failed to create product: %v", err)
		}
	}

	handler := &ListHandler{Repo: repo}

	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	req.Pattern = "GET /api/products"
	w := httptest.NewRecorder()

	HandleJSON(handler.Handle)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var result []product.Product
	if err := json.UnmarshalRead(w.Body, &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 products, got %d", len(result))
	}
}
