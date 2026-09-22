// Package server provides HTTP server configuration and route registration.
package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Rhionin/pantry/internal/events"
	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
	"github.com/Rhionin/pantry/internal/scanlistener"
	"github.com/Rhionin/pantry/internal/shopping"
	"github.com/Rhionin/pantry/internal/suggestion"
	"github.com/Rhionin/pantry/internal/webui"
)

// defaultStockInBarcode and defaultStockOutBarcode mirror the headless
// listener's env defaults (STOCK_IN_CONTROL_BARCODE / STOCK_OUT_CONTROL_BARCODE
// in cmd/server/main.go), so GET /api/scanner/config reports usable values even
// when NewHandler is called without WithScannerConfig.
const (
	defaultStockInBarcode  = "STOCK_IN"
	defaultStockOutBarcode = "STOCK_OUT"
)

// config holds the optional configuration for NewHandler.
type config struct {
	broadcaster   *events.Broadcaster
	statusFn      func() scanlistener.Status
	scannerConfig ScannerConfig
}

// Option is a functional option for NewHandler.
type Option func(*config)

// WithBroadcaster supplies an externally owned Broadcaster so the scan
// listener can publish mode changes to the same subscribers the HTTP handlers
// publish to. When unset, NewHandler creates its own, as today.
func WithBroadcaster(b *events.Broadcaster) Option {
	return func(c *config) {
		c.broadcaster = b
	}
}

// WithScannerConfig supplies the reserved control-barcode strings that
// GET /api/scanner/config exposes to the browser, sourced from the same
// env-derived values the headless listener uses. When unset, NewHandler falls
// back to the "STOCK_IN"/"STOCK_OUT" defaults so behavior is unchanged out of
// the box.
func WithScannerConfig(cfg ScannerConfig) Option {
	return func(c *config) {
		c.scannerConfig = cfg
	}
}

// WithScannerStatus supplies the scan listener's status for GET /health. When
// unset, the health response omits the scanner object entirely, which is what
// every existing test sees.
func WithScannerStatus(fn func() scanlistener.Status) Option {
	return func(c *config) {
		c.statusFn = fn
	}
}

// NewHandler creates and configures the HTTP handler with all application
// routes. It also returns the scan.Queue it constructs internally, already
// wired to the same Broadcaster the handler's routes publish through, so
// callers with a second entry point into scan mutations (e.g. a headless
// scan listener) can reuse it instead of constructing an independent Queue
// that would silently skip event broadcasting.
func NewHandler(
	catalog *product.Catalog,
	lookupService *product.LookupService,
	refresher *product.Refresher,
	db *sql.DB,
	opts ...Option,
) (http.Handler, *scan.Queue) {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Build the API mux containing all existing routes
	apiMux, scanQueue := newAPIMux(catalog, lookupService, refresher, db, cfg)

	// Create root mux that composes API routes with web UI
	root := http.NewServeMux()
	root.Handle("/api", apiMux)
	root.Handle("/api/", apiMux)
	root.Handle("/health", apiMux)
	root.Handle("/health/", apiMux)
	root.Handle("/", webui.NewHandler())

	return root, scanQueue
}

// newAPIMux creates the API-only mux with all existing route registrations.
// This extraction allows tests to access the bare API mux for comparison
// while keeping the route definitions in one place.
func newAPIMux(
	catalog *product.Catalog,
	lookupService *product.LookupService,
	refresher *product.Refresher,
	db *sql.DB,
	cfg *config,
) (*http.ServeMux, *scan.Queue) {
	apiMux := http.NewServeMux()

	broadcaster := events.NewBroadcaster()
	if cfg != nil && cfg.broadcaster != nil {
		broadcaster = cfg.broadcaster
	}

	apiMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		response := `{"status":"ok"}`
		if cfg != nil && cfg.statusFn != nil {
			status := cfg.statusFn()
			scannerJSON, _ := json.Marshal(status)
			response = fmt.Sprintf(`%s,"scanner":%s}`, response[:len(response)-1], scannerJSON)
		}
		fmt.Fprintln(w, response)
	})

	eventsHandler := &EventsHandler{Broadcaster: broadcaster}
	apiMux.HandleFunc("GET /api/events", eventsHandler.Handle)

	// Scanner mode + config handlers. The mode handler publishes through the
	// same broadcaster GET /api/events uses, so a browser-initiated mode switch
	// reaches every subscriber exactly as a headless control-barcode scan does.
	// Fall back per field so a caller that sets only one control barcode keeps
	// the value it provided instead of reverting both to defaults.
	scannerConfig := ScannerConfig{StockInBarcode: defaultStockInBarcode, StockOutBarcode: defaultStockOutBarcode}
	if cfg != nil {
		if cfg.scannerConfig.StockInBarcode != "" {
			scannerConfig.StockInBarcode = cfg.scannerConfig.StockInBarcode
		}
		if cfg.scannerConfig.StockOutBarcode != "" {
			scannerConfig.StockOutBarcode = cfg.scannerConfig.StockOutBarcode
		}
	}
	// One scannerMode instance is shared by the mode-switch and config handlers,
	// so GET /api/scanner/config can seed a newly connected browser with the
	// current direction instead of the browser guessing a default that may
	// disagree with the backend.
	mode := newScannerMode()
	scannerModeHandler := &ScannerModeHandler{Mode: mode, Broadcaster: broadcaster}
	scannerConfigHandler := &ScannerConfigHandler{Config: scannerConfig, Mode: mode}
	apiMux.HandleFunc("POST /api/scanner/mode", HandleJSON(scannerModeHandler.Handle))
	apiMux.HandleFunc("GET /api/scanner/config", HandleJSON(scannerConfigHandler.Handle))

	// Product handlers
	lookupHandler := &LookupHandler{Service: lookupService}
	listHandler := &ListHandler{Catalog: catalog}
	createHandler := &CreateHandler{Catalog: catalog}
	updateHandler := &UpdateHandler{Catalog: catalog}
	overrideHandler := &OverrideCreateHandler{Catalog: catalog}
	refreshHandler := &RefreshHandler{Refresher: refresher, Catalog: catalog}

	apiMux.HandleFunc("GET /api/products/lookup", HandleJSON(lookupHandler.Handle))
	apiMux.HandleFunc("GET /api/products", HandleJSON(listHandler.Handle))
	apiMux.HandleFunc("POST /api/products", HandleJSON(createHandler.Handle))
	apiMux.HandleFunc("PUT /api/products/{id}", HandleJSON(updateHandler.Handle))
	apiMux.HandleFunc("POST /api/products/overrides", HandleJSON(overrideHandler.Handle))
	apiMux.HandleFunc("POST /api/products/{id}/refresh", HandleJSON(refreshHandler.Handle))

	// Scan queue handlers
	scanQueue := scan.NewQueue(db)
	scanQueue.Broadcaster = broadcaster
	scanCreateHandler := &ScanCreateHandler{
		Queue:         scanQueue,
		LookupService: lookupService,
	}
	scanListHandler := &ScanListHandler{Queue: scanQueue}
	scanHistoryHandler := &ScanHistoryHandler{Queue: scanQueue}
	scanUpdateHandler := &ScanUpdateHandler{Queue: scanQueue}
	scanCommitHandler := &ScanCommitHandler{Queue: scanQueue}
	scanBatchCommitHandler := &ScanBatchCommitHandler{Queue: scanQueue}

	apiMux.HandleFunc("POST /api/scans", HandleJSON(scanCreateHandler.Handle))
	apiMux.HandleFunc("GET /api/scans", HandleJSON(scanListHandler.Handle))
	apiMux.HandleFunc("GET /api/scans/history", HandleJSON(scanHistoryHandler.Handle))
	apiMux.HandleFunc("PATCH /api/scans/{id}", HandleJSON(scanUpdateHandler.Handle))
	apiMux.HandleFunc("POST /api/scans/{id}/commit", HandleJSON(scanCommitHandler.Handle))
	apiMux.HandleFunc("POST /api/scans/batch-commit", HandleJSON(scanBatchCommitHandler.Handle))

	// Inventory handlers
	pantry := inventory.NewPantry(db)
	pantry.Broadcaster = broadcaster
	scanQueue.Pantry = pantry
	inventoryListHandler := &InventoryListHandler{Pantry: pantry}
	inventoryInstancesListHandler := &InventoryInstancesListHandler{Pantry: pantry}
	inventoryInstanceCreateHandler := &InventoryInstanceCreateHandler{Pantry: pantry}
	inventoryInstanceDeleteHandler := &InventoryInstanceDeleteHandler{Pantry: pantry}

	apiMux.HandleFunc("GET /api/inventory", HandleJSON(inventoryListHandler.Handle))
	apiMux.HandleFunc("GET /api/inventory/{itemId}/instances", HandleJSON(inventoryInstancesListHandler.Handle))
	apiMux.HandleFunc("POST /api/inventory/{itemId}/instances", HandleJSON(inventoryInstanceCreateHandler.Handle))
	apiMux.HandleFunc("DELETE /api/inventory/instances/{instanceId}", HandleJSON(inventoryInstanceDeleteHandler.Handle))

	// Suggestion and target-quantity handlers
	consumptionLog := suggestion.NewConsumptionLog(db)
	suggestionGetHandler := &SuggestionGetHandler{
		ConsumptionLog: consumptionLog,
		Pantry:         pantry,
	}
	setTargetQuantityHandler := &SetTargetQuantityHandler{
		Pantry: pantry,
	}

	apiMux.HandleFunc("GET /api/suggestions/{itemId}", HandleJSON(suggestionGetHandler.Handle))
	apiMux.HandleFunc("POST /api/items/{itemId}/target-quantity", HandleJSON(setTargetQuantityHandler.Handle))

	// Shopping list handlers
	shoppingList := shopping.NewStore(db)
	shoppingListGetHandler := &ShoppingListGetHandler{
		ShoppingList: shoppingList,
		Pantry:       pantry,
	}
	shoppingListItemCreateHandler := &ShoppingListItemCreateHandler{
		ShoppingList: shoppingList,
		Pantry:       pantry,
	}
	shoppingListItemDeleteHandler := &ShoppingListItemDeleteHandler{
		ShoppingList: shoppingList,
	}
	shoppingListItemUpdateHandler := &ShoppingListItemUpdateHandler{
		ShoppingList: shoppingList,
	}
	shoppingListExportHandler := &ShoppingListExportHandler{
		ShoppingList: shoppingList,
		Pantry:       pantry,
		Exporter:     &shopping.NoOpExporter{},
	}

	apiMux.HandleFunc("GET /api/shopping-list", HandleJSON(shoppingListGetHandler.Handle))
	apiMux.HandleFunc("POST /api/shopping-list/items", HandleJSON(shoppingListItemCreateHandler.Handle))
	apiMux.HandleFunc("DELETE /api/shopping-list/items/{id}", HandleJSON(shoppingListItemDeleteHandler.Handle))
	apiMux.HandleFunc("PATCH /api/shopping-list/items/{id}", HandleJSON(shoppingListItemUpdateHandler.Handle))
	apiMux.HandleFunc("POST /api/shopping-list/export", HandleJSON(shoppingListExportHandler.Handle))

	return apiMux, scanQueue
}
