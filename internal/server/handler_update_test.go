package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/go-json-experiment/json"
)

func TestUpdateHandler(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := product.NewRepo(db)

	prod := product.Product{ID: "prod-1", Name: "Old Name", Category: "Old Category"}
	if err := repo.CreateProduct(context.Background(), prod); err != nil {
		t.Fatalf("failed to create product: %v", err)
	}

	handler := &UpdateHandler{Repo: repo}

	reqBody := bytes.NewBufferString(`{"name":"New Name","category":"New Category","unitOfMeasure":"kg"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/products/prod-1", reqBody)
	req.SetPathValue("id", "prod-1")
	req.Pattern = "PUT /api/products/{id}"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	HandleJSON(handler.Handle)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var result product.Product
	if err := json.UnmarshalRead(w.Body, &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Name != "New Name" {
		t.Errorf("expected name 'New Name', got %q", result.Name)
	}
}
