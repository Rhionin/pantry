package history

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/google/uuid"
)

// moveRow is one stored stock move.
type moveRow struct {
	ID          string
	ItemID      string
	ProductID   string
	Direction   string
	Quantity    int
	At          time.Time
	Source      string
	ScanEntryID sql.NullString
	Tracked     bool
}

// ensure groups older shelf rows that were written before moves existed.
// The history_repair row is updated first so two readers cannot both insert.
func ensure(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `UPDATE history_repair SET id = 1 WHERE id = 1`); err != nil {
		return err
	}
	if err := ensureStockIns(ctx, tx); err != nil {
		return err
	}
	if err := ensureStockOuts(ctx, tx); err != nil {
		return err
	}
	return ensureRemoved(ctx, tx)
}

type instanceRow struct {
	ID        string
	ItemID    string
	ProductID string
	UserID    string
	At        time.Time
	Removed   sql.NullTime
	Reason    string
}

func ensureStockIns(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT ii.id, ii.item_id, i.product_id, i.user_id, ii.stock_in_at
		FROM item_instances ii
		JOIN items i ON i.id = ii.item_id
		WHERE ii.stock_move_id IS NULL
		ORDER BY ii.item_id, ii.stock_in_at, ii.id`)
	if err != nil {
		return err
	}
	var loaded []instanceRow
	for rows.Next() {
		var row instanceRow
		if err := rows.Scan(&row.ID, &row.ItemID, &row.ProductID, &row.UserID, &row.At); err != nil {
			rows.Close()
			return err
		}
		loaded = append(loaded, row)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}

	var current []instanceRow
	flush := func() error {
		if len(current) == 0 {
			return nil
		}
		head := current[0]
		ids := make([]string, len(current))
		for i, row := range current {
			ids[i] = row.ID
		}
		scanID, source := committedScan(ctx, tx, head.UserID, head.ProductID, "stock_in", head.At)
		tracked, err := stockInTracked(ctx, tx, head.ProductID, head.At)
		if err != nil {
			return err
		}
		current = nil
		return AttachStockIn(ctx, tx, InNote{
			ItemID: head.ItemID, ProductID: head.ProductID, At: head.At,
			Source: source, ScanEntryID: scanID, Tracked: tracked, InstanceIDs: ids,
		})
	}
	for _, row := range loaded {
		if len(current) > 0 {
			prev := current[0]
			if prev.ItemID != row.ItemID || !prev.At.Equal(row.At) {
				if err := flush(); err != nil {
					return err
				}
			}
		}
		current = append(current, row)
	}
	return flush()
}

func ensureStockOuts(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT c.id, c.item_id, i.product_id, i.user_id, c.consumed_at, c.scan_entry_id
		FROM consumption_events c
		JOIN items i ON i.id = c.item_id
		WHERE c.stock_move_id IS NULL
		ORDER BY c.item_id, c.consumed_at, c.scan_entry_id, c.id`)
	if err != nil {
		return err
	}
	type useRow struct {
		ID      string
		ItemID  string
		Product string
		UserID  string
		At      time.Time
		ScanID  sql.NullString
	}
	var loaded []useRow
	for rows.Next() {
		var row useRow
		if err := rows.Scan(&row.ID, &row.ItemID, &row.Product, &row.UserID, &row.At, &row.ScanID); err != nil {
			rows.Close()
			return err
		}
		loaded = append(loaded, row)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}

	var current []useRow
	flush := func() error {
		if len(current) == 0 {
			return nil
		}
		head := current[0]
		id := uuid.NewString()
		source := "manual"
		var scan any
		if head.ScanID.Valid && head.ScanID.String != "" {
			source = "scan"
			scan = head.ScanID.String
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO stock_moves (id, item_id, product_id, direction, quantity, at, source, scan_entry_id, tracked)
			VALUES (?, ?, ?, 'out', ?, ?, ?, ?, 1)`,
			id, head.ItemID, head.Product, len(current), head.At, source, scan,
		); err != nil {
			return err
		}
		for _, row := range current {
			if _, err := tx.ExecContext(ctx, `
				UPDATE consumption_events SET stock_move_id = ? WHERE id = ?`, id, row.ID); err != nil {
				return err
			}
		}
		if err := linkRemoved(ctx, tx, head.ItemID, id, head.At, len(current)); err != nil {
			return err
		}
		current = nil
		return nil
	}
	for _, row := range loaded {
		if len(current) > 0 {
			prev := current[0]
			sameScan := prev.ScanID.String == row.ScanID.String && prev.ScanID.Valid == row.ScanID.Valid
			if prev.ItemID != row.ItemID || !prev.At.Equal(row.At) || !sameScan {
				if err := flush(); err != nil {
					return err
				}
			}
		}
		current = append(current, row)
	}
	return flush()
}

func linkRemoved(ctx context.Context, tx *sql.Tx, itemID, moveID string, at time.Time, quantity int) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, removed_at FROM item_instances
		WHERE item_id = ? AND removed_at IS NOT NULL AND removed_by_move_id IS NULL
			AND (removal_reason IS NULL OR removal_reason = 'consumed')`,
		itemID)
	if err != nil {
		return err
	}
	type candidate struct {
		id string
		at time.Time
	}
	var candidates []candidate
	for rows.Next() {
		var id string
		var removed time.Time
		if err := rows.Scan(&id, &removed); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, candidate{id: id, at: removed})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	sort.Slice(candidates, func(i, j int) bool {
		left := candidates[i].at.Sub(at)
		if left < 0 {
			left = -left
		}
		right := candidates[j].at.Sub(at)
		if right < 0 {
			right = -right
		}
		if left == right {
			return candidates[i].id < candidates[j].id
		}
		return left < right
	})
	n := quantity
	if n > len(candidates) {
		n = len(candidates)
	}
	for i := 0; i < n; i++ {
		if _, err := tx.ExecContext(ctx, `
			UPDATE item_instances SET removed_by_move_id = ? WHERE id = ?`,
			moveID, candidates[i].id,
		); err != nil {
			return err
		}
	}
	return nil
}

func ensureRemoved(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT ii.id, ii.item_id, i.product_id, i.user_id, ii.removed_at
		FROM item_instances ii
		JOIN items i ON i.id = ii.item_id
		WHERE ii.removed_at IS NOT NULL AND ii.removed_by_move_id IS NULL
		ORDER BY ii.removed_at, ii.id`)
	if err != nil {
		return err
	}
	type removedRow struct {
		id, itemID, productID, userID string
		at                            time.Time
	}
	var loaded []removedRow
	for rows.Next() {
		var row removedRow
		if err := rows.Scan(&row.id, &row.itemID, &row.productID, &row.userID, &row.at); err != nil {
			rows.Close()
			return err
		}
		loaded = append(loaded, row)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, row := range loaded {
		scanID, source := committedScan(ctx, tx, row.userID, row.productID, "stock_out", row.at)
		if err := AttachStockOut(ctx, tx, OutNote{
			ItemID: row.itemID, ProductID: row.productID, At: row.at,
			Source: source, ScanEntryID: scanID, Tracked: false, InstanceID: row.id,
		}); err != nil {
			return err
		}
	}
	return nil
}

func committedScan(ctx context.Context, tx *sql.Tx, userID, productID, direction string, at time.Time) (*string, string) {
	var id string
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM scan_entries
		WHERE user_id = ? AND product_id = ? AND direction = ? AND status = 'committed' AND scanned_at = ?
		LIMIT 1`, userID, productID, direction, at).Scan(&id)
	if err != nil || id == "" {
		return nil, "manual"
	}
	return &id, "scan"
}

func stockInTracked(ctx context.Context, tx *sql.Tx, productID string, at time.Time) (bool, error) {
	var one int
	err := tx.QueryRowContext(ctx, `
		SELECT 1 FROM stock_in_events
		WHERE product_id = ? AND at = ? AND stock_move_id IS NULL
		LIMIT 1`, productID, at).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
