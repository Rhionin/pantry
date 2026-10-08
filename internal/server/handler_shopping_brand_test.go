package server

import (
	"net/http"
	"testing"
	"time"
)

func TestShoppingDeals(t *testing.T) {
	beans := func(env testEnv) {
		createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-gv", "Great Value Cut Green Beans", "item-gv")
		createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-kr", "Kroger Cut Green Beans", "item-kr")
		createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-wf", "Western Family Cut Green Beans", "item-wf")
		beginUsing(env.T, env.DB, time.Now())
		setSupplyQuantity(env.T, env.DB, "prod-gv", 4)
	}

	tests := []handlerTestCase{
		{
			name:  "a noted sale does not change the product on the line",
			setup: beans,
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/shopping-list/deals",
				body:           `{"itemId":"item-kr","priceCents":79,"regularPriceCents":125,"label":"Weekly ad"}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.itemId", value: "item-kr"},
					{path: "$.priceCents", value: float64(79)},
					{path: "$.source", value: "recorded"},
				},
			},
			afterRequest: exchanges(
				httpExchange{
					method:         "POST",
					path:           "/api/shopping-list/fill",
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$[0].itemId", value: "item-gv"},
						{path: "$[0].quantity", value: float64(3)},
						{path: "$[0].source", value: "auto"},
					},
				},
				httpExchange{
					method:         "POST",
					path:           "/api/shopping-list/export",
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$.exported", value: float64(0)},
						{path: "$.items[0].itemId", value: "item-gv"},
						{path: "$.items[0].quantity", value: float64(3)},
					},
				},
				httpExchange{
					method:         "POST",
					path:           "/api/shopping-list/export",
					body:           `{"useItemIds":{"item-gv":"item-kr"}}`,
					expectedStatus: http.StatusUnprocessableEntity,
					assertions: []assertion{
						{path: "$.error", value: "Choose a product on this line."},
					},
				},
			),
		},
		{
			name: "export refuses a different product on the line",
			setup: func(env testEnv) {
				beans(env)
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-corn", "Kroger Whole Kernel Corn", "item-corn")
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/shopping-list/fill",
				expectedStatus: http.StatusOK,
			},
			afterRequest: exchanges(httpExchange{
				method:         "POST",
				path:           "/api/shopping-list/export",
				body:           `{"useItemIds":{"item-gv":"item-corn"}}`,
				expectedStatus: http.StatusUnprocessableEntity,
				assertions: []assertion{
					{path: "$.error", value: "Choose a product on this line."},
				},
			}),
		},
		{
			name: "a sale needs a brand",
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/shopping-list/deals",
				body:           `{"priceCents":79}`,
				expectedStatus: http.StatusUnprocessableEntity,
				assertions: []assertion{
					{path: "$.error", value: "Choose a brand to mark on sale"},
				},
			},
		},
		{
			name: "a sale needs a price or a note",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-note", "Kroger Cut Green Beans", "item-note")
			},
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/shopping-list/deals",
				body:           `{"itemId":"item-note"}`,
				expectedStatus: http.StatusUnprocessableEntity,
				assertions: []assertion{
					{path: "$.error", value: "Add a sale price or a short note"},
				},
			},
		},
		{
			name: "a sale for an unknown item is not saved",
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/shopping-list/deals",
				body:           `{"itemId":"missing","priceCents":79}`,
				expectedStatus: http.StatusNotFound,
				assertions: []assertion{
					{path: "$.error", value: "item not found"},
				},
			},
		},
		{
			name: "negative sale price is rejected",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-kr-neg", "Kroger Cut Green Beans", "item-kr-neg")
			},
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/shopping-list/deals",
				body:           `{"itemId":"item-kr-neg","priceCents":-1}`,
				expectedStatus: http.StatusUnprocessableEntity,
				assertions: []assertion{
					{path: "$.error", value: "Sale price can't be negative"},
				},
			},
		},
		{
			name:  "clearing a sale succeeds",
			setup: beans,
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/shopping-list/deals",
				body:           `{"itemId":"item-kr","priceCents":79,"label":"Weekly ad"}`,
				expectedStatus: http.StatusOK,
			},
			afterRequest: exchanges(httpExchange{
				method:         "DELETE",
				path:           "/api/shopping-list/deals/item-kr",
				expectedStatus: http.StatusOK,
			}),
		},
	}

	runHandlerTests(t, tests)
}
