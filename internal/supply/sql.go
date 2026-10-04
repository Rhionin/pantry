package supply

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Rhionin/pantry/internal/shopping"
)

const (
	keyStarted    = "onboarding_started_at"
	keyMonths     = "supply_months"
	householdUser = "user-1"
)

func readPhaseAndMonths(ctx context.Context, tx *sql.Tx) (Phase, Months, error) {
	phase, err := readPhase(ctx, tx)
	if err != nil {
		return Phase{}, 0, err
	}
	months, err := readMonths(ctx, tx)
	if err != nil {
		return Phase{}, 0, err
	}
	return phase, months, nil
}

func readSettings(ctx context.Context, db *sql.DB) (Settings, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Settings{}, fmt.Errorf("could not read supply settings: %w", err)
	}
	defer tx.Rollback()
	phase, months, err := readPhaseAndMonths(ctx, tx)
	if err != nil {
		return Settings{}, err
	}
	if err := tx.Commit(); err != nil {
		return Settings{}, fmt.Errorf("could not read supply settings: %w", err)
	}
	settings := Settings{
		Months:     months,
		Opening:    phase.isOpening(),
		WipePhrase: WipePhrase,
	}
	if started, ok := phase.StartedAt(); ok {
		settings.StartedAt = started
	}
	return settings, nil
}

func readPhase(ctx context.Context, q queryer) (Phase, error) {
	var raw string
	err := q.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, keyStarted).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Phase{}, nil
	}
	if err != nil {
		return Phase{}, fmt.Errorf("could not read the supply start date: %w", err)
	}
	started, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return Phase{}, fmt.Errorf("could not read the supply start date: %w", err)
	}
	return Phase{started: started.UTC()}, nil
}

func readMonths(ctx context.Context, q queryer) (Months, error) {
	var raw string
	err := q.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, keyMonths).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultMonths, nil
	}
	if err != nil {
		return 0, fmt.Errorf("could not read the supply length: %w", err)
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("could not read the supply length: %w", err)
	}
	months, err := ParseMonths(n)
	if err != nil {
		return 0, fmt.Errorf("could not read the supply length: %w", err)
	}
	return months, nil
}

func completeOpening(ctx context.Context, db *sql.DB, now time.Time) (Phase, error) {
	stamp := now.UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO app_settings (key, value)
		SELECT ?, ?
		WHERE NOT EXISTS (SELECT 1 FROM app_settings WHERE key = ?)`,
		keyStarted, stamp, keyStarted,
	); err != nil {
		return Phase{}, fmt.Errorf("could not save the supply start date: %w", err)
	}
	return readPhase(ctx, db)
}

func writeMonths(ctx context.Context, db *sql.DB, months Months) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		keyMonths, strconv.Itoa(int(months)),
	)
	if err != nil {
		return fmt.Errorf("could not save the supply length: %w", err)
	}
	return nil
}

func writeOverride(ctx context.Context, db *sql.DB, id ProductID, o Override) error {
	if o.kind == overrideInherit {
		if _, err := db.ExecContext(ctx, `DELETE FROM supply_overrides WHERE product_id = ?`, string(id)); err != nil {
			return fmt.Errorf("could not clear the supply override: %w", err)
		}
		return nil
	}
	var window, qty any
	if months, ok := o.Window(); ok {
		window = int(months)
	}
	if units, ok := o.Quantity(); ok {
		qty = int(units)
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO supply_overrides (product_id, window_months, quantity) VALUES (?, ?, ?)
		ON CONFLICT(product_id) DO UPDATE SET
			window_months = excluded.window_months,
			quantity = excluded.quantity`,
		string(id), window, qty,
	)
	if err != nil {
		return fmt.Errorf("could not save the supply override: %w", err)
	}
	return nil
}

func readOverride(ctx context.Context, db *sql.DB, id ProductID) (Override, error) {
	var window, qty sql.NullInt64
	err := db.QueryRowContext(ctx, `
		SELECT window_months, quantity FROM supply_overrides WHERE product_id = ?`,
		string(id),
	).Scan(&window, &qty)
	if errors.Is(err, sql.ErrNoRows) {
		return Override{}, nil
	}
	if err != nil {
		return Override{}, fmt.Errorf("could not read the supply override: %w", err)
	}
	if window.Valid {
		return Window(Months(window.Int64))
	}
	if qty.Valid {
		return Quantity(Units(qty.Int64))
	}
	return Override{}, nil
}

func wipeHousehold(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not wipe inventory: %w", err)
	}
	defer tx.Rollback()

	statements := []string{
		`DELETE FROM shopping_list_entry_adjustments
		 WHERE entry_id IN (SELECT id FROM shopping_list_items WHERE user_id = ?)`,
		`DELETE FROM staged_cart_skips WHERE user_id = ?`,
		`DELETE FROM shopping_list_items WHERE user_id = ?`,
		`DELETE FROM consumption_events WHERE item_id IN (SELECT id FROM items WHERE user_id = ?)`,
		`DELETE FROM item_instances WHERE item_id IN (SELECT id FROM items WHERE user_id = ?)`,
		`DELETE FROM stock_in_events WHERE product_id IN (SELECT product_id FROM items WHERE user_id = ?)`,
		`DELETE FROM items WHERE user_id = ?`,
	}
	for _, query := range statements {
		if _, err := tx.ExecContext(ctx, query, householdUser); err != nil {
			return fmt.Errorf("could not wipe inventory: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_settings WHERE key = ?`, keyStarted); err != nil {
		return fmt.Errorf("could not wipe inventory: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not wipe inventory: %w", err)
	}
	return nil
}

func loadFacts(ctx context.Context, tx *sql.Tx, phase Phase) ([]Fact, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT i.product_id, p.name, COALESCE(p.unit_of_measure, ''),
			(SELECT COUNT(*) FROM item_instances inst
			 WHERE inst.item_id = i.id AND inst.removed_at IS NULL)
		FROM items i
		JOIN products p ON p.id = i.product_id
		WHERE i.user_id = ?
		ORDER BY i.product_id`, householdUser)
	if err != nil {
		return nil, fmt.Errorf("could not read supply: %w", err)
	}
	defer rows.Close()

	facts := map[ProductID]*Fact{}
	var order []ProductID
	for rows.Next() {
		var id, name, unit string
		var onHand int
		if err := rows.Scan(&id, &name, &unit, &onHand); err != nil {
			return nil, fmt.Errorf("could not read supply: %w", err)
		}
		product := ProductID(id)
		fact := facts[product]
		if fact == nil {
			group := GroupID("")
			if key, ok := shopping.NeedKey(name, unit); ok {
				group = GroupID(key)
			}
			fact = &Fact{Product: product, Group: group}
			facts[product] = fact
			order = append(order, product)
		}
		fact.OnHand += Units(onHand)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read supply: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("could not read supply: %w", err)
	}

	if err := attachOverrides(ctx, tx, facts); err != nil {
		return nil, err
	}
	if err := attachManuals(ctx, tx, facts); err != nil {
		return nil, err
	}
	if started, ok := phase.StartedAt(); ok {
		if err := attachEvents(ctx, tx, facts, started); err != nil {
			return nil, err
		}
	}

	out := make([]Fact, 0, len(order))
	for _, id := range order {
		out = append(out, *facts[id])
	}
	return out, nil
}

func attachOverrides(ctx context.Context, tx *sql.Tx, facts map[ProductID]*Fact) error {
	rows, err := tx.QueryContext(ctx, `SELECT product_id, window_months, quantity FROM supply_overrides`)
	if err != nil {
		return fmt.Errorf("could not read supply: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var window, qty sql.NullInt64
		if err := rows.Scan(&id, &window, &qty); err != nil {
			return fmt.Errorf("could not read supply: %w", err)
		}
		fact := facts[ProductID(id)]
		if fact == nil {
			continue
		}
		switch {
		case window.Valid:
			override, err := Window(Months(window.Int64))
			if err != nil {
				return err
			}
			fact.Override = override
		case qty.Valid:
			override, err := Quantity(Units(qty.Int64))
			if err != nil {
				return err
			}
			fact.Override = override
		}
	}
	return rows.Err()
}

func attachManuals(ctx context.Context, tx *sql.Tx, facts map[ProductID]*Fact) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT i.product_id, SUM(s.quantity)
		FROM shopping_list_items s
		JOIN items i ON i.id = s.item_id
		WHERE s.user_id = ? AND s.source = 'manual' AND s.purchased_at IS NULL
		GROUP BY i.product_id`, householdUser)
	if err != nil {
		return fmt.Errorf("could not read supply: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var qty int
		if err := rows.Scan(&id, &qty); err != nil {
			return fmt.Errorf("could not read supply: %w", err)
		}
		fact := facts[ProductID(id)]
		if fact == nil || qty <= 0 {
			continue
		}
		units := Units(qty)
		fact.Manual = &units
	}
	return rows.Err()
}

func attachEvents(ctx context.Context, tx *sql.Tx, facts map[ProductID]*Fact, started time.Time) error {
	ins, err := tx.QueryContext(ctx, `
		SELECT product_id, at FROM stock_in_events
		WHERE at > ? AND product_id IN (SELECT product_id FROM items WHERE user_id = ?)
		ORDER BY product_id, at`, started, householdUser)
	if err != nil {
		return fmt.Errorf("could not read supply: %w", err)
	}
	defer ins.Close()
	for ins.Next() {
		var id string
		var at time.Time
		if err := ins.Scan(&id, &at); err != nil {
			return fmt.Errorf("could not read supply: %w", err)
		}
		if fact := facts[ProductID(id)]; fact != nil {
			fact.StockIns = append(fact.StockIns, at.UTC())
		}
	}
	if err := ins.Err(); err != nil {
		return fmt.Errorf("could not read supply: %w", err)
	}
	if err := ins.Close(); err != nil {
		return fmt.Errorf("could not read supply: %w", err)
	}

	outs, err := tx.QueryContext(ctx, `
		SELECT i.product_id, c.consumed_at
		FROM consumption_events c
		JOIN items i ON i.id = c.item_id
		WHERE i.user_id = ? AND c.consumed_at > ?
		ORDER BY i.product_id, c.consumed_at`, householdUser, started)
	if err != nil {
		return fmt.Errorf("could not read supply: %w", err)
	}
	defer outs.Close()
	for outs.Next() {
		var id string
		var at time.Time
		if err := outs.Scan(&id, &at); err != nil {
			return fmt.Errorf("could not read supply: %w", err)
		}
		if fact := facts[ProductID(id)]; fact != nil {
			fact.StockOuts = append(fact.StockOuts, Withdrawal{At: at.UTC(), Qty: 1})
		}
	}
	return outs.Err()
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
