package server

import (
	"context"
	"sync"

	"github.com/Rhionin/pantry/internal/product"
)

// fakeOpenFoodFacts implements product.OpenFoodFactsClient for testing.
// It maintains an in-memory store of products keyed by barcode, simulating the real Open Food Facts API.
//
// Background revalidation goroutines call LookupBarcode concurrently with the test body, so every
// map is guarded by mu.
type fakeOpenFoodFacts struct {
	mu       sync.Mutex
	store    map[string]*product.ProductSummary
	errs     map[string]error
	calls    map[string]int
	blocking map[string]<-chan struct{}
}

// newFakeOpenFoodFacts creates an empty fake Open Food Facts client.
// At some future point, we can seed this with realistic test data.
func newFakeOpenFoodFacts() *fakeOpenFoodFacts {
	return &fakeOpenFoodFacts{
		store:    make(map[string]*product.ProductSummary),
		errs:     make(map[string]error),
		calls:    make(map[string]int),
		blocking: make(map[string]<-chan struct{}),
	}
}

// LookupBarcode looks up a product by barcode in the in-memory store.
// Returns ErrProductNotFound if the barcode is not in the store.
//
// Precedence: a registered blocking channel is waited on first, then a seeded error is
// returned if present, then the store is consulted. Every call is counted, including
// blocked, error, and not-found calls.
func (f *fakeOpenFoodFacts) LookupBarcode(ctx context.Context, barcode string) (*product.ProductSummary, error) {
	f.mu.Lock()
	f.calls[barcode]++
	release := f.blocking[barcode]
	f.mu.Unlock()

	if release != nil {
		<-release
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if err, ok := f.errs[barcode]; ok {
		return nil, err
	}

	prod, ok := f.store[barcode]
	if !ok {
		return nil, product.ErrProductNotFound
	}
	return prod, nil
}

// Seed adds a product to the fake's in-memory store for the given barcode.
// This allows tests to simulate products existing in Open Food Facts.
//
// Seed overwrites any existing entry for barcode, and LookupBarcode reads the map at call
// time. Re-seeding a barcode between the cache fill and the refresh is how tests make the
// upstream answer differ from what was cached.
func (f *fakeOpenFoodFacts) Seed(barcode string, prod *product.ProductSummary) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.store[barcode] = prod
}

// SeedError records an error to return for barcode instead of a product. This is distinct
// from an absent barcode, which keeps yielding ErrProductNotFound.
func (f *fakeOpenFoodFacts) SeedError(barcode string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs[barcode] = err
}

// SeedBlocking makes LookupBarcode for barcode block until release is closed or receives a
// value, before falling through to the seeded error or store lookup. This lets tests hold a
// call open for concurrency assertions.
func (f *fakeOpenFoodFacts) SeedBlocking(barcode string, release <-chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blocking[barcode] = release
}

// CallCount returns the number of times LookupBarcode has been called for barcode.
func (f *fakeOpenFoodFacts) CallCount(barcode string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[barcode]
}
