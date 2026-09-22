package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Rhionin/pantry/internal/app"
	"github.com/Rhionin/pantry/internal/events"
	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scanlistener"
	"github.com/Rhionin/pantry/internal/server"
	_ "modernc.org/sqlite"
)

// defaultProductCacheTTL is used when PRODUCT_CACHE_TTL is unset, empty, or
// unparseable.
const defaultProductCacheTTL = 30 * 24 * time.Hour

// defaultMissTTL is used when PRODUCT_MISS_TTL is unset, empty, or unparseable.
const defaultMissTTL = 7 * 24 * time.Hour

// productCacheTTL reads PRODUCT_CACHE_TTL and returns the TTL a product
// cache row uses before Refresher.ScheduleRefresh considers it stale. An
// unparseable value is logged and defaulted rather than failing startup. A
// value that does parse is used as given even if non-positive: 0s is a
// legitimate "always revalidate" debugging setting, and the Refresher's
// single-flight guard keeps it from stampeding upstream.
func productCacheTTL() time.Duration {
	raw := os.Getenv("PRODUCT_CACHE_TTL")
	if raw == "" {
		return defaultProductCacheTTL
	}
	ttl, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("invalid PRODUCT_CACHE_TTL %q, using default of %s", raw, defaultProductCacheTTL)
		return defaultProductCacheTTL
	}
	return ttl
}

// productMissTTL reads PRODUCT_MISS_TTL and returns the age at which a
// confirmed-miss record expires and the barcode becomes eligible for another
// fan-out. Shaped exactly like productCacheTTL: an unparseable value is logged
// and defaulted rather than failing startup, and a value that parses is used as
// given even if non-positive, since 0s is a legitimate "never cache a miss"
// debugging setting.
func productMissTTL() time.Duration {
	raw := os.Getenv("PRODUCT_MISS_TTL")
	if raw == "" {
		return defaultMissTTL
	}
	ttl, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("invalid PRODUCT_MISS_TTL %q, using default of %s", raw, defaultMissTTL)
		return defaultMissTTL
	}
	return ttl
}

func main() {
	dbPath := envOrDefault("DB_PATH", "pantry.db")
	addr := envOrDefault("ADDR", ":8080")

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer sqlDB.Close()

	// SQLite is single-writer; limit to one connection to avoid SQLITE_BUSY.
	sqlDB.SetMaxOpenConns(1)

	if err := app.RunMigrations(sqlDB); err != nil {
		log.Fatalf("run migrations: %v", err)
	}
	log.Println("migrations applied")

	catalog := product.NewCatalog(sqlDB)
	externalLookupEnabled := os.Getenv("DISABLE_EXTERNAL_PRODUCT_LOOKUP") != "true"
	var upstream product.UpstreamDatabases = product.NewExternalLookup(product.DefaultProductOpenerClients())
	if !externalLookupEnabled {
		upstream = disabledExternalLookup{}
	}
	refresher := &product.Refresher{
		Catalog:               catalog,
		Upstream:              upstream,
		TTL:                   productCacheTTL(),
		ExternalLookupEnabled: externalLookupEnabled,
	}
	lookupService := &product.LookupService{
		Catalog:   catalog,
		Upstream:  upstream,
		Refresher: refresher,
		MissTTL:   productMissTTL(),
	}

	// One Broadcaster shared by the HTTP handlers and the headless listener, so
	// a mode change from either capture path reaches the same GET /api/events
	// subscribers.
	broadcaster := events.NewBroadcaster()

	stockInBarcode := envOrDefault("STOCK_IN_CONTROL_BARCODE", "STOCK_IN")
	stockOutBarcode := envOrDefault("STOCK_OUT_CONTROL_BARCODE", "STOCK_OUT")

	handler, scanQueue := server.NewHandler(catalog, lookupService, refresher, sqlDB,
		server.WithBroadcaster(broadcaster),
		server.WithScannerConfig(server.ScannerConfig{
			StockInBarcode:  stockInBarcode,
			StockOutBarcode: stockOutBarcode,
		}),
	)

	if listener, ok := loadScanListenerConfigWithSource(); ok {
		listener.Queue = scanQueue
		listener.LookupService = lookupService
		listener.ModePublisher = broadcaster
		go listener.Run(context.Background())
	}

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

type disabledExternalLookup struct{}

func (disabledExternalLookup) Lookup(context.Context, string) product.FanOutResult {
	return product.FanOutResult{Outcome: product.FanOutUnresolved}
}

func (disabledExternalLookup) LookupIn(context.Context, product.ExternalSource, string) (*product.ProductSummary, error) {
	return nil, product.ErrProductNotFound
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// loadScanListenerConfig is deprecated; use loadScanListenerConfigWithSource.
func loadScanListenerConfig() (*scanlistener.ScanListener, bool) {
	stockIn := envOrDefault("STOCK_IN_CONTROL_BARCODE", "STOCK_IN")
	stockOut := envOrDefault("STOCK_OUT_CONTROL_BARCODE", "STOCK_OUT")
	if stockIn == stockOut {
		log.Printf("scan listener: STOCK_IN_CONTROL_BARCODE and STOCK_OUT_CONTROL_BARCODE must differ, not starting scan listener")
		return nil, false
	}

	return &scanlistener.ScanListener{
		StockInBarcode:  stockIn,
		StockOutBarcode: stockOut,
		HeadlessUserID:  envOrDefault("HEADLESS_USER_ID", "user-1"),
	}, true
}

// loadScanListenerConfigWithSource reads the ScanListener's env-configurable
// settings and returns a listener with Source, DevicePath, and ScanInputSource
// configured, ready to have its Queue, LookupService, and ModePublisher assigned.
// It returns ok=false when the stock-in and stock-out control barcodes are
// identical, when the source is unrecognized, or when SCAN_INPUT has an
// invalid value. Default source is "device"; default device path is "/dev/pantry-scanner".
func loadScanListenerConfigWithSource() (*scanlistener.ScanListener, bool) {
	source, ok := scanlistener.ParseSource(envOrDefault("SCAN_INPUT", ""))
	if !ok {
		log.Printf("scan listener: invalid SCAN_INPUT value, defaulting to device")
	}
	if source == scanlistener.SourceStdin {
		log.Printf("scan listener: reading scans from standard input (SCAN_INPUT=stdin)")
	}

	stockIn := envOrDefault("STOCK_IN_CONTROL_BARCODE", "STOCK_IN")
	stockOut := envOrDefault("STOCK_OUT_CONTROL_BARCODE", "STOCK_OUT")
	if stockIn == stockOut {
		log.Printf("scan listener: STOCK_IN_CONTROL_BARCODE and STOCK_OUT_CONTROL_BARCODE must differ, not starting scan listener")
		return nil, false
	}

	listener := scanlistener.New()
	listener.Source = source
	listener.StockInBarcode = stockIn
	listener.StockOutBarcode = stockOut
	listener.HeadlessUserID = envOrDefault("HEADLESS_USER_ID", "user-1")
	listener.DevicePath = envOrDefault("SCANNER_DEVICE", "/dev/pantry-scanner")

	log.Printf("scan listener: configured with source=%s device=%s", source, listener.DevicePath)

	return listener, true
}
