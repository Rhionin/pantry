package product_test

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/Rhionin/pantry/internal/app"
	"github.com/Rhionin/pantry/internal/product"
)

// newTestRepo opens an in-memory SQLite database, applies all migrations, and returns a product Repo.
func newTestRepo(t *testing.T) *product.Repo {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory SQLite: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return product.NewRepo(conn)
}

// --------------------------------------------------------------------------
// TestCreateProduct
// --------------------------------------------------------------------------

func TestCreateProduct(t *testing.T) {
	tests := []struct {
		name          string
		id            string
		product       product.Product
		wantIDEmpty   bool
		wantRoundtrip bool
	}{
		{
			name:          "explicit ID persisted",
			id:            "prod-1",
			product:       product.Product{ID: "prod-1", Name: "Whole Milk", Category: "Dairy", UnitOfMeasure: "gallon"},
			wantIDEmpty:   false,
			wantRoundtrip: true,
		},
		{
			name:          "generate UUID when ID empty",
			id:            "",
			product:       product.Product{Name: "Bread"},
			wantIDEmpty:   true,
			wantRoundtrip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newTestRepo(t)
			ctx := context.Background()

			// Create product
			if err := repo.CreateProduct(ctx, tt.product); err != nil {
				t.Fatalf("CreateProduct: %v", err)
			}

			// If UUID generation test, retrieve from list and verify ID was generated
			if tt.wantIDEmpty {
				products, err := repo.ListProducts(ctx)
				if err != nil {
					t.Fatalf("ListProducts: %v", err)
				}
				if len(products) != 1 {
					t.Fatalf("expected 1 product, got %d", len(products))
				}
				if products[0].ID == "" {
					t.Error("expected generated UUID, got empty string")
				}
				return
			}

			// Roundtrip test: verify GetProductByID retrieves the exact product
			if tt.wantRoundtrip {
				got, err := repo.GetProductByID(ctx, tt.id)
				if err != nil {
					t.Fatalf("GetProductByID: %v", err)
				}
				if got == nil {
					t.Fatal("expected product, got nil")
				}
				if got.ID != tt.product.ID {
					t.Errorf("ID: want %q, got %q", tt.product.ID, got.ID)
				}
				if got.Name != tt.product.Name {
					t.Errorf("Name: want %q, got %q", tt.product.Name, got.Name)
				}
				if got.Category != tt.product.Category {
					t.Errorf("Category: want %q, got %q", tt.product.Category, got.Category)
				}
				if got.UnitOfMeasure != tt.product.UnitOfMeasure {
					t.Errorf("UnitOfMeasure: want %q, got %q", tt.product.UnitOfMeasure, got.UnitOfMeasure)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestGetProductByID
// --------------------------------------------------------------------------

func TestGetProductByID(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		setup       func(t *testing.T, repo *product.Repo, ctx context.Context)
		expectFound bool
	}{
		{
			name: "found",
			id:   "prod-1",
			setup: func(t *testing.T, repo *product.Repo, ctx context.Context) {
				p := product.Product{ID: "prod-1", Name: "Whole Milk", Category: "Dairy"}
				if err := repo.CreateProduct(ctx, p); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}
			},
			expectFound: true,
		},
		{
			name:        "not found",
			id:          "no-such-id",
			setup:       func(t *testing.T, repo *product.Repo, ctx context.Context) {},
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newTestRepo(t)
			ctx := context.Background()
			tt.setup(t, repo, ctx)

			got, err := repo.GetProductByID(ctx, tt.id)
			if err != nil {
				t.Fatalf("GetProductByID: %v", err)
			}
			if tt.expectFound && got == nil {
				t.Fatal("expected product, got nil")
			}
			if !tt.expectFound && got != nil {
				t.Errorf("expected nil, got %+v", got)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestListProducts
// --------------------------------------------------------------------------

func TestListProducts(t *testing.T) {
	tests := []struct {
		name          string
		productsToAdd []product.Product
		expectCount   int
		expectOrder   []string // product names in expected order
	}{
		{
			name:          "empty",
			productsToAdd: []product.Product{},
			expectCount:   0,
			expectOrder:   []string{},
		},
		{
			name: "single product",
			productsToAdd: []product.Product{
				{ID: "p1", Name: "Apple Juice"},
			},
			expectCount: 1,
			expectOrder: []string{"Apple Juice"},
		},
		{
			name: "multiple products ordered by name",
			productsToAdd: []product.Product{
				{ID: "p1", Name: "Apple Juice"},
				{ID: "p2", Name: "Butter"},
				{ID: "p3", Name: "Cheese", Category: "Dairy"},
			},
			expectCount: 3,
			expectOrder: []string{"Apple Juice", "Butter", "Cheese"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newTestRepo(t)
			ctx := context.Background()

			// Add products
			for _, p := range tt.productsToAdd {
				if err := repo.CreateProduct(ctx, p); err != nil {
					t.Fatalf("CreateProduct %q: %v", p.Name, err)
				}
			}

			// List and verify
			got, err := repo.ListProducts(ctx)
			if err != nil {
				t.Fatalf("ListProducts: %v", err)
			}
			if len(got) != tt.expectCount {
				t.Fatalf("expected %d products, got %d", tt.expectCount, len(got))
			}

			// Verify order by name
			for i, wantName := range tt.expectOrder {
				if got[i].Name != wantName {
					t.Errorf("[%d] Name: want %q, got %q", i, wantName, got[i].Name)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestUpdateProduct
// --------------------------------------------------------------------------

func TestUpdateProduct(t *testing.T) {
	tests := []struct {
		name            string
		id              string
		originalProd    *product.Product // nil means don't create
		updatedProd     product.Product
		expectError     bool
		expectNameAfter string
	}{
		{
			name: "found and updated",
			id:   "p1",
			originalProd: &product.Product{
				ID:            "p1",
				Name:          "Whole Milk",
				Category:      "Dairy",
				UnitOfMeasure: "gallon",
			},
			updatedProd: product.Product{
				ID:            "p1",
				Name:          "Skim Milk",
				Category:      "Dairy",
				UnitOfMeasure: "half-gallon",
			},
			expectError:     false,
			expectNameAfter: "Skim Milk",
		},
		{
			name:            "not found",
			id:              "ghost",
			originalProd:    nil,
			updatedProd:     product.Product{ID: "ghost", Name: "Ghost Product"},
			expectError:     true,
			expectNameAfter: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newTestRepo(t)
			ctx := context.Background()

			// Setup
			if tt.originalProd != nil {
				if err := repo.CreateProduct(ctx, *tt.originalProd); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}
			}

			// Update
			err := repo.UpdateProduct(ctx, tt.updatedProd)
			if tt.expectError && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Fatalf("UpdateProduct: %v", err)
			}

			// Verify if successful
			if !tt.expectError {
				got, err := repo.GetProductByID(ctx, tt.id)
				if err != nil {
					t.Fatalf("GetProductByID: %v", err)
				}
				if got.Name != tt.expectNameAfter {
					t.Errorf("Name: want %q, got %q", tt.expectNameAfter, got.Name)
				}
				if got.UnitOfMeasure != tt.updatedProd.UnitOfMeasure {
					t.Errorf("UnitOfMeasure: want %q, got %q", tt.updatedProd.UnitOfMeasure, got.UnitOfMeasure)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestUpsertBarcodeMapping
// --------------------------------------------------------------------------

func TestUpsertBarcodeMapping(t *testing.T) {
	tests := []struct {
		name           string
		barcode        string
		productID1     string
		productID2     string // for replace test
		expectIDAfter1 string
		expectIDAfter2 string // for replace test
	}{
		{
			name:           "insert new mapping",
			barcode:        "012345678901",
			productID1:     "p1",
			productID2:     "",
			expectIDAfter1: "p1",
			expectIDAfter2: "",
		},
		{
			name:           "replace existing mapping",
			barcode:        "012345678901",
			productID1:     "p1",
			productID2:     "p2",
			expectIDAfter1: "p1",
			expectIDAfter2: "p2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newTestRepo(t)
			ctx := context.Background()

			// Create products
			p1 := product.Product{ID: "p1", Name: "Product One"}
			p2 := product.Product{ID: "p2", Name: "Product Two"}
			for _, p := range []product.Product{p1, p2} {
				if err := repo.CreateProduct(ctx, p); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}
			}

			// Insert mapping
			if err := repo.UpsertBarcodeMapping(ctx, tt.barcode, tt.productID1, "global", ""); err != nil {
				t.Fatalf("UpsertBarcodeMapping (insert): %v", err)
			}

			single, _, err := repo.LookupByBarcode(ctx, tt.barcode, "user-123")
			if err != nil {
				t.Fatalf("LookupByBarcode: %v", err)
			}
			if single == nil || single.ID != tt.expectIDAfter1 {
				t.Fatalf("after insert: expected %s, got %v", tt.expectIDAfter1, single)
			}

			// If we're testing replace, do it now
			if tt.productID2 != "" {
				if err := repo.UpsertBarcodeMapping(ctx, tt.barcode, tt.productID2, "global", ""); err != nil {
					t.Fatalf("UpsertBarcodeMapping (replace): %v", err)
				}

				single, _, err = repo.LookupByBarcode(ctx, tt.barcode, "user-123")
				if err != nil {
					t.Fatalf("LookupByBarcode after replace: %v", err)
				}
				if single == nil || single.ID != tt.expectIDAfter2 {
					t.Fatalf("after replace: expected %s, got %v", tt.expectIDAfter2, single)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestLookupByBarcode
// --------------------------------------------------------------------------

func TestLookupByBarcode(t *testing.T) {
	tests := []struct {
		name            string
		barcode         string
		userID          string
		setupProducts   []product.Product
		setupMappings   []struct{ barcode, productID, source, userID string }
		expectSingle    *string // nil means expect nil single, otherwise product ID
		expectListCount int
	}{
		{
			name:            "no match",
			barcode:         "000000000000",
			userID:          "user-1",
			setupProducts:   []product.Product{},
			setupMappings:   []struct{ barcode, productID, source, userID string }{},
			expectSingle:    nil,
			expectListCount: 0,
		},
		{
			name:    "user override takes precedence over global",
			barcode: "111222333444",
			userID:  "user-abc",
			setupProducts: []product.Product{
				{ID: "global-prod", Name: "Global Product"},
				{ID: "override-prod", Name: "Override Product"},
			},
			setupMappings: []struct{ barcode, productID, source, userID string }{
				{"111222333444", "global-prod", "global", ""},
				{"111222333444", "override-prod", "user_override", "user-abc"},
			},
			expectSingle:    stringPtr("override-prod"),
			expectListCount: 0,
		},
		{
			name:    "different user sees global (not another user's override)",
			barcode: "555666777888",
			userID:  "user-B",
			setupProducts: []product.Product{
				{ID: "global-prod", Name: "Global Product"},
				{ID: "override-prod", Name: "Override Product"},
			},
			setupMappings: []struct{ barcode, productID, source, userID string }{
				{"555666777888", "global-prod", "global", ""},
				{"555666777888", "override-prod", "user_override", "user-A"},
			},
			expectSingle:    stringPtr("global-prod"),
			expectListCount: 0,
		},
		{
			name:    "global-only lookup",
			barcode: "999888777666",
			userID:  "user-xyz",
			setupProducts: []product.Product{
				{ID: "p1", Name: "Product A"},
			},
			setupMappings: []struct{ barcode, productID, source, userID string }{
				{"999888777666", "p1", "global", ""},
			},
			expectSingle:    stringPtr("p1"),
			expectListCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newTestRepo(t)
			ctx := context.Background()

			// Create products
			for _, p := range tt.setupProducts {
				if err := repo.CreateProduct(ctx, p); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}
			}

			// Create mappings
			for _, m := range tt.setupMappings {
				if err := repo.UpsertBarcodeMapping(ctx, m.barcode, m.productID, m.source, m.userID); err != nil {
					t.Fatalf("UpsertBarcodeMapping: %v", err)
				}
			}

			// Lookup
			single, list, err := repo.LookupByBarcode(ctx, tt.barcode, tt.userID)
			if err != nil {
				t.Fatalf("LookupByBarcode: %v", err)
			}

			// Verify single result
			if tt.expectSingle == nil {
				if single != nil {
					t.Errorf("expected nil single, got %v", single)
				}
			} else {
				if single == nil {
					t.Fatal("expected single result, got nil")
				}
				if single.ID != *tt.expectSingle {
					t.Errorf("expected single ID %q, got %q", *tt.expectSingle, single.ID)
				}
			}

			// Verify list
			if len(list) != tt.expectListCount {
				t.Errorf("expected list count %d, got %d", tt.expectListCount, len(list))
			}
		})
	}
}

// stringPtr is a helper to convert a string to *string for table-driven tests.
func stringPtr(s string) *string {
	return &s
}
