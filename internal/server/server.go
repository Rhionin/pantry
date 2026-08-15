// Package server provides HTTP server configuration and route registration.
package server

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
)

// NewHandler creates and configures the HTTP handler with all application routes.
func NewHandler(
	productRepo *product.Repo,
	lookupService *product.LookupService,
	db *sql.DB,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	// Product handlers
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

	// Scan queue handlers
	scanRepo := scan.NewRepo(db)
	scanCreateHandler := &ScanCreateHandler{
		Repo:          scanRepo,
		LookupService: lookupService,
	}
	scanListHandler := &ScanListHandler{Repo: scanRepo}
	scanHistoryHandler := &ScanHistoryHandler{Repo: scanRepo}
	scanUpdateHandler := &ScanUpdateHandler{Repo: scanRepo}
	scanCommitHandler := &ScanCommitHandler{Repo: scanRepo}
	scanBatchCommitHandler := &ScanBatchCommitHandler{Repo: scanRepo}

	mux.HandleFunc("POST /api/scans", HandleJSON(scanCreateHandler.Handle))
	mux.HandleFunc("GET /api/scans", HandleJSON(scanListHandler.Handle))
	mux.HandleFunc("GET /api/scans/history", HandleJSON(scanHistoryHandler.Handle))
	mux.HandleFunc("PATCH /api/scans/{id}", HandleJSON(scanUpdateHandler.Handle))
	mux.HandleFunc("POST /api/scans/{id}/commit", HandleJSON(scanCommitHandler.Handle))
	mux.HandleFunc("POST /api/scans/batch-commit", HandleJSON(scanBatchCommitHandler.Handle))

	return mux
}
