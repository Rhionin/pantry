package scan_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Rhionin/pantry/internal/app"
	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
)

// newTestRepo opens an in-memory SQLite database, applies all migrations, and returns a scan Repo.
func newTestRepo(t *testing.T) (*scan.Repo, *sql.DB) {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory SQLite: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return scan.NewRepo(conn), conn
}

// --------------------------------------------------------------------------
// TestCreateScanEntry
// --------------------------------------------------------------------------

func TestCreateScanEntry(t *testing.T) {
	tests := []struct {
		name          string
		entry         scan.ScanEntry
		wantIDEmpty   bool
		wantStatus    scan.ScanStatus
		wantDirection *scan.ScanDirection
	}{
		{
			name: "explicit ID and pending status",
			entry: scan.ScanEntry{
				ID:        "scan-1",
				UserID:    "user-1",
				Barcode:   "123456789012",
				ScannedAt: time.Now(),
				UnitCount: 1,
				Status:    scan.Pending,
			},
			wantIDEmpty:   false,
			wantStatus:    scan.Pending,
			wantDirection: nil,
		},
		{
			name: "generate UUID when ID empty",
			entry: scan.ScanEntry{
				UserID:    "user-1",
				Barcode:   "987654321098",
				ScannedAt: time.Now(),
				UnitCount: 2,
			},
			wantIDEmpty:   true,
			wantStatus:    scan.Pending,
			wantDirection: nil,
		},
		{
			name: "with pre-selected direction",
			entry: scan.ScanEntry{
				ID:        "scan-2",
				UserID:    "user-1",
				Barcode:   "111222333444",
				ScannedAt: time.Now(),
				Direction: ptrScanDirection(scan.StockIn),
				UnitCount: 3,
			},
			wantIDEmpty:   false,
			wantStatus:    scan.Pending,
			wantDirection: ptrScanDirection(scan.StockIn),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := newTestRepo(t)
			ctx := context.Background()

			// Create scan entry
			created, err := repo.CreateScanEntry(ctx, tt.entry)
			if err != nil {
				t.Fatalf("CreateScanEntry: %v", err)
			}

			// Verify ID generation if needed
			if tt.wantIDEmpty && created.ID == "" {
				t.Error("expected generated UUID, got empty string")
			}
			if !tt.wantIDEmpty && created.ID != tt.entry.ID {
				t.Errorf("ID: want %q, got %q", tt.entry.ID, created.ID)
			}

			// Verify status
			if created.Status != tt.wantStatus {
				t.Errorf("Status: want %q, got %q", tt.wantStatus, created.Status)
			}

			// Verify direction
			if tt.wantDirection == nil && created.Direction != nil {
				t.Errorf("Direction: want nil, got %v", *created.Direction)
			}
			if tt.wantDirection != nil {
				if created.Direction == nil {
					t.Error("Direction: want non-nil, got nil")
				} else if *created.Direction != *tt.wantDirection {
					t.Errorf("Direction: want %v, got %v", *tt.wantDirection, *created.Direction)
				}
			}

			// Verify barcode preserved
			if created.Barcode != tt.entry.Barcode {
				t.Errorf("Barcode: want %q, got %q", tt.entry.Barcode, created.Barcode)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestGetScanEntry
// --------------------------------------------------------------------------

func TestGetScanEntry(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		setup       func(t *testing.T, repo *scan.Repo, ctx context.Context)
		expectFound bool
	}{
		{
			name: "found",
			id:   "scan-1",
			setup: func(t *testing.T, repo *scan.Repo, ctx context.Context) {
				entry := scan.ScanEntry{
					ID:        "scan-1",
					UserID:    "user-1",
					Barcode:   "123456789012",
					ScannedAt: time.Now(),
					UnitCount: 1,
				}
				_, err := repo.CreateScanEntry(ctx, entry)
				if err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			expectFound: true,
		},
		{
			name:        "not found",
			id:          "no-such-id",
			setup:       func(t *testing.T, repo *scan.Repo, ctx context.Context) {},
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := newTestRepo(t)
			ctx := context.Background()
			tt.setup(t, repo, ctx)

			got, err := repo.GetScanEntry(ctx, tt.id)
			if err != nil {
				t.Fatalf("GetScanEntry: %v", err)
			}
			if tt.expectFound && got == nil {
				t.Fatal("expected scan entry, got nil")
			}
			if !tt.expectFound && got != nil {
				t.Errorf("expected nil, got %+v", got)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestListScanEntries
// --------------------------------------------------------------------------

func TestListScanEntries(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name        string
		userID      string
		status      scan.ScanStatus
		setup       func(t *testing.T, repo *scan.Repo, ctx context.Context)
		expectCount int
		expectOrder []string // barcodes in expected chronological order
	}{
		{
			name:        "empty",
			userID:      "user-1",
			status:      scan.Pending,
			setup:       func(t *testing.T, repo *scan.Repo, ctx context.Context) {},
			expectCount: 0,
			expectOrder: []string{},
		},
		{
			name:   "single entry",
			userID: "user-1",
			status: scan.Pending,
			setup: func(t *testing.T, repo *scan.Repo, ctx context.Context) {
				entry := scan.ScanEntry{
					ID:        "scan-1",
					UserID:    "user-1",
					Barcode:   "111111111111",
					ScannedAt: now,
					UnitCount: 1,
					Status:    scan.Pending,
				}
				_, err := repo.CreateScanEntry(ctx, entry)
				if err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			expectCount: 1,
			expectOrder: []string{"111111111111"},
		},
		{
			name:   "multiple entries ordered chronologically",
			userID: "user-1",
			status: scan.Pending,
			setup: func(t *testing.T, repo *scan.Repo, ctx context.Context) {
				entries := []scan.ScanEntry{
					{
						ID:        "scan-1",
						UserID:    "user-1",
						Barcode:   "333333333333",
						ScannedAt: now.Add(2 * time.Minute),
						UnitCount: 1,
						Status:    scan.Pending,
					},
					{
						ID:        "scan-2",
						UserID:    "user-1",
						Barcode:   "111111111111",
						ScannedAt: now,
						UnitCount: 1,
						Status:    scan.Pending,
					},
					{
						ID:        "scan-3",
						UserID:    "user-1",
						Barcode:   "222222222222",
						ScannedAt: now.Add(1 * time.Minute),
						UnitCount: 1,
						Status:    scan.Pending,
					},
				}
				for _, e := range entries {
					_, err := repo.CreateScanEntry(ctx, e)
					if err != nil {
						t.Fatalf("CreateScanEntry: %v", err)
					}
				}
			},
			expectCount: 3,
			expectOrder: []string{"111111111111", "222222222222", "333333333333"},
		},
		{
			name:   "filter by status",
			userID: "user-1",
			status: scan.Flagged,
			setup: func(t *testing.T, repo *scan.Repo, ctx context.Context) {
				entries := []scan.ScanEntry{
					{
						ID:        "scan-1",
						UserID:    "user-1",
						Barcode:   "111111111111",
						ScannedAt: now,
						UnitCount: 1,
						Status:    scan.Pending,
					},
					{
						ID:        "scan-2",
						UserID:    "user-1",
						Barcode:   "222222222222",
						ScannedAt: now.Add(1 * time.Minute),
						UnitCount: 1,
						Status:    scan.Flagged,
					},
					{
						ID:        "scan-3",
						UserID:    "user-1",
						Barcode:   "333333333333",
						ScannedAt: now.Add(2 * time.Minute),
						UnitCount: 1,
						Status:    scan.Committed,
					},
				}
				for _, e := range entries {
					_, err := repo.CreateScanEntry(ctx, e)
					if err != nil {
						t.Fatalf("CreateScanEntry: %v", err)
					}
				}
			},
			expectCount: 1,
			expectOrder: []string{"222222222222"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := newTestRepo(t)
			ctx := context.Background()
			tt.setup(t, repo, ctx)

			// List and verify
			got, err := repo.ListScanEntries(ctx, tt.userID, tt.status)
			if err != nil {
				t.Fatalf("ListScanEntries: %v", err)
			}
			if len(got) != tt.expectCount {
				t.Fatalf("expected %d entries, got %d", tt.expectCount, len(got))
			}

			// Verify chronological order
			for i, wantBarcode := range tt.expectOrder {
				if got[i].Barcode != wantBarcode {
					t.Errorf("[%d] Barcode: want %q, got %q", i, wantBarcode, got[i].Barcode)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestUpdateScanEntry
// --------------------------------------------------------------------------

func TestUpdateScanEntry(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	tests := []struct {
		name          string
		id            string
		direction     *scan.ScanDirection
		unitCount     *int
		expiresAt     *time.Time
		productID     *string
		status        *scan.ScanStatus
		expectError   bool
		verifyUpdated func(t *testing.T, entry *scan.ScanEntry)
	}{
		{
			name:      "update direction",
			id:        "scan-1",
			direction: ptrScanDirection(scan.StockIn),
			verifyUpdated: func(t *testing.T, entry *scan.ScanEntry) {
				if entry.Direction == nil || *entry.Direction != scan.StockIn {
					t.Errorf("Direction: want %v, got %v", scan.StockIn, entry.Direction)
				}
			},
		},
		{
			name:      "update unit count",
			id:        "scan-2",
			unitCount: ptrInt(5),
			verifyUpdated: func(t *testing.T, entry *scan.ScanEntry) {
				if entry.UnitCount != 5 {
					t.Errorf("UnitCount: want 5, got %d", entry.UnitCount)
				}
			},
		},
		{
			name:      "update expiry",
			id:        "scan-3",
			expiresAt: &expiresAt,
			verifyUpdated: func(t *testing.T, entry *scan.ScanEntry) {
				if entry.ExpiresAt == nil {
					t.Error("ExpiresAt: want non-nil, got nil")
				} else if !entry.ExpiresAt.Equal(expiresAt) {
					t.Errorf("ExpiresAt: want %v, got %v", expiresAt, *entry.ExpiresAt)
				}
			},
		},
		{
			name:      "update product ID",
			id:        "scan-4",
			productID: ptrString("prod-123"),
			verifyUpdated: func(t *testing.T, entry *scan.ScanEntry) {
				if entry.ProductID == nil || *entry.ProductID != "prod-123" {
					t.Errorf("ProductID: want prod-123, got %v", entry.ProductID)
				}
			},
		},
		{
			name:   "update status",
			id:     "scan-5",
			status: ptrScanStatus(scan.Committed),
			verifyUpdated: func(t *testing.T, entry *scan.ScanEntry) {
				if entry.Status != scan.Committed {
					t.Errorf("Status: want %v, got %v", scan.Committed, entry.Status)
				}
			},
		},
		{
			name:        "not found",
			id:          "no-such-id",
			direction:   ptrScanDirection(scan.StockIn),
			expectError: true,
			verifyUpdated: func(t *testing.T, entry *scan.ScanEntry) {
				// Not called when expectError is true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := newTestRepo(t)
			ctx := context.Background()

			// Setup: create scan entry
			if !tt.expectError {
				entry := scan.ScanEntry{
					ID:        tt.id,
					UserID:    "user-1",
					Barcode:   "123456789012",
					ScannedAt: now,
					UnitCount: 1,
					Status:    scan.Pending,
				}
				_, err := repo.CreateScanEntry(ctx, entry)
				if err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			}

			// Update
			err := repo.UpdateScanEntry(ctx, tt.id, tt.direction, tt.unitCount, tt.expiresAt, tt.productID, tt.status)
			if tt.expectError && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Fatalf("UpdateScanEntry: %v", err)
			}

			// Verify if successful
			if !tt.expectError {
				got, err := repo.GetScanEntry(ctx, tt.id)
				if err != nil {
					t.Fatalf("GetScanEntry: %v", err)
				}
				tt.verifyUpdated(t, got)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestCommitScanEntry
// --------------------------------------------------------------------------

func TestCommitScanEntry(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		id          string
		setup       func(t *testing.T, repo *scan.Repo, ctx context.Context)
		expectError bool
	}{
		{
			name: "commit successfully",
			id:   "scan-1",
			setup: func(t *testing.T, repo *scan.Repo, ctx context.Context) {
				entry := scan.ScanEntry{
					ID:        "scan-1",
					UserID:    "user-1",
					Barcode:   "123456789012",
					ScannedAt: now,
					UnitCount: 1,
					Status:    scan.Pending,
				}
				_, err := repo.CreateScanEntry(ctx, entry)
				if err != nil {
					t.Fatalf("CreateScanEntry: %v", err)
				}
			},
			expectError: false,
		},
		{
			name:        "entry not found",
			id:          "no-such-id",
			setup:       func(t *testing.T, repo *scan.Repo, ctx context.Context) {},
			expectError: false, // CommitScanEntry doesn't check for existence before updating
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := newTestRepo(t)
			ctx := context.Background()
			tt.setup(t, repo, ctx)

			// Commit
			err := repo.CommitScanEntry(ctx, tt.id)
			if tt.expectError && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Fatalf("CommitScanEntry: %v", err)
			}

			// Verify committed status
			if !tt.expectError && tt.id == "scan-1" {
				got, err := repo.GetScanEntry(ctx, tt.id)
				if err != nil {
					t.Fatalf("GetScanEntry: %v", err)
				}
				if got == nil {
					return // Entry didn't exist, which is OK in this test
				}
				if got.Status != scan.Committed {
					t.Errorf("Status: want %v, got %v", scan.Committed, got.Status)
				}
				if got.CommittedAt == nil {
					t.Error("CommittedAt: want non-nil, got nil")
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestBatchUpdateScanEntries
// --------------------------------------------------------------------------

func TestBatchUpdateScanEntries(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	tests := []struct {
		name           string
		ids            []string
		direction      *scan.ScanDirection
		unitCount      *int
		expiresAt      *time.Time
		setup          func(t *testing.T, repo *scan.Repo, ctx context.Context)
		expectError    bool
		verifyAllMatch func(t *testing.T, repo *scan.Repo, ctx context.Context, ids []string)
	}{
		{
			name:      "batch update direction and expiry",
			ids:       []string{"scan-1", "scan-2", "scan-3"},
			direction: ptrScanDirection(scan.StockIn),
			expiresAt: &expiresAt,
			setup: func(t *testing.T, repo *scan.Repo, ctx context.Context) {
				for i, id := range []string{"scan-1", "scan-2", "scan-3"} {
					entry := scan.ScanEntry{
						ID:        id,
						UserID:    "user-1",
						Barcode:   string(rune('1' + i)),
						ScannedAt: now.Add(time.Duration(i) * time.Minute),
						UnitCount: 1,
						Status:    scan.Pending,
					}
					_, err := repo.CreateScanEntry(ctx, entry)
					if err != nil {
						t.Fatalf("CreateScanEntry: %v", err)
					}
				}
			},
			expectError: false,
			verifyAllMatch: func(t *testing.T, repo *scan.Repo, ctx context.Context, ids []string) {
				for _, id := range ids {
					got, err := repo.GetScanEntry(ctx, id)
					if err != nil {
						t.Fatalf("GetScanEntry %s: %v", id, err)
					}
					if got.Direction == nil || *got.Direction != scan.StockIn {
						t.Errorf("%s Direction: want %v, got %v", id, scan.StockIn, got.Direction)
					}
					if got.ExpiresAt == nil {
						t.Errorf("%s ExpiresAt: want non-nil, got nil", id)
					}
				}
			},
		},
		{
			name:      "empty batch",
			ids:       []string{},
			direction: ptrScanDirection(scan.StockIn),
			setup:     func(t *testing.T, repo *scan.Repo, ctx context.Context) {},
			verifyAllMatch: func(t *testing.T, repo *scan.Repo, ctx context.Context, ids []string) {
				// Nothing to verify
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := newTestRepo(t)
			ctx := context.Background()
			tt.setup(t, repo, ctx)

			// Batch update
			err := repo.BatchUpdateScanEntries(ctx, tt.ids, tt.direction, tt.unitCount, tt.expiresAt)
			if tt.expectError && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Fatalf("BatchUpdateScanEntries: %v", err)
			}

			// Verify
			if !tt.expectError {
				tt.verifyAllMatch(t, repo, ctx, tt.ids)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestScanEntryWithProductJoin
// --------------------------------------------------------------------------

func TestScanEntryWithProductJoin(t *testing.T) {
	now := time.Now()

	repo, db := newTestRepo(t)
	ctx := context.Background()

	// Create a product
	productRepo := product.NewRepo(db)
	prod := product.Product{
		ID:            "prod-1",
		Name:          "Whole Milk",
		Category:      "Dairy",
		UnitOfMeasure: "gallon",
	}
	if err := productRepo.CreateProduct(ctx, prod); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	// Create a scan entry with product ID
	productID := "prod-1"
	entry := scan.ScanEntry{
		ID:        "scan-1",
		UserID:    "user-1",
		Barcode:   "123456789012",
		ScannedAt: now,
		UnitCount: 1,
		ProductID: &productID,
		Status:    scan.Pending,
	}
	_, err := repo.CreateScanEntry(ctx, entry)
	if err != nil {
		t.Fatalf("CreateScanEntry: %v", err)
	}

	// Retrieve and verify product join
	got, err := repo.GetScanEntry(ctx, "scan-1")
	if err != nil {
		t.Fatalf("GetScanEntry: %v", err)
	}

	if got.Product == nil {
		t.Fatal("Product: want non-nil, got nil")
	}
	if got.Product.ID != "prod-1" {
		t.Errorf("Product.ID: want prod-1, got %s", got.Product.ID)
	}
	if got.Product.Name != "Whole Milk" {
		t.Errorf("Product.Name: want Whole Milk, got %s", got.Product.Name)
	}
}

// --------------------------------------------------------------------------
// Helper functions
// --------------------------------------------------------------------------

func ptrScanDirection(d scan.ScanDirection) *scan.ScanDirection {
	return &d
}

func ptrInt(i int) *int {
	return &i
}

func ptrString(s string) *string {
	return &s
}

func ptrScanStatus(s scan.ScanStatus) *scan.ScanStatus {
	return &s
}
