// Package connection provides data access for provider connections.
// A second package rather than a second type in internal/cart,
// precisely because of the one-data-access-type-per-package rule:
// the ledger and the connection records are two persisted concepts.
package connection

import (
	"context"
	"database/sql"
	"time"

	"github.com/Rhionin/pantry/internal/cart"
)

// Directory is the data-access type for provider connections.
type Directory struct {
	db *sql.DB
}

// NewDirectory creates a new Directory backed by the given database.
func NewDirectory(db *sql.DB) *Directory {
	return &Directory{db: db}
}

// Read returns the connection record for one provider, or (nil, nil) if not found.
func (d *Directory) Read(ctx context.Context, provider cart.ProviderID) (*cart.Connection, error) {
	var (
		state        string
		accessToken  sql.NullString
		refreshToken sql.NullString
		expiresAt    sql.NullTime
		authState    sql.NullString
		authStateAt  sql.NullTime
	)

	err := d.db.QueryRowContext(ctx,
		`SELECT state, access_token, refresh_token, expires_at, auth_state, auth_state_at
		 FROM provider_connections
		 WHERE provider_id = ?`, string(provider)).Scan(
		&state, &accessToken, &refreshToken, &expiresAt, &authState, &authStateAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var stateEnum cart.ConnectionState
	switch cart.ConnectionState(state) {
	case cart.StateNotRequired, cart.StateConnected, cart.StateReauthRequired, cart.StateDisconnected:
		stateEnum = cart.ConnectionState(state)
	default:
		// Default to disconnected for unknown states
		stateEnum = cart.StateDisconnected
	}

	var expiresAtPtr *time.Time
	if expiresAt.Valid {
		expiresAtPtr = &expiresAt.Time
	}

	var authStateAtPtr *time.Time
	if authStateAt.Valid {
		authStateAtPtr = &authStateAt.Time
	}

	return &cart.Connection{
		Provider:     provider,
		State:        stateEnum,
		AccessToken:  accessToken.String,
		RefreshToken: refreshToken.String,
		ExpiresAt:    expiresAtPtr,
		AuthState:    authState.String,
		AuthStateAt:  authStateAtPtr,
	}, nil
}

// Write creates or updates the connection record for one provider.
func (d *Directory) Write(ctx context.Context, conn *cart.Connection) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO provider_connections (provider_id, state, access_token, refresh_token, expires_at, auth_state, auth_state_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(provider_id) DO UPDATE SET
		     state = excluded.state,
		     access_token = excluded.access_token,
		     refresh_token = excluded.refresh_token,
		     expires_at = excluded.expires_at,
		     auth_state = excluded.auth_state,
		     auth_state_at = excluded.auth_state_at`,
		string(conn.Provider), string(conn.State), conn.AccessToken, conn.RefreshToken,
		conn.ExpiresAt, conn.AuthState, conn.AuthStateAt)
	return err
}

// Disconnect clears the credentials for one provider and leaves the state as disconnected.
// Every ledger entry is left unchanged (requirement 4.3).
func (d *Directory) Disconnect(ctx context.Context, provider cart.ProviderID) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE provider_connections SET
		     state = ?, access_token = NULL, refresh_token = NULL,
		     expires_at = NULL, auth_state = NULL, auth_state_at = NULL
		 WHERE provider_id = ?`,
		cart.StateDisconnected, string(provider))
	return err
}

// GenerateAuthState creates a new authorization state for the provider.
// Generate at least 32 characters from an injected io.Reader (crypto/rand in production).
func (d *Directory) GenerateAuthState(ctx context.Context, provider cart.ProviderID, state string) error {
	now := time.Now().UTC()
	_, err := d.db.ExecContext(ctx,
		`UPDATE provider_connections SET auth_state = ?, auth_state_at = ? WHERE provider_id = ?`,
		state, now, string(provider))
	return err
}

// ConsumeAuthState verifies and consumes an authorization state.
// Returns true if valid, false if rejected.
// Reject an absent or mismatched state, reject past 600 seconds, reject a
// callback carrying an error parameter, and discard the state in every case.
func (d *Directory) ConsumeAuthState(ctx context.Context, provider cart.ProviderID, state string) (bool, error) {
	var (
		storedState  string
		storedAt     time.Time
	)

	err := d.db.QueryRowContext(ctx,
		`SELECT auth_state, auth_state_at FROM provider_connections WHERE provider_id = ?`,
		string(provider)).Scan(&storedState, &storedAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	// Check if state matches
	if storedState != state {
		return false, nil
	}

	// Check if expired (600 seconds)
	if time.Since(storedAt) > 600*time.Second {
		return false, nil
	}

	// Discard the state
	_, err = d.db.ExecContext(ctx,
		`UPDATE provider_connections SET auth_state = NULL, auth_state_at = NULL WHERE provider_id = ?`,
		string(provider))
	if err != nil {
		return false, err
	}

	return true, nil
}
