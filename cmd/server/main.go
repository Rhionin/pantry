package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"

	"github.com/Rhionin/pantry/internal/app"
	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/server"
	_ "modernc.org/sqlite"
)

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
	var externalProducts interface {
		LookupBarcode(context.Context, string) (*product.ProductSummary, error)
	} = product.NewOpenFoodFactsClient()
	if os.Getenv("DISABLE_EXTERNAL_PRODUCT_LOOKUP") == "true" {
		externalProducts = disabledProductLookup{}
	}
	lookupService := &product.LookupService{
		Catalog:       catalog,
		OpenFoodFacts: externalProducts,
	}

	handler := server.NewHandler(catalog, lookupService, sqlDB)

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
