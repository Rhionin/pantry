package server

import (
	"net/http"
	"testing"
)

func TestSetTargetQuantity(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "sets target quantity and returns updated value",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-tq-1", "prod-tq-1", "Cheese", "item-tq-1")
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
		},
		{
			name: "overwrites a previously set target quantity",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-tq-2", "prod-tq-2", "Juice", "item-tq-2")
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
		},
		{
			name: "accepts zero as a valid target quantity",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-tq-3", "prod-tq-3", "Cream", "item-tq-3")
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
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-tq-5", "prod-tq-5", "Yogurt", "item-tq-5")
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
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-tq-6", "prod-tq-6", "Butter", "item-tq-6")
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/items/item-tq-6/target-quantity",
				body:           `{"targetQuantity":-1}`,
				expectedStatus: http.StatusUnprocessableEntity,
			},
		},
	}

	runHandlerTests(t, tests)
}
