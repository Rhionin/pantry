package server

import (
	"fmt"

	"github.com/Rhionin/pantry/internal/cart"
	"github.com/Rhionin/pantry/internal/cart/connection"
)

// ProvidersHandler handles provider-related operations.
type ProvidersHandler struct {
	Registry      *cart.Registry
	ConnectionDir *connection.Directory
}

// ProviderInfo represents information about a registered provider.
type ProviderInfo struct {
	ID                    string               `json:"id"`
	DisplayName           string               `json:"displayName"`
	Capabilities          ProviderCapabilities `json:"capabilities"`
	ConnectionState       string               `json:"connectionState"`
	CredentialsConfigured bool                 `json:"credentialsConfigured"`
}

// ProviderCapabilities represents the five capability dimensions of a provider.
type ProviderCapabilities struct {
	Auth         string `json:"auth"`
	Delivery     string `json:"delivery"`
	Confirmation string `json:"confirmation"`
	Mutation     string `json:"mutation"`
	Identity     string `json:"identity"`
}

// ListProvidersHandler handles GET /api/providers.
// Returns information about all registered providers.
type ListProvidersHandler struct {
	ProvidersHandler
}

// Handle returns a list of all registered providers.
func (h *ListProvidersHandler) Handle(req Request[struct{}, struct{}]) ([]ProviderInfo, error) {
	// Get all registered providers from the registry
	// For now, return empty list - this will be fixed when we add the List method to Registry
	return []ProviderInfo{}, nil
}

// ProviderAuthorizeHandler handles GET /api/providers/{providerId}/authorize.
// Initiates the OAuth2 authorization flow.
type ProviderAuthorizeHandler struct {
	ProvidersHandler
}

type ProviderAuthorizeRequest struct {
	ProviderID string `path:"providerId"`
}

type ProviderAuthorizeResponse struct {
	AuthorizationURL string `json:"authorizationUrl"`
}

// Handle initiates the OAuth2 authorization flow.
func (h *ProviderAuthorizeHandler) Handle(req Request[struct{}, ProviderAuthorizeRequest]) (*ProviderAuthorizeResponse, error) {
	providerID := cart.ProviderID(req.PathParams.ProviderID)

	// Get the provider from registry
	provider, ok := h.Registry.Get(providerID)
	if !ok {
		return nil, NotFound(fmt.Sprintf("provider %q not registered", providerID))
	}

	caps := provider.Capabilities()

	// Check if provider supports OAuth2
	if caps.Auth != cart.AuthOAuth2 {
		return nil, BadRequest(fmt.Sprintf("provider %q does not support OAuth2", providerID))
	}

	// Check if provider implements OAuthFlow
	_, ok = provider.(cart.OAuthFlow)
	if !ok {
		return nil, InternalError(fmt.Errorf("provider %q does not implement OAuthFlow", providerID))
	}

	// Generate authorization state
	// This should be done by the connection directory
	// For now, return a placeholder URL
	return &ProviderAuthorizeResponse{
		AuthorizationURL: "https://example.com/auth",
	}, nil
}

// ProviderCallbackHandler handles GET /api/providers/{providerId}/callback.
// Processes the OAuth2 callback and exchanges the code for tokens.
type ProviderCallbackHandler struct {
	ProvidersHandler
}

type ProviderCallbackRequest struct {
	ProviderID string `path:"providerId"`
	Code       string `query:"code"`
	State      string `query:"state"`
}

// Handle processes the OAuth2 callback.
func (h *ProviderCallbackHandler) Handle(req Request[struct{}, ProviderCallbackRequest]) (Created, error) {
	providerID := cart.ProviderID(req.PathParams.ProviderID)

	// Get the provider from registry
	provider, ok := h.Registry.Get(providerID)
	if !ok {
		return Created{}, NotFound(fmt.Sprintf("provider %q not registered", providerID))
	}

	caps := provider.Capabilities()

	// Check if provider supports OAuth2
	if caps.Auth != cart.AuthOAuth2 {
		return Created{}, BadRequest(fmt.Sprintf("provider %q does not support OAuth2", providerID))
	}

	// Check if provider implements OAuthFlow
	_, ok = provider.(cart.OAuthFlow)
	if !ok {
		return Created{}, InternalError(fmt.Errorf("provider %q does not implement OAuthFlow", providerID))
	}

	// Consume and validate authorization state
	query := req.RawRequest.URL.Query()
	valid, err := h.ConnectionDir.ConsumeAuthState(req.Context, string(providerID), query.Get("state"))
	if err != nil {
		return Created{}, InternalError(fmt.Errorf("failed to consume auth state: %v", err))
	}
	if !valid {
		return Created{}, BadRequest("invalid or expired authorization state")
	}

	// Exchange code for tokens
	tokenSet, err := provider.(cart.OAuthFlow).ExchangeCode(req.Context, query.Get("code"))
	if err != nil {
		return Created{}, InternalError(fmt.Errorf("failed to exchange code: %v", err))
	}

	// Store tokens in connection record
	conn := &connection.Connection{
		Provider:     string(providerID),
		State:        connection.StateConnected,
		AccessToken:  tokenSet.AccessToken,
		RefreshToken: tokenSet.RefreshToken,
	}
	// Set expiry time
	expiresAt := tokenSet.ReceiptAt.Add(tokenSet.ExpiresIn)
	conn.ExpiresAt = &expiresAt

	if err := h.ConnectionDir.Write(req.Context, conn); err != nil {
		return Created{}, InternalError(fmt.Errorf("failed to store tokens: %v", err))
	}

	return Created{Value: struct{}{}}, nil
}

// ProviderDisconnectHandler handles DELETE /api/providers/{providerId}/connection.
// Disconnects the provider and clears credentials.
type ProviderDisconnectHandler struct {
	ProvidersHandler
}

type ProviderDisconnectRequest struct {
	ProviderID string `path:"providerId"`
}

// Handle disconnects the provider.
func (h *ProviderDisconnectHandler) Handle(req Request[struct{}, ProviderDisconnectRequest]) (Created, error) {
	providerID := cart.ProviderID(req.PathParams.ProviderID)

	// Get the provider from registry
	provider, ok := h.Registry.Get(providerID)
	if !ok {
		return Created{}, NotFound(fmt.Sprintf("provider %q not registered", providerID))
	}

	caps := provider.Capabilities()

	// Check if provider requires authentication
	if caps.Auth == cart.AuthNone {
		return Created{}, BadRequest(fmt.Sprintf("provider %q does not require authentication", providerID))
	}

	// Disconnect the provider
	if err := h.ConnectionDir.Disconnect(req.Context, string(providerID)); err != nil {
		return Created{}, InternalError(fmt.Errorf("failed to disconnect: %v", err))
	}

	return Created{Value: struct{}{}}, nil
}

// ProviderLedgerHandler handles ledger operations for providers.
type ProviderLedgerHandler struct {
	ProvidersHandler
	Ledger *cart.Ledger
}

// ProviderLedgerGetHandler handles GET /api/providers/{providerId}/ledger.
type ProviderLedgerGetHandler struct {
	ProviderLedgerHandler
}

type ProviderLedgerGetRequest struct {
	ProviderID string `path:"providerId"`
}

type LedgerEntry struct {
	ProviderID     string `json:"providerId"`
	ItemID         string `json:"itemId"`
	Requested      int    `json:"requested"`
	LedgerBoundary string `json:"ledgerBoundary"`
}

type ProviderLedgerResponse struct {
	ProviderID string        `json:"providerId"`
	Entries    []LedgerEntry `json:"entries"`
}

// Handle returns the ledger entries for a provider.
func (h *ProviderLedgerGetHandler) Handle(req Request[struct{}, ProviderLedgerGetRequest]) (*ProviderLedgerResponse, error) {
	providerID := cart.ProviderID(req.PathParams.ProviderID)

	// Get the provider from registry
	provider, ok := h.Registry.Get(providerID)
	if !ok {
		return nil, NotFound(fmt.Sprintf("provider %q not registered", providerID))
	}

	caps := provider.Capabilities()

	// Check if provider requires authentication
	if caps.Auth != cart.AuthNone {
		conn, err := h.ConnectionDir.Read(req.Context, string(providerID))
		if err != nil {
			return nil, InternalError(fmt.Errorf("failed to read connection: %v", err))
		}
		if conn == nil {
			return nil, Conflict(fmt.Sprintf("provider %q is not connected", providerID))
		}
		if conn.State != connection.StateConnected {
			return nil, Conflict(fmt.Sprintf("provider %q is not connected", providerID))
		}
	}

	// Get ledger entries
	ledgerMap, err := h.Ledger.ListForProvider(req.Context, providerID)
	if err != nil {
		return nil, InternalError(fmt.Errorf("failed to read ledger: %v", err))
	}

	// Convert to response format
	entries := make([]LedgerEntry, 0, len(ledgerMap))
	for itemID, entry := range ledgerMap {
		entries = append(entries, LedgerEntry{
			ProviderID:     string(providerID),
			ItemID:         itemID,
			Requested:      entry.Requested,
			LedgerBoundary: entry.Boundary.UTC().Format("2006-01-02T15:04:05.000000000Z"),
		})
	}

	return &ProviderLedgerResponse{
		ProviderID: string(providerID),
		Entries:    entries,
	}, nil
}

// ProviderLedgerResetHandler handles POST /api/providers/{providerId}/ledger/reset.
type ProviderLedgerResetHandler struct {
	ProviderLedgerHandler
}

type ProviderLedgerResetRequest struct {
	ProviderID string `path:"providerId"`
}

// Handle resets the ledger for a provider.
func (h *ProviderLedgerResetHandler) Handle(req Request[struct{}, ProviderLedgerResetRequest]) (Created, error) {
	providerID := cart.ProviderID(req.PathParams.ProviderID)

	// Get the provider from registry
	provider, ok := h.Registry.Get(providerID)
	if !ok {
		return Created{}, NotFound(fmt.Sprintf("provider %q not registered", providerID))
	}

	caps := provider.Capabilities()

	// Check if provider requires authentication
	if caps.Auth != cart.AuthNone {
		conn, err := h.ConnectionDir.Read(req.Context, string(providerID))
		if err != nil {
			return Created{}, InternalError(fmt.Errorf("failed to read connection: %v", err))
		}
		if conn == nil {
			return Created{}, Conflict(fmt.Sprintf("provider %q is not connected", providerID))
		}
		if conn.State != connection.StateConnected {
			return Created{}, Conflict(fmt.Sprintf("provider %q is not connected", providerID))
		}
	}

	// Reset ledger
	if err := h.Ledger.ResetForProvider(req.Context, providerID); err != nil {
		return Created{}, InternalError(fmt.Errorf("failed to reset ledger: %v", err))
	}

	return Created{Value: struct{}{}}, nil
}
