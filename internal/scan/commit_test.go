package scan_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
	"github.com/google/uuid"
)

// --------------------------------------------------------------------------
// Test Property: Stock-in commit creates exactly N instances
// --------------------------------------------------------------------------
// Feature: pantry-management, Property 5: Stock-in commit creates exactly N instances
//
// For any scan entry with direction `stock_in` and unit count N ≥ 1,
// committing that entry SHALL create exactly N new item instances for the
// corresponding item, each with the scan entry's stock-in timestamp and
// expiration date, and the item's total instance count SHALL increase by
// exactly N.

func TestProperty_StockInCreatesExactlyNInstances(t *testing.T) {
	repo, db := newTestRepo(t)
	ctx := context.Background()
	now := time.Now()

	// Test with various unit counts
	testCases := []struct {
		unitCount int
		expiresAt *time.Time
	}{
		{unitCount: 1, expiresAt: nil},
		{unitCount: 1, expiresAt: ptrTime(now.Add(7 * 24 * time.Hour))},
		{unitCount: 2, expiresAt: ptrTime(now.Add(14 * 24 * time.Hour))},
		{unitCount: 5, expiresAt: nil},
		{unitCount: 10, expiresAt: ptrTime(now.Add(30 * 24 * time.Hour))},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("unit_count_%d", tc.unitCount), func(t *testing.T) {
			// Create product
			productRepo := product.NewRepo(db)
			productID := uuid.NewString()
			prod := product.Product{
				ID:            productID,
				Name:          fmt.Sprintf("Product %s", productID),
				Category:      "Test",
				UnitOfMeasure: "unit",
			}
			if err := productRepo.CreateProduct(ctx, prod); err != nil {
				t.Fatalf("CreateProduct: %v", err)
			}

			// Create scan entry
			userID := uuid.NewString()
			direction := scan.StockIn
			scanEntry := scan.ScanEntry{
				ID:        uuid.NewString(),
				UserID:    userID,
				Barcode:   uuid.NewString(),
				ScannedAt: now,
				Direction: &direction,
				UnitCount: tc.unitCount,
				ExpiresAt: tc.expiresAt,
				ProductID: &productID,
				Status:    scan.Pending,
			}
			created, err := repo.CreateScanEntry(ctx, scanEntry)
			if err != nil {
				t.Fatalf("CreateScanEntry: %v", err)
			}

			// Count instances before commit
			var beforeCount int
			err = db.QueryRowContext(ctx, `
				SELECT COUNT(*)
				FROM item_instances ii
				JOIN items i ON i.id = ii.item_id
				WHERE i.user_id = ? AND i.product_id = ? AND ii.removed_at IS NULL`,
				userID, productID,
			).Scan(&beforeCount)
			if err != nil {
				t.Fatalf("count before: %v", err)
			}

			// Commit stock-in
			if err := repo.CommitStockIn(ctx, created); err != nil {
				t.Fatalf("CommitStockIn: %v", err)
			}

			// Count instances after commit
			var afterCount int
			err = db.QueryRowContext(ctx, `
				SELECT COUNT(*)
				FROM item_instances ii
				JOIN items i ON i.id = ii.item_id
				WHERE i.user_id = ? AND i.product_id = ? AND ii.removed_at IS NULL`,
				userID, productID,
			).Scan(&afterCount)
			if err != nil {
				t.Fatalf("count after: %v", err)
			}

			// Property: instance count increased by exactly N
			increase := afterCount - beforeCount
			if increase != tc.unitCount {
				t.Errorf("Property violation: expected instance count to increase by %d, but increased by %d (before=%d, after=%d)",
					tc.unitCount, increase, beforeCount, afterCount)
			}

			// Verify each instance has correct timestamps
			rows, err := db.QueryContext(ctx, `
				SELECT ii.stock_in_at, ii.expires_at
				FROM item_instances ii
				JOIN items i ON i.id = ii.item_id
				WHERE i.user_id = ? AND i.product_id = ? AND ii.removed_at IS NULL`,
				userID, productID,
			)
			if err != nil {
				t.Fatalf("query instances: %v", err)
			}
			defer rows.Close()

			instanceCount := 0
			for rows.Next() {
				var stockInAt time.Time
				var expiresAt sql.NullTime

				if err := rows.Scan(&stockInAt, &expiresAt); err != nil {
					t.Fatalf("scan instance: %v", err)
				}

				instanceCount++

				// Verify stock_in_at matches scan entry
				if !stockInAt.Equal(now) {
					t.Errorf("instance %d: stock_in_at want %v, got %v", instanceCount, now, stockInAt)
				}

				// Verify expires_at matches scan entry
				if tc.expiresAt == nil && expiresAt.Valid {
					t.Errorf("instance %d: expires_at want NULL, got %v", instanceCount, expiresAt.Time)
				}
				if tc.expiresAt != nil {
					if !expiresAt.Valid {
						t.Errorf("instance %d: expires_at want %v, got NULL", instanceCount, *tc.expiresAt)
					} else if !expiresAt.Time.Equal(*tc.expiresAt) {
						t.Errorf("instance %d: expires_at want %v, got %v", instanceCount, *tc.expiresAt, expiresAt.Time)
					}
				}
			}

			if instanceCount != tc.unitCount {
				t.Errorf("Property violation: expected %d instances with correct timestamps, found %d",
					tc.unitCount, instanceCount)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Test Property: Stock-out commit removes use-oldest-first
// --------------------------------------------------------------------------
// Feature: pantry-management, Property 6: Stock-out commit removes use-oldest-first
//
// For any scan entry with direction `stock_out` and no instance specified,
// committing that entry SHALL remove one existing item instance, and the
// instance with the nearest (soonest) non-NULL expiration date (use-oldest-first)
// SHALL be the one removed, and the item's total instance count SHALL decrease
// by exactly 1.

func TestProperty_StockOutRemovesUseOldestFirst(t *testing.T) {
	repo, db := newTestRepo(t)
	ctx := context.Background()
	now := time.Now()

	// Test cases with various expiration date combinations
	testCases := []struct {
		name               string
		expirationDates    []*time.Time // instances to create
		expectedRemovedIdx int          // which instance should be removed (0-indexed)
	}{
		{
			name: "all dated instances - select earliest",
			expirationDates: []*time.Time{
				ptrTime(now.Add(10 * 24 * time.Hour)),
				ptrTime(now.Add(3 * 24 * time.Hour)), // earliest - should be removed
				ptrTime(now.Add(7 * 24 * time.Hour)),
			},
			expectedRemovedIdx: 1,
		},
		{
			name: "mix of dated and NULL - select dated over NULL",
			expirationDates: []*time.Time{
				nil,
				ptrTime(now.Add(5 * 24 * time.Hour)), // only dated - should be removed
				nil,
			},
			expectedRemovedIdx: 1,
		},
		{
			name: "all NULL expiration dates - select first one",
			expirationDates: []*time.Time{
				nil,
				nil,
				nil,
			},
			expectedRemovedIdx: 0, // when all NULL, order is indeterminate but one will be removed
		},
		{
			name: "single instance - select it",
			expirationDates: []*time.Time{
				ptrTime(now.Add(14 * 24 * time.Hour)),
			},
			expectedRemovedIdx: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create product
			productRepo := product.NewRepo(db)
			productID := uuid.NewString()
			prod := product.Product{
				ID:            productID,
				Name:          fmt.Sprintf("Product %s", productID),
				Category:      "Test",
				UnitOfMeasure: "unit",
			}
			if err := productRepo.CreateProduct(ctx, prod); err != nil {
				t.Fatalf("CreateProduct: %v", err)
			}

			// Create item
			userID := uuid.NewString()
			itemID := uuid.NewString()
			_, err := db.ExecContext(ctx, `
				INSERT INTO items (id, user_id, product_id)
				VALUES (?, ?, ?)`,
				itemID, userID, productID)
			if err != nil {
				t.Fatalf("create item: %v", err)
			}

			// Create instances with provided expiration dates
			instanceIDs := make([]string, len(tc.expirationDates))
			for i, expiresAt := range tc.expirationDates {
				instanceIDs[i] = uuid.NewString()
				_, err := db.ExecContext(ctx, `
					INSERT INTO item_instances (id, item_id, stock_in_at, expires_at)
					VALUES (?, ?, ?, ?)`,
					instanceIDs[i], itemID, now.Add(-24*time.Hour), nullableTime(expiresAt))
				if err != nil {
					t.Fatalf("create instance %d: %v", i, err)
				}
			}

			// Count instances before commit
			var beforeCount int
			err = db.QueryRowContext(ctx, `
				SELECT COUNT(*)
				FROM item_instances
				WHERE item_id = ? AND removed_at IS NULL`,
				itemID,
			).Scan(&beforeCount)
			if err != nil {
				t.Fatalf("count before: %v", err)
			}

			// Create and commit stock-out scan entry (no specific instanceID)
			direction := scan.StockOut
			scanEntry := scan.ScanEntry{
				ID:        uuid.NewString(),
				UserID:    userID,
				Barcode:   uuid.NewString(),
				ScannedAt: now,
				Direction: &direction,
				UnitCount: 1,
				ProductID: &productID,
				Status:    scan.Pending,
			}
			created, err := repo.CreateScanEntry(ctx, scanEntry)
			if err != nil {
				t.Fatalf("CreateScanEntry: %v", err)
			}

			if err := repo.CommitStockOut(ctx, created, nil); err != nil {
				t.Fatalf("CommitStockOut: %v", err)
			}

			// Count instances after commit
			var afterCount int
			err = db.QueryRowContext(ctx, `
				SELECT COUNT(*)
				FROM item_instances
				WHERE item_id = ? AND removed_at IS NULL`,
				itemID,
			).Scan(&afterCount)
			if err != nil {
				t.Fatalf("count after: %v", err)
			}

			// Property: instance count decreased by exactly 1
			decrease := beforeCount - afterCount
			if decrease != 1 {
				t.Errorf("Property violation: expected instance count to decrease by 1, but decreased by %d (before=%d, after=%d)",
					decrease, beforeCount, afterCount)
			}

			// Verify consumption event was created
			var consumptionCount int
			err = db.QueryRowContext(ctx, `
				SELECT COUNT(*)
				FROM consumption_events
				WHERE item_id = ? AND scan_entry_id = ?`,
				itemID, created.ID,
			).Scan(&consumptionCount)
			if err != nil {
				t.Fatalf("count consumption events: %v", err)
			}

			if consumptionCount != 1 {
				t.Errorf("Property violation: expected 1 consumption event, found %d", consumptionCount)
			}

			// For specific test cases, verify the correct instance was removed
			if tc.name != "all NULL expiration dates" { // Skip this check for indeterminate case
				// Get the removed instance ID
				var removedInstanceID string
				err = db.QueryRowContext(ctx, `
					SELECT id FROM item_instances
					WHERE item_id = ? AND removed_at IS NOT NULL AND removal_reason = 'consumed'
					ORDER BY removed_at DESC
					LIMIT 1`,
					itemID,
				).Scan(&removedInstanceID)
				if err != nil {
					t.Fatalf("query removed instance: %v", err)
				}

				expectedRemovedID := instanceIDs[tc.expectedRemovedIdx]
				if removedInstanceID != expectedRemovedID {
					t.Errorf("Property violation: expected instance at index %d (%s) to be removed, but instance %s was removed",
						tc.expectedRemovedIdx, expectedRemovedID, removedInstanceID)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// Helper functions
// --------------------------------------------------------------------------

func ptrTime(t time.Time) *time.Time {
	return &t
}

func nullableTime(t *time.Time) sql.NullTime {
	if t == nil || t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}
