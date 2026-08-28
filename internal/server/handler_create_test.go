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
			afterRequest: exchanges([]httpExchange{
				{
					method:         "GET",
					path:           "/api/products",
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$", value: []interface{}{}},
					},
				},
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
				{
					method:         "GET",
					path:           "/api/products",
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$[0].Name", value: "Apple"},
						{path: "$[0].Category", value: "Fruit"},
					},
				},
				{
					method:         "POST",
					path:           "/api/products",
					body:           `{"name":"Banana","category":"Fruit"}`,
					expectedStatus: http.StatusCreated,
				},
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
