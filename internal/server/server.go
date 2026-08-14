// Package server provides HTTP server configuration and route registration.
package server

import (
	"fmt"
	"net/http"

	"github.com/Rhionin/pantry/internal/product"
)

// NewHandler creates and configures the HTTP handler with all application routes.
func NewHandler(
	productRepo *product.Repo,
	lookupService *product.LookupService,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	lookupHandler := &LookupHandler{Service: lookupService}
	listHandler := &ListHandler{Repo: productRepo}
	createHandler := &CreateHandler{Repo: productRepo}
	updateHandler := &UpdateHandler{Repo: productRepo}
	overrideHandler := &OverrideCreateHandler{Repo: productRepo}

	mux.HandleFunc("GET /api/products/lookup", HandleJSON(lookupHandler.Handle))
	mux.HandleFunc("GET /api/products", HandleJSON(listHandler.Handle))
	mux.HandleFunc("POST /api/products", HandleJSON(createHandler.Handle))
	mux.HandleFunc("PUT /api/products/{id}", HandleJSON(updateHandler.Handle))
	mux.HandleFunc("POST /api/products/overrides", HandleJSON(overrideHandler.Handle))

	return mux
}
