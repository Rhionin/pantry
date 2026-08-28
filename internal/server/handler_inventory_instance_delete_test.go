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

	tests := []handlerTestCase{
		{
			name: "deletes instance and it no longer appears in the list",
			setup: func(env testEnv) {
				if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
					ID: "prod-1", Name: "Milk", Category: "Dairy", UnitOfMeasure: "gallon",
				}); err != nil {
					env.T.Fatalf("CreateProduct: %v", err)
				}
				if _, err := env.DB.ExecContext(context.Background(),
					`INSERT INTO items (id, user_id, product_id) VALUES ('item-1', 'user-1', 'prod-1')`); err != nil {
					env.T.Fatalf("create item: %v", err)
				}
				if _, err := env.DB.ExecContext(context.Background(), `
					INSERT INTO item_instances (id, item_id, stock_in_at, expires_at) VALUES ('inst-1', 'item-1', ?, ?)`,
					now, sql.NullTime{Time: expiresAt, Valid: true}); err != nil {
					env.T.Fatalf("create instance: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "DELETE",
				path:           "/api/inventory/instances/inst-1",
				expectedStatus: http.StatusOK,
			},
			afterRequest: exchanges(httpExchange{
				method:         "GET",
				path:           "/api/inventory/item-1/instances",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$", value: []interface{}{}},
				},
			}),
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
			setup: func(env testEnv) {
				if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
					ID: "prod-removed", Name: "Bread", Category: "Bakery", UnitOfMeasure: "loaf",
				}); err != nil {
					env.T.Fatalf("CreateProduct: %v", err)
				}
				if _, err := env.DB.ExecContext(context.Background(),
					`INSERT INTO items (id, user_id, product_id) VALUES ('item-removed', 'user-1', 'prod-removed')`); err != nil {
					env.T.Fatalf("create item: %v", err)
				}
				if _, err := env.DB.ExecContext(context.Background(), `
					INSERT INTO item_instances (id, item_id, stock_in_at, expires_at, removed_at, removal_reason)
					VALUES ('inst-removed', 'item-removed', ?, ?, ?, 'consumed')`,
					now.Add(-24*time.Hour),
					sql.NullTime{Time: expiresAt, Valid: true},
					sql.NullTime{Time: now, Valid: true}); err != nil {
					env.T.Fatalf("create removed instance: %v", err)
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
			setup: func(env testEnv) {
				if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
					ID: "prod-multi", Name: "Eggs", Category: "Dairy", UnitOfMeasure: "dozen",
				}); err != nil {
					env.T.Fatalf("CreateProduct: %v", err)
				}
				if _, err := env.DB.ExecContext(context.Background(),
					`INSERT INTO items (id, user_id, product_id) VALUES ('item-multi', 'user-1', 'prod-multi')`); err != nil {
					env.T.Fatalf("create item: %v", err)
				}
				for i := 1; i <= 3; i++ {
					if _, err := env.DB.ExecContext(context.Background(), `
						INSERT INTO item_instances (id, item_id, stock_in_at, expires_at) VALUES (?, 'item-multi', ?, ?)`,
						"inst-multi-"+string(rune('0'+i)), now, sql.NullTime{Time: expiresAt, Valid: true}); err != nil {
						env.T.Fatalf("create instance %d: %v", i, err)
					}
				}
			},
			httpExchange: httpExchange{
				method:         "DELETE",
				path:           "/api/inventory/instances/inst-multi-2",
				expectedStatus: http.StatusOK,
			},
			afterRequest: exchanges(httpExchange{
				method:         "GET",
				path:           "/api/inventory/item-multi/instances",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].ID", value: "inst-multi-1"},
					{path: "$[1].ID", value: "inst-multi-3"},
				},
			}),
		},
	}

	runHandlerTests(t, tests)
}
