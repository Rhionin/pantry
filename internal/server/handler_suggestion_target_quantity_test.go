package server

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	"github.com/Rhionin/pantry/internal/product"
)

func TestSetTargetQuantity(t *testing.T) {
	tests := []scanHandlerTestCase{
		{
			name: "sets target quantity and returns updated value",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, _ *fakeOpenFoodFacts) {
				createItemViaStockIn(t, db, repo, "user-tq-1", "prod-tq-1", "Cheese", "item-tq-1")
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/items/item-tq-1/target-quantity",
				body:           `{"targetQuantity":5}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.itemId", value: "item-tq-1"},
					{path: "$.targetQuantity", value: float64(5)},
				},
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				var tq sql.NullInt64
				if err := db.QueryRowContext(context.Background(),
					`SELECT target_quantity FROM items WHERE id = ?`, "item-tq-1",
				).Scan(&tq); err != nil {
					t.Fatalf("query target_quantity: %v", err)
				}
				if !tq.Valid || tq.Int64 != 5 {
					t.Errorf("expected target_quantity=5, got %v", tq)
				}
			},
		},
		{
			name: "overwrites a previously set target quantity",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, _ *fakeOpenFoodFacts) {
				createItemViaStockIn(t, db, repo, "user-tq-2", "prod-tq-2", "Juice", "item-tq-2")
				if _, err := db.ExecContext(context.Background(),
					`UPDATE items SET target_quantity = 3 WHERE id = 'item-tq-2'`,
				); err != nil {
					t.Fatalf("seed target_quantity: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/items/item-tq-2/target-quantity",
				body:           `{"targetQuantity":7}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.targetQuantity", value: float64(7)},
				},
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				var tq sql.NullInt64
				if err := db.QueryRowContext(context.Background(),
					`SELECT target_quantity FROM items WHERE id = ?`, "item-tq-2",
				).Scan(&tq); err != nil {
					t.Fatalf("query target_quantity: %v", err)
				}
				if !tq.Valid || tq.Int64 != 7 {
					t.Errorf("expected target_quantity=7, got %v", tq)
				}
			},
		},
		{
			name: "accepts zero as a valid target quantity",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, _ *fakeOpenFoodFacts) {
				createItemViaStockIn(t, db, repo, "user-tq-3", "prod-tq-3", "Cream", "item-tq-3")
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/items/item-tq-3/target-quantity",
				body:           `{"targetQuantity":0}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.targetQuantity", value: float64(0)},
				},
			},
		},
		{
			name: "404 for nonexistent item",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/items/does-not-exist/target-quantity",
				body:           `{"targetQuantity":3}`,
				expectedStatus: http.StatusNotFound,
			},
		},
		{
			name: "422 when targetQuantity is missing",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, _ *fakeOpenFoodFacts) {
				createItemViaStockIn(t, db, repo, "user-tq-5", "prod-tq-5", "Yogurt", "item-tq-5")
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/items/item-tq-5/target-quantity",
				body:           `{}`,
				expectedStatus: http.StatusUnprocessableEntity,
			},
		},
		{
			name: "422 when targetQuantity is negative",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, _ *fakeOpenFoodFacts) {
				createItemViaStockIn(t, db, repo, "user-tq-6", "prod-tq-6", "Butter", "item-tq-6")
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/items/item-tq-6/target-quantity",
				body:           `{"targetQuantity":-1}`,
				expectedStatus: http.StatusUnprocessableEntity,
			},
		},
	}

	runScanHandlerTests(t, tests)
}
