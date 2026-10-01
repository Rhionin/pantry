package shopping

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	// DealSourceRecorded is a sale the household noted themselves.
	DealSourceRecorded = "recorded"
	// DealSourceRetailer is a sale supplied by a store price adapter.
	DealSourceRetailer = "retailer"
)

// Preference is the brand a household wants for one shared replenishment need.
// IgnorePrice means a cheaper sale must not replace that brand.
type Preference struct {
	NeedKey     string
	ItemID      string
	IgnorePrice bool
}

// Deal is a sale price for one pantry item.
type Deal struct {
	ItemID            string
	PriceCents        *int
	RegularPriceCents *int
	Label             string
	Source            string
}

// OnSale reports whether the deal should be offered against another brand.
// A note with only a label, or a price and no higher regular price, counts:
// the household recorded it because it is the deal they saw.
func (d Deal) OnSale() bool {
	if strings.TrimSpace(d.Label) != "" {
		return true
	}
	if d.PriceCents == nil {
		return false
	}
	if d.RegularPriceCents == nil {
		return true
	}
	return *d.PriceCents < *d.RegularPriceCents
}

// ListPreferences returns saved brand choices for the user, ordered by need.
func (s *Store) ListPreferences(ctx context.Context, userID string) ([]Preference, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT need_key, item_id, ignore_price
		 FROM brand_preferences
		 WHERE user_id = ?
		 ORDER BY need_key`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("could not load brand preferences: %w", err)
	}
	defer rows.Close()

	var prefs []Preference
	for rows.Next() {
		var pref Preference
		var ignore int
		if err := rows.Scan(&pref.NeedKey, &pref.ItemID, &ignore); err != nil {
			return nil, fmt.Errorf("could not read brand preference: %w", err)
		}
		pref.IgnorePrice = ignore != 0
		prefs = append(prefs, pref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not list brand preferences: %w", err)
	}
	return prefs, nil
}

// SavePreference inserts or replaces the brand choice for one need.
func (s *Store) SavePreference(ctx context.Context, userID string, pref Preference) error {
	ignore := 0
	if pref.IgnorePrice {
		ignore = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO brand_preferences (user_id, need_key, item_id, ignore_price, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, need_key) DO UPDATE SET
		   item_id = excluded.item_id,
		   ignore_price = excluded.ignore_price,
		   updated_at = excluded.updated_at`,
		userID, pref.NeedKey, pref.ItemID, ignore, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("could not save brand preference: %w", err)
	}
	return nil
}

// DeletePreference removes the brand choice for one need. Missing rows are fine.
func (s *Store) DeletePreference(ctx context.Context, userID, needKey string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM brand_preferences WHERE user_id = ? AND need_key = ?`,
		userID, needKey,
	)
	if err != nil {
		return fmt.Errorf("could not clear brand preference: %w", err)
	}
	return nil
}

// ListDeals returns noted sales for the user, ordered by item.
func (s *Store) ListDeals(ctx context.Context, userID string) ([]Deal, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT item_id, price_cents, regular_price_cents, label, source
		 FROM item_deals
		 WHERE user_id = ?
		 ORDER BY item_id`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("could not load sales: %w", err)
	}
	defer rows.Close()

	var deals []Deal
	for rows.Next() {
		deal, err := scanDeal(rows)
		if err != nil {
			return nil, fmt.Errorf("could not read sale: %w", err)
		}
		deals = append(deals, deal)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not list sales: %w", err)
	}
	return deals, nil
}

// SaveDeal inserts or replaces the noted sale for one item.
// An empty source is stored as recorded.
func (s *Store) SaveDeal(ctx context.Context, userID string, deal Deal) error {
	if deal.Source == "" {
		deal.Source = DealSourceRecorded
	}
	if deal.Source != DealSourceRecorded && deal.Source != DealSourceRetailer {
		return fmt.Errorf("could not save sale: unknown source %q", deal.Source)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO item_deals (user_id, item_id, price_cents, regular_price_cents, label, source, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, item_id) DO UPDATE SET
		   price_cents = excluded.price_cents,
		   regular_price_cents = excluded.regular_price_cents,
		   label = excluded.label,
		   source = excluded.source,
		   updated_at = excluded.updated_at`,
		userID, deal.ItemID, nullableInt(deal.PriceCents), nullableInt(deal.RegularPriceCents),
		deal.Label, deal.Source, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("could not save sale: %w", err)
	}
	return nil
}

// DeleteDeal removes a noted sale. Missing rows are fine.
func (s *Store) DeleteDeal(ctx context.Context, userID, itemID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM item_deals WHERE user_id = ? AND item_id = ?`,
		userID, itemID,
	)
	if err != nil {
		return fmt.Errorf("could not clear sale: %w", err)
	}
	return nil
}

func scanDeal(row scanner) (Deal, error) {
	var deal Deal
	var price, regular sql.NullInt64
	if err := row.Scan(&deal.ItemID, &price, &regular, &deal.Label, &deal.Source); err != nil {
		return Deal{}, err
	}
	deal.PriceCents = intPtrFromNull(price)
	deal.RegularPriceCents = intPtrFromNull(regular)
	return deal, nil
}

func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func intPtrFromNull(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}
