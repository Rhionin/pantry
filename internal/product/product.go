// Package product provides product and barcode management functionality.
package product

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Product represents a type of pantry item identified by name and optionally
// by one or more barcodes.
type Product struct {
	ID            string
	Name          string
	Category      string
	UnitOfMeasure string
	CreatedAt     time.Time
}

// ProductSummary is a lightweight projection of Product used by API responses
// and barcode-lookup results.
type ProductSummary struct {
	ID            string
	Name          string
	Category      string
	UnitOfMeasure string
}

// Repo provides database operations for products and barcodes.
type Repo struct {
	db *sql.DB
}

// NewRepo creates a new Repo with the given database connection.
func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// CreateProduct inserts a new product row. If product.ID is empty a new UUID
// is generated. The caller should set product.Name; Category and
// UnitOfMeasure are optional.
func (r *Repo) CreateProduct(ctx context.Context, product Product) error {
	if product.ID == "" {
		product.ID = uuid.NewString()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO products (id, name, category, unit_of_measure) VALUES (?, ?, ?, ?)`,
		product.ID, product.Name, nullableString(product.Category), nullableString(product.UnitOfMeasure),
	)
	if err != nil {
		return fmt.Errorf("CreateProduct: %w", err)
	}
	return nil
}

// GetProductByID returns the product with the given ID, or nil if no such row
// exists.
func (r *Repo) GetProductByID(ctx context.Context, id string) (*Product, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, COALESCE(category, ''), COALESCE(unit_of_measure, ''), created_at
		 FROM products WHERE id = ?`, id)

	var p Product
	if err := row.Scan(&p.ID, &p.Name, &p.Category, &p.UnitOfMeasure, &p.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetProductByID: %w", err)
	}
	return &p, nil
}

// ListProducts returns all products, ordered by name.
func (r *Repo) ListProducts(ctx context.Context) ([]Product, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, COALESCE(category, ''), COALESCE(unit_of_measure, ''), created_at
		 FROM products ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("ListProducts: %w", err)
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Category, &p.UnitOfMeasure, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("ListProducts scan: %w", err)
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListProducts rows: %w", err)
	}
	return products, nil
}

// UpdateProduct updates the name, category, and unit_of_measure of an existing
// product identified by product.ID. It does not change created_at.
func (r *Repo) UpdateProduct(ctx context.Context, product Product) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE products SET name = ?, category = ?, unit_of_measure = ? WHERE id = ?`,
		product.Name, nullableString(product.Category), nullableString(product.UnitOfMeasure), product.ID,
	)
	if err != nil {
		return fmt.Errorf("UpdateProduct: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("UpdateProduct rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("UpdateProduct: product %q not found", product.ID)
	}
	return nil
}

// UpsertBarcodeMapping inserts or replaces a row in the barcodes table.
// source must be either "global" or "user_override".
// For global entries pass userID = "".
func (r *Repo) UpsertBarcodeMapping(ctx context.Context, barcode, productID, source, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO barcodes (barcode, product_id, source, user_id)
		 VALUES (?, ?, ?, ?)`,
		barcode, productID, source, userID,
	)
	if err != nil {
		return fmt.Errorf("UpsertBarcodeMapping: %w", err)
	}
	return nil
}

// LookupByBarcode looks up the product(s) associated with a barcode.
//
// Priority order:
//  1. user_override rows matching the given userID
//  2. global rows (user_id = '')
//
// Return values:
//   - (product, nil, nil)        — exactly one product found
//   - (nil, []ProductSummary, nil) — multiple products found (disambiguation needed)
//   - (nil, nil, nil)             — no product found
//   - (nil, nil, err)             — database error
func (r *Repo) LookupByBarcode(ctx context.Context, barcode, userID string) (*ProductSummary, []ProductSummary, error) {
	// Collect all matching products, annotated with their source priority.
	// We use a CASE expression so user_override rows sort before global rows.
	rows, err := r.db.QueryContext(ctx, `
		SELECT p.id, p.name, COALESCE(p.category, ''), COALESCE(p.unit_of_measure, ''),
		       CASE b.source WHEN 'user_override' THEN 0 ELSE 1 END AS priority
		FROM barcodes b
		JOIN products p ON p.id = b.product_id
		WHERE b.barcode = ?
		  AND (
		        (b.source = 'user_override' AND b.user_id = ?)
		     OR (b.source = 'global'        AND b.user_id = '')
		      )
		ORDER BY priority ASC, p.name ASC`,
		barcode, userID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("LookupByBarcode query: %w", err)
	}
	defer rows.Close()

	// Collect all matches, but stop collecting additional products once we
	// have found at least one user_override match — user overrides take full
	// precedence, so if every row at priority=0 maps to the same product, that
	// is the unambiguous answer.
	type row struct {
		summary  ProductSummary
		priority int
	}
	var found []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.summary.ID, &r.summary.Name, &r.summary.Category, &r.summary.UnitOfMeasure, &r.priority); err != nil {
			return nil, nil, fmt.Errorf("LookupByBarcode scan: %w", err)
		}
		found = append(found, r)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("LookupByBarcode rows: %w", err)
	}

	if len(found) == 0 {
		return nil, nil, nil
	}

	// Check whether we have any user_override hits (priority=0).
	// If so, only consider those rows for the result.
	hasOverride := found[0].priority == 0
	var candidates []ProductSummary
	for _, r := range found {
		if hasOverride && r.priority != 0 {
			break
		}
		candidates = append(candidates, r.summary)
	}

	// Deduplicate by product ID (the same product can appear under both a
	// global and an override row).
	seen := make(map[string]struct{}, len(candidates))
	unique := candidates[:0]
	for _, c := range candidates {
		if _, ok := seen[c.ID]; !ok {
			seen[c.ID] = struct{}{}
			unique = append(unique, c)
		}
	}

	if len(unique) == 1 {
		p := unique[0]
		return &p, nil, nil
	}
	return nil, unique, nil
}

// nullableString converts an empty string to a SQL NULL so that optional text
// columns are stored as NULL rather than empty string.
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
