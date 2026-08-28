package server

import (
	"net/http"
	"testing"
)

func TestShoppingListItemCreate(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "creates manual item successfully",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-slc-1", "Butter", "item-slc-1")
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/shopping-list/items",
				body:           `{"itemId":"item-slc-1","quantity":3}`,
				expectedStatus: http.StatusCreated,
				assertions: []assertion{
					{path: "$.itemId", value: "item-slc-1"},
					{path: "$.quantity", value: float64(3)},
					{path: "$.source", value: "manual"},
				},
			},
			afterRequest: exchanges(httpExchange{
				method:         "GET",
				path:           "/api/shopping-list",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].itemId", value: "item-slc-1"},
					{path: "$[0].quantity", value: float64(3)},
				},
			}),
		},
		{
			name: "returns 422 when itemId is missing",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/shopping-list/items",
				body:           `{"quantity":1}`,
				expectedStatus: http.StatusUnprocessableEntity,
			},
		},
		{
			name: "returns 422 when quantity is 0",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-slc-q0", "Cheese", "item-slc-q0")
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/shopping-list/items",
				body:           `{"itemId":"item-slc-q0","quantity":0}`,
				expectedStatus: http.StatusUnprocessableEntity,
			},
		},
		{
			name: "returns 404 when item does not exist",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/shopping-list/items",
				body:           `{"itemId":"nonexistent","quantity":1}`,
				expectedStatus: http.StatusNotFound,
			},
		},
	}

	runHandlerTests(t, tests)
}
