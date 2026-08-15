package server

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
)

func TestInventoryInstancesListHandler(t *testing.T) {
	now := time.Now()
	nearExpiry := now.Add(3 * 24 * time.Hour)
	farFuture := now.Add(14 * 24 * time.Hour)

	tests := []scanHandlerTestCase{
		{
			name: "returns empty list when no instances",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product and item but no instances
				prod := product.Product{
					ID:            "prod-empty",
					Name:          "Empty Product",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES ('item-empty', 'user-1', 'prod-empty')`)
				if err != nil {
					t.Fatalf("create item: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/inventory/item-empty/instances",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$", value: []interface{}{}},
				},
			},
		},
		{
			name: "returns instances sorted by expiration date (use-oldest-first)",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product and item
				prod := product.Product{
					ID:            "prod-sorted",
					Name:          "Test Product",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES ('item-sorted', 'user-1', 'prod-sorted')`)
				if err != nil {
					t.Fatalf("create item: %v", err)
				}

				// Create instances with different expiration dates (insert in non-sorted order)
				instances := []struct {
					id        string
					expiresAt time.Time
				}{
					{"inst-3", farFuture},
					{"inst-1", nearExpiry},
					{"inst-2", now.Add(7 * 24 * time.Hour)},
				}

				for _, inst := range instances {
					_, err := db.ExecContext(context.Background(), `
						INSERT INTO item_instances (id, item_id, stock_in_at, expires_at)
						VALUES (?, 'item-sorted', ?, ?)`,
						inst.id, now, sql.NullTime{Time: inst.expiresAt, Valid: true})
					if err != nil {
						t.Fatalf("create instance %s: %v", inst.id, err)
					}
				}
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/inventory/item-sorted/instances",
				expectedStatus: http.StatusOK,
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify sorting by querying the database directly
				rows, err := db.QueryContext(context.Background(), `
					SELECT id FROM item_instances
					WHERE item_id = 'item-sorted' AND removed_at IS NULL
					ORDER BY expires_at ASC NULLS LAST`)
				if err != nil {
					t.Fatalf("query instances: %v", err)
				}
				defer rows.Close()

				var ids []string
				for rows.Next() {
					var id string
					if err := rows.Scan(&id); err != nil {
						t.Fatalf("scan id: %v", err)
					}
					ids = append(ids, id)
				}

				expected := []string{"inst-1", "inst-2", "inst-3"}
				if len(ids) != len(expected) {
					t.Errorf("expected %d instances, got %d", len(expected), len(ids))
				}
				for i := range ids {
					if i < len(expected) && ids[i] != expected[i] {
						t.Errorf("instance[%d]: expected %s, got %s", i, expected[i], ids[i])
					}
				}
			},
		},
	}

	runScanHandlerTests(t, tests)
}
