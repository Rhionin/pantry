package server

import (
	"bytes"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// TestPublicBrandAndLegalPages locks the stable URLs a grocery developer app
// stores. Caddy is what removes basic auth; these cases lock the bytes the
// proxy forwards to, and that neighboring routes are not the same documents.
func TestPublicBrandAndLegalPages(t *testing.T) {
	logo := readRepoFile(t, filepath.Join("..", "brand", "logo.png"))
	terms := readRepoFile(t, filepath.Join("..", "brand", "terms.html"))
	privacy := readRepoFile(t, filepath.Join("..", "brand", "privacy.html"))

	runHandlerTests(t, []handlerTestCase{
		{
			name: "GET /brand/logo.png serves the unhashed pantry mark",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/brand/logo.png",
				expectedStatus: http.StatusOK,
				expectedHeaders: map[string]string{
					"Content-Type":  "image/png",
					"Cache-Control": "no-cache",
				},
			},
			afterRequest: func(env testEnv) {
				body := readBody(env)
				if !bytes.Equal(body, logo) {
					env.T.Fatalf("logo bytes = %d, file = %d", len(body), len(logo))
				}
				cfg, err := png.DecodeConfig(bytes.NewReader(body))
				if err != nil {
					env.T.Fatalf("decode logo: %v", err)
				}
				if cfg.Width != 512 || cfg.Height != 512 {
					env.T.Fatalf("logo size = %dx%d, want 512x512", cfg.Width, cfg.Height)
				}
				img, err := png.Decode(bytes.NewReader(body))
				if err != nil {
					env.T.Fatalf("decode logo image: %v", err)
				}
				if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
					env.T.Fatal("logo background should be transparent so it sits on a light consent screen")
				}
			},
		},
		{
			name: "GET /terms is the public terms page",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/terms",
				expectedStatus: http.StatusOK,
				expectedHeaders: map[string]string{
					"Content-Type":  "text/html; charset=utf-8",
					"Cache-Control": "no-cache",
				},
				bodyContains: []string{
					"personal household pantry",
					"credentials and the tokens for a connected account stay on the server you run",
					"Kroger account data is used only to search products and write a cart",
					"not an official Kroger product",
					"personal project",
					"Questions about these terms: this website",
				},
				bodyExcludes: []string{placeholderMarker, "LLC", "Inc.", "Corporation", "P.O. Box", "@"},
			},
			afterRequest: func(env testEnv) {
				if got := readBody(env); !bytes.Equal(got, terms) {
					env.T.Fatal("GET /terms did not serve terms.html")
				}
			},
		},
		{
			name: "GET /privacy is the public privacy policy",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/privacy",
				expectedStatus: http.StatusOK,
				expectedHeaders: map[string]string{
					"Content-Type":  "text/html; charset=utf-8",
					"Cache-Control": "no-cache",
				},
				bodyContains: []string{
					"personal project",
					"stores the inventory, the shopping cart",
					"Kroger tokens",
					"not sold",
					"not sent to a third party except Kroger, when you connect an account and write a cart",
					"not an official Kroger product",
					"Questions about this policy: this website",
				},
				bodyExcludes: []string{placeholderMarker, "LLC", "Inc.", "Corporation", "P.O. Box", "@"},
			},
			afterRequest: func(env testEnv) {
				if got := readBody(env); !bytes.Equal(got, privacy) {
					env.T.Fatal("GET /privacy did not serve privacy.html")
				}
			},
		},
		{
			name: "GET /brand/logo.jpg is not the public mark",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/brand/logo.jpg",
				expectedStatus: http.StatusNotFound,
				expectedHeaders: map[string]string{
					"Cache-Control": "no-store",
				},
			},
			afterRequest: assertBodyExcludesPlaceholder,
		},
		{
			name: "GET /terms/extra stays the app shell",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/terms/extra",
				expectedStatus: http.StatusOK,
				bodyContains:   []string{placeholderMarker},
				bodyExcludes:   []string{"Kroger account data is used only to search products and write a cart"},
			},
		},
		{
			name: "GET /privacy.html is the app shell, not the policy",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/privacy.html",
				expectedStatus: http.StatusOK,
				bodyContains:   []string{placeholderMarker},
				bodyExcludes:   []string{"not sent to a third party except Kroger"},
			},
		},
		{
			name: "GET /terms/ is not the terms page",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/terms/",
				expectedStatus: http.StatusOK,
				bodyContains:   []string{placeholderMarker},
				bodyExcludes:   []string{"Kroger account data is used only to search products and write a cart"},
			},
		},
		{
			name: "GET /privacy/ is not the privacy policy",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/privacy/",
				expectedStatus: http.StatusOK,
				bodyContains:   []string{placeholderMarker},
				bodyExcludes:   []string{"not sent to a third party except Kroger"},
			},
		},
		{
			name: "GET /favicon.ico stays a static miss",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/favicon.ico",
				expectedStatus: http.StatusNotFound,
			},
			afterRequest: assertBodyExcludesPlaceholder,
		},
		{
			name: "GET /health stays the health document",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/health",
				expectedStatus: http.StatusOK,
				bodyContains:   []string{`"status":"ok"`},
				bodyExcludes:   []string{"Privacy policy", "Terms of use"},
			},
		},
	})
}

func readRepoFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return data
}

func readBody(env testEnv) []byte {
	env.T.Helper()
	defer env.Res.Body.Close()
	body, err := io.ReadAll(env.Res.Body)
	if err != nil {
		env.T.Fatalf("read body: %v", err)
	}
	return body
}
