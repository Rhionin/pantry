package server

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/product"
	"github.com/go-json-experiment/json"
	"github.com/google/uuid"
)

func TestInventoryListHandler(t *testing.T) {
	now := time.Now()

	tests := []scanHandlerTestCase{
		{
			name: "returns empty list when no inventory",
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/inventory",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$", value: []interface{}{}},
				},
			},
		},
		{
			name: "returns inventory with instance counts",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create products
				prod1 := product.Product{
					ID:            "prod-1",
					Name:          "Milk",
					Category:      "Dairy",
					UnitOfMeasure: "gallon",
				}
				prod2 := product.Product{
					ID:            "prod-2",
					Name:          "Bread",
					Category:      "Bakery",
					UnitOfMeasure: "loaf",
				}
				if err := repo.CreateProduct(context.Background(), prod1); err != nil {
					t.Fatalf("CreateProduct prod1: %v", err)
				}
				if err := repo.CreateProduct(context.Background(), prod2); err != nil {
					t.Fatalf("CreateProduct prod2: %v", err)
				}

				// Create items
				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES ('item-1', 'user-1', 'prod-1'), ('item-2', 'user-1', 'prod-2')`)
				if err != nil {
					t.Fatalf("create items: %v", err)
				}

				farFuture := now.Add(14 * 24 * time.Hour)
				// Create instances for item-1 (3 instances)
				for i := 1; i <= 3; i++ {
					_, err := db.ExecContext(context.Background(), `
						INSERT INTO item_instances (id, item_id, stock_in_at, expires_at)
						VALUES (?, 'item-1', ?, ?)`,
						"inst-1-"+string(rune('0'+i)), now, sql.NullTime{Time: farFuture, Valid: true})
					if err != nil {
						t.Fatalf("create instance: %v", err)
					}
				}

				// Create instances for item-2 (2 instances)
				for i := 1; i <= 2; i++ {
					_, err := db.ExecContext(context.Background(), `
						INSERT INTO item_instances (id, item_id, stock_in_at, expires_at)
						VALUES (?, 'item-2', ?, ?)`,
						"inst-2-"+string(rune('0'+i)), now, sql.NullTime{Time: farFuture, Valid: true})
					if err != nil {
						t.Fatalf("create instance: %v", err)
					}
				}
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/inventory",
				expectedStatus: http.StatusOK,
			},
		},
	}

	runScanHandlerTests(t, tests)
}

func TestInventoryListHandler_WithSearchQuery(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	invRepo := inventory.NewRepo(db)
	prodRepo := product.NewRepo(db)
	ctx := context.Background()
	userID := "user-1"

	// Create multiple products
	products := []struct {
		name     string
		category string
	}{
		{"Milk", "Dairy"},
		{"Bread", "Bakery"},
		{"Cheese", "Dairy"},
	}

	for _, p := range products {
		prodID := uuid.NewString()
		err := prodRepo.CreateProduct(ctx, product.Product{
			ID:            prodID,
			Name:          p.name,
			Category:      p.category,
			UnitOfMeasure: "unit",
		})
		if err != nil {
			t.Fatalf("CreateProduct failed: %v", err)
		}

		_, err = invRepo.GetOrCreateItem(ctx, userID, prodID)
		if err != nil {
			t.Fatalf("GetOrCreateItem failed: %v", err)
		}
	}

	tests := []struct {
		name         string
		queryParam   string
		expectedLen  int
		expectedName string
	}{
		{
			name:        "no query returns all",
			queryParam:  "",
			expectedLen: 3,
		},
		{
			name:         "search by name",
			queryParam:   "milk",
			expectedLen:  1,
			expectedName: "Milk",
		},
		{
			name:        "search by category",
			queryParam:  "dairy",
			expectedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/api/inventory"
			if tt.queryParam != "" {
				path += "?q=" + tt.queryParam
			}

			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()

			handler := &InventoryListHandler{Repo: invRepo, UserID: userID}
			h := HandleJSON(handler.Handle)
			h(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
			}

			var items []inventory.InventoryItem
			if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if len(items) != tt.expectedLen {
				t.Fatalf("expected %d items, got %d", tt.expectedLen, len(items))
			}

			if tt.expectedName != "" && items[0].Item.Product.Name != tt.expectedName {
				t.Fatalf("expected item name %s, got %s", tt.expectedName, items[0].Item.Product.Name)
			}
		})
	}
}
