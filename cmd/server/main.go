package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Rhionin/pantry/internal/app"
	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/server"
	_ "modernc.org/sqlite"
)

// defaultProductCacheTTL is used when PRODUCT_CACHE_TTL is unset, empty, or
// unparseable.
const defaultProductCacheTTL = 30 * 24 * time.Hour

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
	var externalProducts interface {
		LookupBarcode(context.Context, string) (*product.ProductSummary, error)
	} = product.NewOpenFoodFactsClient()
	if !externalLookupEnabled {
		externalProducts = disabledProductLookup{}
	}
	refresher := &product.Refresher{
		Catalog:               catalog,
		OpenFoodFacts:         externalProducts,
		TTL:                   productCacheTTL(),
		ExternalLookupEnabled: externalLookupEnabled,
	}
	lookupService := &product.LookupService{
		Catalog:       catalog,
		OpenFoodFacts: externalProducts,
		Refresher:     refresher,
	}

	handler := server.NewHandler(catalog, lookupService, refresher, sqlDB)

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

type disabledProductLookup struct{}

func (disabledProductLookup) LookupBarcode(context.Context, string) (*product.ProductSummary, error) {
	return nil, product.ErrProductNotFound
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
