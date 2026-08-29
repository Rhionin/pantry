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

	tests := []handlerTestCase{
		{
			name: "creates instance with expiration date",
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
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/inventory/item-1/instances",
				body:           `{"expiresAt":"` + expiresAt.Format(time.RFC3339) + `"}`,
				expectedStatus: http.StatusCreated,
				assertions: []assertion{
					{path: "$.itemId", value: "item-1"},
				},
			},
			afterRequest: exchanges(httpExchange{
				method:         "GET",
				path:           "/api/inventory/item-1/instances",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].itemId", value: "item-1"},
				},
			}),
		},
		{
			name: "creates instance without expiration date",
			setup: func(env testEnv) {
				if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
					ID: "prod-2", Name: "Canned Beans", Category: "Canned", UnitOfMeasure: "can",
				}); err != nil {
					env.T.Fatalf("CreateProduct: %v", err)
				}
				if _, err := env.DB.ExecContext(context.Background(),
					`INSERT INTO items (id, user_id, product_id) VALUES ('item-2', 'user-1', 'prod-2')`); err != nil {
					env.T.Fatalf("create item: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/inventory/item-2/instances",
				body:           `{}`,
				expectedStatus: http.StatusCreated,
				assertions: []assertion{
					{path: "$.itemId", value: "item-2"},
				},
			},
			afterRequest: exchanges(httpExchange{
				method:         "GET",
				path:           "/api/inventory/item-2/instances",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].itemId", value: "item-2"},
				},
			}),
		},
		{
			name: "allows multiple instances for same item",
			setup: func(env testEnv) {
				if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
					ID: "prod-multi", Name: "Yogurt", Category: "Dairy", UnitOfMeasure: "cup",
				}); err != nil {
					env.T.Fatalf("CreateProduct: %v", err)
				}
				if _, err := env.DB.ExecContext(context.Background(),
					`INSERT INTO items (id, user_id, product_id) VALUES ('item-multi', 'user-1', 'prod-multi')`); err != nil {
					env.T.Fatalf("create item: %v", err)
				}
				if _, err := env.DB.ExecContext(context.Background(), `
					INSERT INTO item_instances (id, item_id, stock_in_at, expires_at)
					VALUES ('inst-existing', 'item-multi', ?, ?)`,
					now, sql.NullTime{Time: expiresAt, Valid: true}); err != nil {
					env.T.Fatalf("create existing instance: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/inventory/item-multi/instances",
				body:           `{"expiresAt":"` + expiresAt.Format(time.RFC3339) + `"}`,
				expectedStatus: http.StatusCreated,
			},
			afterRequest: exchanges(httpExchange{
				method:         "GET",
				path:           "/api/inventory/item-multi/instances",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].itemId", value: "item-multi"},
					{path: "$[1].itemId", value: "item-multi"},
				},
			}),
		},
	}

	runHandlerTests(t, tests)
}
