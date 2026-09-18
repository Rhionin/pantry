// Package webui serves the compiled frontend assets embedded in the binary.
//
// The package uses //go:embed to include the built React application, providing
// both individual asset serving and SPA (Single Page Application) fallback
// behavior for client-side routes.
package webui

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// Register missing MIME types for font files that Go's built-in table lacks
// and distroless containers don't provide via /etc/mime.types
func init() {
	mime.AddExtensionType(".woff2", "font/woff2")
	mime.AddExtensionType(".woff", "font/woff")
	mime.AddExtensionType(".ttf", "font/ttf")
}

// embedded holds the compiled frontend assets.
// The all: prefix ensures .vite/ metadata and underscore-prefixed chunk names
// are included, which the default //go:embed pattern silently skips.
//
//go:embed all:assets
var embedded embed.FS

// NewHandler serves the frontend assets compiled into the binary.
//
// Assets are served from the embedded filesystem with proper Content-Type
// headers and cache controls. Requests that don't match embedded assets
// fall back to serving index.html (SPA fallback behavior).
func NewHandler() http.Handler {
	assets, err := fs.Sub(embedded, "assets")
	if err != nil {
		// This should never happen with a valid embed, but handle gracefully
		panic("failed to create assets subtree: " + err.Error())
	}
	return NewHandlerFS(assets)
}

// NewHandlerFS serves an arbitrary asset tree.
//
// NewHandler is NewHandlerFS over the embedded tree; tests use this to
// inject synthetic asset trees. If the filesystem contains no readable
// index.html, fallback requests will return HTTP 500.
func NewHandlerFS(assets fs.FS) http.Handler {
	// Read index.html once during construction and hold it in memory
	var indexHTML []byte
	var indexErr error

	if file, err := assets.Open("index.html"); err != nil {
		indexErr = err
	} else {
		defer file.Close()
		if data, err := io.ReadAll(file); err != nil {
			indexErr = err
		} else {
			indexHTML = data
		}
	}

	return &handler{
		assets:    assets,
		indexHTML: indexHTML,
		indexErr:  indexErr,
	}
}

// handler implements the web UI serving logic.
type handler struct {
	assets    fs.FS
	indexHTML []byte
	indexErr  error
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Method gate - only allow GET and HEAD, return 405 for everything else
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Name resolution - clean path and remove leading slash
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")

	// Special case: treat literal "index.html" as SPA fallback to avoid redirect
	if name == "index.html" {
		h.serveFallback(w, r)
		return
	}

	// Try to stat the requested file
	if info, err := fs.Stat(h.assets, name); err == nil && info.Mode().IsRegular() {
		// Asset hit - set cache headers before delegating to FileServer
		h.setCacheHeaders(w, r.URL.Path)
		http.FileServerFS(h.assets).ServeHTTP(w, r)
		return
	}

	// SPA fallback - empty name, directory, stat error, or miss
	h.serveFallback(w, r)
}

// serveFallback serves the index.html document for SPA routing
func (h *handler) serveFallback(w http.ResponseWriter, r *http.Request) {
	// If index.html couldn't be read during construction, return 500
	if h.indexErr != nil {
		http.Error(w, "Internal Server Error: "+h.indexErr.Error(), http.StatusInternalServerError)
		return
	}

	// Set no-cache for index.html to prevent stale cached versions after updates
	w.Header().Set("Cache-Control", "no-cache")

	// Use ServeContent for proper HEAD handling and content-type detection
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(h.indexHTML))
}

// setCacheHeaders sets appropriate cache headers based on the request path
func (h *handler) setCacheHeaders(w http.ResponseWriter, requestPath string) {
	// Assets under /assets/ are content-hashed by Vite, so they can be cached indefinitely
	if strings.HasPrefix(requestPath, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		// Other files (like direct asset requests not under /assets/) get no-cache
		w.Header().Set("Cache-Control", "no-cache")
	}
}
