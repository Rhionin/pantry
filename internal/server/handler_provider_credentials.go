package server

import (
	"context"
	"net/url"
	"strings"

	"github.com/Rhionin/pantry/internal/cart"
	"github.com/Rhionin/pantry/internal/cart/appcred"
)

type providerCredentialParams struct {
	ProviderID string `json:"providerId"`
}

type providerCredentialBody struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	RedirectURI  string `json:"redirectUri"`
	Modality     string `json:"modality"`
}

// ProviderCredentialPutHandler handles PUT /api/providers/{providerId}/credentials.
type ProviderCredentialPutHandler struct {
	ProvidersHandler
}

// ProviderCredentialDeleteHandler handles DELETE /api/providers/{providerId}/credentials.
type ProviderCredentialDeleteHandler struct {
	ProvidersHandler
}

func (h *ProviderCredentialPutHandler) Handle(req Request[providerCredentialBody, providerCredentialParams]) (*CredentialView, error) {
	provider, sink, err := h.credentialSink(req.PathParams.ProviderID)
	if err != nil {
		return nil, err
	}
	if h.EnvFallback.Disabled {
		return nil, Conflict(provider.DisplayName() + " is disabled")
	}

	clientID := strings.TrimSpace(req.Body.ClientID)
	secret := strings.TrimSpace(req.Body.ClientSecret)
	redirectURI := strings.TrimSpace(req.Body.RedirectURI)
	modality := strings.ToUpper(strings.TrimSpace(req.Body.Modality))
	if modality == "" {
		modality = "PICKUP"
	}
	if clientID == "" {
		return nil, &HTTPError{Code: 422, Message: "Client ID is required."}
	}
	if modality != "PICKUP" && modality != "DELIVERY" {
		return nil, &HTTPError{Code: 422, Message: "Modality must be PICKUP or DELIVERY."}
	}
	if !acceptableRedirectURI(redirectURI) {
		return nil, &HTTPError{Code: 422, Message: "Redirect URI must be https, or http on localhost."}
	}

	providerID := string(provider.ID())
	if secret == "" && h.Credentials != nil {
		existing, ok, loadErr := h.Credentials.Load(req.Context, providerID)
		if loadErr != nil {
			return nil, InternalError(loadErr)
		}
		if ok {
			secret = existing.ClientSecret
		}
	}
	if secret == "" {
		return nil, &HTTPError{Code: 422, Message: "Client secret is required."}
	}
	if h.Credentials == nil {
		return nil, errCredentialStore
	}
	if err := h.Credentials.Save(req.Context, appcred.Saved{
		ProviderID:   providerID,
		ClientID:     clientID,
		ClientSecret: secret,
		RedirectURI:  redirectURI,
		Modality:     modality,
	}); err != nil {
		return nil, InternalError(err)
	}
	if err := sink.ApplyAppCredentials(clientID, secret, redirectURI, modality); err != nil {
		return nil, &HTTPError{Code: 422, Message: "Those credentials could not be applied."}
	}
	if h.Registry != nil {
		h.Registry.SetCredentialsConfigured(provider.ID(), true)
	}
	view := CredentialView{
		ClientID:    clientID,
		RedirectURI: redirectURI,
		Modality:    modality,
		SecretSet:   true,
		Source:      "saved",
	}
	return &view, nil
}

func (h *ProviderCredentialDeleteHandler) Handle(req Request[struct{}, providerCredentialParams]) (*CredentialView, error) {
	provider, sink, err := h.credentialSink(req.PathParams.ProviderID)
	if err != nil {
		return nil, err
	}
	if h.Credentials != nil {
		if err := h.Credentials.Clear(req.Context, string(provider.ID())); err != nil {
			return nil, InternalError(err)
		}
	}
	if h.EnvFallback.Complete() {
		if err := sink.ApplyAppCredentials(h.EnvFallback.ClientID, h.EnvFallback.ClientSecret, h.EnvFallback.RedirectURI, h.EnvFallback.Modality); err != nil {
			return nil, InternalError(err)
		}
		if h.Registry != nil {
			h.Registry.SetCredentialsConfigured(provider.ID(), true)
		}
	} else {
		sink.ClearAppCredentials()
		if h.Registry != nil {
			h.Registry.SetCredentialsConfigured(provider.ID(), false)
		}
	}
	view, err := h.credentialView(req.Context, provider)
	if err != nil {
		return nil, InternalError(err)
	}
	return &view, nil
}

var errCredentialStore = &HTTPError{Code: 500, Message: "Credential store is not available."}

// acceptableRedirectURI is the OAuth redirect the browser will be sent to with
// the authorization code in the query string. https is required except for
// loopback, so a public hostname cannot receive that code over cleartext.
func acceptableRedirectURI(raw string) bool {
	if raw == "" || strings.ContainsAny(raw, " \t\r\n\\") {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	switch parsed.Scheme {
	case "https":
		return host != ""
	case "http":
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	default:
		return false
	}
}

func (h *ProvidersHandler) credentialSink(providerID string) (cart.Provider, cart.AppCredentialSink, error) {
	if h.Registry == nil {
		return nil, nil, NotFound("provider is not registered")
	}
	provider, ok := h.Registry.Get(cart.ProviderID(providerID))
	if !ok {
		return nil, nil, NotFound("provider is not registered")
	}
	sink, ok := provider.(cart.AppCredentialSink)
	if !ok {
		return nil, nil, &HTTPError{Code: 422, Message: "This provider does not take application credentials."}
	}
	return provider, sink, nil
}

func (h *ProvidersHandler) credentialView(ctx context.Context, provider cart.Provider) (CredentialView, error) {
	if h.Credentials != nil {
		saved, ok, err := h.Credentials.Public(ctx, string(provider.ID()))
		if err != nil {
			return CredentialView{}, err
		}
		if ok {
			return CredentialView{
				ClientID:    saved.ClientID,
				RedirectURI: saved.RedirectURI,
				Modality:    saved.Modality,
				SecretSet:   saved.SecretSet,
				Source:      "saved",
			}, nil
		}
	}
	sink, ok := provider.(cart.AppCredentialSink)
	if !ok {
		return CredentialView{Source: "none", Modality: "PICKUP"}, nil
	}
	snap := sink.PublicAppCredentials()
	source := "none"
	if h.Registry != nil && h.Registry.CredentialsConfigured(provider.ID()) {
		source = "environment"
	}
	modality := snap.Modality
	if modality == "" {
		modality = "PICKUP"
	}
	return CredentialView{
		ClientID:    snap.ClientID,
		RedirectURI: snap.RedirectURI,
		Modality:    modality,
		SecretSet:   snap.SecretSet && source == "environment",
		Source:      source,
	}, nil
}
