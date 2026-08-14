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

// LookupByBarcode looks up the product associated with a barcode.
//
// Priority order:
//  1. user_override rows matching the given userID
//  2. global rows (user_id = '')
//
// Returns the first matching product, or nil if no match is found.
func (r *Repo) LookupByBarcode(ctx context.Context, barcode, userID string) (*ProductSummary, error) {
	// Find the highest-priority match.
	// We use a CASE expression so user_override rows sort before global rows.
	row := r.db.QueryRowContext(ctx, `
		SELECT p.id, p.name, COALESCE(p.category, ''), COALESCE(p.unit_of_measure, '')
		FROM barcodes b
		JOIN products p ON p.id = b.product_id
		WHERE b.barcode = ?
		  AND (
		        (b.source = 'user_override' AND b.user_id = ?)
		     OR (b.source = 'global'        AND b.user_id = '')
		      )
		ORDER BY CASE b.source WHEN 'user_override' THEN 0 ELSE 1 END ASC, p.name ASC
		LIMIT 1`,
		barcode, userID,
	)

	var summary ProductSummary
	if err := row.Scan(&summary.ID, &summary.Name, &summary.Category, &summary.UnitOfMeasure); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("LookupByBarcode: %w", err)
	}

	return &summary, nil
}

// nullableString converts an empty string to a SQL NULL so that optional text
// columns are stored as NULL rather than empty string.
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
