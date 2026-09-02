package shopping

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SyncDerivedItems gives derived entries stable IDs and preserves purchase dismissals.
// A purchased gap stays hidden until its quantity changes or the item reaches its target.
func (s *Store) SyncDerivedItems(
	ctx context.Context,
	userID string,
	derived []DerivedEntry,
) ([]ShoppingListItem, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not begin shopping list update: %w", err)
	}
	defer tx.Rollback()

	latestByItemID, storedItemIDs, err := loadLatestAutoItems(ctx, tx, userID)
	if err != nil {
		return nil, err
	}

	derivedByItemID := make(map[string]DerivedEntry, len(derived))
	for _, entry := range derived {
		derivedByItemID[entry.ItemID] = entry
	}
	for itemID := range storedItemIDs {
		if _, stillDerived := derivedByItemID[itemID]; stillDerived {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM shopping_list_items WHERE user_id = ? AND item_id = ? AND source = 'auto'`,
			userID, itemID,
		); err != nil {
			return nil, fmt.Errorf("could not clear fulfilled shopping list item %q: %w", itemID, err)
		}
	}

	active := make([]ShoppingListItem, 0, len(derived))
	for _, entry := range derived {
		latest := latestByItemID[entry.ItemID]
		switch {
		case latest == nil:
			item, err := insertAutoItem(ctx, tx, userID, entry)
			if err != nil {
				return nil, err
			}
			active = append(active, item)
		case latest.PurchasedAt == nil:
			if _, err := tx.ExecContext(ctx,
				`UPDATE shopping_list_items SET quantity = ? WHERE id = ?`,
				entry.Quantity, latest.ID,
			); err != nil {
				return nil, fmt.Errorf("could not refresh shopping list item %q: %w", entry.ItemID, err)
			}
			latest.Quantity = entry.Quantity
			active = append(active, *latest)
		case latest.Quantity != entry.Quantity:
			item, err := insertAutoItem(ctx, tx, userID, entry)
			if err != nil {
				return nil, err
			}
			active = append(active, item)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not save shopping list updates: %w", err)
	}
	return active, nil
}

func loadLatestAutoItems(
	ctx context.Context,
	tx *sql.Tx,
	userID string,
) (map[string]*ShoppingListItem, map[string]struct{}, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, user_id, item_id, quantity, source, purchased_at, created_at
		 FROM shopping_list_items
		 WHERE user_id = ? AND source = 'auto'
		 ORDER BY created_at DESC, rowid DESC`,
		userID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("could not load derived shopping list items: %w", err)
	}

	latestByItemID := make(map[string]*ShoppingListItem)
	storedItemIDs := make(map[string]struct{})
	for rows.Next() {
		item, scanErr := scanShoppingListItem(rows)
		if scanErr != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("could not read derived shopping list item: %w", scanErr)
		}
		storedItemIDs[item.ItemID] = struct{}{}
		if _, exists := latestByItemID[item.ItemID]; !exists {
			latestByItemID[item.ItemID] = item
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, fmt.Errorf("could not iterate derived shopping list items: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, fmt.Errorf("could not close derived shopping list items: %w", err)
	}
	return latestByItemID, storedItemIDs, nil
}

func insertAutoItem(
	ctx context.Context,
	tx *sql.Tx,
	userID string,
	entry DerivedEntry,
) (ShoppingListItem, error) {
	item := ShoppingListItem{
		ID:        uuid.NewString(),
		UserID:    userID,
		ItemID:    entry.ItemID,
		Quantity:  entry.Quantity,
		Source:    "auto",
		CreatedAt: time.Now().UTC(),
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO shopping_list_items (id, user_id, item_id, quantity, source, created_at)
		 VALUES (?, ?, ?, ?, 'auto', ?)`,
		item.ID, item.UserID, item.ItemID, item.Quantity, item.CreatedAt,
	)
	if err != nil {
		return ShoppingListItem{}, fmt.Errorf("could not add derived shopping list item %q: %w", entry.ItemID, err)
	}
	return item, nil
}
