package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/go-json-experiment/json"
	_ "modernc.org/sqlite"
)

func TestCreateHandler(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := product.NewRepo(db)
	handler := &CreateHandler{Repo: repo}

	reqBody := bytes.NewBufferString(`{"name":"Orange","category":"Fruit","unitOfMeasure":"each"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/products", reqBody)
	req.Pattern = "POST /api/products"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	HandleJSON(handler.Handle)(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	var result product.Product
	if err := json.UnmarshalRead(w.Body, &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Name != "Orange" {
		t.Errorf("expected name 'Orange', got %q", result.Name)
	}
	if result.Category != "Fruit" {
		t.Errorf("expected category 'Fruit', got %q", result.Category)
	}
}

func TestCreateHandler_MissingName(t *testing.T) {
	handler := &CreateHandler{Repo: nil}

	reqBody := bytes.NewBufferString(`{"category":"Fruit"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/products", reqBody)
	req.Pattern = "POST /api/products"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	HandleJSON(handler.Handle)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}
