package server

import (
	"net/http"
	"testing"
)

func TestLookupHandler(t *testing.T) {
	tests := []handlerTestCase{
		{
			name:  "successful lookup",
			setup: setupProductWithBarcode("prod-1", "Test Product", "Test", "123456"),
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "123456"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.Product.Name", value: "Test Product"},
					{path: "$.Product.Category", value: "Test"},
					{path: "$.Product.ID", value: "prod-1"},
				},
			},
		},
		{
			name: "missing barcode parameter",
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				expectedStatus: http.StatusBadRequest,
			},
		},
		{
			name: "product not found",
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "nonexistent"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.Product", value: nil},
				},
			},
		},
	}

	runHandlerTests(t, tests)
}
