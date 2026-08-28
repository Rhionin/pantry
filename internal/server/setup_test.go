package server

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	"github.com/Rhionin/pantry/internal/app"
	"github.com/Rhionin/pantry/internal/product"
	_ "modernc.org/sqlite"
)

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

// setupTestWithDB creates a fully configured HTTP handler and returns the DB along with it.
func setupTestWithDB(t *testing.T) (http.Handler, *product.Catalog, *fakeOpenFoodFacts, *sql.DB) {
	t.Helper()
	db := setupTestDB(t)

	productRepo := product.NewCatalog(db)
	fake := newFakeOpenFoodFacts()
	lookupService := &product.LookupService{
		Catalog:       productRepo,
		OpenFoodFacts: fake,
	}

	return NewHandler(productRepo, lookupService, db), productRepo, fake, db
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

func setupMultipleProducts(products []product.Product) func(env testEnv) {
	return func(env testEnv) {
		for _, p := range products {
			if err := env.ProductStore.CreateProduct(context.Background(), p); err != nil {
				env.T.Fatalf("failed to create product: %v", err)
			}
		}
	}
}
