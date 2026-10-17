package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/Rhionin/stockbot/internal/product"
)

func TestRestockHandler(t *testing.T) {
	seed := func(env testEnv) {
		if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
			ID: "bananas", Name: "Bananas", UnitOfMeasure: "bunches",
		}); err != nil {
			env.T.Fatal(err)
		}
		if _, err := env.DB.Exec(`INSERT INTO items (id, user_id, product_id) VALUES ('item-bananas', 'user-1', 'bananas')`); err != nil {
			env.T.Fatal(err)
		}
	}

	runHandlerTests(t, []handlerTestCase{
		{
			name: "household default is three months and the starter categories exist",
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/restock",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.household.kind", value: "months"},
					{path: "$.household.value", value: float64(3)},
					{path: "$.categories[0].name", value: "Produce"},
					{path: "$.notRestocked", value: float64(0)},
				},
			},
		},
		{
			name:  "a household default of zero means don't restock",
			setup: seed,
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/restock/household",
				body:           `{"kind":"months","value":0}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.household.value", value: float64(0)},
				},
			},
			afterRequest: exchanges(httpExchange{
				method:         "GET",
				path:           "/api/restock",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.items[0].name", value: "Bananas"},
					{path: "$.items[0].target.value", value: float64(0)},
					{path: "$.items[0].level", value: "household"},
					{path: "$.items[0].sourceLabel", value: "Household default"},
				},
			}),
		},
		{
			name:  "don't restock one item immediately and leave the category alone",
			setup: seed,
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/restock/items/item-bananas/dont-restock",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.title", value: "Bananas won't be restocked"},
					{path: "$.detail", value: "Its target is now 0."},
					{path: "$.previous", value: nil},
				},
			},
			afterRequest: exchanges(httpExchange{
				method:         "GET",
				path:           "/api/restock",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.items[0].target.value", value: float64(0)},
					{path: "$.items[0].level", value: "item"},
					{path: "$.items[0].sourceLabel", value: "Set on this item"},
					{path: "$.items[0].setLabel", value: "Change"},
				},
			}),
		},
		{
			name: "an unknown item is not found",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/restock/items/missing/dont-restock",
				expectedStatus: http.StatusNotFound,
			},
		},
	})
}
