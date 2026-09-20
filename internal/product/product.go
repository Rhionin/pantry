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
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Category       string         `json:"category"`
	UnitOfMeasure  string         `json:"unitOfMeasure"`
	ImageURL       string         `json:"imageUrl,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
	Source         string         `json:"source,omitempty"`
	ExternalSource ExternalSource `json:"externalSource,omitempty"`
	RefreshedAt    *time.Time     `json:"refreshedAt,omitempty"`
	NameOverridden bool           `json:"nameOverridden,omitempty"`
}

// ProductSummary is a lightweight projection of Product used by API responses
// and barcode-lookup results.
type ProductSummary struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Category       string         `json:"category"`
	UnitOfMeasure  string         `json:"unitOfMeasure"`
	ImageURL       string         `json:"imageUrl,omitempty"`
	ExternalSource ExternalSource `json:"externalSource,omitempty"`
}

// Catalog provides database operations for products and barcodes.
type Catalog struct {
	db *sql.DB
}

// NewCatalog creates a new Catalog with the given database connection.
func NewCatalog(db *sql.DB) *Catalog {
	return &Catalog{db: db}
}

// Source values for products.source. SQLite cannot enforce this value set
// with a CHECK constraint added via ALTER TABLE, so CreateProduct enforces it
// in Go.
const (
	SourceExternal = "external"
	SourceUser     = "user"
)

// CreateProduct inserts a new product row. If product.ID is empty a new UUID
// is generated. The caller should set product.Name; Category and
// UnitOfMeasure are optional. An empty product.Source defaults to SourceUser;
// any other value must be SourceExternal or SourceUser. An empty product.ExternalSource
// defaults to empty (no external provenance); any other value must be a valid ExternalSource.
func (r *Catalog) CreateProduct(ctx context.Context, product Product) error {
	if product.ID == "" {
		product.ID = uuid.NewString()
	}
	source := product.Source
	if source == "" {
		source = SourceUser
	}
	if source != SourceExternal && source != SourceUser {
		return fmt.Errorf("CreateProduct: invalid source %q", product.Source)
	}
	if !product.ExternalSource.Valid() {
		return fmt.Errorf("CreateProduct: invalid external_source %q", product.ExternalSource)
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO products (id, name, category, unit_of_measure, image_url, source, external_source, refreshed_at, name_overridden)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		product.ID, product.Name, nullableString(product.Category), nullableString(product.UnitOfMeasure),
		nullableString(product.ImageURL), source, nullableString(string(product.ExternalSource)), nullableTime(product.RefreshedAt), product.NameOverridden,
	)
	if err != nil {
		return fmt.Errorf("CreateProduct: %w", err)
	}
	return nil
}

// GetProductByID returns the product with the given ID, or nil if no such row
// exists.
func (r *Catalog) GetProductByID(ctx context.Context, id string) (*Product, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, COALESCE(category, ''), COALESCE(unit_of_measure, ''), COALESCE(image_url, ''), created_at,
		        source, COALESCE(external_source, ''), refreshed_at, COALESCE(name_overridden, 0)
		 FROM products WHERE id = ?`, id)

	var p Product
	var refreshedAt sql.NullTime
	var externalSource string
	if err := row.Scan(&p.ID, &p.Name, &p.Category, &p.UnitOfMeasure, &p.ImageURL, &p.CreatedAt,
		&p.Source, &externalSource, &refreshedAt, &p.NameOverridden); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetProductByID: %w", err)
	}
	if refreshedAt.Valid {
		p.RefreshedAt = &refreshedAt.Time
	}
	p.ExternalSource = ExternalSource(externalSource)
	return &p, nil
}

// ListProducts returns all products, ordered by name.
func (r *Catalog) ListProducts(ctx context.Context) ([]Product, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, COALESCE(category, ''), COALESCE(unit_of_measure, ''), COALESCE(image_url, ''), created_at,
		        source, COALESCE(external_source, ''), refreshed_at, COALESCE(name_overridden, 0)
		 FROM products ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("ListProducts: %w", err)
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		var refreshedAt sql.NullTime
		var externalSource string
		if err := rows.Scan(&p.ID, &p.Name, &p.Category, &p.UnitOfMeasure, &p.ImageURL, &p.CreatedAt,
			&p.Source, &externalSource, &refreshedAt, &p.NameOverridden); err != nil {
			return nil, fmt.Errorf("ListProducts scan: %w", err)
		}
		if refreshedAt.Valid {
			p.RefreshedAt = &refreshedAt.Time
		}
		p.ExternalSource = ExternalSource(externalSource)
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListProducts rows: %w", err)
	}
	return products, nil
}

// UpdateProduct updates the name, category, unit_of_measure, and image_url of
// an existing product identified by product.ID. It does not change created_at.
func (r *Catalog) UpdateProduct(ctx context.Context, product Product) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE products SET name = ?, category = ?, unit_of_measure = ?, image_url = ?,
		        name_overridden = CASE
		            WHEN source = 'external' AND name <> ? THEN 1
		            ELSE name_overridden
		        END
		 WHERE id = ?`,
		product.Name, nullableString(product.Category), nullableString(product.UnitOfMeasure),
		nullableString(product.ImageURL), product.Name, product.ID,
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

// SaveRefresh writes the field values a revalidation merged from Product Opener
// (name, category, unit_of_measure, image_url) and stamps refreshed_at and
// external_source. It does not touch id, source, created_at, or name_overridden,
// and it does not write the barcodes table, so every mapping keeps its barcode,
// source, and user_id.
func (r *Catalog) SaveRefresh(ctx context.Context, product Product, refreshedAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE products SET name = ?, category = ?, unit_of_measure = ?, image_url = ?, external_source = ?, refreshed_at = ? WHERE id = ?`,
		product.Name, nullableString(product.Category), nullableString(product.UnitOfMeasure),
		nullableString(product.ImageURL), nullableString(string(product.ExternalSource)), refreshedAt, product.ID,
	)
	if err != nil {
		return fmt.Errorf("SaveRefresh: %w", err)
	}
	return nil
}

// MarkRefreshed stamps refreshed_at without writing any other field. It is
// used by the two revalidation failure paths (upstream reports the barcode
// unknown, or the Open Food Facts request itself fails) where there is no
// upstream field data to write.
func (r *Catalog) MarkRefreshed(ctx context.Context, id string, refreshedAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE products SET refreshed_at = ? WHERE id = ?`,
		refreshedAt, id,
	)
	if err != nil {
		return fmt.Errorf("MarkRefreshed: %w", err)
	}
	return nil
}

// UpsertBarcodeMapping inserts or replaces a row in the barcodes table.
// source must be either "global" or "user_override".
// For global entries pass userID = "".
func (r *Catalog) UpsertBarcodeMapping(ctx context.Context, barcode, productID, source, userID string) error {
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
//  2. global rows (user_id = ”)
//
// Returns the first matching product, or nil if no match is found.
func (r *Catalog) LookupByBarcode(ctx context.Context, barcode, userID string) (*ProductSummary, error) {
	// Find the highest-priority match.
	// We use a CASE expression so user_override rows sort before global rows.
	row := r.db.QueryRowContext(ctx, `
		SELECT p.id, p.name, COALESCE(p.category, ''), COALESCE(p.unit_of_measure, ''), COALESCE(p.image_url, ''),
		       COALESCE(p.external_source, '')
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
	var externalSource string
	if err := row.Scan(&summary.ID, &summary.Name, &summary.Category, &summary.UnitOfMeasure, &summary.ImageURL, &externalSource); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("LookupByBarcode: %w", err)
	}

	summary.ExternalSource = ExternalSource(externalSource)
	return &summary, nil
}

// RecordBarcodeMiss stamps barcode as confirmed-unknown at checkedAt,
// inserting or re-stamping as needed. The upsert serves both the first
// recording and the re-stamp of an existing miss with no read-modify-write.
func (r *Catalog) RecordBarcodeMiss(ctx context.Context, barcode string, checkedAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO barcode_misses (barcode, checked_at) VALUES (?, ?)
		 ON CONFLICT(barcode) DO UPDATE SET checked_at = excluded.checked_at`,
		barcode, checkedAt,
	)
	if err != nil {
		return fmt.Errorf("RecordBarcodeMiss: %w", err)
	}
	return nil
}

// GetBarcodeMiss returns when barcode was last confirmed unknown, or nil if
// there is no such record.
func (r *Catalog) GetBarcodeMiss(ctx context.Context, barcode string) (*time.Time, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT checked_at FROM barcode_misses WHERE barcode = ?`, barcode)

	var checkedAt time.Time
	if err := row.Scan(&checkedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetBarcodeMiss: %w", err)
	}
	return &checkedAt, nil
}

// DeleteBarcodeMiss removes any confirmed-unknown record for barcode.
func (r *Catalog) DeleteBarcodeMiss(ctx context.Context, barcode string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM barcode_misses WHERE barcode = ?`, barcode)
	if err != nil {
		return fmt.Errorf("DeleteBarcodeMiss: %w", err)
	}
	return nil
}

// ListBarcodesForProduct returns all barcodes associated with a product,
// sorted ascending lexicographically.
func (r *Catalog) ListBarcodesForProduct(ctx context.Context, productID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT barcode FROM barcodes WHERE product_id = ? ORDER BY barcode ASC`,
		productID)
	if err != nil {
		return nil, fmt.Errorf("ListBarcodesForProduct: %w", err)
	}
	defer rows.Close()

	var barcodes []string
	for rows.Next() {
		var barcode string
		if err := rows.Scan(&barcode); err != nil {
			return nil, fmt.Errorf("ListBarcodesForProduct scan: %w", err)
		}
		barcodes = append(barcodes, barcode)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListBarcodesForProduct rows: %w", err)
	}
	return barcodes, nil
}

// nullableString converts an empty string to a SQL NULL so that optional text
// columns are stored as NULL rather than empty string.
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullableTime converts a nil *time.Time to a SQL NULL.
func nullableTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}
