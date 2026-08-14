package server

import (
	"net/http"
	"testing"

	"github.com/Rhionin/pantry/internal/product"
)

func TestListHandler(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "list multiple products",
			setup: setupMultipleProducts([]product.Product{
				{ID: "prod-1", Name: "Apple", Category: "Fruit"},
				{ID: "prod-2", Name: "Banana", Category: "Fruit"},
			}),
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].Name", value: "Apple"},
					{path: "$[0].Category", value: "Fruit"},
					{path: "$[1].Name", value: "Banana"},
					{path: "$[1].Category", value: "Fruit"},
				},
			},
		},
		{
			name: "empty list",
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$", value: []interface{}{}},
				},
			},
		},
	}

	runHandlerTests(t, tests)
}
