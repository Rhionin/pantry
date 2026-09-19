package connection

import (
	"context"
	"sync"
	"time"

	"github.com/Rhionin/pantry/internal/cart"
)

// Ptr returns a pointer to t.
func Ptr[T any](v T) *T {
	return &v
}

// refreshCall holds a pending token exchange.
type refreshCall struct {
	done chan struct{}
	err  error
	tokenSet cart.TokenSet
}

// TokenBroker implements result-sharing single flight for token refresh.
// Every waiting caller receives the token the one in-progress exchange produced.
type TokenBroker struct {
	mu       sync.Mutex
	inFlight map[string]*refreshCall
	Directory *Directory
}

// NewTokenBroker creates a new TokenBroker.
func NewTokenBroker(dir *Directory) *TokenBroker {
	return &TokenBroker{
		inFlight:  make(map[string]*refreshCall),
		Directory: dir,
	}
}

// AccessToken returns the current access token for the provider, refreshing it if needed.
// Refresh if and only if the remaining access-token lifetime is 60 seconds or less,
// or no access token is persisted.
// The key is ProviderID, so a slow provider's flight blocks no request to another provider.
func (b *TokenBroker) AccessToken(ctx context.Context, provider cart.ProviderID, flow cart.OAuthFlow) (cart.Credential, error) {
	b.mu.Lock()
	if call, ok := b.inFlight[string(provider)]; ok {
		// Another request is already refreshing this provider's token
		b.mu.Unlock()
		<-call.done
		if call.err != nil {
			return nil, call.err
		}
		return cart.NewBearerCredential(call.tokenSet.AccessToken), nil
	}

	// Check if we need to refresh (single-flight setup)
	// Read the connection record
	conn, err := b.Directory.Read(ctx, provider)
	if err != nil {
		b.mu.Unlock()
		return nil, err
	}

	// Check if we have a valid token or need refresh
	shouldRefresh := true
	if conn != nil && conn.AccessToken != "" && conn.ExpiresAt != nil {
		remaining := conn.ExpiresAt.Sub(time.Now().UTC())
		if remaining > 60*time.Second {
			shouldRefresh = false
		}
	}

	if !shouldRefresh {
		// Return existing token
		b.mu.Unlock()
		return cart.NewBearerCredential(conn.AccessToken), nil
	}

	// Start a new in-flight call
	call := &refreshCall{done: make(chan struct{})}
	b.inFlight[string(provider)] = call
	b.mu.Unlock()

	// Perform the refresh
	tokenSet, err := b.refreshAccessToken(ctx, provider, flow, conn)

	// Complete the call
	b.mu.Lock()
	delete(b.inFlight, string(provider))
	b.mu.Unlock()

	close(call.done)
	call.tokenSet = tokenSet
	call.err = err

	if err != nil {
		return nil, err
	}

	return cart.NewBearerCredential(tokenSet.AccessToken), nil
}

// refreshAccessToken performs the actual token refresh.
func (b *TokenBroker) refreshAccessToken(ctx context.Context, provider cart.ProviderID, flow cart.OAuthFlow, conn *cart.Connection) (cart.TokenSet, error) {
	var refreshToken string
	if conn != nil {
		refreshToken = conn.RefreshToken
	}

	if refreshToken == "" {
		// No refresh token - can't refresh
		return cart.TokenSet{}, nil
	}

	tokenSet, err := flow.RefreshAccessToken(ctx, refreshToken)
	if err != nil {
		return cart.TokenSet{}, err
	}

	// Update the connection record with new tokens
	expiresAt := tokenSet.ReceiptAt.Add(tokenSet.ExpiresIn)
	newConn := &cart.Connection{
		Provider:     provider,
		State:        cart.StateConnected,
		AccessToken:  tokenSet.AccessToken,
		RefreshToken: tokenSet.RefreshToken,
		ExpiresAt:    &expiresAt,
	}
	if err := b.Directory.Write(ctx, newConn); err != nil {
		return cart.TokenSet{}, err
	}

	return tokenSet, nil
}
