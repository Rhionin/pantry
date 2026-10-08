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
		SELECT id FROM shopping_list_items
		WHERE user_id = ? AND item_id = ? AND source = 'auto'
		ORDER BY created_at ASC, rowid ASC`, userID, itemID)
	if err != nil {
		return fmt.Errorf("could not load shopping list item %q: %w", itemID, err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("could not read shopping list item %q: %w", itemID, err)
		}
		ids = append(ids, id)
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
	if len(ids) == 0 {
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
		    group_id = ?,
		    purchased_at = CASE WHEN quantity = ? AND note = ? THEN purchased_at ELSE NULL END
		WHERE id = ?`,
		quantity, note, storedGroup, quantity, note, ids[0],
	)
	if err != nil {
		return fmt.Errorf("could not refresh shopping list item %q: %w", itemID, err)
	}
	for _, extra := range ids[1:] {
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
		SELECT id, item_id, touched FROM shopping_list_items
		WHERE user_id = ? AND source = 'auto'`, userID)
	if err != nil {
		return fmt.Errorf("could not load derived shopping list items: %w", err)
	}
	defer rows.Close()
	var drop []string
	for rows.Next() {
		var id, itemID string
		var touched int
		if err := rows.Scan(&id, &itemID, &touched); err != nil {
			return fmt.Errorf("could not read derived shopping list item: %w", err)
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
		if _, err := tx.ExecContext(ctx, `DELETE FROM shopping_list_entry_adjustments WHERE entry_id = ?`, id); err != nil {
			return fmt.Errorf("could not clear shopping list item: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM shopping_list_items WHERE id = ?`, id); err != nil {
			return fmt.Errorf("could not clear shopping list item: %w", err)
		}
	}
	return nil
}
