package webui

import (
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"pgregory.net/rapid"
)

// TestReadOnlyMethodGate implements Property 1: Read-only method gate
// **Validates: Requirements 3.8**
//
// Requirement 3.8: "WHEN a request for a path outside the embedded asset set uses
// a method other than `GET` or `HEAD`, THE Pantry_Server SHALL respond with status 405
// without applying SPA_Fallback."
//
// This property test verifies that ALL non-GET/HEAD methods return 405 with proper
// Allow header and do NOT apply SPA fallback, regardless of whether the path exists
// in the embedded asset set or not.
func TestReadOnlyMethodGate(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate an asset tree with various file types
		assetTree := generateAssetTree(t)

		// Generate a request path - could be existing asset, missing path, /, or directory
		requestPath := generateRequestPath(t, assetTree)

		// Generate any HTTP method other than GET/HEAD
		forbiddenMethod := generateForbiddenMethod(t)

		// Create handler with the generated asset tree
		handler := NewHandlerFS(assetTree)

		// Make request with forbidden method
		req := httptest.NewRequest(forbiddenMethod, requestPath, nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		// Assert: status must be 405
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("Expected status 405 for %s %s, got %d", forbiddenMethod, requestPath, w.Code)
		}

		// Assert: Allow header must be exactly "GET, HEAD"
		allow := w.Header().Get("Allow")
		if allow != "GET, HEAD" {
			t.Fatalf("Expected Allow header 'GET, HEAD' for %s %s, got %q", forbiddenMethod, requestPath, allow)
		}

		// Assert: body must NOT be the index.html document (no fallback)
		body, err := io.ReadAll(w.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		// Get the index.html content from the asset tree to compare against
		if indexFile, exists := assetTree["index.html"]; exists {
			indexContent := string(indexFile.Data)
			bodyStr := string(body)

			if strings.Contains(bodyStr, indexContent) || strings.Contains(indexContent, bodyStr) {
				t.Fatalf("Response body should not contain index.html content for %s %s. Body: %q", forbiddenMethod, requestPath, bodyStr)
			}
		}
	})
}

// generateAssetTree creates a random asset tree with various file types and structures
func generateAssetTree(t *rapid.T) fstest.MapFS {
	tree := make(fstest.MapFS)

	// Always include index.html
	tree["index.html"] = &fstest.MapFile{
		Data: []byte("<html><head><title>Test SPA</title></head><body>SPA Application Content</body></html>"),
	}

	// Generate random number of additional assets (0-10)
	numAssets := rapid.IntRange(0, 10).Draw(t, "numAssets")

	// Possible extensions for generated files
	extensions := []string{".js", ".css", ".html", ".svg", ".png", ".json", ".woff2", ".woff", ".ttf"}

	for i := 0; i < numAssets; i++ {
		// Generate file path - could be at root or under assets/
		var filePath string
		if rapid.Bool().Draw(t, "underAssets") {
			// Under assets/ directory
			filename := rapid.StringMatching(`[a-z0-9-]+`).Draw(t, "assetFilename")
			ext := rapid.SampledFrom(extensions).Draw(t, "assetExt")
			filePath = "assets/" + filename + ext
		} else {
			// At root level
			filename := rapid.StringMatching(`[a-z0-9-]+`).Draw(t, "rootFilename")
			ext := rapid.SampledFrom(extensions).Draw(t, "rootExt")
			filePath = filename + ext
		}

		// Generate random content
		contentSize := rapid.IntRange(1, 1000).Draw(t, "contentSize")
		content := rapid.SliceOfN(rapid.Byte(), contentSize, contentSize).Draw(t, "content")

		tree[filePath] = &fstest.MapFile{
			Data: content,
		}
	}

	// Sometimes add directories (which should trigger fallback when accessed)
	numDirs := rapid.IntRange(0, 3).Draw(t, "numDirs")
	for i := 0; i < numDirs; i++ {
		dirName := rapid.StringMatching(`[a-z0-9-]+`).Draw(t, "dirName")
		// Add a file inside the directory to make it exist
		filename := rapid.StringMatching(`[a-z0-9-]+`).Draw(t, "dirFilename")
		ext := rapid.SampledFrom(extensions).Draw(t, "dirFileExt")
		filePath := dirName + "/" + filename + ext

		content := rapid.SliceOfN(rapid.Byte(), 10, 100).Draw(t, "dirContent")
		tree[filePath] = &fstest.MapFile{
			Data: content,
		}
	}

	return tree
}

// generateRequestPath creates various types of request paths to test against
func generateRequestPath(t *rapid.T, assetTree fstest.MapFS) string {
	pathType := rapid.IntRange(0, 4).Draw(t, "pathType")

	switch pathType {
	case 0:
		// Root path
		return "/"
	case 1:
		// Existing asset path
		var assets []string
		for path := range assetTree {
			assets = append(assets, "/"+path)
		}
		if len(assets) > 0 {
			return rapid.SampledFrom(assets).Draw(t, "existingAsset")
		}
		return "/"
	case 2:
		// Missing/non-existent path
		segments := rapid.IntRange(1, 4).Draw(t, "segments")
		var pathParts []string
		for i := 0; i < segments; i++ {
			segment := rapid.StringMatching(`[a-z0-9-]+`).Draw(t, "segment")
			pathParts = append(pathParts, segment)
		}
		return "/" + strings.Join(pathParts, "/")
	case 3:
		// Traversal-like path (should be cleaned and fallback)
		maliciousPaths := []string{
			"/../etc/passwd",
			"/assets/../../secrets",
			"/../index.html",
			"/./assets/../admin",
		}
		return rapid.SampledFrom(maliciousPaths).Draw(t, "traversalPath")
	default:
		return "/"
	}
}

// generateForbiddenMethod creates HTTP methods other than GET and HEAD
func generateForbiddenMethod(t *rapid.T) string {
	forbiddenMethods := []string{
		"POST", "PUT", "DELETE", "PATCH", "OPTIONS", "TRACE", "CONNECT",
		// Include some non-standard methods too
		"PURGE", "COPY", "LOCK", "UNLOCK", "PROPFIND", "PROPPATCH",
	}
	return rapid.SampledFrom(forbiddenMethods).Draw(t, "forbiddenMethod")
}

// TestAssetFidelity implements Property 2: Embedded assets are served faithfully
// **Validates: Requirements 2.4, 3.2, 3.3**
//
// Requirement 2.4: "WHERE a Frontend_Build has been copied into the Web_UI_Package embed
// directory before compilation, THE Web_UI_Package SHALL embed and serve those built assets
// in place of the Placeholder_Assets."
//
// Requirement 3.2: "WHEN a `GET` request for an embedded asset path such as `/assets/{hashed_file}`
// is received, THE Pantry_Server SHALL respond with status 200 and the bytes of that embedded asset."
//
// Requirement 3.3: "WHEN a `GET` request for an embedded asset is served, THE Pantry_Server
// SHALL set a `Content-Type` header matching the asset's file extension."
//
// This property test verifies that embedded assets are served with correct content,
// status codes, and Content-Type headers.
func TestAssetFidelity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a nested tree of files with varied extensions and random byte contents
		assetTree := generateAssetTreeForFidelity(t)

		// Create handler with the generated asset tree
		handler := NewHandlerFS(assetTree)

		// Test each file in the tree
		for filePath, fileData := range assetTree {
			// Skip index.html as it has special fallback behavior
			if filePath == "index.html" {
				continue
			}

			// Make GET request for this file
			requestPath := "/" + filePath
			req := httptest.NewRequest("GET", requestPath, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			// Assert: status must be 200
			if w.Code != http.StatusOK {
				t.Fatalf("Expected status 200 for GET %s, got %d", requestPath, w.Code)
			}

			// Assert: body must be byte-identical to file contents
			body, err := io.ReadAll(w.Body)
			if err != nil {
				t.Fatalf("Failed to read response body for %s: %v", requestPath, err)
			}

			if len(body) != len(fileData.Data) {
				t.Fatalf("Content length mismatch for %s: expected %d bytes, got %d bytes",
					requestPath, len(fileData.Data), len(body))
			}

			for i, b := range body {
				if b != fileData.Data[i] {
					t.Fatalf("Content mismatch for %s at byte %d: expected %02x, got %02x",
						requestPath, i, fileData.Data[i], b)
				}
			}

			// Assert: Content-Type media type must match mime.TypeByExtension
			contentType := w.Header().Get("Content-Type")
			if contentType == "" {
				t.Fatalf("Missing Content-Type header for %s", requestPath)
			}

			// Parse media type (ignoring charset and other parameters)
			mediaType, _, err := mime.ParseMediaType(contentType)
			if err != nil {
				t.Fatalf("Failed to parse Content-Type %q for %s: %v", contentType, requestPath, err)
			}

			// Get expected media type from file extension
			ext := ""
			if dotIndex := strings.LastIndex(filePath, "."); dotIndex >= 0 {
				ext = filePath[dotIndex:]
			}

			expectedType := mime.TypeByExtension(ext)
			if expectedType != "" {
				expectedMediaType, _, _ := mime.ParseMediaType(expectedType)
				if mediaType != expectedMediaType {
					t.Fatalf("Content-Type mismatch for %s: expected media type %q, got %q",
						requestPath, expectedMediaType, mediaType)
				}
			}
		}
	})
}

// generateAssetTreeForFidelity creates a tree specifically for testing asset fidelity
// Includes varied extensions and ensures at least one .woff2 file is present
func generateAssetTreeForFidelity(t *rapid.T) fstest.MapFS {
	tree := make(fstest.MapFS)

	// Always include index.html
	tree["index.html"] = &fstest.MapFile{
		Data: []byte("<html><head><title>Test SPA</title></head><body>SPA Application Content</body></html>"),
	}

	// Required extensions that must be tested - ensure at least one .woff2
	requiredExts := []string{".woff2"}

	// All extensions to test
	allExts := []string{".js", ".css", ".html", ".svg", ".png", ".json", ".woff2", ".woff", ".ttf"}

	// Add required extensions first
	for _, ext := range requiredExts {
		filename := rapid.StringMatching(`[a-z0-9-]+`).Draw(t, "requiredFile")
		filePath := "assets/" + filename + ext

		// Generate random byte content
		contentSize := rapid.IntRange(10, 500).Draw(t, "requiredContentSize")
		content := rapid.SliceOfN(rapid.Byte(), contentSize, contentSize).Draw(t, "requiredContent")

		tree[filePath] = &fstest.MapFile{
			Data: content,
		}
	}

	// Generate additional random files with varied extensions
	numAdditionalFiles := rapid.IntRange(5, 15).Draw(t, "numAdditionalFiles")
	for i := 0; i < numAdditionalFiles; i++ {
		filename := rapid.StringMatching(`[a-z0-9-]+`).Draw(t, "filename")
		ext := rapid.SampledFrom(allExts).Draw(t, "ext")

		// Create nested structure sometimes
		var filePath string
		if rapid.Bool().Draw(t, "nested") {
			dir := rapid.StringMatching(`[a-z0-9-]+`).Draw(t, "dir")
			filePath = dir + "/" + filename + ext
		} else {
			filePath = "assets/" + filename + ext
		}

		// Generate random byte content
		contentSize := rapid.IntRange(1, 1000).Draw(t, "contentSize")
		content := rapid.SliceOfN(rapid.Byte(), contentSize, contentSize).Draw(t, "content")

		tree[filePath] = &fstest.MapFile{
			Data: content,
		}
	}

	return tree
}

// TestSPAFallbackProperty implements Property 3: SPA fallback for unmatched paths
// **Validates: Requirements 2.3, 3.1, 3.4**
//
// Requirement 2.3: "WHEN `go test ./...` runs in a clean checkout with no Frontend_Build
// present, THE Web_UI_Package SHALL serve the Placeholder_Assets rather than failing to initialize."
//
// Requirement 3.1: "WHEN a `GET` request for path `/` is received, THE Pantry_Server
// SHALL respond with status 200 and the embedded `index.html` document."
//
// Requirement 3.4: "WHEN a `GET` request for a client-side route path that matches no
// embedded asset is received, THE Pantry_Server SHALL apply SPA_Fallback and respond
// with status 200 and the embedded `index.html` document."
//
// This property test verifies that unmatched paths fall back to index.html content.
func TestSPAFallbackProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate an asset tree
		assetTree := generateAssetTreeForFallback(t)

		// Create handler with the generated asset tree
		handler := NewHandlerFS(assetTree)

		// Generate a GET path that names no file in the tree
		unmatchedPath := generateUnmatchedPath(t, assetTree)

		// Make GET request for the unmatched path
		req := httptest.NewRequest("GET", unmatchedPath, nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		// Assert: status must be 200
		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200 for fallback GET %s, got %d", unmatchedPath, w.Code)
		}

		// Assert: body must be byte-identical to index.html
		body, err := io.ReadAll(w.Body)
		if err != nil {
			t.Fatalf("Failed to read response body for %s: %v", unmatchedPath, err)
		}

		// Get expected index.html content
		indexFile, exists := assetTree["index.html"]
		if !exists {
			t.Fatal("Asset tree must contain index.html")
		}

		expectedContent := indexFile.Data
		if len(body) != len(expectedContent) {
			t.Fatalf("Fallback content length mismatch for %s: expected %d bytes, got %d bytes",
				unmatchedPath, len(expectedContent), len(body))
		}

		for i, b := range body {
			if b != expectedContent[i] {
				t.Fatalf("Fallback content mismatch for %s at byte %d: expected %02x, got %02x",
					unmatchedPath, i, expectedContent[i], b)
			}
		}

		// Assert: Content-Type should be text/html
		contentType := w.Header().Get("Content-Type")
		if !strings.HasPrefix(contentType, "text/html") {
			t.Fatalf("Expected Content-Type to start with 'text/html' for fallback %s, got %q",
				unmatchedPath, contentType)
		}

		// Assert: Cache-Control should be no-cache
		cacheControl := w.Header().Get("Cache-Control")
		if cacheControl != "no-cache" {
			t.Fatalf("Expected Cache-Control 'no-cache' for fallback %s, got %q",
				unmatchedPath, cacheControl)
		}
	})
}

// TestSPAFallbackPlaceholderOnlyProperty implements Property 3 for the placeholder-only case
// This tests Requirement 2.3 specifically - the case with only index.html present
func TestSPAFallbackPlaceholderOnlyProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Create asset tree with ONLY index.html (Placeholder_Assets case)
		// index.html is at the root because NewHandlerFS opens "index.html" directly
		assetTree := fstest.MapFS{
			"index.html": &fstest.MapFile{
				Data: []byte("<html><head><title>Placeholder</title></head><body>Placeholder Assets - run npm build</body></html>"),
			},
		}

		// Create handler with placeholder-only tree
		handler := NewHandlerFS(assetTree)

		// Generate unmatched path
		unmatchedPath := generateUnmatchedPath(t, assetTree)

		// Make GET request
		req := httptest.NewRequest("GET", unmatchedPath, nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		// Assert: status must be 200 (proves clean checkout serves UI shell)
		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200 for placeholder fallback GET %s, got %d", unmatchedPath, w.Code)
		}

		// Assert: body must be the placeholder index.html content
		body, err := io.ReadAll(w.Body)
		if err != nil {
			t.Fatalf("Failed to read response body for %s: %v", unmatchedPath, err)
		}

		expectedContent := assetTree["index.html"].Data
		if len(body) != len(expectedContent) {
			t.Fatalf("Placeholder fallback content length mismatch for %s: expected %d bytes, got %d bytes",
				unmatchedPath, len(expectedContent), len(body))
		}

		for i, b := range body {
			if b != expectedContent[i] {
				t.Fatalf("Placeholder fallback content mismatch for %s at byte %d: expected %02x, got %02x",
					unmatchedPath, i, expectedContent[i], b)
			}
		}
	})
}

// generateAssetTreeForFallback creates a tree for testing SPA fallback behavior
func generateAssetTreeForFallback(t *rapid.T) fstest.MapFS {
	tree := make(fstest.MapFS)

	// index.html is at the root of the assets tree because NewHandlerFS expects
	// to open "index.html" directly (like fs.Sub(embedded, "assets") would give)
	tree["index.html"] = &fstest.MapFile{
		Data: []byte("<html><head><title>SPA Root</title></head><body>Single Page Application Root Content</body></html>"),
	}

	// Add some assets so we can generate paths that don't match them
	numAssets := rapid.IntRange(2, 8).Draw(t, "numAssets")
	for i := 0; i < numAssets; i++ {
		filename := rapid.StringMatching(`[a-z0-9-]+`).Draw(t, "assetFilename")
		ext := rapid.SampledFrom([]string{".js", ".css", ".png", ".svg"}).Draw(t, "assetExt")
		filePath := "assets/" + filename + ext

		content := rapid.SliceOfN(rapid.Byte(), 10, 100).Draw(t, "assetContent")
		tree[filePath] = &fstest.MapFile{
			Data: content,
		}
	}

	return tree
}

// generateUnmatchedPath creates paths that should trigger SPA fallback
func generateUnmatchedPath(t *rapid.T, assetTree fstest.MapFS) string {
	pathType := rapid.IntRange(0, 5).Draw(t, "unmatchedPathType")

	switch pathType {
	case 0:
		// Root path "/"
		return "/"
	case 1:
		// Multi-segment client-route shapes like /inventory/123
		segments := rapid.IntRange(1, 4).Draw(t, "clientRouteSegments")
		var parts []string
		for i := 0; i < segments; i++ {
			segment := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]*`).Draw(t, "clientRouteSegment")
			parts = append(parts, segment)
		}
		return "/" + strings.Join(parts, "/")
	case 2:
		// Traversal-shaped inputs that should be cleaned and fallback
		traversalPaths := []string{
			"/../etc/passwd",
			"/assets/../../x",
			"/../admin",
			"/./hidden/../config",
		}
		return rapid.SampledFrom(traversalPaths).Draw(t, "traversalPath")
	case 3:
		// Paths that look like assets but don't exist
		filename := rapid.StringMatching(`[a-z0-9-]+`).Draw(t, "fakeAssetName")
		ext := rapid.SampledFrom([]string{".js", ".css", ".png", ".json"}).Draw(t, "fakeAssetExt")
		
		// Ensure the path doesn't exist in the tree (tree uses assets/XXX paths, not /assets/XXX)
		checkPath := "assets/" + filename + ext
		for assetTree[checkPath] != nil {
			filename = filename + "x"
			checkPath = "assets/" + filename + ext
		}
		
		return "/" + checkPath
	case 4:
		// Directory paths (should fallback if they don't contain a real file)
		dirname := rapid.StringMatching(`[a-z0-9-]+`).Draw(t, "dirname")

		// Make sure this directory doesn't actually exist in our tree
		for path := range assetTree {
			if strings.HasPrefix(path, dirname+"/") {
				// This directory exists, try another name
				dirname = dirname + "missing"
				break
			}
		}

		return "/" + dirname
	case 5:
		// The literal index.html path (special case - should fallback, not redirect)
		return "/index.html"
	default:
		return "/"
	}
}
