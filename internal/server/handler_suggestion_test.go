package server

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
)

func TestSuggestionGet(t *testing.T) {
	now := time.Now()

	tests := []scanHandlerTestCase{
		{
			name: "data insufficient with 0 events",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, _ *fakeOpenFoodFacts) {
				createItemViaStockIn(t, db, repo, "user-sug-1", "prod-sug-1", "Milk", "item-sug-1")
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/suggestions/item-sug-1",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.dataInsufficient", value: true},
					{path: "$.itemId", value: "item-sug-1"},
				},
			},
		},
		{
			name: "data insufficient with 2 events",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, _ *fakeOpenFoodFacts) {
				createItemViaStockIn(t, db, repo, "user-sug-2", "prod-sug-2", "Butter", "item-sug-2")
				insertConsumptionEvent(t, db, "ce-2a", "item-sug-2", now.Add(-14*24*time.Hour))
				insertConsumptionEvent(t, db, "ce-2b", "item-sug-2", now.Add(-7*24*time.Hour))
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/suggestions/item-sug-2",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.dataInsufficient", value: true},
				},
			},
		},
		{
			// 3 events 7 days apart → median interval 7 days → ceil(14/7)+1 = 3
			name: "returns numeric suggestion with 3 events",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, _ *fakeOpenFoodFacts) {
				createItemViaStockIn(t, db, repo, "user-sug-3", "prod-sug-3", "Eggs", "item-sug-3")
				insertConsumptionEvent(t, db, "ce-3a", "item-sug-3", now.Add(-14*24*time.Hour))
				insertConsumptionEvent(t, db, "ce-3b", "item-sug-3", now.Add(-7*24*time.Hour))
				insertConsumptionEvent(t, db, "ce-3c", "item-sug-3", now)
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/suggestions/item-sug-3",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.dataInsufficient", value: false},
					{path: "$.suggestedQuantity", value: float64(3)},
					{path: "$.itemId", value: "item-sug-3"},
				},
			},
		},
		{
			name: "404 for nonexistent item",
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/suggestions/does-not-exist",
				expectedStatus: http.StatusNotFound,
			},
		},
	}

	runScanHandlerTests(t, tests)
}

// createItemViaStockIn creates a product and inserts an item row with the given
// deterministic itemID, then seeds one committed stock-in so the item exists in inventory.
func createItemViaStockIn(t *testing.T, db *sql.DB, repo *product.Repo, userID, productID, productName, itemID string) {
	t.Helper()

	if err := repo.CreateProduct(context.Background(), product.Product{
		ID:            productID,
		Name:          productName,
		Category:      "Test",
		UnitOfMeasure: "unit",
	}); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO items (id, user_id, product_id) VALUES (?, ?, ?)`,
		itemID, userID, productID,
	); err != nil {
		t.Fatalf("insert item: %v", err)
	}

	scanRepo := scan.NewRepo(db)
	direction := scan.StockIn
	entry := scan.ScanEntry{
		ID:        "seed-scan-" + productID,
		UserID:    userID,
		Barcode:   "000000000000",
		ScannedAt: time.Now(),
		Direction: &direction,
		UnitCount: 1,
		ProductID: &productID,
		Status:    scan.Pending,
	}
	if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
		t.Fatalf("CreateScanEntry: %v", err)
	}
	if err := scanRepo.CommitStockIn(context.Background(), &entry); err != nil {
		t.Fatalf("CommitStockIn: %v", err)
	}
}

// insertConsumptionEvent inserts a single consumption event for test setup.
func insertConsumptionEvent(t *testing.T, db *sql.DB, id, itemID string, consumedAt time.Time) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO consumption_events (id, item_id, consumed_at) VALUES (?, ?, ?)`,
		id, itemID, consumedAt,
	); err != nil {
		t.Fatalf("insertConsumptionEvent: %v", err)
	}
}
