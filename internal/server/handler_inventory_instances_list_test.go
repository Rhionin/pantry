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

	tests := []handlerTestCase{
		{
			name: "returns empty list when no instances",
			setup: func(env testEnv) {
				if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
					ID: "prod-empty", Name: "Empty Product", Category: "Test", UnitOfMeasure: "unit",
				}); err != nil {
					env.T.Fatalf("CreateProduct: %v", err)
				}
				if _, err := env.DB.ExecContext(context.Background(),
					`INSERT INTO items (id, user_id, product_id) VALUES ('item-empty', 'user-1', 'prod-empty')`); err != nil {
					env.T.Fatalf("create item: %v", err)
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
			setup: func(env testEnv) {
				if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
					ID: "prod-sorted", Name: "Test Product", Category: "Test", UnitOfMeasure: "unit",
				}); err != nil {
					env.T.Fatalf("CreateProduct: %v", err)
				}
				if _, err := env.DB.ExecContext(context.Background(),
					`INSERT INTO items (id, user_id, product_id) VALUES ('item-sorted', 'user-1', 'prod-sorted')`); err != nil {
					env.T.Fatalf("create item: %v", err)
				}

				// Insert in non-sorted order to confirm DB ordering
				for _, inst := range []struct {
					id        string
					expiresAt time.Time
				}{
					{"inst-3", farFuture},
					{"inst-1", nearExpiry},
					{"inst-2", now.Add(7 * 24 * time.Hour)},
				} {
					if _, err := env.DB.ExecContext(context.Background(), `
						INSERT INTO item_instances (id, item_id, stock_in_at, expires_at) VALUES (?, 'item-sorted', ?, ?)`,
						inst.id, now, sql.NullTime{Time: inst.expiresAt, Valid: true}); err != nil {
						env.T.Fatalf("create instance %s: %v", inst.id, err)
					}
				}
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/inventory/item-sorted/instances",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].id", value: "inst-1"},
					{path: "$[1].id", value: "inst-2"},
					{path: "$[2].id", value: "inst-3"},
				},
			},
		},
	}

	runHandlerTests(t, tests)
}
