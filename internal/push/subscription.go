package push

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
)

// Subscription is one browser registered for scan alerts.
type Subscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

// Problem is a sentence for the person who sent the subscription, or empty
// when the browser payload can be stored.
func (s Subscription) Problem() string {
	endpoint := strings.TrimSpace(s.Endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "Enter a push subscription from this browser."
	}
	if len(endpoint) > 2000 || !pushKey(s.P256dh, 200) || !pushKey(s.Auth, 100) {
		return "Enter a push subscription from this browser."
	}
	return ""
}

func pushKey(value string, max int) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > max || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	return true
}

// Subscriptions stores the browsers that asked for scan alerts.
type Subscriptions struct {
	db *sql.DB
}

// NewSubscriptions uses the pantry database.
func NewSubscriptions(db *sql.DB) *Subscriptions {
	return &Subscriptions{db: db}
}

// Save inserts or replaces the keys for an endpoint. A browser refreshes
// those keys without changing the endpoint.
func (s *Subscriptions) Save(ctx context.Context, sub Subscription) error {
	sub.Endpoint = strings.TrimSpace(sub.Endpoint)
	sub.P256dh = strings.TrimSpace(sub.P256dh)
	sub.Auth = strings.TrimSpace(sub.Auth)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO push_subscriptions (endpoint, p256dh, auth)
		VALUES (?, ?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET
			p256dh = excluded.p256dh,
			auth = excluded.auth,
			updated_at = CURRENT_TIMESTAMP`,
		sub.Endpoint, sub.P256dh, sub.Auth,
	)
	if err != nil {
		return fmt.Errorf("save push subscription: %w", err)
	}
	return nil
}

// Delete removes one endpoint. A missing row is not an error.
func (s *Subscriptions) Delete(ctx context.Context, endpoint string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE endpoint = ?`, strings.TrimSpace(endpoint))
	if err != nil {
		return fmt.Errorf("delete push subscription: %w", err)
	}
	return nil
}

// List returns every stored browser, oldest first.
func (s *Subscriptions) List(ctx context.Context) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT endpoint, p256dh, auth
		FROM push_subscriptions
		ORDER BY created_at, endpoint`)
	if err != nil {
		return nil, fmt.Errorf("list push subscriptions: %w", err)
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(&sub.Endpoint, &sub.P256dh, &sub.Auth); err != nil {
			return nil, fmt.Errorf("list push subscriptions: %w", err)
		}
		out = append(out, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list push subscriptions: %w", err)
	}
	return out, nil
}
