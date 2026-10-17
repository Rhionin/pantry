package shopping

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PlannedLine is one supply line ready to persist.
// Manual lines are echoed and left untouched. Auto lines are upserted by item.
type PlannedLine struct {
	ProductID string
	ItemID    string
	GroupKey  string
	Quantity  int
	Note      string
	Manual    bool
}

// ShelfMember is one pantry item in a product group.
type ShelfMember struct {
	ItemID   string
	GroupKey string
}

// SavePlannedLines writes one supply snapshot into the staged cart.
// Untouched auto rows are refreshed. A line the owner changed (touched) stays.
// Auto rows the owner removed stay removed. Manual rows are neither deleted nor resized.
func (s *Store) SavePlannedLines(ctx context.Context, userID string, lines []PlannedLine, members []ShelfMember) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin shopping list update: %w", err)
	}
	defer tx.Rollback()

	membersOf := map[string]map[string]struct{}{}
	for _, member := range members {
		if member.GroupKey == "" {
			continue
		}
		if membersOf[member.GroupKey] == nil {
			membersOf[member.GroupKey] = map[string]struct{}{}
		}
		membersOf[member.GroupKey][member.ItemID] = struct{}{}
	}

	touched, touchedGroups, err := listTouchedAutoItems(ctx, tx, userID)
	if err != nil {
		return err
	}
	skipped, skippedGroups, err := listSkippedItems(ctx, tx, userID)
	if err != nil {
		return err
	}
	groupOf := map[string]string{}
	for _, member := range members {
		if member.GroupKey == "" {
			continue
		}
		groupOf[member.ItemID] = member.GroupKey
	}
	for itemID := range touched {
		if key := groupOf[itemID]; key != "" {
			touchedGroups[key] = struct{}{}
		}
	}

	keep := map[string]struct{}{}
	for _, line := range lines {
		if line.Manual || line.Quantity < 1 || line.ItemID == "" {
			continue
		}
		itemID := line.ItemID
		key := line.GroupKey
		if key == "" {
			key = groupOf[itemID]
		}
		if groupSkipped(skipped, skippedGroups, key, itemID, membersOf) {
			continue
		}
		if _, held := touched[itemID]; held {
			continue
		}
		if key != "" {
			if _, held := touchedGroups[key]; held {
				continue
			}
		}
		if err := upsertAutoLine(ctx, tx, userID, itemID, line.GroupKey, line.Quantity, line.Note); err != nil {
			return err
		}
		keep[itemID] = struct{}{}
	}
	if err := deleteUnplannedAuto(ctx, tx, userID, keep); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not save shopping list updates: %w", err)
	}
	return nil
}

func upsertAutoLine(ctx context.Context, tx *sql.Tx, userID, itemID, groupID string, quantity int, note string) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, quantity, note, purchased_at FROM shopping_list_items
		WHERE user_id = ? AND item_id = ? AND source = 'auto'
		ORDER BY created_at ASC, rowid ASC`, userID, itemID)
	if err != nil {
		return fmt.Errorf("could not load shopping list item %q: %w", itemID, err)
	}
	var openID string
	var extras []string
	alreadyBought := false
	for rows.Next() {
		var id, rowNote string
		var rowQty int
		var purchasedAt sql.NullTime
		if err := rows.Scan(&id, &rowQty, &rowNote, &purchasedAt); err != nil {
			rows.Close()
			return fmt.Errorf("could not read shopping list item %q: %w", itemID, err)
		}
		if purchasedAt.Valid {
			// A purchase counts only until the product is stocked in after it.
			// A later identical line is a new need once that stock-in exists.
			stocked, err := stockedAfterPurchase(ctx, tx, itemID, purchasedAt.Time)
			if err != nil {
				rows.Close()
				return err
			}
			if stocked {
				continue
			}
			if rowQty == quantity && rowNote == note {
				alreadyBought = true
			}
			continue
		}
		if openID == "" {
			openID = id
			continue
		}
		extras = append(extras, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("could not read shopping list item %q: %w", itemID, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("could not read shopping list item %q: %w", itemID, err)
	}

	var storedGroup any
	if groupID != "" {
		storedGroup = groupID
	}
	if openID == "" {
		// A checked-off row for this exact line stays checked off. A different
		// plan is a new unpurchased line, so the purchase is not rewritten.
		if alreadyBought {
			return nil
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO shopping_list_items (id, user_id, item_id, quantity, source, note, created_at, group_id)
			VALUES (?, ?, ?, ?, 'auto', ?, ?, ?)`,
			uuid.NewString(), userID, itemID, quantity, note, time.Now().UTC(), storedGroup,
		)
		if err != nil {
			return fmt.Errorf("could not add shopping list item %q: %w", itemID, err)
		}
		return nil
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE shopping_list_items
		SET quantity = ?,
		    note = ?,
		    group_id = ?
		WHERE id = ?`,
		quantity, note, storedGroup, openID,
	)
	if err != nil {
		return fmt.Errorf("could not refresh shopping list item %q: %w", itemID, err)
	}
	for _, extra := range extras {
		if _, err := tx.ExecContext(ctx, `DELETE FROM shopping_list_items WHERE id = ?`, extra); err != nil {
			return fmt.Errorf("could not refresh shopping list item %q: %w", itemID, err)
		}
	}
	return nil
}

func listTouchedAutoItems(ctx context.Context, tx *sql.Tx, userID string) (map[string]struct{}, map[string]struct{}, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT item_id, COALESCE(group_id, '') FROM shopping_list_items
		WHERE user_id = ? AND source = 'auto' AND touched = 1 AND purchased_at IS NULL`,
		userID)
	if err != nil {
		return nil, nil, fmt.Errorf("could not load shopping list edits: %w", err)
	}
	defer rows.Close()
	touched := map[string]struct{}{}
	groups := map[string]struct{}{}
	for rows.Next() {
		var itemID, groupID string
		if err := rows.Scan(&itemID, &groupID); err != nil {
			return nil, nil, fmt.Errorf("could not read shopping list edit: %w", err)
		}
		touched[itemID] = struct{}{}
		if groupID != "" {
			groups[groupID] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("could not read shopping list edits: %w", err)
	}
	return touched, groups, nil
}

func listSkippedItems(ctx context.Context, tx *sql.Tx, userID string) (map[string]struct{}, map[string]struct{}, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT item_id, COALESCE(group_id, '') FROM staged_cart_skips WHERE user_id = ?`, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("could not load removed shopping list items: %w", err)
	}
	defer rows.Close()
	skipped := map[string]struct{}{}
	groups := map[string]struct{}{}
	for rows.Next() {
		var itemID, groupID string
		if err := rows.Scan(&itemID, &groupID); err != nil {
			return nil, nil, fmt.Errorf("could not read removed shopping list item: %w", err)
		}
		skipped[itemID] = struct{}{}
		if groupID != "" {
			groups[groupID] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("could not read removed shopping list items: %w", err)
	}
	return skipped, groups, nil
}

func groupSkipped(skipped, skippedGroups map[string]struct{}, key, itemID string, membersOf map[string]map[string]struct{}) bool {
	if _, ok := skipped[itemID]; ok {
		return true
	}
	if key == "" {
		return false
	}
	if _, ok := skippedGroups[key]; ok {
		return true
	}
	for member := range membersOf[key] {
		if _, ok := skipped[member]; ok {
			return true
		}
	}
	return false
}

func deleteUnplannedAuto(ctx context.Context, tx *sql.Tx, userID string, keep map[string]struct{}) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, item_id, touched, purchased_at FROM shopping_list_items
		WHERE user_id = ? AND source = 'auto'`, userID)
	if err != nil {
		return fmt.Errorf("could not load derived shopping list items: %w", err)
	}
	defer rows.Close()
	var drop []string
	for rows.Next() {
		var id, itemID string
		var touched int
		var purchasedAt sql.NullTime
		if err := rows.Scan(&id, &itemID, &touched, &purchasedAt); err != nil {
			return fmt.Errorf("could not read derived shopping list item: %w", err)
		}
		if purchasedAt.Valid {
			// Target changes and undo keep a purchase that is still waiting
			// to be stocked in. Once a stock-in follows it, master deletes
			// the row the same way it deletes any other unplanned auto line.
			stocked, err := stockedAfterPurchase(ctx, tx, itemID, purchasedAt.Time)
			if err != nil {
				return err
			}
			if stocked {
				drop = append(drop, id)
			}
			continue
		}
		if touched != 0 {
			continue
		}
		if _, ok := keep[itemID]; ok {
			continue
		}
		drop = append(drop, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("could not read derived shopping list items: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("could not read derived shopping list items: %w", err)
	}
	for _, id := range drop {
		if err := deleteShoppingRow(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

// stockedAfterPurchase reports whether this product was stocked in after the
// purchase. Times are compared in Go: sqlite stores each instant as a
// zone-specific string, and those strings do not sort in time order.
func stockedAfterPurchase(ctx context.Context, tx *sql.Tx, itemID string, purchasedAt time.Time) (bool, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT at FROM stock_in_events
		WHERE product_id = (SELECT product_id FROM items WHERE id = ?)`, itemID)
	if err != nil {
		return false, fmt.Errorf("could not read stock-in events: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var at time.Time
		if err := rows.Scan(&at); err != nil {
			return false, fmt.Errorf("could not read stock-in events: %w", err)
		}
		if at.After(purchasedAt) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("could not read stock-in events: %w", err)
	}
	return false, nil
}

// EndTrip drops checked-off auto rows. Starting a new cart ends the
// protection those rows had during a target change, so the next list can
// plan the item again.
func (s *Store) EndTrip(ctx context.Context, userID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start a new cart: %w", err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
		SELECT id FROM shopping_list_items
		WHERE user_id = ? AND source = 'auto' AND purchased_at IS NOT NULL`, userID)
	if err != nil {
		return fmt.Errorf("could not start a new cart: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("could not start a new cart: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("could not start a new cart: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("could not start a new cart: %w", err)
	}
	for _, id := range ids {
		if err := deleteShoppingRow(ctx, tx, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not start a new cart: %w", err)
	}
	return nil
}

func deleteShoppingRow(ctx context.Context, tx *sql.Tx, id string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM shopping_list_entry_adjustments WHERE entry_id = ?`, id); err != nil {
		return fmt.Errorf("could not clear shopping list item: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM shopping_list_items WHERE id = ?`, id); err != nil {
		return fmt.Errorf("could not clear shopping list item: %w", err)
	}
	return nil
}
