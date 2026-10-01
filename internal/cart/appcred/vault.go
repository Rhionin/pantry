// Package appcred stores cart-provider application credentials.
// A provider's client secret is readable only inside this package. Callers that
// build an HTTP response use Public, which has no secret field.
package appcred

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// userID is the placeholder owner until an auth layer supplies a real user.
const userID = "user-1"

// Saved is one provider's application credentials, including the secret.
// It is not safe to marshal.
type Saved struct {
	ProviderID   string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Modality     string
}

// Public is the view safe to return from GET. It records that a secret exists
// without carrying the secret.
type Public struct {
	ClientID    string
	RedirectURI string
	Modality    string
	SecretSet   bool
	Source      string
}

// Vault is the data-access type for provider application credentials.
type Vault struct {
	db *sql.DB
}

// NewVault creates a Vault backed by the given database.
func NewVault(db *sql.DB) *Vault {
	return &Vault{db: db}
}

// Load returns the saved credentials for one provider. ok is false when no row
// exists. The secret is included for applying it to the provider, not for an
// HTTP response.
func (v *Vault) Load(ctx context.Context, providerID string) (Saved, bool, error) {
	if v == nil || v.db == nil {
		return Saved{}, false, nil
	}
	var saved Saved
	err := v.db.QueryRowContext(ctx, `
		SELECT provider_id, client_id, client_secret, redirect_uri, modality
		FROM provider_app_credentials
		WHERE user_id = ? AND provider_id = ?`,
		userID, providerID,
	).Scan(&saved.ProviderID, &saved.ClientID, &saved.ClientSecret, &saved.RedirectURI, &saved.Modality)
	if err == sql.ErrNoRows {
		return Saved{}, false, nil
	}
	if err != nil {
		return Saved{}, false, fmt.Errorf("read provider credentials: %w", err)
	}
	return saved, true, nil
}

// Public returns the non-secret view of a saved row. ok is false when nothing
// is saved. Source is "saved" when a row exists.
func (v *Vault) Public(ctx context.Context, providerID string) (Public, bool, error) {
	saved, ok, err := v.Load(ctx, providerID)
	if err != nil || !ok {
		return Public{}, ok, err
	}
	return Public{
		ClientID:    saved.ClientID,
		RedirectURI: saved.RedirectURI,
		Modality:    saved.Modality,
		SecretSet:   saved.ClientSecret != "",
		Source:      "saved",
	}, true, nil
}

// Save inserts or replaces the saved credentials. An empty secret keeps the
// secret already stored. The first save must include a secret.
func (v *Vault) Save(ctx context.Context, saved Saved) error {
	if v == nil || v.db == nil {
		return fmt.Errorf("credential store is not available")
	}
	existing, ok, err := v.Load(ctx, saved.ProviderID)
	if err != nil {
		return err
	}
	secret := saved.ClientSecret
	if secret == "" {
		if !ok || existing.ClientSecret == "" {
			return fmt.Errorf("client secret is required")
		}
		secret = existing.ClientSecret
	}
	id := userID + ":" + saved.ProviderID
	_, err = v.db.ExecContext(ctx, `
		INSERT INTO provider_app_credentials
			(id, user_id, provider_id, client_id, client_secret, redirect_uri, modality, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, provider_id) DO UPDATE SET
			client_id = excluded.client_id,
			client_secret = excluded.client_secret,
			redirect_uri = excluded.redirect_uri,
			modality = excluded.modality,
			updated_at = excluded.updated_at`,
		id, userID, saved.ProviderID, saved.ClientID, secret, saved.RedirectURI, saved.Modality, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("save provider credentials: %w", err)
	}
	return nil
}

// Clear removes the saved credentials for one provider.
func (v *Vault) Clear(ctx context.Context, providerID string) error {
	if v == nil || v.db == nil {
		return nil
	}
	_, err := v.db.ExecContext(ctx, `
		DELETE FROM provider_app_credentials WHERE user_id = ? AND provider_id = ?`,
		userID, providerID,
	)
	if err != nil {
		return fmt.Errorf("clear provider credentials: %w", err)
	}
	return nil
}
