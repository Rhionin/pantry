package product

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/Rhionin/pantry/internal/app"
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

// mockOpenFoodFacts creates a mock Open Food Facts API with the given lookup function.
type mockOpenFoodFacts struct {
	lookupFn func(ctx context.Context, barcode string) (*ProductSummary, error)
}

func (m mockOpenFoodFacts) LookupBarcode(ctx context.Context, barcode string) (*ProductSummary, error) {
	return m.lookupFn(ctx, barcode)
}

// TestLookupService verifies the three-tier lookup behavior using table-driven tests.
func TestLookupService(t *testing.T) {
	tests := []struct {
		name              string
		setupDB           func(t *testing.T, repo *Repo)
		openFoodFacts     func(ctx context.Context, barcode string) (*ProductSummary, error)
		barcode           string
		userID            string
		expectedFound     bool
		expectedSource    string
		expectedProductID string
		errMsg            string
	}{
		{
			name: "user override takes precedence over global",
			setupDB: func(t *testing.T, repo *Repo) {
				global := Product{ID: "prod-global", Name: "Global Product"}
				override := Product{ID: "prod-override", Name: "Override Product"}
				if err := repo.CreateProduct(context.Background(), global); err != nil {
					t.Fatal(err)
				}
				if err := repo.CreateProduct(context.Background(), override); err != nil {
					t.Fatal(err)
				}
				if err := repo.UpsertBarcodeMapping(context.Background(), "12345", global.ID, "global", ""); err != nil {
					t.Fatal(err)
				}
				if err := repo.UpsertBarcodeMapping(context.Background(), "12345", override.ID, "user_override", "user-1"); err != nil {
					t.Fatal(err)
				}
			},
			openFoodFacts: func(ctx context.Context, barcode string) (*ProductSummary, error) {
				return nil, ErrProductNotFound
			},
			barcode:           "12345",
			userID:            "user-1",
			expectedFound:     true,
			expectedSource:    "global",
			expectedProductID: "prod-override",
		},
		{
			name: "global DB used when no user override exists",
			setupDB: func(t *testing.T, repo *Repo) {
				product := Product{ID: "prod-1", Name: "Milk"}
				if err := repo.CreateProduct(context.Background(), product); err != nil {
					t.Fatal(err)
				}
				if err := repo.UpsertBarcodeMapping(context.Background(), "11111", product.ID, "global", ""); err != nil {
					t.Fatal(err)
				}
			},
			openFoodFacts: func(ctx context.Context, barcode string) (*ProductSummary, error) {
				return nil, ErrProductNotFound
			},
			barcode:           "11111",
			userID:            "user-1",
			expectedFound:     true,
			expectedSource:    "global",
			expectedProductID: "prod-1",
		},
		{
			name:    "external API fallback when no DB match",
			setupDB: func(t *testing.T, repo *Repo) {},
			openFoodFacts: func(ctx context.Context, barcode string) (*ProductSummary, error) {
				if barcode == "99999" {
					return &ProductSummary{ID: "99999", Name: "External Product", Category: "Food"}, nil
				}
				return nil, ErrProductNotFound
			},
			barcode:           "99999",
			userID:            "user-1",
			expectedFound:     true,
			expectedSource:    "external",
			expectedProductID: "99999",
		},
		{
			name:    "not found in any tier",
			setupDB: func(t *testing.T, repo *Repo) {},
			openFoodFacts: func(ctx context.Context, barcode string) (*ProductSummary, error) {
				return nil, ErrProductNotFound
			},
			barcode:       "00000",
			userID:        "user-1",
			expectedFound: false,
		},
		{
			name:    "external API error treated as not found",
			setupDB: func(t *testing.T, repo *Repo) {},
			openFoodFacts: func(ctx context.Context, barcode string) (*ProductSummary, error) {
				return nil, errors.New("network timeout")
			},
			barcode:       "88888",
			userID:        "user-1",
			expectedFound: false,
		},
		{
			name:    "empty barcode returns error",
			setupDB: func(t *testing.T, repo *Repo) {},
			openFoodFacts: func(ctx context.Context, barcode string) (*ProductSummary, error) {
				return nil, ErrProductNotFound
			},
			barcode: "",
			userID:  "user-1",
			errMsg:  "barcode cannot be empty",
		},
		{
			name:    "empty userID returns error",
			setupDB: func(t *testing.T, repo *Repo) {},
			openFoodFacts: func(ctx context.Context, barcode string) (*ProductSummary, error) {
				return nil, ErrProductNotFound
			},
			barcode: "12345",
			userID:  "",
			errMsg:  "userID cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			repo := NewRepo(db)
			tt.setupDB(t, repo)

			service := &LookupService{
				Repo:          repo,
				OpenFoodFacts: mockOpenFoodFacts{lookupFn: tt.openFoodFacts},
			}
			actual, err := service.Lookup(context.Background(), tt.barcode, tt.userID)

			// Check error
			if tt.errMsg != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errMsg)
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, expected to contain %q", err.Error(), tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check found status
			if actual.IsFound() != tt.expectedFound {
				t.Errorf("IsFound() = %v, expected %v", actual.IsFound(), tt.expectedFound)
			}

			// Check product details if found
			if tt.expectedFound {
				if actual.Product == nil {
					t.Fatal("Product is nil, expected non-nil")
				}
				if actual.Product.ID != tt.expectedProductID {
					t.Errorf("Product.ID = %q, expected %q", actual.Product.ID, tt.expectedProductID)
				}
				if actual.Source != tt.expectedSource {
					t.Errorf("Source = %q, expected %q", actual.Source, tt.expectedSource)
				}
			} else {
				if actual.Product != nil {
					t.Errorf("Product = %v, expected nil", actual.Product)
				}
			}
		})
	}
}
