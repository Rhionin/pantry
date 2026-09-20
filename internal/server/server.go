// Package server provides HTTP server configuration and route registration.
package server

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/Rhionin/pantry/internal/cart"
	"github.com/Rhionin/pantry/internal/cart/connection"
	"github.com/Rhionin/pantry/internal/events"
	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
	"github.com/Rhionin/pantry/internal/shopping"
	"github.com/Rhionin/pantry/internal/suggestion"
	"github.com/Rhionin/pantry/internal/webui"
)

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
	registry *cart.Registry,
	ledger *cart.Ledger,
) (http.Handler, *scan.Queue) {
	// Build the API mux containing all existing routes
	apiMux, scanQueue := newAPIMux(catalog, lookupService, refresher, db, registry, ledger)

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
	registry *cart.Registry,
	ledger *cart.Ledger,
) (*http.ServeMux, *scan.Queue) {
	apiMux := http.NewServeMux()

	broadcaster := events.NewBroadcaster()

	apiMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	eventsHandler := &EventsHandler{Broadcaster: broadcaster}
	apiMux.HandleFunc("GET /api/events", eventsHandler.Handle)

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
		Provisioner:  &cart.NoOpProvisioner{},
	}

	apiMux.HandleFunc("GET /api/shopping-list", HandleJSON(shoppingListGetHandler.Handle))
	apiMux.HandleFunc("POST /api/shopping-list/items", HandleJSON(shoppingListItemCreateHandler.Handle))
	apiMux.HandleFunc("DELETE /api/shopping-list/items/{id}", HandleJSON(shoppingListItemDeleteHandler.Handle))
	apiMux.HandleFunc("PATCH /api/shopping-list/items/{id}", HandleJSON(shoppingListItemUpdateHandler.Handle))
	apiMux.HandleFunc("POST /api/shopping-list/export", HandleJSON(shoppingListExportHandler.Handle))

	// Cart integration handlers
	// Note: registry and ledger are passed in from the caller and created in loadCartRegistry()
	connDir := connection.NewDirectory(db)

	providersHandler := &ProvidersHandler{
		Registry:      registry,
		ConnectionDir: connDir,
	}

	listProvidersHandler := &ListProvidersHandler{ProvidersHandler: *providersHandler}
	providerAuthorizeHandler := &ProviderAuthorizeHandler{ProvidersHandler: *providersHandler}
	providerCallbackHandler := &ProviderCallbackHandler{ProvidersHandler: *providersHandler}
	providerDisconnectHandler := &ProviderDisconnectHandler{ProvidersHandler: *providersHandler}
	providerLedgerHandler := &ProviderLedgerHandler{
		ProvidersHandler: *providersHandler,
		Ledger:           ledger,
	}
	providerLedgerGetHandler := &ProviderLedgerGetHandler{ProviderLedgerHandler: *providerLedgerHandler}
	providerLedgerResetHandler := &ProviderLedgerResetHandler{ProviderLedgerHandler: *providerLedgerHandler}

	apiMux.HandleFunc("GET /api/providers", HandleJSON(listProvidersHandler.Handle))
	apiMux.HandleFunc("GET /api/providers/{providerId}/authorize", HandleJSON(providerAuthorizeHandler.Handle))
	apiMux.HandleFunc("GET /api/providers/{providerId}/callback", HandleJSON(providerCallbackHandler.Handle))
	apiMux.HandleFunc("DELETE /api/providers/{providerId}/connection", HandleJSON(providerDisconnectHandler.Handle))
	apiMux.HandleFunc("GET /api/providers/{providerId}/ledger", HandleJSON(providerLedgerGetHandler.Handle))
	apiMux.HandleFunc("POST /api/providers/{providerId}/ledger/reset", HandleJSON(providerLedgerResetHandler.Handle))

	// Replenishment mode handler
	setReplenishmentModeHandler := &SetReplenishmentModeHandler{
		Pantry: pantry,
	}
	apiMux.HandleFunc("POST /api/items/{itemId}/replenishment-mode", HandleJSON(setReplenishmentModeHandler.Handle))

	// Adjustment handler
	shoppingListAdjustmentHandler := &ShoppingListAdjustmentHandler{
		ShoppingList: shoppingList,
	}
	apiMux.HandleFunc("PUT /api/shopping-list/items/{id}/adjustment", HandleJSON(shoppingListAdjustmentHandler.Handle))
	apiMux.HandleFunc("DELETE /api/shopping-list/items/{id}/adjustment", HandleJSON(shoppingListAdjustmentHandler.Handle))

	// Unknown resolution handler
	unknownResolutionHandler := &UnknownResolutionHandler{
		ShoppingList: shoppingList,
		Provisioner:  shoppingListExportHandler.Provisioner,
	}
	apiMux.HandleFunc("POST /api/shopping-list/items/{id}/unknown-resolution", HandleJSON(unknownResolutionHandler.Handle))

	return apiMux, scanQueue
}
