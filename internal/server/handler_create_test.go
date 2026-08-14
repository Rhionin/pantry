package server

import (
	"net/http"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCreateHandler(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "successful create",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/products",
				body:           `{"name":"Orange","category":"Fruit","unitOfMeasure":"each"}`,
				expectedStatus: http.StatusCreated,
				assertions: []assertion{
					{path: "$.Name", value: "Orange"},
					{path: "$.Category", value: "Fruit"},
					{path: "$.UnitOfMeasure", value: "each"},
				},
			},
			// Verify via HTTP that the product was created
			afterRequest: exchanges(httpExchange{
				method:         "GET",
				path:           "/api/products",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].Name", value: "Orange"},
					{path: "$[0].Category", value: "Fruit"},
				},
			}),
		},
		{
			name: "missing name",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/products",
				body:           `{"category":"Fruit"}`,
				expectedStatus: http.StatusBadRequest,
			},
		},
		{
			name: "empty JSON",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/products",
				body:           `{}`,
				expectedStatus: http.StatusBadRequest,
			},
		},
		{
			name: "create flow - empty list, create, verify list",
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products",
				expectedStatus: http.StatusOK,
			},
			// Express the entire test as a series of HTTP exchanges using slice syntax
			afterRequest: exchanges([]httpExchange{
				// First exchange: verify list is empty initially
				{
					method:         "GET",
					path:           "/api/products",
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$", value: []interface{}{}},
					},
				},
				// Second exchange: create a product
				{
					method:         "POST",
					path:           "/api/products",
					body:           `{"name":"Apple","category":"Fruit"}`,
					expectedStatus: http.StatusCreated,
					assertions: []assertion{
						{path: "$.Name", value: "Apple"},
						{path: "$.Category", value: "Fruit"},
					},
				},
				// Third exchange: verify the product appears in the list
				{
					method:         "GET",
					path:           "/api/products",
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$[0].Name", value: "Apple"},
						{path: "$[0].Category", value: "Fruit"},
					},
				},
				// Fourth exchange: create another product
				{
					method:         "POST",
					path:           "/api/products",
					body:           `{"name":"Banana","category":"Fruit"}`,
					expectedStatus: http.StatusCreated,
				},
				// Fifth exchange: verify both products in the list
				{
					method:         "GET",
					path:           "/api/products",
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$[0].Name", value: "Apple"},
						{path: "$[1].Name", value: "Banana"},
					},
				},
			}...),
		},
	}

	runHandlerTests(t, tests)
}
