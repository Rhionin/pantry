package server

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
)

func TestInventoryInstanceCreateHandler(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	tests := []scanHandlerTestCase{
		{
			name: "creates instance with expiration date",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product and item
				prod := product.Product{
					ID:            "prod-1",
					Name:          "Milk",
					Category:      "Dairy",
					UnitOfMeasure: "gallon",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES ('item-1', 'user-1', 'prod-1')`)
				if err != nil {
					t.Fatalf("create item: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/inventory/item-1/instances",
				body:           `{"expiresAt":"` + expiresAt.Format(time.RFC3339) + `"}`,
				expectedStatus: http.StatusCreated,
				assertions: []assertion{
					{path: "$.ItemID", value: "item-1"},
				},
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify instance was created
				var count int
				err := db.QueryRowContext(context.Background(), `
					SELECT COUNT(*) FROM item_instances
					WHERE item_id = 'item-1' AND removed_at IS NULL`,
				).Scan(&count)
				if err != nil {
					t.Fatalf("count instances: %v", err)
				}
				if count != 1 {
					t.Errorf("expected 1 instance, got %d", count)
				}

				// Verify expiration date
				var expiresAtDB sql.NullTime
				err = db.QueryRowContext(context.Background(), `
					SELECT expires_at FROM item_instances
					WHERE item_id = 'item-1' AND removed_at IS NULL`,
				).Scan(&expiresAtDB)
				if err != nil {
					t.Fatalf("query expires_at: %v", err)
				}
				if !expiresAtDB.Valid {
					t.Error("expected expires_at to be set")
				}
			},
		},
		{
			name: "creates instance without expiration date",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product and item
				prod := product.Product{
					ID:            "prod-2",
					Name:          "Canned Beans",
					Category:      "Canned",
					UnitOfMeasure: "can",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES ('item-2', 'user-1', 'prod-2')`)
				if err != nil {
					t.Fatalf("create item: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/inventory/item-2/instances",
				body:           `{}`,
				expectedStatus: http.StatusCreated,
				assertions: []assertion{
					{path: "$.ItemID", value: "item-2"},
				},
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify instance was created
				var count int
				err := db.QueryRowContext(context.Background(), `
					SELECT COUNT(*) FROM item_instances
					WHERE item_id = 'item-2' AND removed_at IS NULL`,
				).Scan(&count)
				if err != nil {
					t.Fatalf("count instances: %v", err)
				}
				if count != 1 {
					t.Errorf("expected 1 instance, got %d", count)
				}

				// Verify expiration date is NULL
				var expiresAtDB sql.NullTime
				err = db.QueryRowContext(context.Background(), `
					SELECT expires_at FROM item_instances
					WHERE item_id = 'item-2' AND removed_at IS NULL`,
				).Scan(&expiresAtDB)
				if err != nil {
					t.Fatalf("query expires_at: %v", err)
				}
				if expiresAtDB.Valid {
					t.Error("expected expires_at to be NULL")
				}
			},
		},
		{
			name: "sets stock_in_at to current time",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product and item
				prod := product.Product{
					ID:            "prod-3",
					Name:          "Bread",
					Category:      "Bakery",
					UnitOfMeasure: "loaf",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES ('item-3', 'user-1', 'prod-3')`)
				if err != nil {
					t.Fatalf("create item: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/inventory/item-3/instances",
				body:           `{}`,
				expectedStatus: http.StatusCreated,
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify stock_in_at is recent (within 5 seconds)
				var stockInAt time.Time
				err := db.QueryRowContext(context.Background(), `
					SELECT stock_in_at FROM item_instances
					WHERE item_id = 'item-3' AND removed_at IS NULL`,
				).Scan(&stockInAt)
				if err != nil {
					t.Fatalf("query stock_in_at: %v", err)
				}

				diff := time.Since(stockInAt)
				if diff > 5*time.Second {
					t.Errorf("stock_in_at is too old: %v ago", diff)
				}
			},
		},
		{
			name: "allows multiple instances for same item",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product and item
				prod := product.Product{
					ID:            "prod-multi",
					Name:          "Yogurt",
					Category:      "Dairy",
					UnitOfMeasure: "cup",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES ('item-multi', 'user-1', 'prod-multi')`)
				if err != nil {
					t.Fatalf("create item: %v", err)
				}

				// Create an existing instance
				_, err = db.ExecContext(context.Background(), `
					INSERT INTO item_instances (id, item_id, stock_in_at, expires_at)
					VALUES ('inst-existing', 'item-multi', ?, ?)`,
					now, sql.NullTime{Time: expiresAt, Valid: true})
				if err != nil {
					t.Fatalf("create existing instance: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/inventory/item-multi/instances",
				body:           `{"expiresAt":"` + expiresAt.Format(time.RFC3339) + `"}`,
				expectedStatus: http.StatusCreated,
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify two instances exist
				var count int
				err := db.QueryRowContext(context.Background(), `
					SELECT COUNT(*) FROM item_instances
					WHERE item_id = 'item-multi' AND removed_at IS NULL`,
				).Scan(&count)
				if err != nil {
					t.Fatalf("count instances: %v", err)
				}
				if count != 2 {
					t.Errorf("expected 2 instances, got %d", count)
				}
			},
		},
	}

	runScanHandlerTests(t, tests)
}
