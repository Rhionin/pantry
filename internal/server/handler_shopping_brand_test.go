package server

import (
	"net/http"
	"testing"
)

func TestShoppingBrandChoice(t *testing.T) {
	beans := func(env testEnv) {
		createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-gv", "Great Value Cut Green Beans", "item-gv")
		createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-kr", "Kroger Cut Green Beans", "item-kr")
		createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-wf", "Western Family Cut Green Beans", "item-wf")
		setTargetQuantity(env.T, env.DB, "item-gv", 4)
		setTargetQuantity(env.T, env.DB, "item-kr", 4)
		setTargetQuantity(env.T, env.DB, "item-wf", 4)
	}

	tests := []handlerTestCase{
		{
			name:  "saved brand becomes the shopping line",
			setup: beans,
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/shopping-list/preferences",
				body:           `{"itemId":"item-kr","ignorePrice":true}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.itemId", value: "item-kr"},
					{path: "$.genericName", value: "cut green beans"},
					{path: "$.ignorePrice", value: true},
				},
			},
			afterRequest: exchanges(httpExchange{
				method:         "GET",
				path:           "/api/shopping-list",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].itemId", value: "item-kr"},
					{path: "$[0].quantity", value: float64(1)},
					{path: "$[0].source", value: "auto"},
					{path: "$[1]", absent: true},
				},
			}),
		},
		{
			name:  "sale on another brand is offered and can be taken at export",
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
					method:         "GET",
					path:           "/api/shopping-list/considerations",
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$.retailerDeals", value: "unavailable"},
						{path: "$.considerations[0].lineItemId", value: "item-gv"},
						{path: "$.considerations[0].genericName", value: "cut green beans"},
						{path: "$.considerations[0].ignorePrice", value: false},
						{path: "$.considerations[0].offer.itemId", value: "item-kr"},
						{path: "$.considerations[0].offer.label", value: "Weekly ad"},
						{path: "$.considerations[0].offer.priceCents", value: float64(79)},
						{path: "$.considerations[1]", absent: true},
					},
				},
				httpExchange{
					method:         "POST",
					path:           "/api/shopping-list/export",
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						// Nothing is sent until a provider is configured. The
						// planned line is still the usual brand.
						{path: "$.exported", value: float64(0)},
						{path: "$.items[0].itemId", value: "item-gv"},
						{path: "$.items[0].quantity", value: float64(1)},
					},
				},
				httpExchange{
					method:         "POST",
					path:           "/api/shopping-list/export",
					body:           `{"useItemIds":{"item-gv":"item-kr"}}`,
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$.exported", value: float64(0)},
						{path: "$.items[0].itemId", value: "item-kr"},
						{path: "$.items[0].name", value: "Kroger Cut Green Beans"},
					},
				},
			),
		},
		{
			name:  "always this brand hides a cheaper sale",
			setup: beans,
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/shopping-list/preferences",
				body:           `{"itemId":"item-kr","ignorePrice":true}`,
				expectedStatus: http.StatusOK,
			},
			afterRequest: exchanges(
				httpExchange{
					method:         "PUT",
					path:           "/api/shopping-list/deals",
					body:           `{"itemId":"item-gv","priceCents":50,"label":"Sale"}`,
					expectedStatus: http.StatusOK,
				},
				httpExchange{
					method:         "GET",
					path:           "/api/shopping-list/considerations",
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$.considerations[0].chosenItemId", value: "item-kr"},
						{path: "$.considerations[0].preferredItemId", value: "item-kr"},
						{path: "$.considerations[0].ignorePrice", value: true},
						{path: "$.considerations[0].offer", value: nil},
					},
				},
			),
		},
		{
			name: "export refuses a brand of a different product",
			setup: func(env testEnv) {
				beans(env)
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-corn", "Kroger Whole Kernel Corn", "item-corn")
				setTargetQuantity(env.T, env.DB, "item-corn", 2)
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/shopping-list/export",
				body:           `{"useItemIds":{"item-gv":"item-corn"}}`,
				expectedStatus: http.StatusUnprocessableEntity,
				assertions: []assertion{
					{path: "$.error", value: "Choose a brand of the same product"},
				},
			},
		},
		{
			name: "brand-only name cannot be saved as a preference",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-brand", "Kroger", "item-brand")
			},
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/shopping-list/preferences",
				body:           `{"itemId":"item-brand","ignorePrice":true}`,
				expectedStatus: http.StatusUnprocessableEntity,
				assertions: []assertion{
					{path: "$.error", value: "This product name is only a brand, so it can't be saved as a preference"},
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
			name:  "clearing a sale removes the offer",
			setup: beans,
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/shopping-list/deals",
				body:           `{"itemId":"item-kr","priceCents":79,"label":"Weekly ad"}`,
				expectedStatus: http.StatusOK,
			},
			afterRequest: exchanges(
				httpExchange{
					method:         "DELETE",
					path:           "/api/shopping-list/deals/item-kr",
					expectedStatus: http.StatusOK,
				},
				httpExchange{
					method:         "GET",
					path:           "/api/shopping-list/considerations",
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$.considerations[0].offer", value: nil},
						{path: "$.considerations[0].members[1].onSale", value: false},
					},
				},
			),
		},
	}

	runHandlerTests(t, tests)
}
