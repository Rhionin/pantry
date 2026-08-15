package server

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
)

func TestInventoryInstanceDeleteHandler(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	tests := []scanHandlerTestCase{
		{
			name: "deletes instance successfully",
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

				// Create instance to delete
				_, err = db.ExecContext(context.Background(), `
					INSERT INTO item_instances (id, item_id, stock_in_at, expires_at)
					VALUES ('inst-1', 'item-1', ?, ?)`,
					now, sql.NullTime{Time: expiresAt, Valid: true})
				if err != nil {
					t.Fatalf("create instance: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "DELETE",
				path:           "/api/inventory/instances/inst-1",
				expectedStatus: http.StatusOK,
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify instance is marked as removed
				var removedAt sql.NullTime
				var removalReason sql.NullString
				err := db.QueryRowContext(context.Background(), `
					SELECT removed_at, removal_reason FROM item_instances
					WHERE id = 'inst-1'`,
				).Scan(&removedAt, &removalReason)
				if err != nil {
					t.Fatalf("query instance: %v", err)
				}

				if !removedAt.Valid {
					t.Error("expected removed_at to be set")
				}
				if removalReason.String != "manual" {
					t.Errorf("expected removal_reason 'manual', got %q", removalReason.String)
				}
			},
		},
		{
			name: "returns 404 when instance does not exist",
			httpExchange: httpExchange{
				method:         "DELETE",
				path:           "/api/inventory/instances/nonexistent",
				expectedStatus: http.StatusNotFound,
			},
		},
		{
			name: "returns 404 when instance already removed",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product and item
				prod := product.Product{
					ID:            "prod-removed",
					Name:          "Bread",
					Category:      "Bakery",
					UnitOfMeasure: "loaf",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES ('item-removed', 'user-1', 'prod-removed')`)
				if err != nil {
					t.Fatalf("create item: %v", err)
				}

				// Create instance that's already removed
				_, err = db.ExecContext(context.Background(), `
					INSERT INTO item_instances (id, item_id, stock_in_at, expires_at, removed_at, removal_reason)
					VALUES ('inst-removed', 'item-removed', ?, ?, ?, 'consumed')`,
					now.Add(-24*time.Hour),
					sql.NullTime{Time: expiresAt, Valid: true},
					sql.NullTime{Time: now, Valid: true})
				if err != nil {
					t.Fatalf("create removed instance: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "DELETE",
				path:           "/api/inventory/instances/inst-removed",
				expectedStatus: http.StatusNotFound,
			},
		},
		{
			name: "does not affect other instances",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product and item
				prod := product.Product{
					ID:            "prod-multi",
					Name:          "Eggs",
					Category:      "Dairy",
					UnitOfMeasure: "dozen",
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

				// Create multiple instances
				for i := 1; i <= 3; i++ {
					_, err = db.ExecContext(context.Background(), `
						INSERT INTO item_instances (id, item_id, stock_in_at, expires_at)
						VALUES (?, 'item-multi', ?, ?)`,
						"inst-multi-"+string(rune('0'+i)), now, sql.NullTime{Time: expiresAt, Valid: true})
					if err != nil {
						t.Fatalf("create instance %d: %v", i, err)
					}
				}
			},
			httpExchange: httpExchange{
				method:         "DELETE",
				path:           "/api/inventory/instances/inst-multi-2",
				expectedStatus: http.StatusOK,
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify inst-multi-2 is removed
				var removedAt sql.NullTime
				err := db.QueryRowContext(context.Background(), `
					SELECT removed_at FROM item_instances WHERE id = 'inst-multi-2'`,
				).Scan(&removedAt)
				if err != nil {
					t.Fatalf("query removed instance: %v", err)
				}
				if !removedAt.Valid {
					t.Error("expected inst-multi-2 to be removed")
				}

				// Verify other instances are still active
				var activeCount int
				err = db.QueryRowContext(context.Background(), `
					SELECT COUNT(*) FROM item_instances
					WHERE item_id = 'item-multi' AND removed_at IS NULL`,
				).Scan(&activeCount)
				if err != nil {
					t.Fatalf("count active instances: %v", err)
				}
				if activeCount != 2 {
					t.Errorf("expected 2 active instances, got %d", activeCount)
				}
			},
		},
		{
			name: "idempotent - attempting to delete again does not change state",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product and item
				prod := product.Product{
					ID:            "prod-idem",
					Name:          "Cheese",
					Category:      "Dairy",
					UnitOfMeasure: "block",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES ('item-idem', 'user-1', 'prod-idem')`)
				if err != nil {
					t.Fatalf("create item: %v", err)
				}

				// Create instance
				_, err = db.ExecContext(context.Background(), `
					INSERT INTO item_instances (id, item_id, stock_in_at, expires_at)
					VALUES ('inst-idem', 'item-idem', ?, ?)`,
					now, sql.NullTime{Time: expiresAt, Valid: true})
				if err != nil {
					t.Fatalf("create instance: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "DELETE",
				path:           "/api/inventory/instances/inst-idem",
				expectedStatus: http.StatusOK,
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify instance is marked as removed
				var removedAt sql.NullTime
				err := db.QueryRowContext(context.Background(), `
					SELECT removed_at FROM item_instances WHERE id = 'inst-idem'`,
				).Scan(&removedAt)
				if err != nil {
					t.Fatalf("query instance: %v", err)
				}
				if !removedAt.Valid {
					t.Error("expected inst-idem to be removed after first delete")
				}
			},
		},
	}

	runScanHandlerTests(t, tests)
}
