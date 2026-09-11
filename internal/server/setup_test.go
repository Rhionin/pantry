package server

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/app"
	"github.com/Rhionin/pantry/internal/product"
	_ "modernc.org/sqlite"
)

// testProductCacheTTL is a test-friendly TTL for the Refresher constructed by
// setupTestWithDB. Tests that need a row to be fresh or stale move env.Clock
// rather than waiting on this value.
const testProductCacheTTL = 30 * 24 * time.Hour

// setupTestDB opens an in-memory SQLite database, applies all migrations, and returns the connection.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory SQLite: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return conn
}

// setupTestWithDB creates a fully configured HTTP handler and returns the DB
// along with it. The returned Refresher and fakeClock are the same instances
// wired into the handler's LookupService, so a test (or exchanges()) that
// needs to build a second handler against the same state must reuse them
// rather than constructing fresh ones — a second Refresher would own its own
// WaitGroup and in-flight map, and env.Refresher.Wait() would return without
// awaiting goroutines the second handler started.
func setupTestWithDB(t *testing.T) (http.Handler, *product.Catalog, *fakeOpenFoodFacts, *sql.DB, *product.Refresher, *fakeClock) {
	t.Helper()
	db := setupTestDB(t)

	productRepo := product.NewCatalog(db)
	fake := newFakeOpenFoodFacts()
	clock := newFakeClock(time.Now())
	refresher := &product.Refresher{
		Catalog:               productRepo,
		OpenFoodFacts:         fake,
		TTL:                   testProductCacheTTL,
		ExternalLookupEnabled: true,
		Now:                   clock.Now,
	}
	lookupService := &product.LookupService{
		Catalog:       productRepo,
		OpenFoodFacts: fake,
		Refresher:     refresher,
		Now:           clock.Now,
	}

	return NewHandler(productRepo, lookupService, refresher, db), productRepo, fake, db, refresher, clock
}

func setupProduct(id, name, category string) func(env testEnv) {
	return func(env testEnv) {
		prod := product.Product{ID: id, Name: name, Category: category}
		if err := env.ProductStore.CreateProduct(context.Background(), prod); err != nil {
			env.T.Fatalf("failed to create product: %v", err)
		}
	}
}

func setupProductWithBarcode(id, name, category, barcode string) func(env testEnv) {
	return func(env testEnv) {
		prod := product.Product{ID: id, Name: name, Category: category}
		if err := env.ProductStore.CreateProduct(context.Background(), prod); err != nil {
			env.T.Fatalf("failed to create product: %v", err)
		}
		if err := env.ProductStore.UpsertBarcodeMapping(context.Background(), barcode, id, "global", ""); err != nil {
			env.T.Fatalf("failed to create barcode mapping: %v", err)
		}
	}
}

// setupExternalProduct creates a source = 'external' product row, the kind
// the Refresher is willing to revalidate. refreshedAt nil leaves the row
// never-checked (stale); a non-nil value stamps it explicitly.
func setupExternalProduct(id, name, category string, refreshedAt *time.Time) func(env testEnv) {
	return func(env testEnv) {
		prod := product.Product{
			ID:          id,
			Name:        name,
			Category:    category,
			Source:      product.SourceExternal,
			RefreshedAt: refreshedAt,
		}
		if err := env.ProductStore.CreateProduct(context.Background(), prod); err != nil {
			env.T.Fatalf("failed to create external product: %v", err)
		}
	}
}

func setupMultipleProducts(products []product.Product) func(env testEnv) {
	return func(env testEnv) {
		for _, p := range products {
			if err := env.ProductStore.CreateProduct(context.Background(), p); err != nil {
				env.T.Fatalf("failed to create product: %v", err)
			}
		}
	}
}
