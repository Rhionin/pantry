package server

import (
	"net/http"
	"testing"
)

func TestShoppingListItemDelete(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "removes item and it no longer appears in list",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-sld-1", "Yogurt", "item-sld-1")
				insertManualShoppingItem(env.T, env.DB, "sli-del-1", "user-1", "item-sld-1", 1)
			},
			httpExchange: httpExchange{
				method:         "DELETE",
				path:           "/api/shopping-list/items/sli-del-1",
				expectedStatus: http.StatusOK,
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
				method:         "DELETE",
				path:           "/api/shopping-list/items/does-not-exist",
				expectedStatus: http.StatusNotFound,
			},
		},
	}

	runHandlerTests(t, tests)
}
