package cart

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderID_Valid(t *testing.T) {
	tests := []struct {
		id   ProviderID
		want bool
	}{
		{"kroger", true},
		{"test-provider", true},
		{"", false},
		{"a", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.id), func(t *testing.T) {
			if got := tt.id.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProviderID_Dimension(t *testing.T) {
	id := ProviderID("kroger")
	if got := id.Dimension(); got != "provider_id" {
		t.Errorf("Dimension() = %q, want %q", got, "provider_id")
	}
}

func TestAuthCapability_Valid(t *testing.T) {
	tests := []struct {
		cap  AuthCapability
		want bool
	}{
		{AuthNone, true},
		{AuthOAuth2, true},
		{AuthAPIKey, true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.cap), func(t *testing.T) {
			if got := tt.cap.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeliveryCapability_Valid(t *testing.T) {
	tests := []struct {
		cap  DeliveryCapability
		want bool
	}{
		{DeliveryServerPush, true},
		{DeliveryClientHandoff, true},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.cap), func(t *testing.T) {
			if got := tt.cap.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfirmationCapability_Valid(t *testing.T) {
	tests := []struct {
		cap  ConfirmationCapability
		want bool
	}{
		{ConfirmPerLine, true},
		{ConfirmPerRequest, true},
		{ConfirmNone, true},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.cap), func(t *testing.T) {
			if got := tt.cap.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMutationCapability_Valid(t *testing.T) {
	tests := []struct {
		cap  MutationCapability
		want bool
	}{
		{MutateAddOnly, true},
		{MutateAddAndUpdate, true},
		{MutateFull, true},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.cap), func(t *testing.T) {
			if got := tt.cap.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIdentityCapability_Valid(t *testing.T) {
	tests := []struct {
		cap  IdentityCapability
		want bool
	}{
		{IdentityDerived, true},
		{IdentityLookedUp, true},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.cap), func(t *testing.T) {
			if got := tt.cap.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCapabilities_Validate(t *testing.T) {
	tests := []struct {
		name    string
		caps    Capabilities
		wantErr bool
	}{
		{"valid capabilities", Capabilities{
			Auth:         AuthOAuth2,
			Delivery:     DeliveryServerPush,
			Confirmation: ConfirmPerRequest,
			Mutation:     MutateAddOnly,
			Identity:     IdentityDerived,
		}, false},
		{"invalid auth", Capabilities{
			Auth:         "invalid",
			Delivery:     DeliveryServerPush,
			Confirmation: ConfirmPerRequest,
			Mutation:     MutateAddOnly,
			Identity:     IdentityDerived,
		}, true},
		{"empty all fields", Capabilities{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.caps.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNoCredential_String(t *testing.T) {
	var nc NoCredential
	if got := nc.String(); got != "none" {
		t.Errorf("String() = %q, want %q", got, "none")
	}
}

func TestBearerCredential_String(t *testing.T) {
	bc := BearerCredential{token: "secret-token"}
	if got := bc.String(); got != "bearer(redacted)" {
		t.Errorf("String() = %q, want %q", got, "bearer(redacted)")
	}
}

func TestHeaderCredential_String(t *testing.T) {
	hc := HeaderCredential{name: "X-API-Key", value: "secret"}
	if got := hc.String(); got != "X-API-Key=redacted" {
		t.Errorf("String() = %q, want %q", got, "X-API-Key=redacted")
	}
}

func TestRegistry_Register_Validation(t *testing.T) {
	// Test duplicate registration
	registry := NewRegistry()
	registry.Register(&mockProvider{id: "test"})
	if err := registry.Register(&mockProvider{id: "test"}); err == nil {
		t.Error("duplicate registration should fail")
	}
}

type mockProvider struct {
	id string
}

func (m *mockProvider) ID() ProviderID      { return ProviderID(m.id) }
func (m *mockProvider) DisplayName() string { return "Mock" }
func (m *mockProvider) Capabilities() Capabilities {
	return Capabilities{
		Auth:         AuthNone,
		Delivery:     DeliveryServerPush,
		Confirmation: ConfirmNone,
		Mutation:     MutateAddOnly,
		Identity:     IdentityDerived,
	}
}

// Each capability type reports the dimension name used in error messages.
func TestCapability_Dimension(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"auth", AuthNone.Dimension(), "auth"},
		{"delivery", DeliveryServerPush.Dimension(), "delivery"},
		{"confirmation", ConfirmPerLine.Dimension(), "confirmation"},
		{"mutation", MutateFull.Dimension(), "mutation"},
		{"identity", IdentityDerived.Dimension(), "identity"},
		{"provider_id", ProviderID("kroger").Dimension(), "provider_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("Dimension() = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func newRequest(t *testing.T) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "https://example.com/cart", nil)
	return req
}

// NoCredential.Apply leaves the request headers untouched.
func TestNoCredential_Apply(t *testing.T) {
	req := newRequest(t)
	NoCredential{}.Apply(req)
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want empty", got)
	}
}

// NewBearerCredential builds a credential whose Apply sets a bearer Authorization header.
func TestNewBearerCredential_Apply(t *testing.T) {
	req := newRequest(t)
	NewBearerCredential("tok-123").Apply(req)
	if got := req.Header.Get("Authorization"); got != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer tok-123")
	}
}

// HeaderCredential.Apply sets its configured header to its value.
func TestHeaderCredential_Apply(t *testing.T) {
	req := newRequest(t)
	HeaderCredential{name: "X-API-Key", value: "secret-value"}.Apply(req)
	if got := req.Header.Get("X-API-Key"); got != "secret-value" {
		t.Errorf("X-API-Key = %q, want %q", got, "secret-value")
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want empty", got)
	}
}
