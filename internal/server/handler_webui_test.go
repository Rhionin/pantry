package server

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// placeholderMarker is the text that the tracked placeholder index.html
// (internal/webui/assets/index.html) names itself with. Every SPA_Fallback
// response carries it, so it doubles as the marker that a routing table row
// either served the SPA (positive cases) or must not have (negative cases).
const placeholderMarker = "Placeholder Assets"

// assertBodyExcludesPlaceholder returns an afterRequest that fails if the
// response body contains the placeholder marker. The framework fields can
// express "body contains X" but not "body does not contain X", so the
// negative routing-table rows keep a minimal afterRequest for that single
// check while their positive status/header assertions go through the fields.
func assertBodyExcludesPlaceholder(env testEnv) {
	defer env.Res.Body.Close()
	body, err := io.ReadAll(env.Res.Body)
	if err != nil {
		env.T.Fatalf("failed to read response body: %v", err)
	}
	if strings.Contains(string(body), placeholderMarker) {
		env.T.Errorf("response body should not contain %q (SPA fallback fired where it should not)", placeholderMarker)
	}
}

// assertContentTypePrefix returns an afterRequest that checks the response
// Content-Type starts with the given media type. ServeContent and the JSON
// writer may append "; charset=utf-8", which an exact-match header assertion
// would reject, so this mirrors how webui_test.go tolerates the suffix.
func assertContentTypePrefix(prefix string) func(env testEnv) {
	return func(env testEnv) {
		got := env.Res.Header.Get("Content-Type")
		if !strings.HasPrefix(got, prefix) {
			env.T.Errorf("expected Content-Type to start with %q, got %q", prefix, got)
		}
	}
}

// TestWebUIRouting covers every row of the design's routing table through the
// composed handler built by setupTestWithDB, using the widened httpExchange
// framework fields for positive status/header/body checks. Negative rows
// additionally assert the SPA document did not leak into the response.
func TestWebUIRouting(t *testing.T) {
	testCases := []handlerTestCase{
		{
			name: "GET / returns SPA document",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/",
				expectedStatus: http.StatusOK,
				expectedHeaders: map[string]string{
					"Cache-Control": "no-cache",
				},
				bodyContains: []string{placeholderMarker},
			},
			afterRequest: assertContentTypePrefix("text/html"),
		},
		{
			name: "GET /inventory (client-side route) returns SPA document",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/inventory",
				expectedStatus: http.StatusOK,
				expectedHeaders: map[string]string{
					"Cache-Control": "no-cache",
				},
				bodyContains: []string{placeholderMarker},
			},
			afterRequest: assertContentTypePrefix("text/html"),
		},
		{
			name: "POST /inventory returns 405 Method Not Allowed",
			httpExchange: httpExchange{
				method:         "POST",
				url:            "/inventory",
				expectedStatus: http.StatusMethodNotAllowed,
				expectedHeaders: map[string]string{
					"Allow": "GET, HEAD",
				},
			},
			afterRequest: assertBodyExcludesPlaceholder,
		},
		{
			name: "GET /api/nope returns 404",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/api/nope",
				expectedStatus: http.StatusNotFound,
			},
			afterRequest: assertBodyExcludesPlaceholder,
		},
		{
			name: "GET /api returns 404",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/api",
				expectedStatus: http.StatusNotFound,
			},
			afterRequest: assertBodyExcludesPlaceholder,
		},
		{
			name: "DELETE /api/products/1 returns 405 Method Not Allowed",
			httpExchange: httpExchange{
				method:         "DELETE",
				url:            "/api/products/1",
				expectedStatus: http.StatusMethodNotAllowed,
			},
			afterRequest: assertBodyExcludesPlaceholder,
		},
		{
			name: "GET /health returns OK",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/health",
				expectedStatus: http.StatusOK,
				bodyContains:   []string{`"status":"ok"`},
			},
			afterRequest: assertContentTypePrefix("application/json"),
		},
		{
			name: "POST /health returns 405 Method Not Allowed",
			httpExchange: httpExchange{
				method:         "POST",
				url:            "/health",
				expectedStatus: http.StatusMethodNotAllowed,
			},
			afterRequest: assertBodyExcludesPlaceholder,
		},
		{
			name: "GET /health/deep returns 404",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/health/deep",
				expectedStatus: http.StatusNotFound,
			},
			afterRequest: assertBodyExcludesPlaceholder,
		},
		{
			name:  "GET /api/products works through composed handler",
			setup: setupProduct("prod-1", "Test Product", "Test Category"),
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/api/products",
				expectedStatus: http.StatusOK,
				bodyContains:   []string{"Test Product"},
			},
			afterRequest: assertBodyExcludesPlaceholder,
		},
	}

	runHandlerTests(t, testCases)
}
