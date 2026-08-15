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

// setupTest creates a fully configured HTTP handler with in-memory database,
// a product repository, and a fake OpenFoodFacts client for testing.
func setupTest(t *testing.T) (http.Handler, *product.Repo, *fakeOpenFoodFacts) {
	t.Helper()
	db := setupTestDB(t)

	productRepo := product.NewRepo(db)
	fake := newFakeOpenFoodFacts()
	lookupService := &product.LookupService{
		Repo:          productRepo,
		OpenFoodFacts: fake,
	}

	return NewHandler(productRepo, lookupService, db), productRepo, fake
}

// setupTestWithDB creates a fully configured HTTP handler and returns the DB along with it.
// Use this when setup functions need access to the DB for creating non-product entities.
func setupTestWithDB(t *testing.T) (http.Handler, *product.Repo, *fakeOpenFoodFacts, *sql.DB) {
	t.Helper()
	db := setupTestDB(t)

	productRepo := product.NewRepo(db)
	fake := newFakeOpenFoodFacts()
	lookupService := &product.LookupService{
		Repo:          productRepo,
		OpenFoodFacts: fake,
	}

	return NewHandler(productRepo, lookupService, db), productRepo, fake, db
}

func setupProduct(id, name, category string) func(t *testing.T, repo *product.Repo, fake *fakeOpenFoodFacts) {
	return func(t *testing.T, repo *product.Repo, fake *fakeOpenFoodFacts) {
		prod := product.Product{ID: id, Name: name, Category: category}
		if err := repo.CreateProduct(context.Background(), prod); err != nil {
			t.Fatalf("failed to create product: %v", err)
		}
	}
}

func setupProductWithBarcode(id, name, category, barcode string) func(t *testing.T, repo *product.Repo, fake *fakeOpenFoodFacts) {
	return func(t *testing.T, repo *product.Repo, fake *fakeOpenFoodFacts) {
		prod := product.Product{ID: id, Name: name, Category: category}
		if err := repo.CreateProduct(context.Background(), prod); err != nil {
			t.Fatalf("failed to create product: %v", err)
		}
		if err := repo.UpsertBarcodeMapping(context.Background(), barcode, id, "global", ""); err != nil {
			t.Fatalf("failed to create barcode mapping: %v", err)
		}
	}
}

func setupMultipleProducts(products []product.Product) func(t *testing.T, repo *product.Repo, fake *fakeOpenFoodFacts) {
	return func(t *testing.T, repo *product.Repo, fake *fakeOpenFoodFacts) {
		for _, p := range products {
			if err := repo.CreateProduct(context.Background(), p); err != nil {
				t.Fatalf("failed to create product: %v", err)
			}
		}
	}
}
