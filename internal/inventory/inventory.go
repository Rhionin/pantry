// Package inventory provides inventory management functionality for items and item instances.
package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/google/uuid"
)

var (
	// ErrInstanceNotFound is returned when an item instance does not exist.
	ErrInstanceNotFound = errors.New("item instance not found")
)

// Item represents a user's specific instance of interest in a product,
// with an optional target quantity.
type Item struct {
	ID             string
	UserID         string
	ProductID      string
	Product        *product.ProductSummary // populated by join queries
	TargetQuantity *int
	CreatedAt      time.Time
}

// ItemInstance represents a single physical unit tracked in the pantry.
type ItemInstance struct {
	ID            string
	ItemID        string
	StockInAt     time.Time
	ExpiresAt     *time.Time
	RemovedAt     *time.Time
	RemovalReason *string
	CreatedAt     time.Time
}

// Repo provides database operations for items and item instances.
type Repo struct {
	db *sql.DB
}

// NewRepo creates a new Repo with the given database connection.
func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// GetOrCreateItem returns the item for the given userID and productID.
// If no such item exists, it creates one and returns it.
func (r *Repo) GetOrCreateItem(ctx context.Context, userID, productID string) (*Item, error) {
	// First try to get existing item
	item, err := r.getItemByUserAndProduct(ctx, userID, productID)
	if err != nil {
		return nil, fmt.Errorf("GetOrCreateItem get: %w", err)
	}
	if item != nil {
		return item, nil
	}

	// Create new item
	itemID := uuid.NewString()
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO items (id, user_id, product_id) VALUES (?, ?, ?)`,
		itemID, userID, productID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetOrCreateItem insert: %w", err)
	}

	// Return the newly created item
	return r.getItemByID(ctx, itemID)
}

// getItemByUserAndProduct is a helper that returns the item for the given userID and productID.
func (r *Repo) getItemByUserAndProduct(ctx context.Context, userID, productID string) (*Item, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT 
			i.id, i.user_id, i.product_id, i.target_quantity, i.created_at,
			p.id, p.name, COALESCE(p.category, ''), COALESCE(p.unit_of_measure, '')
		FROM items i
		JOIN products p ON p.id = i.product_id
		WHERE i.user_id = ? AND i.product_id = ?`,
		userID, productID,
	)

	item, err := scanItem(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getItemByUserAndProduct: %w", err)
	}
	return item, nil
}

// getItemByID is a helper that returns the item with the given ID.
func (r *Repo) getItemByID(ctx context.Context, itemID string) (*Item, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT 
			i.id, i.user_id, i.product_id, i.target_quantity, i.created_at,
			p.id, p.name, COALESCE(p.category, ''), COALESCE(p.unit_of_measure, '')
		FROM items i
		JOIN products p ON p.id = i.product_id
		WHERE i.id = ?`,
		itemID,
	)

	item, err := scanItem(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getItemByID: %w", err)
	}
	return item, nil
}

// ListItems returns all items for the given userID, ordered by product name.
func (r *Repo) ListItems(ctx context.Context, userID string) ([]Item, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT 
			i.id, i.user_id, i.product_id, i.target_quantity, i.created_at,
			p.id, p.name, COALESCE(p.category, ''), COALESCE(p.unit_of_measure, '')
		FROM items i
		JOIN products p ON p.id = i.product_id
		WHERE i.user_id = ?
		ORDER BY p.name`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("ListItems: %w", err)
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("ListItems scan: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListItems rows: %w", err)
	}
	return items, nil
}

// ListItemInstances returns all item instances for the given itemID,
// ordered by expiration date ascending (use-oldest-first).
func (r *Repo) ListItemInstances(ctx context.Context, itemID string) ([]ItemInstance, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, item_id, stock_in_at, expires_at, removed_at, removal_reason, created_at
		FROM item_instances
		WHERE item_id = ? AND removed_at IS NULL
		ORDER BY expires_at ASC NULLS LAST`,
		itemID,
	)
	if err != nil {
		return nil, fmt.Errorf("ListItemInstances: %w", err)
	}
	defer rows.Close()

	var instances []ItemInstance
	for rows.Next() {
		instance, err := scanItemInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("ListItemInstances scan: %w", err)
		}
		instances = append(instances, *instance)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListItemInstances rows: %w", err)
	}
	return instances, nil
}

// AddInstance adds a new item instance to the inventory.
// If instance.ID is empty, a new UUID is generated.
func (r *Repo) AddInstance(ctx context.Context, instance ItemInstance) (*ItemInstance, error) {
	if instance.ID == "" {
		instance.ID = uuid.NewString()
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO item_instances (id, item_id, stock_in_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		instance.ID,
		instance.ItemID,
		instance.StockInAt,
		nullableTime(instance.ExpiresAt),
	)
	if err != nil {
		return nil, fmt.Errorf("AddInstance: %w", err)
	}

	return r.GetInstance(ctx, instance.ID)
}

// RemoveInstance marks an item instance as removed by setting removed_at
// and removal_reason. Returns ErrInstanceNotFound if the instance does not exist
// or is already removed.
func (r *Repo) RemoveInstance(ctx context.Context, instanceID string, reason string) error {
	// First check if the instance exists and is not already removed
	existing, err := r.GetInstance(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("RemoveInstance: %w", err)
	}
	if existing == nil || existing.RemovedAt != nil {
		return ErrInstanceNotFound
	}

	now := time.Now()
	res, err := r.db.ExecContext(ctx,
		`UPDATE item_instances SET removed_at = ?, removal_reason = ? WHERE id = ? AND removed_at IS NULL`,
		now, reason, instanceID,
	)
	if err != nil {
		return fmt.Errorf("RemoveInstance: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("RemoveInstance rows affected: %w", err)
	}
	if n == 0 {
		return ErrInstanceNotFound
	}

	return nil
}

// UpdateTargetQuantity sets the target_quantity for the given item.
// Returns ErrInstanceNotFound if no item with that ID exists.
func (r *Repo) UpdateTargetQuantity(ctx context.Context, itemID string, qty int) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE items SET target_quantity = ? WHERE id = ?`,
		qty, itemID,
	)
	if err != nil {
		return fmt.Errorf("UpdateTargetQuantity: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("UpdateTargetQuantity rows affected: %w", err)
	}
	if n == 0 {
		return ErrInstanceNotFound
	}
	return nil
}

// GetItem returns the item with the given ID, or nil if no such item exists.
func (r *Repo) GetItem(ctx context.Context, itemID string) (*Item, error) {
	return r.getItemByID(ctx, itemID)
}

// GetInstance returns the item instance with the given ID, or nil if no such
// instance exists.
func (r *Repo) GetInstance(ctx context.Context, instanceID string) (*ItemInstance, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, item_id, stock_in_at, expires_at, removed_at, removal_reason, created_at
		 FROM item_instances WHERE id = ?`,
		instanceID,
	)

	instance, err := scanItemInstance(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetInstance: %w", err)
	}
	return instance, nil
}

// scanner interface for scanItem and scanItemInstance helpers
type scanner interface {
	Scan(dest ...interface{}) error
}

// scanItem is a helper that scans a row into an Item struct.
func scanItem(row scanner) (*Item, error) {
	var item Item
	var targetQuantity sql.NullInt64
	var productID, productName, productCategory, productUnitOfMeasure string

	err := row.Scan(
		&item.ID,
		&item.UserID,
		&item.ProductID,
		&targetQuantity,
		&item.CreatedAt,
		&productID,
		&productName,
		&productCategory,
		&productUnitOfMeasure,
	)
	if err != nil {
		return nil, err
	}

	if targetQuantity.Valid {
		tq := int(targetQuantity.Int64)
		item.TargetQuantity = &tq
	}

	item.Product = &product.ProductSummary{
		ID:            productID,
		Name:          productName,
		Category:      productCategory,
		UnitOfMeasure: productUnitOfMeasure,
	}

	return &item, nil
}

// scanItemInstance is a helper that scans a row into an ItemInstance struct.
func scanItemInstance(row scanner) (*ItemInstance, error) {
	var instance ItemInstance
	var expiresAt, removedAt sql.NullTime
	var removalReason sql.NullString

	err := row.Scan(
		&instance.ID,
		&instance.ItemID,
		&instance.StockInAt,
		&expiresAt,
		&removedAt,
		&removalReason,
		&instance.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if expiresAt.Valid {
		instance.ExpiresAt = &expiresAt.Time
	}
	if removedAt.Valid {
		instance.RemovedAt = &removedAt.Time
	}
	if removalReason.Valid {
		instance.RemovalReason = &removalReason.String
	}

	return &instance, nil
}

// nullableTime converts a *time.Time to sql.NullTime for nullable DATETIME columns.
func nullableTime(t *time.Time) sql.NullTime {
	if t == nil || t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}
