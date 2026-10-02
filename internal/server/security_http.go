package server

import (
	"net/http"
	"strings"
)

// hardenHTTP sets response headers that do not depend on TLS terminating in
// this process, and rejects cross-site browser requests that would change
// state. Browsers send Sec-Fetch-Site; curl and the test suite do not, so an
// absent header is allowed. The OAuth callback is a cross-site GET by design
// and stays allowed. HSTS is left to the public proxy: this server is also
// reached over plain HTTP on the LAN.
func hardenHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(self), microphone=(), geolocation=()")
		if crossSiteForbidden(r) {
			writeError(w, http.StatusForbidden, "This request was rejected.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func crossSiteForbidden(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") != "cross-site" {
		return false
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		// Starting OAuth writes a single-use state. A cross-site GET would
		// replace it. The callback itself must remain reachable from the
		// provider's redirect.
		path := r.URL.Path
		return strings.HasPrefix(path, "/api/providers/") && strings.HasSuffix(path, "/authorize")
	case http.MethodOptions:
		return false
	default:
		return true
	}
}
