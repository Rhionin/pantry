package server

import (
	"context"
	"net/http"
	"testing"
)

func TestShoppingListItemUpdate(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "marks item as purchased and it disappears from active list",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-slu-1", "Juice", "item-slu-1")
				insertManualShoppingItem(env.T, env.DB, "sli-upd-1", "user-1", "item-slu-1", 2)
			},
			httpExchange: httpExchange{
				method:         "PATCH",
				path:           "/api/shopping-list/items/sli-upd-1",
				body:           `{"purchased":true}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.id", value: "sli-upd-1"},
				},
			},
			afterRequest: exchanges(httpExchange{
				method:         "GET",
				path:           "/api/shopping-list",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$", value: []interface{}{}},
				},
			}),
		},
		{
			name: "marks derived item purchased without changing inventory",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-slu-auto", "Rice", "item-slu-auto")
				setTargetQuantity(env.T, env.DB, "item-slu-auto", 3)
				if _, err := env.DB.ExecContext(context.Background(),
					`INSERT INTO shopping_list_items (id, user_id, item_id, quantity, source) VALUES (?, ?, ?, ?, 'auto')`,
					"sli-upd-auto", "user-1", "item-slu-auto", 2,
				); err != nil {
					env.T.Fatalf("insert derived shopping item: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "PATCH",
				path:           "/api/shopping-list/items/sli-upd-auto",
				body:           `{"purchased":true}`,
				expectedStatus: http.StatusOK,
			},
			afterRequest: exchanges(
				httpExchange{
					method:         "GET",
					path:           "/api/shopping-list",
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$", value: []interface{}{}},
					},
				},
				httpExchange{
					method:         "GET",
					path:           "/api/inventory/item-slu-auto/instances",
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$[0].itemId", value: "item-slu-auto"},
					},
				},
			),
		},
		{
			name: "returns 404 for nonexistent item",
			httpExchange: httpExchange{
				method:         "PATCH",
				path:           "/api/shopping-list/items/does-not-exist",
				body:           `{"purchased":true}`,
				expectedStatus: http.StatusNotFound,
			},
		},
	}

	runHandlerTests(t, tests)
}
