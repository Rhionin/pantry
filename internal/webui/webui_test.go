package webui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestNewHandler_ServesPlaceholderAssets(t *testing.T) {
	// Test that NewHandler() over the real embedded tree serves the tracked placeholder
	handler := NewHandler()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	bodyStr := string(body)

	// Verify it's the placeholder HTML
	if !strings.Contains(bodyStr, "Placeholder Assets") {
		t.Error("response should contain placeholder marker")
	}

	if !strings.Contains(bodyStr, "npm run build") {
		t.Error("response should mention npm run build")
	}

	// Verify content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("expected Content-Type to start with 'text/html', got %q", contentType)
	}
}

// Test method gate behavior - task 1.2
func TestMethodGate(t *testing.T) {
	testFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<html><body>Test</body></html>"),
		},
		"assets/app.js": &fstest.MapFile{
			Data: []byte("console.log('app');"),
		},
	}

	handler := NewHandlerFS(testFS)

	testCases := []struct {
		method         string
		path           string
		expectedStatus int
		expectAllow    bool
	}{
		{"GET", "/", http.StatusOK, false},
		{"HEAD", "/", http.StatusOK, false},
		{"POST", "/", http.StatusMethodNotAllowed, true},
		{"PUT", "/", http.StatusMethodNotAllowed, true},
		{"DELETE", "/", http.StatusMethodNotAllowed, true},
		{"PATCH", "/", http.StatusMethodNotAllowed, true},
		// Method gate applies even to existing assets
		{"POST", "/assets/app.js", http.StatusMethodNotAllowed, true},
		{"GET", "/assets/app.js", http.StatusOK, false},
	}

	for _, tc := range testCases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, w.Code)
			}

			if tc.expectAllow {
				allow := w.Header().Get("Allow")
				if allow != "GET, HEAD" {
					t.Errorf("expected Allow header 'GET, HEAD', got %q", allow)
				}
			}
		})
	}
}

// Test asset serving with proper Content-Type - task 1.2
func TestAssetServing(t *testing.T) {
	testFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<html><body>Index</body></html>"),
		},
		"assets/app.js": &fstest.MapFile{
			Data: []byte("console.log('app');"),
		},
		"assets/style.css": &fstest.MapFile{
			Data: []byte("body { margin: 0; }"),
		},
		"assets/font.woff2": &fstest.MapFile{
			Data: []byte("WOFF2_DATA"),
		},
	}

	handler := NewHandlerFS(testFS)

	testCases := []struct {
		path         string
		expectedType string
		expectedBody string
	}{
		{"/assets/app.js", "text/javascript", "console.log('app');"},
		{"/assets/style.css", "text/css", "body { margin: 0; }"},
		{"/assets/font.woff2", "font/woff2", "WOFF2_DATA"},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", w.Code)
			}

			contentType := w.Header().Get("Content-Type")
			if !strings.HasPrefix(contentType, tc.expectedType) {
				t.Errorf("expected Content-Type to start with %q, got %q", tc.expectedType, contentType)
			}

			body := w.Body.String()
			if body != tc.expectedBody {
				t.Errorf("expected body %q, got %q", tc.expectedBody, body)
			}

			// Verify cache headers for assets under /assets/
			cacheControl := w.Header().Get("Cache-Control")
			if strings.HasPrefix(tc.path, "/assets/") {
				expected := "public, max-age=31536000, immutable"
				if cacheControl != expected {
					t.Errorf("expected Cache-Control %q for asset, got %q", expected, cacheControl)
				}
			}
		})
	}
}

// Test SPA fallback behavior - task 1.2/1.3
func TestSPAFallback(t *testing.T) {
	testFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<html><body>SPA Root</body></html>"),
		},
		"assets/app.js": &fstest.MapFile{
			Data: []byte("console.log('app');"),
		},
	}

	handler := NewHandlerFS(testFS)

	// Test paths that should fall back to index.html
	fallbackPaths := []string{
		"/",              // root
		"/inventory",     // client-side route
		"/products/123",  // nested client-side route
		"/nonexistent",   // missing file
		"/index.html",    // literal index.html (special case)
		"/../etc/passwd", // traversal attempt (cleaned by path.Clean)
	}

	for _, path := range fallbackPaths {
		t.Run("fallback_"+path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200 for fallback, got %d", w.Code)
			}

			body := w.Body.String()
			if !strings.Contains(body, "SPA Root") {
				t.Error("fallback should return index.html content")
			}

			contentType := w.Header().Get("Content-Type")
			if !strings.HasPrefix(contentType, "text/html") {
				t.Errorf("expected Content-Type to start with 'text/html', got %q", contentType)
			}

			// Verify no-cache headers for fallback
			cacheControl := w.Header().Get("Cache-Control")
			if cacheControl != "no-cache" {
				t.Errorf("expected Cache-Control 'no-cache' for fallback, got %q", cacheControl)
			}
		})
	}
}

// Test HEAD method support - task 1.2
func TestHEADSupport(t *testing.T) {
	testFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<html><body>Test</body></html>"),
		},
		"assets/app.js": &fstest.MapFile{
			Data: []byte("console.log('app');"),
		},
	}

	handler := NewHandlerFS(testFS)

	testCases := []struct {
		path         string
		expectedType string
	}{
		{"/", "text/html"},
		{"/assets/app.js", "text/javascript"},
		{"/nonexistent", "text/html"}, // should fallback
	}

	for _, tc := range testCases {
		t.Run("HEAD_"+tc.path, func(t *testing.T) {
			req := httptest.NewRequest("HEAD", tc.path, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", w.Code)
			}

			// HEAD should have no body
			if w.Body.Len() > 0 {
				t.Errorf("HEAD response should have empty body, got %d bytes", w.Body.Len())
			}

			// But should have the correct headers
			contentType := w.Header().Get("Content-Type")
			if !strings.HasPrefix(contentType, tc.expectedType) {
				t.Errorf("expected Content-Type to start with %q, got %q", tc.expectedType, contentType)
			}
		})
	}
}

// Test error handling for bad filesystem - task 1.2
func TestBadFilesystem(t *testing.T) {
	// Test filesystem with no readable index.html
	testFS := fstest.MapFS{
		"assets/app.js": &fstest.MapFile{
			Data: []byte("console.log('app');"),
		},
		// No index.html file
	}

	handler := NewHandlerFS(testFS)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 when index.html is missing, got %d", w.Code)
	}
}
