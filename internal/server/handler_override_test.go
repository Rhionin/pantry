package server

import (
	"net/http"
	"testing"
)

func TestOverrideCreateHandler(t *testing.T) {
	tests := []handlerTestCase{
		{
			name:  "successful override creation",
			setup: setupProduct("prod-1", "Test Product", ""),
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/products/overrides",
				body:           `{"barcode":"999888","productId":"prod-1"}`,
				expectedStatus: http.StatusCreated,
			},
			// Verify via HTTP that the override was created by looking up the barcode
			afterRequest: exchanges(httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "999888"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.Product.Name", value: "Test Product"},
					{path: "$.Product.ID", value: "prod-1"},
				},
			}),
		},
		{
			name: "missing barcode",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/products/overrides",
				body:           `{"productId":"prod-1"}`,
				expectedStatus: http.StatusBadRequest,
			},
		},
		{
			name: "missing product ID",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/products/overrides",
				body:           `{"barcode":"999888"}`,
				expectedStatus: http.StatusBadRequest,
			},
		},
	}

	runHandlerTests(t, tests)
}
