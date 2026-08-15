// Package scan provides scan queue management functionality.
package scan

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/google/uuid"
)

// ScanDirection represents the direction of a scan (stock-in or stock-out).
type ScanDirection string

const (
	StockIn  ScanDirection = "stock_in"
	StockOut ScanDirection = "stock_out"
)

// ScanStatus represents the status of a scan entry.
type ScanStatus string

const (
	Pending   ScanStatus = "pending"
	Flagged   ScanStatus = "flagged"
	Committed ScanStatus = "committed"
	Cancelled ScanStatus = "cancelled"
)

// ScanEntry represents a single item in the scan queue.
type ScanEntry struct {
	ID          string
	UserID      string
	Barcode     string
	ScannedAt   time.Time
	Direction   *ScanDirection // nil means not yet set
	UnitCount   int
	ExpiresAt   *time.Time
	Status      ScanStatus
	ProductID   *string
	Product     *product.ProductSummary // populated by join queries
	CommittedAt *time.Time
	CreatedAt   time.Time
}

// Repo provides database operations for scan entries.
type Repo struct {
	db *sql.DB
}

// NewRepo creates a new Repo with the given database connection.
func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// CreateScanEntry inserts a new scan entry. If entry.ID is empty, a new UUID
// is generated. Status defaults to "pending" if not set.
func (r *Repo) CreateScanEntry(ctx context.Context, entry ScanEntry) (*ScanEntry, error) {
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	if entry.Status == "" {
		entry.Status = Pending
	}

	var directionStr *string
	if entry.Direction != nil {
		directionStr = ptr(string(*entry.Direction))
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO scan_entries (id, user_id, barcode, scanned_at, direction, unit_count, expires_at, status, product_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID,
		entry.UserID,
		entry.Barcode,
		entry.ScannedAt,
		nullableString(directionStr),
		entry.UnitCount,
		nullableTime(entry.ExpiresAt),
		string(entry.Status),
		nullableString(entry.ProductID),
	)
	if err != nil {
		return nil, fmt.Errorf("CreateScanEntry: %w", err)
	}

	return r.GetScanEntry(ctx, entry.ID)
}

// GetScanEntry returns the scan entry with the given ID, or nil if no such
// entry exists. Includes joined product information if available.
func (r *Repo) GetScanEntry(ctx context.Context, id string) (*ScanEntry, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT 
			se.id, se.user_id, se.barcode, se.scanned_at, se.direction, se.unit_count, 
			se.expires_at, se.status, se.product_id, se.committed_at, se.created_at,
			p.id, p.name, COALESCE(p.category, ''), COALESCE(p.unit_of_measure, '')
		FROM scan_entries se
		LEFT JOIN products p ON p.id = se.product_id
		WHERE se.id = ?`,
		id,
	)

	entry, err := scanScanEntry(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetScanEntry: %w", err)
	}
	return entry, nil
}

// ListScanEntries returns scan entries filtered by status and ordered by
// scanned_at ascending (chronological order). If status is empty, all entries
// are returned.
func (r *Repo) ListScanEntries(ctx context.Context, userID string, status ScanStatus) ([]ScanEntry, error) {
	query := `
		SELECT 
			se.id, se.user_id, se.barcode, se.scanned_at, se.direction, se.unit_count, 
			se.expires_at, se.status, se.product_id, se.committed_at, se.created_at,
			p.id, p.name, COALESCE(p.category, ''), COALESCE(p.unit_of_measure, '')
		FROM scan_entries se
		LEFT JOIN products p ON p.id = se.product_id
		WHERE se.user_id = ?`

	args := []interface{}{userID}

	if status != "" {
		query += ` AND se.status = ?`
		args = append(args, string(status))
	}

	query += ` ORDER BY se.scanned_at ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListScanEntries: %w", err)
	}
	defer rows.Close()

	var entries []ScanEntry
	for rows.Next() {
		entry, err := scanScanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("ListScanEntries scan: %w", err)
		}
		entries = append(entries, *entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListScanEntries rows: %w", err)
	}
	return entries, nil
}

// UpdateScanEntry updates direction, unit_count, expires_at, product_id, and
// status for the given scan entry ID. This is the single allowed UPDATE path
// for scan entries (enforcing append-only semantics at the application layer).
//
// Pass nil for fields that should not be updated. To clear a nullable field,
// pass a pointer to a zero value (e.g., &time.Time{} for expires_at).
func (r *Repo) UpdateScanEntry(ctx context.Context, id string, direction *ScanDirection, unitCount *int, expiresAt *time.Time, productID *string, status *ScanStatus) error {
	query := "UPDATE scan_entries SET"
	args := []interface{}{}
	updates := []string{}

	if direction != nil {
		updates = append(updates, " direction = ?")
		args = append(args, nullableString(ptr(string(*direction))))
	}
	if unitCount != nil {
		updates = append(updates, " unit_count = ?")
		args = append(args, *unitCount)
	}
	if expiresAt != nil {
		updates = append(updates, " expires_at = ?")
		args = append(args, nullableTime(expiresAt))
	}
	if productID != nil {
		updates = append(updates, " product_id = ?")
		args = append(args, nullableString(productID))
	}
	if status != nil {
		updates = append(updates, " status = ?")
		args = append(args, string(*status))
	}

	if len(updates) == 0 {
		return nil
	}

	for i, u := range updates {
		query += u
		if i < len(updates)-1 {
			query += ","
		}
	}

	query += " WHERE id = ?"
	args = append(args, id)

	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("UpdateScanEntry: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("UpdateScanEntry rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("UpdateScanEntry: scan entry %q not found", id)
	}

	return nil
}

// CommitScanEntry marks a scan entry as committed and records the committed_at
// timestamp. This is a convenience wrapper around UpdateScanEntry.
func (r *Repo) CommitScanEntry(ctx context.Context, id string) error {
	now := time.Now()
	committed := Committed

	_, err := r.db.ExecContext(ctx,
		`UPDATE scan_entries SET status = ?, committed_at = ? WHERE id = ?`,
		string(committed), now, id,
	)
	if err != nil {
		return fmt.Errorf("CommitScanEntry: %w", err)
	}

	return nil
}

// BatchUpdateScanEntries applies the same direction, unit_count, and expires_at
// to multiple scan entries identified by their IDs.
func (r *Repo) BatchUpdateScanEntries(ctx context.Context, ids []string, direction *ScanDirection, unitCount *int, expiresAt *time.Time) error {
	if len(ids) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("BatchUpdateScanEntries begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, id := range ids {
		err := r.updateScanEntryInTx(ctx, tx, id, direction, unitCount, expiresAt, nil, nil)
		if err != nil {
			return fmt.Errorf("BatchUpdateScanEntries: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("BatchUpdateScanEntries commit: %w", err)
	}

	return nil
}

// updateScanEntryInTx is a helper that updates a scan entry within a transaction.
func (r *Repo) updateScanEntryInTx(ctx context.Context, tx *sql.Tx, id string, direction *ScanDirection, unitCount *int, expiresAt *time.Time, productID *string, status *ScanStatus) error {
	query := "UPDATE scan_entries SET"
	args := []interface{}{}
	updates := []string{}

	if direction != nil {
		updates = append(updates, " direction = ?")
		args = append(args, nullableString(ptr(string(*direction))))
	}
	if unitCount != nil {
		updates = append(updates, " unit_count = ?")
		args = append(args, *unitCount)
	}
	if expiresAt != nil {
		updates = append(updates, " expires_at = ?")
		args = append(args, nullableTime(expiresAt))
	}
	if productID != nil {
		updates = append(updates, " product_id = ?")
		args = append(args, nullableString(productID))
	}
	if status != nil {
		updates = append(updates, " status = ?")
		args = append(args, string(*status))
	}

	if len(updates) == 0 {
		return nil
	}

	for i, u := range updates {
		query += u
		if i < len(updates)-1 {
			query += ","
		}
	}

	query += " WHERE id = ?"
	args = append(args, id)

	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("updateScanEntryInTx: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("updateScanEntryInTx rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("updateScanEntryInTx: scan entry %q not found", id)
	}

	return nil
}

// scanScanEntry is a helper that scans a row into a ScanEntry struct.
// It handles nullable fields and optional product join data.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanScanEntry(row scanner) (*ScanEntry, error) {
	var entry ScanEntry
	var direction, productIDCol sql.NullString
	var expiresAt, committedAt sql.NullTime
	var productID, productName, productCategory, productUnitOfMeasure sql.NullString

	err := row.Scan(
		&entry.ID,
		&entry.UserID,
		&entry.Barcode,
		&entry.ScannedAt,
		&direction,
		&entry.UnitCount,
		&expiresAt,
		&entry.Status,
		&productIDCol,
		&committedAt,
		&entry.CreatedAt,
		&productID,
		&productName,
		&productCategory,
		&productUnitOfMeasure,
	)
	if err != nil {
		return nil, err
	}

	if direction.Valid {
		d := ScanDirection(direction.String)
		entry.Direction = &d
	}
	if expiresAt.Valid {
		entry.ExpiresAt = &expiresAt.Time
	}
	if productIDCol.Valid {
		entry.ProductID = &productIDCol.String
	}
	if committedAt.Valid {
		entry.CommittedAt = &committedAt.Time
	}

	if productID.Valid {
		entry.Product = &product.ProductSummary{
			ID:            productID.String,
			Name:          productName.String,
			Category:      productCategory.String,
			UnitOfMeasure: productUnitOfMeasure.String,
		}
	}

	return &entry, nil
}

// Helper functions for nullable types

func ptr[T any](v T) *T {
	return &v
}

func nullableString(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func nullableTime(t *time.Time) sql.NullTime {
	if t == nil || t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}
