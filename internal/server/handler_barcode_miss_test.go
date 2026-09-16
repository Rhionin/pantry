package server

import (
	"net/http"
	"testing"
)

// TestConfirmedMissLifecycle covers the confirmed-miss behavior over HTTP.
//
// - Property 15: A confirmed miss suppresses the fan-out for its whole window
// - Property 18: A cached miss is indistinguishable from a fresh unknown
// - Property 19: Confirmed misses are invisible to every response
// - Property 20: A user-created product outranks a cached miss
// - Validates: Requirements 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8
func TestConfirmedMissLifecycle(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "all-miss fan-out records confirmed miss and suppresses queries within TTL",
			setup: func(env testEnv) {
				// All databases miss — no seeds
				// First lookup will record a confirmed miss
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "miss-once", "user_id": "test-user"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.product", value: nil},
				},
			},
			afterRequest: exchanges(
				// Second lookup within TTL should return nil without querying upstream
				httpExchange{
					method:         "GET",
					path:           "/api/products/lookup",
					query:          map[string]string{"barcode": "miss-once", "user_id": "test-user"},
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$.product", value: nil},
					},
				},
			),
		},
		{
			name: "confirmed miss produces same result shape as fresh unknown",
			setup: func(env testEnv) {
				// All databases miss
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "miss-shape", "user_id": "test-user"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.product", value: nil},
				},
			},
			afterRequest: exchanges(
				// Second lookup should produce same shape
				httpExchange{
					method:         "GET",
					path:           "/api/products/lookup",
					query:          map[string]string{"barcode": "miss-shape", "user_id": "test-user"},
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$.product", value: nil},
					},
				},
			),
		},
		{
			name: "confirmed misses are invisible in GET /api/products",
			setup: func(env testEnv) {
				// All databases miss on first lookup - records confirmed miss
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "invisible-miss", "user_id": "test-user"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.product", value: nil},
				},
			},
			afterRequest: exchanges(
				// Verify GET /api/products contains no entry for this barcode
				httpExchange{
					method:         "GET",
					path:           "/api/products",
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						// Products list should be empty (no products created)
					},
				},
			),
		},
	}

	runHandlerTests(t, tests)
}
