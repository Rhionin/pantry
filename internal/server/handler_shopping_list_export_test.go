package server

import (
	"net/http"
	"testing"
)

func TestShoppingListExport(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "export with no-op exporter returns 200 and exported count",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-slx-1", "Bread", "item-slx-1")
				insertManualShoppingItem(env.T, env.DB, "sli-exp-1", "user-1", "item-slx-1", 1)
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/shopping-list/export",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.exported", value: float64(0)},
				},
			},
		},
		{
			name: "export with empty list returns 0",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/shopping-list/export",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.exported", value: float64(0)},
				},
			},
		},
	}

	runHandlerTests(t, tests)
}
