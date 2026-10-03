// Package brand serves the stable public mark and the legal pages a grocery
// developer app stores by URL. The paths are exact and unhashed so a later
// edit replaces the bytes without changing the URL.
package brand

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"net/http"
	"time"
)

//go:embed logo.png
var logoPNG []byte

//go:embed terms.html
var termsHTML []byte

//go:embed privacy.html
var privacyHTML []byte

// Register mounts GET /brand/logo.png, GET /terms, and GET /privacy.
func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /brand/logo.png", serve("logo.png", "image/png", logoPNG))
	mux.HandleFunc("GET /terms", serve("terms.html", "text/html; charset=utf-8", termsHTML))
	mux.HandleFunc("GET /privacy", serve("privacy.html", "text/html; charset=utf-8", privacyHTML))
}

// no-cache lets a replaced file show up at the same path. A long-lived
// immutable cache would pin the previous mark or page to the URL Kroger stored.
func serve(name, contentType string, body []byte) http.HandlerFunc {
	tag := weakETag(body)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("ETag", tag)
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(body))
	}
}

// weakETag stays valid after Caddy gzip, which does not rewrite weak validators.
func weakETag(data []byte) string {
	sum := sha256.Sum256(data)
	return `W/"` + hex.EncodeToString(sum[:]) + `"`
}
