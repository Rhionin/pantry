package scan

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// CommitStockIn commits a stock-in scan entry by creating N item instances
// (where N = scanEntry.UnitCount) and marking the scan entry as committed.
// Each item instance receives the scan entry's ScannedAt as stock_in_at and
// the scan entry's ExpiresAt (if set).
//
// This function handles the complete stock-in flow:
// 1. Ensures an item exists for the user+product combination
// 2. Creates N item instances with the scan entry's timestamps
// 3. Marks the scan entry as committed
//
// All operations are performed within a transaction to ensure atomicity.
func (r *Repo) CommitStockIn(ctx context.Context, scanEntry *ScanEntry) error {
	if scanEntry.Direction == nil || *scanEntry.Direction != StockIn {
		return fmt.Errorf("scan entry direction must be stock_in, got %v", scanEntry.Direction)
	}

	if scanEntry.ProductID == nil || *scanEntry.ProductID == "" {
		return fmt.Errorf("scan entry must have a product_id")
	}

	if scanEntry.UnitCount < 1 {
		return fmt.Errorf("unit count must be at least 1, got %d", scanEntry.UnitCount)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	itemID, err := r.findOrCreateItem(ctx, tx, scanEntry.UserID, *scanEntry.ProductID)
	if err != nil {
		return fmt.Errorf("find/create item: %w", err)
	}

	for i := 0; i < scanEntry.UnitCount; i++ {
		instanceID := uuid.NewString()
		_, err := tx.ExecContext(ctx, `
			INSERT INTO item_instances (id, item_id, stock_in_at, expires_at)
			VALUES (?, ?, ?, ?)`,
			instanceID,
			itemID,
			scanEntry.ScannedAt,
			nullableTime(scanEntry.ExpiresAt),
		)
		if err != nil {
			return fmt.Errorf("create instance %d: %w", i+1, err)
		}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE scan_entries 
		SET status = ?, committed_at = CURRENT_TIMESTAMP 
		WHERE id = ?`,
		string(Committed),
		scanEntry.ID,
	)
	if err != nil {
		return fmt.Errorf("update scan entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// findOrCreateItem retrieves the item ID for a user+product combination,
// creating the item if it doesn't already exist.
func (r *Repo) findOrCreateItem(ctx context.Context, tx *sql.Tx, userID, productID string) (string, error) {
	var itemID string
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM items 
		WHERE user_id = ? AND product_id = ?`,
		userID, productID,
	).Scan(&itemID)

	if err == nil {
		return itemID, nil
	}

	if err != sql.ErrNoRows {
		return "", fmt.Errorf("query item: %w", err)
	}

	itemID = uuid.NewString()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO items (id, user_id, product_id)
		VALUES (?, ?, ?)`,
		itemID, userID, productID,
	)
	if err != nil {
		return "", fmt.Errorf("create item: %w", err)
	}

	return itemID, nil
}

// CommitStockOut commits a stock-out scan entry by removing one item instance
// from the inventory and marking the scan entry as committed.
//
// If instanceID is nil, the function selects the use-oldest-first instance
// (ORDER BY expires_at ASC NULLS LAST) to remove. If instanceID is provided,
// that specific instance is removed.
//
// This function handles the complete stock-out flow:
// 1. Ensures an item exists for the user+product combination
// 2. Selects the instance to remove (specific or use-oldest-first)
// 3. Marks the instance as removed with removal_reason 'consumed'
// 4. Creates a consumption event record
// 5. Marks the scan entry as committed
//
// All operations are performed within a transaction to ensure atomicity.
func (r *Repo) CommitStockOut(ctx context.Context, scanEntry *ScanEntry, instanceID *string) error {
	if scanEntry.Direction == nil || *scanEntry.Direction != StockOut {
		return fmt.Errorf("scan entry direction must be stock_out, got %v", scanEntry.Direction)
	}

	if scanEntry.ProductID == nil || *scanEntry.ProductID == "" {
		return fmt.Errorf("scan entry must have a product_id")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	var itemID string
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM items 
		WHERE user_id = ? AND product_id = ?`,
		scanEntry.UserID, *scanEntry.ProductID,
	).Scan(&itemID)

	if err == sql.ErrNoRows {
		return fmt.Errorf("no item found for user %q and product %q", scanEntry.UserID, *scanEntry.ProductID)
	}
	if err != nil {
		return fmt.Errorf("find item: %w", err)
	}

	var selectedInstanceID string
	if instanceID != nil {
		selectedInstanceID = *instanceID
		var exists bool
		err = tx.QueryRowContext(ctx, `
			SELECT 1 FROM item_instances 
			WHERE id = ? AND item_id = ? AND removed_at IS NULL`,
			selectedInstanceID, itemID,
		).Scan(&exists)
		if err == sql.ErrNoRows {
			return fmt.Errorf("instance %q not found or already removed", selectedInstanceID)
		}
		if err != nil {
			return fmt.Errorf("verify instance: %w", err)
		}
	} else {
		// Use oldest-first: prioritize instances closest to expiration
		err = tx.QueryRowContext(ctx, `
			SELECT id FROM item_instances
			WHERE item_id = ? AND removed_at IS NULL
			ORDER BY expires_at ASC NULLS LAST
			LIMIT 1`,
			itemID,
		).Scan(&selectedInstanceID)
		if err == sql.ErrNoRows {
			return fmt.Errorf("no available instances for item %q", itemID)
		}
		if err != nil {
			return fmt.Errorf("select instance: %w", err)
		}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE item_instances 
		SET removed_at = CURRENT_TIMESTAMP, removal_reason = 'consumed'
		WHERE id = ?`,
		selectedInstanceID,
	)
	if err != nil {
		return fmt.Errorf("remove instance: %w", err)
	}

	consumptionEventID := uuid.NewString()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO consumption_events (id, item_id, consumed_at, scan_entry_id)
		VALUES (?, ?, ?, ?)`,
		consumptionEventID,
		itemID,
		scanEntry.ScannedAt,
		scanEntry.ID,
	)
	if err != nil {
		return fmt.Errorf("create consumption event: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE scan_entries 
		SET status = ?, committed_at = CURRENT_TIMESTAMP 
		WHERE id = ?`,
		string(Committed),
		scanEntry.ID,
	)
	if err != nil {
		return fmt.Errorf("update scan entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// ResolveFlaggedEntry resolves a flagged scan entry by associating it with a product.
// This function creates a user_override barcode mapping and transitions the scan entry
// from flagged to pending status.
//
// This function handles the complete flagged entry resolution flow:
// 1. Validates the scan entry exists and has status 'flagged'
// 2. Creates a product override (barcodes row with source='user_override')
// 3. Sets the product_id on the scan entry
// 4. Transitions the scan entry status from 'flagged' to 'pending'
//
// All operations are performed within a transaction to ensure atomicity.
func (r *Repo) ResolveFlaggedEntry(ctx context.Context, scanEntryID, productID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	var userID, barcode, status string
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, barcode, status FROM scan_entries WHERE id = ?`,
		scanEntryID,
	).Scan(&userID, &barcode, &status)

	if err == sql.ErrNoRows {
		return fmt.Errorf("scan entry %q not found", scanEntryID)
	}
	if err != nil {
		return fmt.Errorf("query scan entry: %w", err)
	}

	if status != string(Flagged) {
		return fmt.Errorf("scan entry %q has status %q, expected %q", scanEntryID, status, Flagged)
	}

	var productExists bool
	err = tx.QueryRowContext(ctx, `
		SELECT 1 FROM products WHERE id = ?`,
		productID,
	).Scan(&productExists)

	if err == sql.ErrNoRows {
		return fmt.Errorf("product %q not found", productID)
	}
	if err != nil {
		return fmt.Errorf("verify product: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO barcodes (barcode, product_id, source, user_id)
		VALUES (?, ?, 'user_override', ?)
		ON CONFLICT(barcode, source, user_id) DO UPDATE SET product_id = excluded.product_id`,
		barcode,
		productID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("create barcode override: %w", err)
	}

	pendingStatus := Pending
	err = r.updateScanEntryInTx(ctx, tx, scanEntryID, nil, nil, nil, &productID, &pendingStatus)
	if err != nil {
		return fmt.Errorf("update scan entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
