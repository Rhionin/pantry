package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Rhionin/pantry/internal/product"
)

func TestWebUIRouting(t *testing.T) {
	testCases := []handlerTestCase{
		{
			name: "GET / returns SPA document",
			httpExchange: httpExchange{
				method: "GET",
				url:    "/",
			},
		},
		{
			name: "GET /inventory (client-side route) returns SPA document",
			httpExchange: httpExchange{
				method: "GET",
				url:    "/inventory",
			},
		},
		{
			name: "POST /inventory returns 405 Method Not Allowed",
			httpExchange: httpExchange{
				method:         "POST",
				url:            "/inventory",
				expectedStatus: http.StatusMethodNotAllowed,
			},
		},
		{
			name: "GET /api/nope returns 404",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/api/nope",
				expectedStatus: http.StatusNotFound,
			},
		},
		{
			name: "GET /api returns 404",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/api",
				expectedStatus: http.StatusNotFound,
			},
		},
		{
			name: "GET /health returns OK",
			httpExchange: httpExchange{
				method: "GET",
				url:    "/health",
			},
			bodyContains: []string{`"status":"ok"`},
		},
		{
			name: "POST /health returns 405 Method Not Allowed",
			httpExchange: httpExchange{
				method:         "POST",
				url:            "/health",
				expectedStatus: http.StatusMethodNotAllowed,
			},
		},
		{
			name: "GET /health/deep returns 404",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/health/deep",
				expectedStatus: http.StatusNotFound,
			},
		},
	}

	// Add one test case that verifies an existing API route works through the composed handler
	testCases = append(testCases, handlerTestCase{
		name: "GET /api/products works through composed handler",
		setup: func(env testEnv) {
			// Seed a product
			err := env.ProductStore.CreateProduct(context.Background(), product.Product{
				Name:     "Test Product",
				Category: "Test Category",
			})
			if err != nil {
				env.T.Fatalf("failed to seed product: %v", err)
			}
		},
		httpExchange: httpExchange{
			method: "GET",
			url:    "/api/products",
		},
		bodyContains: []string{"Test Product"},
	})

	// Add custom afterRequest checks for specific cases that need header and body content verification
	for i := range testCases {
		tc := &testCases[i]

		// Add afterRequest validation for cases that need specific checks
		switch tc.name {
		case "GET / returns SPA document", "GET /inventory (client-side route) returns SPA document":
			tc.afterRequest = func(env testEnv) {
				// Check content type
				contentType := env.Res.Header.Get("Content-Type")
				if !strings.HasPrefix(contentType, "text/html") {
					env.T.Errorf("expected Content-Type to start with 'text/html', got %q", contentType)
				}

				// Check cache control
				cacheControl := env.Res.Header.Get("Cache-Control")
				if cacheControl != "no-cache" {
					env.T.Errorf("expected Cache-Control 'no-cache', got %q", cacheControl)
				}

				// Check body content
				defer env.Res.Body.Close()
				body, err := io.ReadAll(env.Res.Body)
				if err != nil {
					env.T.Fatalf("failed to read response body: %v", err)
				}
				bodyStr := string(body)
				if !strings.Contains(bodyStr, "Placeholder Assets") {
					env.T.Error("response should contain 'Placeholder Assets'")
				}
			}
		case "POST /inventory returns 405 Method Not Allowed", "POST /health returns 405 Method Not Allowed":
			tc.afterRequest = func(env testEnv) {
				// Check Allow header
				allow := env.Res.Header.Get("Allow")
				if !strings.Contains(allow, "GET") {
					env.T.Errorf("Allow header should contain GET, got %q", allow)
				}
			}
		case "GET /api/nope returns 404", "GET /api returns 404", "GET /health/deep returns 404":
			tc.afterRequest = func(env testEnv) {
				// Ensure these don't contain SPA content
				defer env.Res.Body.Close()
				body, err := io.ReadAll(env.Res.Body)
				if err != nil {
					env.T.Fatalf("failed to read response body: %v", err)
				}
				bodyStr := string(body)
				if strings.Contains(bodyStr, "Placeholder Assets") {
					env.T.Error("API 404 responses should not contain SPA content")
				}
			}
		case "GET /health returns OK":
			tc.afterRequest = func(env testEnv) {
				// Check content type
				contentType := env.Res.Header.Get("Content-Type")
				if !strings.HasPrefix(contentType, "application/json") {
					env.T.Errorf("expected Content-Type to start with 'application/json', got %q", contentType)
				}
			}
		}
	}

	runHandlerTests(t, testCases)
}
