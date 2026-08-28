package server

import (
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
