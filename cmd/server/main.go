package main

import (
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

	productRepo := product.NewRepo(sqlDB)
	lookupService := &product.LookupService{
		Repo:          productRepo,
		OpenFoodFacts: &product.OpenFoodFactsClient{},
	}

	handler := server.NewHandler(productRepo, lookupService)

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
