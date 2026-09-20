// Package shopping provides shopping list management functionality.
package shopping

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrItemNotFound is returned when a shopping list item does not exist.
var ErrItemNotFound = errors.New("shopping list item not found")

// ShoppingListItem represents a single entry on a user's shopping list.
type ShoppingListItem struct {
	ID          string
	UserID      string
	ItemID      string
	Quantity    int
	Source      string // "manual" or "auto"
	PurchasedAt *time.Time
	CreatedAt   time.Time
}

// Store provides data access for shopping list items.
type Store struct {
	db *sql.DB
}

// NewStore creates a new Store with the given database connection.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// AddManualItem inserts a new manual shopping list item for the given user and item.
func (s *Store) AddManualItem(ctx context.Context, userID, itemID string, quantity int) (*ShoppingListItem, error) {
	id := uuid.NewString()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO shopping_list_items (id, user_id, item_id, quantity, source)
		 VALUES (?, ?, ?, ?, 'manual')`,
		id, userID, itemID, quantity,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to add shopping list item: %w", err)
	}
	return s.getByID(ctx, id)
}

// RemoveItem deletes the shopping list item with the given ID.
// Returns ErrItemNotFound if no such item exists.
func (s *Store) RemoveItem(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM shopping_list_items WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to remove shopping list item: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if n == 0 {
		return ErrItemNotFound
	}
	return nil
}

// MarkPurchased sets the purchased_at timestamp on the item to the current time.
// Returns ErrItemNotFound if no such item exists.
// Does not modify any item_instances rows.
func (s *Store) MarkPurchased(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE shopping_list_items SET purchased_at = ? WHERE id = ?`,
		time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("failed to mark shopping list item as purchased: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if n == 0 {
		return ErrItemNotFound
	}
	return nil
}

// ListManualItems returns all unpurchased manual shopping list items for the given user,
// ordered by created_at ascending.
func (s *Store) ListManualItems(ctx context.Context, userID string) ([]ShoppingListItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, item_id, quantity, source, purchased_at, created_at
		 FROM shopping_list_items
		 WHERE user_id = ? AND source = 'manual' AND purchased_at IS NULL
		 ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list shopping list items: %w", err)
	}
	defer rows.Close()

	var items []ShoppingListItem
	for rows.Next() {
		item, err := scanShoppingListItem(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to read shopping list item: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate shopping list items: %w", err)
	}
	return items, nil
}

// GetItemByID retrieves a single shopping list item by its ID.
// Returns nil if no such item exists.
func (s *Store) GetItemByID(ctx context.Context, id string) (*ShoppingListItem, error) {
	return s.getByID(ctx, id)
}

// getByID retrieves a single shopping list item by its ID.
func (s *Store) getByID(ctx context.Context, id string) (*ShoppingListItem, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, item_id, quantity, source, purchased_at, created_at
		 FROM shopping_list_items WHERE id = ?`,
		id,
	)
	item, err := scanShoppingListItem(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get shopping list item: %w", err)
	}
	return item, nil
}

// scanner is a common interface for *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

// scanShoppingListItem scans a row into a ShoppingListItem.
func scanShoppingListItem(row scanner) (*ShoppingListItem, error) {
	var item ShoppingListItem
	var purchasedAt sql.NullTime

	err := row.Scan(
		&item.ID,
		&item.UserID,
		&item.ItemID,
		&item.Quantity,
		&item.Source,
		&purchasedAt,
		&item.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if purchasedAt.Valid {
		item.PurchasedAt = &purchasedAt.Time
	}

	return &item, nil
}

// Adjustment holds a shopping list entry adjustment for a provider.
type Adjustment struct {
	EntryID    string
	ProviderID string
	Quantity   int // 0-999
	CreatedAt  time.Time
}

// SetAdjustment sets the adjustment quantity for one entry and provider.
// Validates quantity is in range 0-999.
func (s *Store) SetAdjustment(ctx context.Context, entryID, providerID string, quantity int) error {
	if quantity < 0 || quantity > 999 {
		return fmt.Errorf("adjustment quantity must be 0-999, got %d", quantity)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO shopping_list_entry_adjustments (entry_id, provider_id, quantity)
		 VALUES (?, ?, ?)
		 ON CONFLICT(entry_id, provider_id) DO UPDATE SET quantity = ?`,
		entryID, providerID, quantity, quantity)
	return err
}

// GetAdjustment returns the adjustment for one entry and provider, or (nil, nil) if not found.
func (s *Store) GetAdjustment(ctx context.Context, entryID, providerID string) (*Adjustment, error) {
	var adj Adjustment
	err := s.db.QueryRowContext(ctx,
		`SELECT entry_id, provider_id, quantity, updated_at
		 FROM shopping_list_entry_adjustments
		 WHERE entry_id = ? AND provider_id = ?`,
		entryID, providerID).Scan(&adj.EntryID, &adj.ProviderID, &adj.Quantity, &adj.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get adjustment: %w", err)
	}
	return &adj, nil
}

// ClearAdjustmentTx clears the adjustment for one entry and provider inside an existing transaction.
// This must be called in the same transaction as ledger reset.
func (s *Store) ClearAdjustmentTx(ctx context.Context, tx *sql.Tx, entryID, providerID string) error {
	_, err := tx.ExecContext(ctx,
		`DELETE FROM shopping_list_entry_adjustments WHERE entry_id = ? AND provider_id = ?`,
		entryID, providerID)
	return err
}

// RemoveAdjustment removes the adjustment for one entry and provider.
func (s *Store) RemoveAdjustment(ctx context.Context, entryID, providerID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM shopping_list_entry_adjustments WHERE entry_id = ? AND provider_id = ?`,
		entryID, providerID)
	return err
}
