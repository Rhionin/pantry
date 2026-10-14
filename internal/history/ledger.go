package history

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/Rhionin/pantry/internal/product"
)

// Ledger is every stock move the household can see, newest first.
type Ledger struct {
	Moves []LedgerMove `json:"moves"`
}

// LedgerMove is one stock in or stock out, with the product it changed.
type LedgerMove struct {
	ID          string `json:"id"`
	Direction   string `json:"direction"`
	Quantity    int    `json:"quantity"`
	At          string `json:"at"`
	Source      string `json:"source"`
	ProductID   string `json:"productId"`
	ProductName string `json:"productName"`
	ImageURL    string `json:"imageUrl,omitempty"`
	GroupID     string `json:"groupId,omitempty"`
	GroupName   string `json:"groupName,omitempty"`
}

// LedgerFilter chooses which moves the household history includes.
// Direction is in, out, or all. Query matches a product name or a group name.
type LedgerFilter struct {
	Direction string
	Query     string
}

// Household lists every stock move, newest first.
func (j *Journal) Household(ctx context.Context, filter LedgerFilter) (Ledger, error) {
	direction, err := canonicalDirection(filter.Direction)
	if err != nil {
		return Ledger{}, err
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return Ledger{}, err
	}
	defer tx.Rollback()
	if err := ensure(ctx, tx); err != nil {
		return Ledger{}, err
	}
	moves, err := listHousehold(ctx, tx, direction, strings.TrimSpace(filter.Query))
	if err != nil {
		return Ledger{}, err
	}
	if err := tx.Commit(); err != nil {
		return Ledger{}, err
	}
	return Ledger{Moves: moves}, nil
}

func canonicalDirection(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all":
		return "", nil
	case "in":
		return "in", nil
	case "out":
		return "out", nil
	default:
		return "", &InputError{Message: "Choose in, out, or all."}
	}
}

func listHousehold(ctx context.Context, tx *sql.Tx, direction, query string) ([]LedgerMove, error) {
	where := `i.user_id = ?`
	args := []any{householdUser}
	if direction != "" {
		where += ` AND m.direction = ?`
		args = append(args, direction)
	}
	if query != "" {
		where += ` AND (INSTR(LOWER(p.name), LOWER(?)) > 0 OR INSTR(LOWER(COALESCE(g.name, '')), LOWER(?)) > 0)`
		args = append(args, query, query)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT m.id, m.direction, m.quantity, m.at, m.source,
			m.product_id, p.name, COALESCE(p.image_url, ''),
			COALESCE(g.id, ''), COALESCE(g.name, '')
		FROM stock_moves m
		JOIN items i ON i.id = m.item_id
		JOIN products p ON p.id = m.product_id
		LEFT JOIN product_group_members gm ON gm.product_id = p.id
		LEFT JOIN product_groups g ON g.id = gm.group_id AND g.user_id = i.user_id
		WHERE `+where+`
		ORDER BY m.at DESC, m.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	moves := []LedgerMove{}
	for rows.Next() {
		var move LedgerMove
		var at time.Time
		var image, groupID, groupName string
		if err := rows.Scan(
			&move.ID, &move.Direction, &move.Quantity, &at, &move.Source,
			&move.ProductID, &move.ProductName, &image, &groupID, &groupName,
		); err != nil {
			return nil, err
		}
		move.At = at.UTC().Format(time.RFC3339)
		move.ImageURL = product.SafeImageURL(image)
		move.GroupID = groupID
		move.GroupName = groupName
		moves = append(moves, move)
	}
	return moves, rows.Err()
}
