package server

import (
	"net/http"
	"strings"
)

// browserCSP is the policy for both the LAN server and the public proxy.
// Styles need 'unsafe-inline' because the UI sets style attributes. Scripts
// stay on this origin. Camera access is a Permissions-Policy decision, not CSP.
const browserCSP = "default-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; object-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' https: data:; font-src 'self' data:; connect-src 'self'"

// secureHeaders sets response headers that do not depend on Caddy being in
// front of the process. HSTS is left to the HTTPS proxy: sending it on the
// plain LAN listener would not help, and upgrade-insecure-requests would
// break that listener.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", browserCSP)
		h.Set("Permissions-Policy", "camera=(self), microphone=(), geolocation=()")
		if r.URL.Path == "/health" || strings.HasPrefix(r.URL.Path, "/api") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
