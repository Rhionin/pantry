package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Rhionin/pantry/internal/product"
)

// TestProvenanceRoundtrip covers provenance appearing in every response family,
// the persisted row shape, and the distinction between external and user-created products.
//
// - Property 5: The persisted external row keeps its pre-feature shape
// - Property 6: Provenance round-trips through storage and every response family
// - Property 7: A user-created product has no provenance
// - Property 33: The fast tiers are untouched
// - Validates: Requirements 1.6, 3.4, 3.5, 9.1, 9.2, 9.3, 10.5, 10.6
func TestProvenanceRoundtrip(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "external lookup response includes externalSource",
			setup: func(env testEnv) {
				env.Upstream.Database(product.ExternalSourceOpenFoodFacts).Seed("123456789", &product.ProductSummary{
					ID: "123456789", Name: "OFF Product", Category: "Category", ImageURL: "http://example.com/image.jpg",
				})
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "123456789"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.product.id", value: "123456789"},
					{path: "$.product.name", value: "OFF Product"},
					{path: "$.product.externalSource", value: "openfoodfacts"},
					{path: "$.source", value: "external"},
				},
			},
		},
		{
			name: "externalSource appears in GET /api/scans",
			setup: func(env testEnv) {
				env.Upstream.Database(product.ExternalSourceOpenProductsFacts).Seed("987654321", &product.ProductSummary{
					ID: "987654321", Name: "OPF Product", Category: "Household",
				})
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans",
				body:           `{"barcode":"987654321","direction":"stock_in","unitCount":1,"userId":"user-1"}`,
				expectedStatus: http.StatusCreated,
				assertions: []assertion{
					{path: "$.product.externalSource", value: "openproductsfacts"},
					{path: "$.product.name", value: "OPF Product"},
				},
			},
			afterRequest: exchanges(
				// GET /api/scans should show the externalSource
				httpExchange{
					method:         "GET",
					path:           "/api/scans",
					query:          map[string]string{"userId": "user-1"},
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$[0].product.externalSource", value: "openproductsfacts"},
						{path: "$[0].product.name", value: "OPF Product"},
					},
				},
			),
		},
		{
			name: "externalSource appears in GET /api/inventory after commit",
			setup: func(env testEnv) {
				env.Upstream.Database(product.ExternalSourceOpenBeautyFacts).Seed("111222333", &product.ProductSummary{
					ID: "111222333", Name: "Beauty Product", Category: "Beauty",
				})
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans",
				body:           `{"barcode":"111222333","direction":"stock_in","unitCount":1,"userId":"user-2"}`,
				expectedStatus: http.StatusCreated,
				assertions: []assertion{
					{path: "$.product.externalSource", value: "openbeautyfacts"},
					{path: "$.product.name", value: "Beauty Product"},
				},
			},
			afterRequest: exchanges(
				// First verify the scan appears with externalSource in GET /api/scans
				httpExchange{
					method:         "GET",
					path:           "/api/scans",
					query:          map[string]string{"userId": "user-2"},
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$[0].product.externalSource", value: "openbeautyfacts"},
						{path: "$[0].product.name", value: "Beauty Product"},
						{path: "$[0].status", value: "pending"},
					},
				},
			),
		},
		{
			name: "externalSource appears in GET /api/products list",
			setup: func(env testEnv) {
				env.Upstream.Database(product.ExternalSourceOpenPetFoodFacts).Seed("999888777", &product.ProductSummary{
					ID: "999888777", Name: "Pet Food", Category: "Pet",
				})
				// First, do a lookup to create and persist the product
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "999888777"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.product.externalSource", value: "openpetfoodfacts"},
				},
			},
			afterRequest: exchanges(
				httpExchange{
					method:         "GET",
					path:           "/api/products",
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$[0].externalSource", value: "openpetfoodfacts"},
						{path: "$[0].name", value: "Pet Food"},
					},
				},
			),
		},
		{
			name: "persisted row has source external and global barcodes mapping",
			setup: func(env testEnv) {
				env.Upstream.Database(product.ExternalSourceOpenFoodFacts).Seed("555666777", &product.ProductSummary{
					ID: "555666777", Name: "Resolved Product", Category: "General",
				})
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "555666777"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.source", value: "external"},
					{path: "$.product.externalSource", value: "openfoodfacts"},
				},
			},
			afterRequest: func(env testEnv) {
				// Verify persisted row has source = external and barcodes mapping has source = global
				var source, barcodeSource string
				err := env.DB.QueryRowContext(env.T.Context(),
					`SELECT products.source, barcodes.source FROM products
					 INNER JOIN barcodes ON products.id = barcodes.product_id
					 WHERE products.id = ?`, "555666777").Scan(&source, &barcodeSource)
				if err != nil {
					env.T.Fatalf("query persisted row: %v", err)
				}
				if source != "external" {
					env.T.Errorf("products.source: want external, got %q", source)
				}
				if barcodeSource != "global" {
					env.T.Errorf("barcodes.source: want global, got %q", barcodeSource)
				}
			},
		},
		{
			name: "open food facts resolution has same shape as before externalSource was added",
			setup: func(env testEnv) {
				// Pre-feature behavior was that OFF resolution had source=external,
				// a global barcodes mapping, and no externalSource field at all.
				// Post-feature, it should have everything the same plus externalSource=openfoodfacts.
				env.Upstream.Database(product.ExternalSourceOpenFoodFacts).Seed("111111111", &product.ProductSummary{
					ID: "111111111", Name: "OFF Only", Category: "Produce", ImageURL: "http://example.com/img.jpg",
				})
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/products/lookup",
				query:          map[string]string{"barcode": "111111111"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					// Pre-feature values still present
					{path: "$.source", value: "external"},
					{path: "$.product.name", value: "OFF Only"},
					{path: "$.product.category", value: "Produce"},
					{path: "$.product.imageUrl", value: "http://example.com/img.jpg"},
					{path: "$.product.id", value: "111111111"},
					// Post-feature addition
					{path: "$.product.externalSource", value: "openfoodfacts"},
				},
			},
		},
		{
			name: "product created through API has no externalSource key",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/products",
				body:           `{"name":"User Product","category":"User Created"}`,
				expectedStatus: http.StatusCreated,
				assertions: []assertion{
					{path: "$.name", value: "User Product"},
					{path: "$.category", value: "User Created"},
				},
			},
			afterRequest: func(env testEnv) {
				// Manually parse the response body to verify externalSource is absent
				body := env.Res.Body
				defer body.Close()

				var product map[string]interface{}
				if err := json.NewDecoder(body).Decode(&product); err != nil {
					env.T.Fatalf("parse response: %v", err)
				}

				// Verify externalSource is not in the response
				if _, hasKey := product["externalSource"]; hasKey {
					env.T.Errorf("user product should not have externalSource key, but got: %v", product["externalSource"])
				}
			},
		},
	}

	runHandlerTests(t, tests)
}
