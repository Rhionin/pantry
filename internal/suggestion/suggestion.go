// Package suggestion provides consumption event tracking and target quantity suggestion functionality.
package suggestion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ConsumptionEvent records a single stock-out event for suggestion analytics.
type ConsumptionEvent struct {
	ID          string
	ItemID      string
	ConsumedAt  time.Time
	ScanEntryID *string // optional: links back to the scan entry that caused the consumption
}

// TargetQuantitySuggestion holds a suggested target instance count for an item,
// along with the reasoning and a flag for insufficient data.
type TargetQuantitySuggestion struct {
	ItemID                string `json:"itemId"`
	SuggestedQuantity     int    `json:"suggestedQuantity"`
	Reasoning             string `json:"reasoning"`
	ConsumptionEventCount int    `json:"consumptionEventCount"`
	DataInsufficient      bool   `json:"dataInsufficient"`
}

// Repo provides database operations for consumption events.
type Repo struct {
	db *sql.DB
}

// NewRepo creates a new Repo with the given database connection.
func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// InsertConsumptionEvent inserts a new consumption event record.
// If event.ID is empty, a new UUID is generated.
func (r *Repo) InsertConsumptionEvent(ctx context.Context, event ConsumptionEvent) (*ConsumptionEvent, error) {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO consumption_events (id, item_id, consumed_at, scan_entry_id)
		 VALUES (?, ?, ?, ?)`,
		event.ID,
		event.ItemID,
		event.ConsumedAt,
		nullableString(event.ScanEntryID),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert consumption event: %w", err)
	}

	return r.getByID(ctx, event.ID)
}

// ListConsumptionEvents returns all consumption events for the given itemID,
// ordered ascending by consumed_at (oldest first).
func (r *Repo) ListConsumptionEvents(ctx context.Context, itemID string) ([]ConsumptionEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, item_id, consumed_at, scan_entry_id
		 FROM consumption_events
		 WHERE item_id = ?
		 ORDER BY consumed_at ASC`,
		itemID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list consumption events: %w", err)
	}
	defer rows.Close()

	var events []ConsumptionEvent
	for rows.Next() {
		event, err := scanConsumptionEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to read consumption event: %w", err)
		}
		events = append(events, *event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate consumption events: %w", err)
	}
	return events, nil
}

// getByID fetches a single consumption event by its ID.
func (r *Repo) getByID(ctx context.Context, id string) (*ConsumptionEvent, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, item_id, consumed_at, scan_entry_id
		 FROM consumption_events WHERE id = ?`,
		id,
	)

	event, err := scanConsumptionEvent(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get consumption event: %w", err)
	}
	return event, nil
}

// scanner is a common interface for *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

// scanConsumptionEvent scans a row into a ConsumptionEvent.
func scanConsumptionEvent(row scanner) (*ConsumptionEvent, error) {
	var event ConsumptionEvent
	var scanEntryID sql.NullString

	err := row.Scan(
		&event.ID,
		&event.ItemID,
		&event.ConsumedAt,
		&scanEntryID,
	)
	if err != nil {
		return nil, err
	}

	if scanEntryID.Valid {
		event.ScanEntryID = &scanEntryID.String
	}

	return &event, nil
}

// nullableString converts a *string to sql.NullString for nullable TEXT columns.
func nullableString(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}
