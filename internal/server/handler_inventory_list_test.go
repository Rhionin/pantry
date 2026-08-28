package server

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
)

func TestInventoryListHandler(t *testing.T) {
	now := time.Now()

	tests := []handlerTestCase{
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
			setup: func(env testEnv) {
				if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
					ID: "prod-1", Name: "Milk", Category: "Dairy", UnitOfMeasure: "gallon",
				}); err != nil {
					env.T.Fatalf("CreateProduct prod1: %v", err)
				}
				if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
					ID: "prod-2", Name: "Bread", Category: "Bakery", UnitOfMeasure: "loaf",
				}); err != nil {
					env.T.Fatalf("CreateProduct prod2: %v", err)
				}
				if _, err := env.DB.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES ('item-1', 'user-1', 'prod-1'), ('item-2', 'user-1', 'prod-2')`); err != nil {
					env.T.Fatalf("create items: %v", err)
				}

				farFuture := now.Add(14 * 24 * time.Hour)
				for i := 1; i <= 3; i++ {
					if _, err := env.DB.ExecContext(context.Background(), `
						INSERT INTO item_instances (id, item_id, stock_in_at, expires_at) VALUES (?, 'item-1', ?, ?)`,
						"inst-1-"+string(rune('0'+i)), now, sql.NullTime{Time: farFuture, Valid: true}); err != nil {
						env.T.Fatalf("create instance: %v", err)
					}
				}
				for i := 1; i <= 2; i++ {
					if _, err := env.DB.ExecContext(context.Background(), `
						INSERT INTO item_instances (id, item_id, stock_in_at, expires_at) VALUES (?, 'item-2', ?, ?)`,
						"inst-2-"+string(rune('0'+i)), now, sql.NullTime{Time: farFuture, Valid: true}); err != nil {
						env.T.Fatalf("create instance: %v", err)
					}
				}
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/inventory",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].Item.Product.Name", value: "Bread"},
					{path: "$[1].Item.Product.Name", value: "Milk"},
				},
			},
		},
	}

	runHandlerTests(t, tests)
}

func TestInventoryListHandler_WithSearchQuery(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "no query returns all items",
			setup: func(env testEnv) {
				for _, p := range []struct{ id, name, category string }{
					{"prod-milk", "Milk", "Dairy"},
					{"prod-bread", "Bread", "Bakery"},
					{"prod-cheese", "Cheese", "Dairy"},
				} {
					if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
						ID: p.id, Name: p.name, Category: p.category, UnitOfMeasure: "unit",
					}); err != nil {
						env.T.Fatalf("CreateProduct %s: %v", p.name, err)
					}
					if _, err := env.DB.ExecContext(context.Background(),
						`INSERT INTO items (id, user_id, product_id) VALUES (?, 'user-1', ?)`,
						"item-"+p.id, p.id); err != nil {
						env.T.Fatalf("insert item %s: %v", p.name, err)
					}
				}
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/inventory",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].Item.Product.Name", value: "Bread"},
					{path: "$[1].Item.Product.Name", value: "Cheese"},
					{path: "$[2].Item.Product.Name", value: "Milk"},
				},
			},
		},
		{
			name: "search by name returns matching item",
			setup: func(env testEnv) {
				for _, p := range []struct{ id, name, category string }{
					{"prod-milk", "Milk", "Dairy"},
					{"prod-bread", "Bread", "Bakery"},
					{"prod-cheese", "Cheese", "Dairy"},
				} {
					if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
						ID: p.id, Name: p.name, Category: p.category, UnitOfMeasure: "unit",
					}); err != nil {
						env.T.Fatalf("CreateProduct %s: %v", p.name, err)
					}
					if _, err := env.DB.ExecContext(context.Background(),
						`INSERT INTO items (id, user_id, product_id) VALUES (?, 'user-1', ?)`,
						"item-"+p.id, p.id); err != nil {
						env.T.Fatalf("insert item %s: %v", p.name, err)
					}
				}
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/inventory",
				query:          map[string]string{"q": "milk"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].Item.Product.Name", value: "Milk"},
				},
			},
		},
		{
			name: "search by category returns matching items",
			setup: func(env testEnv) {
				for _, p := range []struct{ id, name, category string }{
					{"prod-milk", "Milk", "Dairy"},
					{"prod-bread", "Bread", "Bakery"},
					{"prod-cheese", "Cheese", "Dairy"},
				} {
					if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
						ID: p.id, Name: p.name, Category: p.category, UnitOfMeasure: "unit",
					}); err != nil {
						env.T.Fatalf("CreateProduct %s: %v", p.name, err)
					}
					if _, err := env.DB.ExecContext(context.Background(),
						`INSERT INTO items (id, user_id, product_id) VALUES (?, 'user-1', ?)`,
						"item-"+p.id, p.id); err != nil {
						env.T.Fatalf("insert item %s: %v", p.name, err)
					}
				}
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/inventory",
				query:          map[string]string{"q": "dairy"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].Item.Product.Name", value: "Cheese"},
					{path: "$[1].Item.Product.Name", value: "Milk"},
				},
			},
		},
	}

	runHandlerTests(t, tests)
}
