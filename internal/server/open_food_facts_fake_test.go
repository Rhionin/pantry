package server

import (
	"context"

	"github.com/Rhionin/pantry/internal/product"
)

// fakeOpenFoodFacts implements product.OpenFoodFactsClient for testing.
// It maintains an in-memory store of products keyed by barcode, simulating the real Open Food Facts API.
type fakeOpenFoodFacts struct {
	store map[string]*product.ProductSummary
}

// newFakeOpenFoodFacts creates an empty fake Open Food Facts client.
// At some future point, we can seed this with realistic test data.
func newFakeOpenFoodFacts() *fakeOpenFoodFacts {
	return &fakeOpenFoodFacts{
		store: make(map[string]*product.ProductSummary),
	}
}

// LookupBarcode looks up a product by barcode in the in-memory store.
// Returns ErrProductNotFound if the barcode is not in the store.
func (f *fakeOpenFoodFacts) LookupBarcode(ctx context.Context, barcode string) (*product.ProductSummary, error) {
	prod, ok := f.store[barcode]
	if !ok {
		return nil, product.ErrProductNotFound
	}
	return prod, nil
}

// Seed adds a product to the fake's in-memory store for the given barcode.
// This allows tests to simulate products existing in Open Food Facts.
func (f *fakeOpenFoodFacts) Seed(barcode string, prod *product.ProductSummary) {
	f.store[barcode] = prod
}
