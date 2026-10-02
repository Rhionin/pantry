package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHardenHTTP(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	h := hardenHTTP(next)

	t.Run("security headers", func(t *testing.T) {
		called = false
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if !called {
			t.Fatal("expected the next handler to run")
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("nosniff header = %q", rec.Header().Get("X-Content-Type-Options"))
		}
		if rec.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatalf("frame header = %q", rec.Header().Get("X-Frame-Options"))
		}
		if rec.Header().Get("Strict-Transport-Security") != "" {
			t.Fatal("HSTS belongs on the HTTPS proxy, not the LAN server")
		}
	})

	t.Run("cross-site post is rejected", func(t *testing.T) {
		called = false
		req := httptest.NewRequest(http.MethodPost, "/api/inventory/wipe", strings.NewReader(`{"confirmation":"WIPE INVENTORY"}`))
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if called {
			t.Fatal("cross-site post reached the handler")
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
	})

	t.Run("same-origin post is allowed", func(t *testing.T) {
		called = false
		req := httptest.NewRequest(http.MethodPost, "/api/scans", nil)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if !called || rec.Code != http.StatusNoContent {
			t.Fatalf("called=%v status=%d", called, rec.Code)
		}
	})

	t.Run("cross-site authorize is rejected", func(t *testing.T) {
		called = false
		req := httptest.NewRequest(http.MethodGet, "/api/providers/kroger/authorize", nil)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if called || rec.Code != http.StatusForbidden {
			t.Fatalf("called=%v status=%d", called, rec.Code)
		}
	})

	t.Run("cross-site oauth callback is allowed", func(t *testing.T) {
		called = false
		req := httptest.NewRequest(http.MethodGet, "/api/providers/kroger/callback?code=x&state=y", nil)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if !called || rec.Code != http.StatusNoContent {
			t.Fatalf("called=%v status=%d", called, rec.Code)
		}
	})

	t.Run("request without fetch metadata is allowed", func(t *testing.T) {
		called = false
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/scans", nil))
		if !called {
			t.Fatal("non-browser client was rejected")
		}
	})
}

func TestWriteErrorHidesInternalDetail(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusInternalServerError, "sql: database is locked near secret-token")
	if strings.Contains(rec.Body.String(), "secret-token") || strings.Contains(rec.Body.String(), "sql:") {
		t.Fatalf("internal detail leaked: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Something went wrong") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestHandleJSONRejectsOversizedBody(t *testing.T) {
	h := HandleJSON(func(Request[struct{ X string }, struct{}]) (struct{}, error) {
		t.Fatal("handler ran for an oversized body")
		return struct{}{}, nil
	})
	payload := `{"x":"` + strings.Repeat("a", maxJSONBodyBytes) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/products", strings.NewReader(payload))
	req.ContentLength = int64(len(payload))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 400 or 413", rec.Code)
	}
}

func TestAcceptableRedirectURI(t *testing.T) {
	ok := []string{
		"https://pantry.rhionin.com/api/providers/kroger/callback",
		"http://localhost:8080/api/providers/kroger/callback",
		"http://127.0.0.1/cb",
		"http://[::1]/cb",
	}
	for _, raw := range ok {
		if !acceptableRedirectURI(raw) {
			t.Errorf("want accept %s", raw)
		}
	}
	bad := []string{
		"http://pantry.rhionin.com/cb",
		"http://evil.example/cb",
		"javascript:alert(1)",
		"https://user:pass@pantry.rhionin.com/cb",
		"https://evil.example\\@pantry.rhionin.com/cb",
		"",
	}
	for _, raw := range bad {
		if acceptableRedirectURI(raw) {
			t.Errorf("want reject %s", raw)
		}
	}
}
