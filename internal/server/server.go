// Package server provides HTTP server configuration and route registration.
package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Rhionin/pantry/internal/cart"
	"github.com/Rhionin/pantry/internal/cart/appcred"
	"github.com/Rhionin/pantry/internal/cart/connection"
	"github.com/Rhionin/pantry/internal/events"
	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
	"github.com/Rhionin/pantry/internal/scanlistener"
	"github.com/Rhionin/pantry/internal/shopping"
	"github.com/Rhionin/pantry/internal/suggestion"
	"github.com/Rhionin/pantry/internal/telemetry"
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
	registry      *cart.Registry
	ledger        *cart.Ledger
	providerEnv   ProviderEnv
	retailer      shopping.RetailerDealConfig
	contributor   product.UpstreamContributor
}

// Option is a functional option for NewHandler.
type Option func(*config)

// WithCartRegistry supplies the grocery-provider registry and fulfillment
// ledger that back the provider and provisioning endpoints. When unset, the
// cart endpoints wire an empty registry and no-op provisioner, so the default
// install exposes no configured provider.
func WithCartRegistry(registry *cart.Registry, ledger *cart.Ledger) Option {
	return func(c *config) {
		c.registry = registry
		c.ledger = ledger
	}
}

// WithProviderEnv supplies deploy-time cart credentials. Saved credentials
// override them until cleared. The secret is never copied into a response.
func WithProviderEnv(env ProviderEnv) Option {
	return func(c *config) {
		c.providerEnv = env
	}
}

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

// WithRetailerDeals records the store-price gate. An empty API key leaves live
// prices disconnected. Recorded sales and brand preferences still apply.
func WithRetailerDeals(apiKey, baseURL string) Option {
	return func(c *config) {
		c.retailer = shopping.RetailerDealConfig{APIKey: apiKey, BaseURL: baseURL}
	}
}

// WithContributor supplies the Product Opener writer used when a person
// explicitly opts in to sharing a product. When unset, sharing is recorded
// locally and no upstream request is made.
func WithContributor(contributor product.UpstreamContributor) Option {
	return func(c *config) {
		c.contributor = contributor
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
	apiMux, scanQueue, reg := newAPIMux(catalog, lookupService, refresher, db, cfg)

	// Create root mux that composes API routes with web UI
	root := http.NewServeMux()
	root.Handle("/api", apiMux)
	root.Handle("/api/", apiMux)
	root.Handle("/health", apiMux)
	root.Handle("/health/", apiMux)
	root.Handle("/", webui.NewHandler())

	return observeHTTP(reg, hardenHTTP(root)), scanQueue
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
) (*http.ServeMux, *scan.Queue, *telemetry.Registry) {
	apiMux := http.NewServeMux()

	reg := telemetry.NewRegistry()
	broadcaster := events.NewBroadcaster()
	if cfg != nil && cfg.broadcaster != nil {
		broadcaster = cfg.broadcaster
	}
	// The headless listener publishes on this same broadcaster, so the observer
	// has to be attached to the shared instance rather than a private one.
	broadcaster.SetObserver(publishObserver{reg: reg})
	reg.SetSubscriberCount(broadcaster.SubscriberCount)

	// Resolve the cart registry and ledger from config; default to an empty
	// registry and no-op provisioner when no provider is configured.
	var registry *cart.Registry
	var ledger *cart.Ledger
	if cfg != nil {
		registry = cfg.registry
		ledger = cfg.ledger
	}
	if registry == nil {
		registry = cart.NewRegistry()
	}
	if ledger == nil {
		ledger = cart.NewLedger(db)
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

	eventsHandler := &EventsHandler{Broadcaster: broadcaster, Telemetry: reg}
	apiMux.HandleFunc("GET /api/events", eventsHandler.Handle)
	apiMux.HandleFunc("GET /api/build", HandleJSON(handleBuildInfo))

	telemetryHandler := &TelemetryHandler{Registry: reg}
	apiMux.HandleFunc("GET /api/telemetry", telemetryHandler.Get)
	apiMux.HandleFunc("POST /api/telemetry/client", telemetryHandler.PostClient)

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
	if cfg != nil {
		scannerConfigHandler.Status = cfg.statusFn
	}
	apiMux.HandleFunc("POST /api/scanner/mode", HandleJSON(scannerModeHandler.Handle))
	apiMux.HandleFunc("GET /api/scanner/config", HandleJSON(scannerConfigHandler.Handle))

	contributor := product.UpstreamContributor(product.UnconfiguredContributor{})
	if cfg != nil && cfg.contributor != nil {
		contributor = cfg.contributor
	}

	// Product handlers
	lookupHandler := &LookupHandler{Service: lookupService}
	listHandler := &ListHandler{Catalog: catalog}
	createHandler := &CreateHandler{Catalog: catalog, Contributor: contributor}
	updateHandler := &UpdateHandler{Catalog: catalog, Contributor: contributor}
	overrideHandler := &OverrideCreateHandler{Catalog: catalog}
	refreshHandler := &RefreshHandler{Refresher: refresher, Catalog: catalog}
	settingsGetHandler := &ContributionSettingsGetHandler{Catalog: catalog, Contributor: contributor}
	settingsPutHandler := &ContributionSettingsPutHandler{Catalog: catalog, Contributor: contributor}
	contributionsHandler := &ContributionsListHandler{Catalog: catalog}
	productContributionsHandler := &ProductContributionsHandler{Catalog: catalog}
	productGetHandler := &ProductGetHandler{Catalog: catalog}

	apiMux.HandleFunc("GET /api/products/lookup", HandleJSON(lookupHandler.Handle))
	apiMux.HandleFunc("GET /api/products", HandleJSON(listHandler.Handle))
	apiMux.HandleFunc("GET /api/products/{id}", HandleJSON(productGetHandler.Handle))
	apiMux.HandleFunc("POST /api/products", HandleJSON(createHandler.Handle))
	apiMux.HandleFunc("PUT /api/products/{id}", HandleJSON(updateHandler.Handle))
	apiMux.HandleFunc("POST /api/products/overrides", HandleJSON(overrideHandler.Handle))
	apiMux.HandleFunc("POST /api/products/{id}/refresh", HandleJSON(refreshHandler.Handle))
	apiMux.HandleFunc("GET /api/products/{id}/contributions", HandleJSON(productContributionsHandler.Handle))
	apiMux.HandleFunc("GET /api/contributions", HandleJSON(contributionsHandler.Handle))
	apiMux.HandleFunc("GET /api/settings/contribution", HandleJSON(settingsGetHandler.Handle))
	apiMux.HandleFunc("PUT /api/settings/contribution", HandleJSON(settingsPutHandler.Handle))

	// Scan queue handlers
	scanQueue := scan.NewQueue(db)
	scanQueue.Broadcaster = broadcaster
	scanQueue.Ledger = providerLedgerReset{ledger: ledger, registry: registry}
	scanCreateHandler := &ScanCreateHandler{
		Queue:         scanQueue,
		LookupService: lookupService,
		Events:        broadcaster,
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
	inventoryWipeHandler := &InventoryWipeHandler{Pantry: pantry}

	apiMux.HandleFunc("GET /api/inventory", HandleJSON(inventoryListHandler.Handle))
	apiMux.HandleFunc("GET /api/inventory/{itemId}/instances", HandleJSON(inventoryInstancesListHandler.Handle))
	apiMux.HandleFunc("POST /api/inventory/{itemId}/instances", HandleJSON(inventoryInstanceCreateHandler.Handle))
	apiMux.HandleFunc("DELETE /api/inventory/instances/{instanceId}", HandleJSON(inventoryInstanceDeleteHandler.Handle))
	apiMux.HandleFunc("POST /api/inventory/wipe", HandleJSON(inventoryWipeHandler.Handle))

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
	connDir := connection.NewDirectory(db)
	credentialVault := appcred.NewVault(db)
	tokenBroker := connection.NewTokenBroker(connDir)
	engine := cart.NewEngine(registry, ledger, tokenBroker)
	engine.SetShoppingList(*shoppingList)
	engine.SetPantry(*pantry)
	engine.SetConsumptionLog(*consumptionLog)
	if catalog != nil {
		engine.SetCatalog(*catalog)
	}

	shoppingListGetHandler := &ShoppingListGetHandler{
		ShoppingList:   shoppingList,
		Pantry:         pantry,
		Ledger:         ledger,
		Registry:       registry,
		ConsumptionLog: consumptionLog,
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
		Provisioner:  engine,
		Ledger:       ledger,
		Registry:     registry,
		Connections:  connDir,
	}
	var retailer shopping.RetailerDealConfig
	if cfg != nil {
		retailer = cfg.retailer
	}
	shoppingConsiderationsHandler := &ShoppingListConsiderationsHandler{
		ShoppingList: shoppingList,
		Pantry:       pantry,
		Retailer:     retailer,
	}
	shoppingPreferencePutHandler := &ShoppingPreferencePutHandler{
		ShoppingList: shoppingList,
		Pantry:       pantry,
	}
	shoppingPreferenceDeleteHandler := &ShoppingPreferenceDeleteHandler{
		ShoppingList: shoppingList,
		Pantry:       pantry,
	}
	shoppingDealPutHandler := &ShoppingDealPutHandler{
		ShoppingList: shoppingList,
		Pantry:       pantry,
	}
	shoppingDealDeleteHandler := &ShoppingDealDeleteHandler{
		ShoppingList: shoppingList,
	}

	apiMux.HandleFunc("GET /api/shopping-list", HandleJSON(shoppingListGetHandler.Handle))
	apiMux.HandleFunc("GET /api/shopping-list/considerations", HandleJSON(shoppingConsiderationsHandler.Handle))
	apiMux.HandleFunc("POST /api/shopping-list/items", HandleJSON(shoppingListItemCreateHandler.Handle))
	apiMux.HandleFunc("DELETE /api/shopping-list/items/{id}", HandleJSON(shoppingListItemDeleteHandler.Handle))
	apiMux.HandleFunc("PATCH /api/shopping-list/items/{id}", HandleJSON(shoppingListItemUpdateHandler.Handle))
	apiMux.HandleFunc("PUT /api/shopping-list/preferences", HandleJSON(shoppingPreferencePutHandler.Handle))
	apiMux.HandleFunc("DELETE /api/shopping-list/preferences/{itemId}", HandleJSON(shoppingPreferenceDeleteHandler.Handle))
	apiMux.HandleFunc("PUT /api/shopping-list/deals", HandleJSON(shoppingDealPutHandler.Handle))
	apiMux.HandleFunc("DELETE /api/shopping-list/deals/{itemId}", HandleJSON(shoppingDealDeleteHandler.Handle))
	apiMux.HandleFunc("POST /api/shopping-list/export", HandleJSON(shoppingListExportHandler.Handle))

	// Cart integration handlers. connDir is created with the shopping list
	// above so the provisioner and these routes share one directory.
	var envFallback ProviderEnv
	if cfg != nil {
		envFallback = cfg.providerEnv
	}
	providersHandler := &ProvidersHandler{
		Registry:      registry,
		ConnectionDir: connDir,
		Credentials:   credentialVault,
		EnvFallback:   envFallback,
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
	apiMux.HandleFunc("PUT /api/providers/{providerId}/credentials", HandleJSON((&ProviderCredentialPutHandler{ProvidersHandler: *providersHandler}).Handle))
	apiMux.HandleFunc("DELETE /api/providers/{providerId}/credentials", HandleJSON((&ProviderCredentialDeleteHandler{ProvidersHandler: *providersHandler}).Handle))
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
		Ledger:       ledger,
	}
	apiMux.HandleFunc("POST /api/shopping-list/items/{id}/unknown-resolution", HandleJSON(unknownResolutionHandler.Handle))

	return apiMux, scanQueue, reg
}
