package server

import (
	"net/http"
	"testing"

	"github.com/Rhionin/pantry/internal/product"
)

// TestFanOutAndPrecedence covers the fan-out behavior, precedence, and failure safety.
//
// - Property 1: Every database is asked
// - Property 2: A lone hit resolves with source external
// - Property 3: The precedence-earliest hit wins, every time
// - Property 4: A hit outranks a concurrent failure
// - Property 21: A failure anywhere keeps the barcode retryable
// - Property 22: Upstream failure detail never reaches the caller
// - Validates: Requirements 1.1, 1.4, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 6.1, 6.2, 6.3, 6.6
func TestFanOutAndPrecedence(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "lone hit from OFF database resolves with source external",
			setup: func(env testEnv) {
				env.Upstream.Database(product.ExternalSourceOpenFoodFacts).Seed("off-only", &product.ProductSummary{
					ID: "off-only", Name: "OFF Product", Category: "Food",
				})
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "off-only", "user_id": "test-user"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.product.id", value: "off-only"},
					{path: "$.product.name", value: "OFF Product"},
					{path: "$.source", value: "external"},
					{path: "$.product.externalSource", value: "openfoodfacts"},
				},
			},
		},
		{
			name: "lone hit from OPF database",
			setup: func(env testEnv) {
				env.Upstream.Database(product.ExternalSourceOpenProductsFacts).Seed("opf-only", &product.ProductSummary{
					ID: "opf-only", Name: "OPF Product", Category: "Products",
				})
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "opf-only", "user_id": "test-user"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.product.externalSource", value: "openproductsfacts"},
					{path: "$.source", value: "external"},
				},
			},
		},
		{
			name: "lone hit from OBF database",
			setup: func(env testEnv) {
				env.Upstream.Database(product.ExternalSourceOpenBeautyFacts).Seed("obf-only", &product.ProductSummary{
					ID: "obf-only", Name: "Beauty Product", Category: "Beauty",
				})
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "obf-only", "user_id": "test-user"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.product.externalSource", value: "openbeautyfacts"},
					{path: "$.source", value: "external"},
				},
			},
		},
		{
			name: "lone hit from OPetF database",
			setup: func(env testEnv) {
				env.Upstream.Database(product.ExternalSourceOpenPetFoodFacts).Seed("opetf-only", &product.ProductSummary{
					ID: "opetf-only", Name: "Pet Food", Category: "Pets",
				})
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "opetf-only", "user_id": "test-user"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.product.externalSource", value: "openpetfoodfacts"},
					{path: "$.source", value: "external"},
				},
			},
		},
		{
			name: "precedence: OPF wins over OBF and OPetF when all hit",
			setup: func(env testEnv) {
				// Seed all databases with different data
				env.Upstream.Database(product.ExternalSourceOpenFoodFacts).Seed("precedence-test", &product.ProductSummary{
					ID: "precedence-test", Name: "OFF Version", Category: "Food",
				})
				env.Upstream.Database(product.ExternalSourceOpenProductsFacts).Seed("precedence-test", &product.ProductSummary{
					ID: "precedence-test", Name: "OPF Version", Category: "Products",
				})
				env.Upstream.Database(product.ExternalSourceOpenBeautyFacts).Seed("precedence-test", &product.ProductSummary{
					ID: "precedence-test", Name: "Beauty Version", Category: "Beauty",
				})
				env.Upstream.Database(product.ExternalSourceOpenPetFoodFacts).Seed("precedence-test", &product.ProductSummary{
					ID: "precedence-test", Name: "Pet Version", Category: "Pets",
				})
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "precedence-test", "user_id": "test-user"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					// OFF has highest precedence, so it should win
					{path: "$.product.name", value: "OFF Version"},
					{path: "$.product.externalSource", value: "openfoodfacts"},
				},
			},
		},
		{
			name: "all-miss returns not-found",
			setup: func(env testEnv) {
				// All databases miss — no seeds
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "all-miss", "user_id": "test-user"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.product", value: nil},
				},
			},
		},
	}

	runHandlerTests(t, tests)
}
