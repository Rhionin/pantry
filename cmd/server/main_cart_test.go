package main

import (
	"database/sql"
	"testing"

	"github.com/Rhionin/pantry/internal/app"
	_ "modernc.org/sqlite"
)

func newCartDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := app.RunMigrations(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestLoadCartRegistryUnconfiguredStillListsKroger(t *testing.T) {
	t.Setenv("DISABLE_KROGER", "")
	t.Setenv("KROGER_CLIENT_ID", "  ")
	t.Setenv("KROGER_CLIENT_SECRET", "")
	t.Setenv("KROGER_REDIRECT_URI", "")
	t.Setenv("KROGER_MODALITY", "")
	t.Setenv("KROGER_BATCH_SIZE", "")

	registry, ledger := loadCartRegistry(newCartDB(t))
	if ledger == nil {
		t.Fatal("ledger is nil")
	}
	provider, ok := registry.Get("kroger")
	if !ok {
		t.Fatal("kroger should be registered when credentials are absent")
	}
	if provider.DisplayName() != "Kroger" {
		t.Fatalf("display name %q", provider.DisplayName())
	}
	if registry.CredentialsConfigured("kroger") {
		t.Fatal("kroger should not be credentials-configured")
	}
}

func TestLoadCartRegistryConfigured(t *testing.T) {
	t.Setenv("DISABLE_KROGER", "false")
	t.Setenv("KROGER_CLIENT_ID", "id")
	t.Setenv("KROGER_CLIENT_SECRET", "secret")
	t.Setenv("KROGER_REDIRECT_URI", "https://app.example/cb")
	t.Setenv("KROGER_MODALITY", "delivery")
	t.Setenv("KROGER_BATCH_SIZE", "10")

	registry, _ := loadCartRegistry(newCartDB(t))
	if !registry.CredentialsConfigured("kroger") {
		t.Fatal("kroger should be credentials-configured")
	}
	if got := registry.BatchSize("kroger"); got != 10 {
		t.Fatalf("batch size %d, want 10", got)
	}
	id, ok := registry.SoleConfigured()
	if !ok || id != "kroger" {
		t.Fatalf("sole configured = %q, %v", id, ok)
	}
}

func TestLoadCartRegistryInvalidModalityStaysConfigured(t *testing.T) {
	t.Setenv("DISABLE_KROGER", "")
	t.Setenv("KROGER_CLIENT_ID", "id")
	t.Setenv("KROGER_CLIENT_SECRET", "secret")
	t.Setenv("KROGER_REDIRECT_URI", "https://app.example/cb")
	t.Setenv("KROGER_MODALITY", "ship")
	t.Setenv("KROGER_BATCH_SIZE", "nope")

	registry, _ := loadCartRegistry(newCartDB(t))
	if !registry.CredentialsConfigured("kroger") {
		t.Fatal("invalid modality should still leave Kroger configured")
	}
	if got := registry.BatchSize("kroger"); got != 50 {
		t.Fatalf("batch size %d, want default 50", got)
	}
}

func TestLoadCartRegistryDisabledIsNotConfigured(t *testing.T) {
	t.Setenv("DISABLE_KROGER", "true")
	t.Setenv("KROGER_CLIENT_ID", "id")
	t.Setenv("KROGER_CLIENT_SECRET", "secret")
	t.Setenv("KROGER_REDIRECT_URI", "https://app.example/cb")
	t.Setenv("KROGER_MODALITY", "PICKUP")
	t.Setenv("KROGER_BATCH_SIZE", "50")

	registry, _ := loadCartRegistry(newCartDB(t))
	if _, ok := registry.Get("kroger"); !ok {
		t.Fatal("disabled kroger should still be listed")
	}
	if registry.CredentialsConfigured("kroger") {
		t.Fatal("DISABLE_KROGER should clear credentials-configured")
	}
}
