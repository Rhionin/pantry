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
					{path: "$[0].item.product.name", value: "Bread"},
					{path: "$[1].item.product.name", value: "Milk"},
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
					{path: "$[0].item.product.name", value: "Bread"},
					{path: "$[1].item.product.name", value: "Cheese"},
					{path: "$[2].item.product.name", value: "Milk"},
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
					{path: "$[0].item.product.name", value: "Milk"},
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
					{path: "$[0].item.product.name", value: "Cheese"},
					{path: "$[1].item.product.name", value: "Milk"},
				},
			},
		},
	}

	runHandlerTests(t, tests)
}

func TestInventoryListHandler_GroupSummary(t *testing.T) {
	now := time.Now()
	tests := []handlerTestCase{
		{
			name: "two members share one summed group",
			setup: func(env testEnv) {
				for _, p := range []struct{ id, name string }{
					{"prod-gv", "Great Value Cut Green Beans"},
					{"prod-kr", "Kroger Cut Green Beans"},
					{"prod-milk", "Milk"},
				} {
					if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
						ID: p.id, Name: p.name, UnitOfMeasure: "can",
					}); err != nil {
						env.T.Fatalf("CreateProduct %s: %v", p.name, err)
					}
				}
				if _, err := env.DB.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id) VALUES
					('item-gv', 'user-1', 'prod-gv'),
					('item-kr', 'user-1', 'prod-kr'),
					('item-milk', 'user-1', 'prod-milk')`); err != nil {
					env.T.Fatal(err)
				}
				for i := 1; i <= 3; i++ {
					if _, err := env.DB.ExecContext(context.Background(), `
						INSERT INTO item_instances (id, item_id, stock_in_at) VALUES (?, 'item-gv', ?)`,
						"gv-"+string(rune('0'+i)), now); err != nil {
						env.T.Fatal(err)
					}
				}
				for i := 1; i <= 2; i++ {
					if _, err := env.DB.ExecContext(context.Background(), `
						INSERT INTO item_instances (id, item_id, stock_in_at) VALUES (?, 'item-kr', ?)`,
						"kr-"+string(rune('0'+i)), now); err != nil {
						env.T.Fatal(err)
					}
				}
				if _, err := env.DB.ExecContext(context.Background(), `
					INSERT INTO product_groups (id, user_id, name, name_key, rule, rule_confirmed)
					VALUES ('g-beans', 'user-1', 'Cut green beans', 'cut green beans', 'same_as_ran_out', 0)`); err != nil {
					env.T.Fatal(err)
				}
				if _, err := env.DB.ExecContext(context.Background(), `
					INSERT INTO product_group_members (product_id, group_id) VALUES
					('prod-gv', 'g-beans'), ('prod-kr', 'g-beans')`); err != nil {
					env.T.Fatal(err)
				}
				if _, err := env.DB.ExecContext(context.Background(), `
					INSERT INTO barcodes (barcode, product_id, source, user_id) VALUES ('111', 'prod-gv', 'global', '')`); err != nil {
					env.T.Fatal(err)
				}
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/inventory",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].item.product.name", value: "Great Value Cut Green Beans"},
					{path: "$[0].group.id", value: "g-beans"},
					{path: "$[0].group.name", value: "Cut green beans"},
					{path: "$[0].group.rule", value: "same_as_ran_out"},
					{path: "$[0].group.ruleConfirmed", value: false},
					{path: "$[0].group.onHand", value: float64(5)},
					{path: "$[0].group.memberCount", value: float64(2)},
					{path: "$[0].group.members[0].name", value: "Great Value Cut Green Beans"},
					{path: "$[0].group.members[0].onHand", value: float64(3)},
					{path: "$[0].group.members[0].barcodes[0]", value: "111"},
					{path: "$[1].group.onHand", value: float64(5)},
					{path: "$[1].group.memberCount", value: float64(2)},
					{path: "$[2].item.product.name", value: "Milk"},
					{path: "$[2].group", absent: true},
				},
			},
		},
	}
	runHandlerTests(t, tests)
}
