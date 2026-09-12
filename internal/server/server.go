// Package server provides HTTP server configuration and route registration.
package server

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/Rhionin/pantry/internal/events"
	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
	"github.com/Rhionin/pantry/internal/shopping"
	"github.com/Rhionin/pantry/internal/suggestion"
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
) (http.Handler, *scan.Queue) {
	mux := http.NewServeMux()

	broadcaster := events.NewBroadcaster()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	eventsHandler := &EventsHandler{Broadcaster: broadcaster}
	mux.HandleFunc("GET /api/events", eventsHandler.Handle)

	// Product handlers
	lookupHandler := &LookupHandler{Service: lookupService}
	listHandler := &ListHandler{Catalog: catalog}
	createHandler := &CreateHandler{Catalog: catalog}
	updateHandler := &UpdateHandler{Catalog: catalog}
	overrideHandler := &OverrideCreateHandler{Catalog: catalog}
	refreshHandler := &RefreshHandler{Refresher: refresher, Catalog: catalog}

	mux.HandleFunc("GET /api/products/lookup", HandleJSON(lookupHandler.Handle))
	mux.HandleFunc("GET /api/products", HandleJSON(listHandler.Handle))
	mux.HandleFunc("POST /api/products", HandleJSON(createHandler.Handle))
	mux.HandleFunc("PUT /api/products/{id}", HandleJSON(updateHandler.Handle))
	mux.HandleFunc("POST /api/products/overrides", HandleJSON(overrideHandler.Handle))
	mux.HandleFunc("POST /api/products/{id}/refresh", HandleJSON(refreshHandler.Handle))

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

	mux.HandleFunc("POST /api/scans", HandleJSON(scanCreateHandler.Handle))
	mux.HandleFunc("GET /api/scans", HandleJSON(scanListHandler.Handle))
	mux.HandleFunc("GET /api/scans/history", HandleJSON(scanHistoryHandler.Handle))
	mux.HandleFunc("PATCH /api/scans/{id}", HandleJSON(scanUpdateHandler.Handle))
	mux.HandleFunc("POST /api/scans/{id}/commit", HandleJSON(scanCommitHandler.Handle))
	mux.HandleFunc("POST /api/scans/batch-commit", HandleJSON(scanBatchCommitHandler.Handle))

	// Inventory handlers
	pantry := inventory.NewPantry(db)
	pantry.Broadcaster = broadcaster
	scanQueue.Pantry = pantry
	inventoryListHandler := &InventoryListHandler{Pantry: pantry}
	inventoryInstancesListHandler := &InventoryInstancesListHandler{Pantry: pantry}
	inventoryInstanceCreateHandler := &InventoryInstanceCreateHandler{Pantry: pantry}
	inventoryInstanceDeleteHandler := &InventoryInstanceDeleteHandler{Pantry: pantry}

	mux.HandleFunc("GET /api/inventory", HandleJSON(inventoryListHandler.Handle))
	mux.HandleFunc("GET /api/inventory/{itemId}/instances", HandleJSON(inventoryInstancesListHandler.Handle))
	mux.HandleFunc("POST /api/inventory/{itemId}/instances", HandleJSON(inventoryInstanceCreateHandler.Handle))
	mux.HandleFunc("DELETE /api/inventory/instances/{instanceId}", HandleJSON(inventoryInstanceDeleteHandler.Handle))

	// Suggestion and target-quantity handlers
	consumptionLog := suggestion.NewConsumptionLog(db)
	suggestionGetHandler := &SuggestionGetHandler{
		ConsumptionLog: consumptionLog,
		Pantry:         pantry,
	}
	setTargetQuantityHandler := &SetTargetQuantityHandler{
		Pantry: pantry,
	}

	mux.HandleFunc("GET /api/suggestions/{itemId}", HandleJSON(suggestionGetHandler.Handle))
	mux.HandleFunc("POST /api/items/{itemId}/target-quantity", HandleJSON(setTargetQuantityHandler.Handle))

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

	mux.HandleFunc("GET /api/shopping-list", HandleJSON(shoppingListGetHandler.Handle))
	mux.HandleFunc("POST /api/shopping-list/items", HandleJSON(shoppingListItemCreateHandler.Handle))
	mux.HandleFunc("DELETE /api/shopping-list/items/{id}", HandleJSON(shoppingListItemDeleteHandler.Handle))
	mux.HandleFunc("PATCH /api/shopping-list/items/{id}", HandleJSON(shoppingListItemUpdateHandler.Handle))
	mux.HandleFunc("POST /api/shopping-list/export", HandleJSON(shoppingListExportHandler.Handle))

	return mux, scanQueue
}
