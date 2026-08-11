package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Rhionin/pantry/internal/app"
	_ "modernc.org/sqlite"
)

func main() {
	dbPath := envOrDefault("DB_PATH", "pantry.db")
	addr := envOrDefault("ADDR", ":8080")

	// Open the SQLite database.
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer sqlDB.Close()

	// SQLite is single-writer; limit to one connection to avoid SQLITE_BUSY.
	sqlDB.SetMaxOpenConns(1)

	// Apply migrations.
	if err := app.RunMigrations(sqlDB); err != nil {
		log.Fatalf("run migrations: %v", err)
	}
	log.Println("migrations applied")

	// Set up the HTTP router using Go 1.22+ ServeMux with method+path patterns.
	mux := http.NewServeMux()

	// Health check — useful for smoke-testing the server.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	// TODO: register additional route handlers as they are implemented in
	// subsequent tasks (scan queue, products, inventory, suggestions, shopping list).

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
