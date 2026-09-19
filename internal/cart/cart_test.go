package cart

import (
	"testing"
)

func TestProviderID_Valid(t *testing.T) {
	tests := []struct {
		id      ProviderID
		want    bool
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
		cap     AuthCapability
		want    bool
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
		cap     DeliveryCapability
		want    bool
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
		cap     ConfirmationCapability
		want    bool
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
		cap     MutationCapability
		want    bool
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
		cap     IdentityCapability
		want    bool
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
