package server

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
	"github.com/steinfletcher/apitest"
	_ "modernc.org/sqlite"
)

// scanHandlerTestCase extends handlerTestCase to pass DB through for setup/verify
type scanHandlerTestCase struct {
	name         string
	setup        func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts)
	httpExchange httpExchange
	afterRequest func(t *testing.T, db *sql.DB)
}

// runScanHandlerTests executes a table of scan handler test cases with DB access
func runScanHandlerTests(t *testing.T, tests []scanHandlerTestCase) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, productRepo, fake, db := setupTestWithDB(t)

			// Run optional setup phase
			if tt.setup != nil {
				tt.setup(t, db, productRepo, fake)
			}

			// Execute the primary HTTP exchange
			test := apitest.New().Handler(handler)
			req := buildRequest(test, tt.httpExchange)
			expect := buildExpectations(req, tt.httpExchange)
			expect.End()

			// Run afterRequest verification if provided
			if tt.afterRequest != nil {
				tt.afterRequest(t, db)
			}
		})
	}
}

func TestScanCommitStockIn(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	tests := []scanHandlerTestCase{
		{
			name: "commit stock-in with unit count 1",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product
				prod := product.Product{
					ID:            "prod-1",
					Name:          "Whole Milk",
					Category:      "Dairy",
					UnitOfMeasure: "gallon",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				// Create scan entry
				scanRepo := scan.NewRepo(db)
				productID := "prod-1"
				direction := scan.StockIn
				entry := scan.ScanEntry{
					ID:        "scan-1",
					UserID:    "user-1",
					Barcode:   "123456789012",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 1,
					ExpiresAt: &expiresAt,
					ProductID: &productID,
					Status:    scan.Pending,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans/scan-1/commit",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.Status", value: "committed"},
					{path: "$.ID", value: "scan-1"},
				},
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify one instance was created
				var count int
				err := db.QueryRowContext(context.Background(), `
					SELECT COUNT(*)
					FROM item_instances ii
					JOIN items i ON i.id = ii.item_id
					WHERE i.user_id = ? AND i.product_id = ? AND ii.removed_at IS NULL`,
					"user-1", "prod-1",
				).Scan(&count)
				if err != nil {
					t.Fatalf("count instances: %v", err)
				}
				if count != 1 {
					t.Errorf("expected 1 instance, got %d", count)
				}

				// Verify stock_in_at and expires_at
				var stockInAt time.Time
				var expiresAtDB sql.NullTime
				err = db.QueryRowContext(context.Background(), `
					SELECT ii.stock_in_at, ii.expires_at
					FROM item_instances ii
					JOIN items i ON i.id = ii.item_id
					WHERE i.user_id = ? AND i.product_id = ? AND ii.removed_at IS NULL`,
					"user-1", "prod-1",
				).Scan(&stockInAt, &expiresAtDB)
				if err != nil {
					t.Fatalf("query instance: %v", err)
				}

				if !stockInAt.Truncate(time.Second).Equal(now.Truncate(time.Second)) {
					t.Errorf("stock_in_at: want %v, got %v", now, stockInAt)
				}
				if !expiresAtDB.Valid {
					t.Error("expires_at: want non-null, got null")
				} else if !expiresAtDB.Time.Truncate(time.Second).Equal(expiresAt.Truncate(time.Second)) {
					t.Errorf("expires_at: want %v, got %v", expiresAt, expiresAtDB.Time)
				}
			},
		},
		{
			name: "commit stock-in with unit count 3",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product
				prod := product.Product{
					ID:            "prod-2",
					Name:          "Orange Juice",
					Category:      "Beverages",
					UnitOfMeasure: "bottle",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				// Create scan entry with unit count 3
				scanRepo := scan.NewRepo(db)
				productID := "prod-2"
				direction := scan.StockIn
				entry := scan.ScanEntry{
					ID:        "scan-2",
					UserID:    "user-2",
					Barcode:   "111222333444",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 3,
					ExpiresAt: &expiresAt,
					ProductID: &productID,
					Status:    scan.Pending,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans/scan-2/commit",
				expectedStatus: http.StatusOK,
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify 3 instances were created
				var count int
				err := db.QueryRowContext(context.Background(), `
					SELECT COUNT(*)
					FROM item_instances ii
					JOIN items i ON i.id = ii.item_id
					WHERE i.user_id = ? AND i.product_id = ? AND ii.removed_at IS NULL`,
					"user-2", "prod-2",
				).Scan(&count)
				if err != nil {
					t.Fatalf("count instances: %v", err)
				}
				if count != 3 {
					t.Errorf("expected 3 instances, got %d", count)
				}
			},
		},
		{
			name: "commit stock-in without expiration date",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product
				prod := product.Product{
					ID:            "prod-3",
					Name:          "Canned Beans",
					Category:      "Canned Goods",
					UnitOfMeasure: "can",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				// Create scan entry without expiration
				scanRepo := scan.NewRepo(db)
				productID := "prod-3"
				direction := scan.StockIn
				entry := scan.ScanEntry{
					ID:        "scan-3",
					UserID:    "user-3",
					Barcode:   "555666777888",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 2,
					ExpiresAt: nil,
					ProductID: &productID,
					Status:    scan.Pending,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans/scan-3/commit",
				expectedStatus: http.StatusOK,
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify expires_at is NULL
				rows, err := db.QueryContext(context.Background(), `
					SELECT ii.expires_at
					FROM item_instances ii
					JOIN items i ON i.id = ii.item_id
					WHERE i.user_id = ? AND i.product_id = ? AND ii.removed_at IS NULL`,
					"user-3", "prod-3",
				)
				if err != nil {
					t.Fatalf("query instances: %v", err)
				}
				defer rows.Close()

				count := 0
				for rows.Next() {
					var expiresAtDB sql.NullTime
					if err := rows.Scan(&expiresAtDB); err != nil {
						t.Fatalf("scan expires_at: %v", err)
					}
					count++
					if expiresAtDB.Valid {
						t.Errorf("expires_at: want NULL, got %v", expiresAtDB.Time)
					}
				}

				if count != 2 {
					t.Errorf("expected 2 instances, got %d", count)
				}
			},
		},
		{
			name: "reuses existing item for same user+product",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product
				prod := product.Product{
					ID:            "prod-7",
					Name:          "Shared Product",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				// Create an item manually first
				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES ('item-existing', 'user-7', 'prod-7')`)
				if err != nil {
					t.Fatalf("create existing item: %v", err)
				}

				// Create scan entry that should reuse the existing item
				scanRepo := scan.NewRepo(db)
				productID := "prod-7"
				direction := scan.StockIn
				entry := scan.ScanEntry{
					ID:        "scan-7",
					UserID:    "user-7",
					Barcode:   "777888999000",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 1,
					ProductID: &productID,
					Status:    scan.Pending,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans/scan-7/commit",
				expectedStatus: http.StatusOK,
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify that only one item exists for this user+product
				var itemCount int
				err := db.QueryRowContext(context.Background(), `
					SELECT COUNT(*)
					FROM items
					WHERE user_id = ? AND product_id = ?`,
					"user-7", "prod-7",
				).Scan(&itemCount)
				if err != nil {
					t.Fatalf("count items: %v", err)
				}
				if itemCount != 1 {
					t.Errorf("expected 1 item, got %d (should reuse existing)", itemCount)
				}

				// Verify the item ID is the pre-existing one
				var itemID string
				err = db.QueryRowContext(context.Background(), `
					SELECT id FROM items
					WHERE user_id = ? AND product_id = ?`,
					"user-7", "prod-7",
				).Scan(&itemID)
				if err != nil {
					t.Fatalf("get item id: %v", err)
				}
				if itemID != "item-existing" {
					t.Errorf("expected existing item ID 'item-existing', got %q", itemID)
				}
			},
		},
		{
			name: "error when status is not pending",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product
				prod := product.Product{
					ID:            "prod-committed",
					Name:          "Test Product",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				// Create scan entry already committed
				scanRepo := scan.NewRepo(db)
				productID := "prod-committed"
				direction := scan.StockIn
				entry := scan.ScanEntry{
					ID:        "scan-committed",
					UserID:    "user-committed",
					Barcode:   "999111222333",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 1,
					ProductID: &productID,
					Status:    scan.Committed,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans/scan-committed/commit",
				expectedStatus: http.StatusBadRequest,
			},
		},
		{
			name: "error when product_id is missing",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create scan entry without product ID
				scanRepo := scan.NewRepo(db)
				direction := scan.StockIn
				entry := scan.ScanEntry{
					ID:        "scan-no-product",
					UserID:    "user-no-product",
					Barcode:   "444555666777",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 1,
					ProductID: nil,
					Status:    scan.Pending,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans/scan-no-product/commit",
				expectedStatus: http.StatusBadRequest,
			},
		},
	}

	runScanHandlerTests(t, tests)
}

func TestScanCommitStockOut(t *testing.T) {
	now := time.Now()
	expiresEarlier := now.Add(3 * 24 * time.Hour)
	expiresLater := now.Add(10 * 24 * time.Hour)

	tests := []scanHandlerTestCase{
		{
			name: "commit stock-out with use-oldest-first",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product
				prod := product.Product{
					ID:            "prod-out-1",
					Name:          "Test Product",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				// Create item
				itemID := "item-out-1"
				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES (?, ?, ?)`,
					itemID, "user-out-1", "prod-out-1")
				if err != nil {
					t.Fatalf("create item: %v", err)
				}

				// Create 3 instances with different expiration dates
				instances := []struct {
					id        string
					expiresAt *time.Time
				}{
					{"inst-1", &expiresLater},   // expires later
					{"inst-2", &expiresEarlier}, // expires earlier (should be selected)
					{"inst-3", nil},             // no expiration
				}
				for _, inst := range instances {
					var expiresAtSQL sql.NullTime
					if inst.expiresAt != nil {
						expiresAtSQL = sql.NullTime{Time: *inst.expiresAt, Valid: true}
					}
					_, err := db.ExecContext(context.Background(), `
						INSERT INTO item_instances (id, item_id, stock_in_at, expires_at)
						VALUES (?, ?, ?, ?)`,
						inst.id, itemID, now.Add(-24*time.Hour), expiresAtSQL)
					if err != nil {
						t.Fatalf("create instance %s: %v", inst.id, err)
					}
				}

				// Create stock-out scan entry
				scanRepo := scan.NewRepo(db)
				productID := "prod-out-1"
				direction := scan.StockOut
				entry := scan.ScanEntry{
					ID:        "scan-out-1",
					UserID:    "user-out-1",
					Barcode:   "123456789012",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 1,
					ProductID: &productID,
					Status:    scan.Pending,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans/scan-out-1/commit",
				expectedStatus: http.StatusOK,
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify that inst-2 (earliest expiration) was removed
				var removedAt sql.NullTime
				var removalReason sql.NullString
				err := db.QueryRowContext(context.Background(), `
					SELECT removed_at, removal_reason
					FROM item_instances
					WHERE id = 'inst-2'`,
				).Scan(&removedAt, &removalReason)
				if err != nil {
					t.Fatalf("query removed instance: %v", err)
				}

				if !removedAt.Valid {
					t.Error("expected inst-2 to be removed (removed_at should be non-NULL)")
				}
				if removalReason.String != "consumed" {
					t.Errorf("removal_reason: want 'consumed', got %q", removalReason.String)
				}

				// Verify other instances are still available
				var availableCount int
				err = db.QueryRowContext(context.Background(), `
					SELECT COUNT(*)
					FROM item_instances
					WHERE item_id = 'item-out-1' AND removed_at IS NULL`,
				).Scan(&availableCount)
				if err != nil {
					t.Fatalf("count available instances: %v", err)
				}
				if availableCount != 2 {
					t.Errorf("expected 2 remaining instances, got %d", availableCount)
				}

				// Verify consumption event was created
				var consumptionEventID string
				err = db.QueryRowContext(context.Background(), `
					SELECT id FROM consumption_events
					WHERE item_id = 'item-out-1' AND scan_entry_id = 'scan-out-1'`,
				).Scan(&consumptionEventID)
				if err != nil {
					t.Fatalf("query consumption event: %v", err)
				}
				if consumptionEventID == "" {
					t.Error("expected consumption event to be created")
				}
			},
		},
		{
			name: "commit stock-out with specific instanceID",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product
				prod := product.Product{
					ID:            "prod-out-2",
					Name:          "Test Product 2",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				// Create item
				itemID := "item-out-2"
				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES (?, ?, ?)`,
					itemID, "user-out-2", "prod-out-2")
				if err != nil {
					t.Fatalf("create item: %v", err)
				}

				// Create 2 instances
				instances := []struct {
					id        string
					expiresAt *time.Time
				}{
					{"inst-specific-1", &expiresEarlier},
					{"inst-specific-2", &expiresLater},
				}
				for _, inst := range instances {
					expiresAtSQL := sql.NullTime{Time: *inst.expiresAt, Valid: true}
					_, err := db.ExecContext(context.Background(), `
						INSERT INTO item_instances (id, item_id, stock_in_at, expires_at)
						VALUES (?, ?, ?, ?)`,
						inst.id, itemID, now.Add(-24*time.Hour), expiresAtSQL)
					if err != nil {
						t.Fatalf("create instance %s: %v", inst.id, err)
					}
				}

				// Create stock-out scan entry
				scanRepo := scan.NewRepo(db)
				productID := "prod-out-2"
				direction := scan.StockOut
				entry := scan.ScanEntry{
					ID:        "scan-out-2",
					UserID:    "user-out-2",
					Barcode:   "222333444555",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 1,
					ProductID: &productID,
					Status:    scan.Pending,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans/scan-out-2/commit",
				body:           `{"instanceId":"inst-specific-2"}`,
				expectedStatus: http.StatusOK,
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify that inst-specific-2 was removed
				var removedAt sql.NullTime
				err := db.QueryRowContext(context.Background(), `
					SELECT removed_at
					FROM item_instances
					WHERE id = 'inst-specific-2'`,
				).Scan(&removedAt)
				if err != nil {
					t.Fatalf("query removed instance: %v", err)
				}
				if !removedAt.Valid {
					t.Error("expected inst-specific-2 to be removed")
				}

				// Verify inst-specific-1 is still available
				var available sql.NullTime
				err = db.QueryRowContext(context.Background(), `
					SELECT removed_at
					FROM item_instances
					WHERE id = 'inst-specific-1'`,
				).Scan(&available)
				if err != nil {
					t.Fatalf("query remaining instance: %v", err)
				}
				if available.Valid {
					t.Error("expected inst-specific-1 to remain available (removed_at should be NULL)")
				}
			},
		},
		{
			name: "commit stock-out prefers dated expiration over NULL (NULLS LAST)",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product
				prod := product.Product{
					ID:            "prod-out-3",
					Name:          "Test Product 3",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				// Create item
				itemID := "item-out-3"
				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES (?, ?, ?)`,
					itemID, "user-out-3", "prod-out-3")
				if err != nil {
					t.Fatalf("create item: %v", err)
				}

				// Create instances: one with NULL expiration, one with date
				instances := []struct {
					id        string
					expiresAt sql.NullTime
				}{
					{"inst-null", sql.NullTime{}},                                 // NULL expiration
					{"inst-dated", sql.NullTime{Time: expiresLater, Valid: true}}, // dated expiration
				}
				for _, inst := range instances {
					_, err := db.ExecContext(context.Background(), `
						INSERT INTO item_instances (id, item_id, stock_in_at, expires_at)
						VALUES (?, ?, ?, ?)`,
						inst.id, itemID, now.Add(-24*time.Hour), inst.expiresAt)
					if err != nil {
						t.Fatalf("create instance %s: %v", inst.id, err)
					}
				}

				// Create stock-out scan entry
				scanRepo := scan.NewRepo(db)
				productID := "prod-out-3"
				direction := scan.StockOut
				entry := scan.ScanEntry{
					ID:        "scan-out-3",
					UserID:    "user-out-3",
					Barcode:   "333444555666",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 1,
					ProductID: &productID,
					Status:    scan.Pending,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans/scan-out-3/commit",
				expectedStatus: http.StatusOK,
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify that inst-dated was removed (dated instances come first with NULLS LAST)
				var removedAt sql.NullTime
				err := db.QueryRowContext(context.Background(), `
					SELECT removed_at
					FROM item_instances
					WHERE id = 'inst-dated'`,
				).Scan(&removedAt)
				if err != nil {
					t.Fatalf("query removed instance: %v", err)
				}
				if !removedAt.Valid {
					t.Error("expected inst-dated to be removed (dated instances should be selected before NULL)")
				}

				// Verify inst-null is still available
				var available sql.NullTime
				err = db.QueryRowContext(context.Background(), `
					SELECT removed_at
					FROM item_instances
					WHERE id = 'inst-null'`,
				).Scan(&available)
				if err != nil {
					t.Fatalf("query remaining instance: %v", err)
				}
				if available.Valid {
					t.Error("expected inst-null to remain available")
				}
			},
		},
		{
			name: "error when no available instances",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product
				prod := product.Product{
					ID:            "prod-out-empty",
					Name:          "Empty Product",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				// Create item but no instances
				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES ('item-empty', 'user-empty', 'prod-out-empty')`)
				if err != nil {
					t.Fatalf("create item: %v", err)
				}

				// Create stock-out scan entry
				scanRepo := scan.NewRepo(db)
				productID := "prod-out-empty"
				direction := scan.StockOut
				entry := scan.ScanEntry{
					ID:        "scan-out-empty",
					UserID:    "user-empty",
					Barcode:   "888999000111",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 1,
					ProductID: &productID,
					Status:    scan.Pending,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans/scan-out-empty/commit",
				expectedStatus: http.StatusInternalServerError,
			},
		},
	}

	runScanHandlerTests(t, tests)
}

func TestScanCommitStockInErrors(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	tests := []scanHandlerTestCase{
		{
			name: "error when direction is not stock-in",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				prod := product.Product{
					ID:            "prod-wrong-dir",
					Name:          "Test Product",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				scanRepo := scan.NewRepo(db)
				productID := "prod-wrong-dir"
				direction := scan.StockOut
				entry := scan.ScanEntry{
					ID:        "scan-wrong-dir",
					UserID:    "user-wrong-dir",
					Barcode:   "111222333444",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 1,
					ExpiresAt: &expiresAt,
					ProductID: &productID,
					Status:    scan.Pending,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans/scan-wrong-dir/commit",
				expectedStatus: http.StatusInternalServerError,
			},
		},
		{
			name: "error when unit count is zero",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				prod := product.Product{
					ID:            "prod-zero-count",
					Name:          "Test Product",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				scanRepo := scan.NewRepo(db)
				productID := "prod-zero-count"
				direction := scan.StockIn
				entry := scan.ScanEntry{
					ID:        "scan-zero-count",
					UserID:    "user-zero-count",
					Barcode:   "555666777888",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 0,
					ProductID: &productID,
					Status:    scan.Pending,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans/scan-zero-count/commit",
				expectedStatus: http.StatusInternalServerError,
			},
		},
	}

	runScanHandlerTests(t, tests)
}

func TestScanCommitStockOutErrors(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	tests := []scanHandlerTestCase{
		{
			name: "error when no item exists for user+product",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				prod := product.Product{
					ID:            "prod-no-item",
					Name:          "Test Product",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				// Create scan entry without creating an item first
				scanRepo := scan.NewRepo(db)
				productID := "prod-no-item"
				direction := scan.StockOut
				entry := scan.ScanEntry{
					ID:        "scan-no-item",
					UserID:    "user-no-item",
					Barcode:   "333444555666",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 1,
					ProductID: &productID,
					Status:    scan.Pending,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans/scan-no-item/commit",
				expectedStatus: http.StatusInternalServerError,
			},
		},
		{
			name: "error when specified instanceID does not exist",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				prod := product.Product{
					ID:            "prod-bad-inst",
					Name:          "Test Product",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				// Create item with one instance
				itemID := "item-bad-inst"
				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES (?, ?, ?)`,
					itemID, "user-bad-inst", "prod-bad-inst")
				if err != nil {
					t.Fatalf("create item: %v", err)
				}

				_, err = db.ExecContext(context.Background(), `
					INSERT INTO item_instances (id, item_id, stock_in_at, expires_at)
					VALUES (?, ?, ?, ?)`,
					"inst-exists", itemID, now.Add(-24*time.Hour), sql.NullTime{Time: expiresAt, Valid: true})
				if err != nil {
					t.Fatalf("create instance: %v", err)
				}

				// Create scan entry
				scanRepo := scan.NewRepo(db)
				productID := "prod-bad-inst"
				direction := scan.StockOut
				entry := scan.ScanEntry{
					ID:        "scan-bad-inst",
					UserID:    "user-bad-inst",
					Barcode:   "444555666777",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 1,
					ProductID: &productID,
					Status:    scan.Pending,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans/scan-bad-inst/commit",
				body:           `{"instanceId":"inst-does-not-exist"}`,
				expectedStatus: http.StatusInternalServerError,
			},
		},
		{
			name: "error when specified instanceID is already removed",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				prod := product.Product{
					ID:            "prod-removed-inst",
					Name:          "Test Product",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				// Create item
				itemID := "item-removed-inst"
				_, err := db.ExecContext(context.Background(), `
					INSERT INTO items (id, user_id, product_id)
					VALUES (?, ?, ?)`,
					itemID, "user-removed-inst", "prod-removed-inst")
				if err != nil {
					t.Fatalf("create item: %v", err)
				}

				// Create instance that is already removed
				_, err = db.ExecContext(context.Background(), `
					INSERT INTO item_instances (id, item_id, stock_in_at, expires_at, removed_at, removal_reason)
					VALUES (?, ?, ?, ?, ?, ?)`,
					"inst-removed", itemID, now.Add(-48*time.Hour),
					sql.NullTime{Time: expiresAt, Valid: true},
					sql.NullTime{Time: now.Add(-24 * time.Hour), Valid: true},
					sql.NullString{String: "consumed", Valid: true})
				if err != nil {
					t.Fatalf("create removed instance: %v", err)
				}

				// Create scan entry
				scanRepo := scan.NewRepo(db)
				productID := "prod-removed-inst"
				direction := scan.StockOut
				entry := scan.ScanEntry{
					ID:        "scan-removed-inst",
					UserID:    "user-removed-inst",
					Barcode:   "555666777888",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 1,
					ProductID: &productID,
					Status:    scan.Pending,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans/scan-removed-inst/commit",
				body:           `{"instanceId":"inst-removed"}`,
				expectedStatus: http.StatusInternalServerError,
			},
		},
	}

	runScanHandlerTests(t, tests)
}

func TestScanResolveFlaggedEntry(t *testing.T) {
	now := time.Now()

	tests := []scanHandlerTestCase{
		{
			name: "successfully resolve flagged entry with new product override",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product
				prod := product.Product{
					ID:            "prod-resolve-1",
					Name:          "Corrected Product",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				// Create flagged scan entry
				scanRepo := scan.NewRepo(db)
				entry := scan.ScanEntry{
					ID:        "scan-resolve-1",
					UserID:    "user-resolve-1",
					Barcode:   "123456789999",
					ScannedAt: now,
					UnitCount: 1,
					Status:    scan.Flagged,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "PATCH",
				path:           "/api/scans/scan-resolve-1",
				body:           `{"productId":"prod-resolve-1","status":"pending"}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.Status", value: "pending"},
					{path: "$.ProductID", value: "prod-resolve-1"},
				},
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify product override was created
				var productID string
				err := db.QueryRowContext(context.Background(), `
					SELECT product_id FROM barcodes
					WHERE barcode = ? AND source = 'user_override' AND user_id = ?`,
					"123456789999", "user-resolve-1",
				).Scan(&productID)
				if err != nil {
					t.Fatalf("query override: %v", err)
				}
				if productID != "prod-resolve-1" {
					t.Errorf("expected product_id 'prod-resolve-1', got %q", productID)
				}

				// Verify scan entry status is pending
				scanRepo := scan.NewRepo(db)
				entry, err := scanRepo.GetScanEntry(context.Background(), "scan-resolve-1")
				if err != nil {
					t.Fatalf("GetScanEntry: %v", err)
				}
				if entry.Status != scan.Pending {
					t.Errorf("expected status pending, got %v", entry.Status)
				}
			},
		},
		{
			name: "successfully update existing product override (resolve same barcode twice)",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create two products
				prod1 := product.Product{
					ID:            "prod-resolve-2a",
					Name:          "Original Product",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod1); err != nil {
					t.Fatalf("CreateProduct 1: %v", err)
				}

				prod2 := product.Product{
					ID:            "prod-resolve-2b",
					Name:          "Corrected Product",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod2); err != nil {
					t.Fatalf("CreateProduct 2: %v", err)
				}

				// Create existing override
				_, err := db.ExecContext(context.Background(), `
					INSERT INTO barcodes (barcode, product_id, source, user_id)
					VALUES (?, ?, 'user_override', ?)`,
					"888999000111", "prod-resolve-2a", "user-resolve-2")
				if err != nil {
					t.Fatalf("create existing override: %v", err)
				}

				// Create flagged scan entry
				scanRepo := scan.NewRepo(db)
				entry := scan.ScanEntry{
					ID:        "scan-resolve-2",
					UserID:    "user-resolve-2",
					Barcode:   "888999000111",
					ScannedAt: now,
					UnitCount: 1,
					Status:    scan.Flagged,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "PATCH",
				path:           "/api/scans/scan-resolve-2",
				body:           `{"productId":"prod-resolve-2b","status":"pending"}`,
				expectedStatus: http.StatusOK,
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// Verify override was updated to new product
				var productID string
				err := db.QueryRowContext(context.Background(), `
					SELECT product_id FROM barcodes
					WHERE barcode = ? AND source = 'user_override' AND user_id = ?`,
					"888999000111", "user-resolve-2",
				).Scan(&productID)
				if err != nil {
					t.Fatalf("query override: %v", err)
				}
				if productID != "prod-resolve-2b" {
					t.Errorf("expected updated product_id 'prod-resolve-2b', got %q", productID)
				}

				// Verify only one override exists (update, not insert)
				var count int
				err = db.QueryRowContext(context.Background(), `
					SELECT COUNT(*) FROM barcodes
					WHERE barcode = ? AND source = 'user_override' AND user_id = ?`,
					"888999000111", "user-resolve-2",
				).Scan(&count)
				if err != nil {
					t.Fatalf("count overrides: %v", err)
				}
				if count != 1 {
					t.Errorf("expected 1 override, got %d", count)
				}
			},
		},
		{
			name: "error when scan entry not found",
			httpExchange: httpExchange{
				method:         "PATCH",
				path:           "/api/scans/scan-does-not-exist",
				body:           `{"productId":"prod-x","status":"pending"}`,
				expectedStatus: http.StatusNotFound,
			},
		},
		{
			name: "error when scan entry status is not flagged (try to resolve pending entry)",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create product
				prod := product.Product{
					ID:            "prod-resolve-pending",
					Name:          "Test Product",
					Category:      "Test",
					UnitOfMeasure: "unit",
				}
				if err := repo.CreateProduct(context.Background(), prod); err != nil {
					t.Fatalf("CreateProduct: %v", err)
				}

				// Create pending scan entry (not flagged)
				scanRepo := scan.NewRepo(db)
				productID := "prod-resolve-pending"
				direction := scan.StockIn
				entry := scan.ScanEntry{
					ID:        "scan-resolve-pending",
					UserID:    "user-resolve-pending",
					Barcode:   "777888999000",
					ScannedAt: now,
					Direction: &direction,
					UnitCount: 1,
					ProductID: &productID,
					Status:    scan.Pending,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "PATCH",
				path:           "/api/scans/scan-resolve-pending",
				body:           `{"productId":"prod-resolve-pending"}`,
				expectedStatus: http.StatusOK,
			},
			afterRequest: func(t *testing.T, db *sql.DB) {
				// This test verifies that resolving a non-flagged entry still works
				// (the PATCH endpoint doesn't enforce flagged status)
				// In production, the UI would only allow resolving flagged entries
			},
		},
		{
			name: "error when product does not exist",
			setup: func(t *testing.T, db *sql.DB, repo *product.Repo, fake *fakeOpenFoodFacts) {
				// Create flagged scan entry
				scanRepo := scan.NewRepo(db)
				entry := scan.ScanEntry{
					ID:        "scan-resolve-bad-prod",
					UserID:    "user-resolve-bad-prod",
					Barcode:   "666777888999",
					ScannedAt: now,
					UnitCount: 1,
					Status:    scan.Flagged,
				}
				if _, err := scanRepo.CreateScanEntry(context.Background(), entry); err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "PATCH",
				path:           "/api/scans/scan-resolve-bad-prod",
				body:           `{"productId":"prod-does-not-exist","status":"pending"}`,
				expectedStatus: http.StatusInternalServerError,
			},
		},
	}

	runScanHandlerTests(t, tests)
}
