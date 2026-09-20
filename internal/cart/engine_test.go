package cart

import (
	"database/sql"
	"testing"

	"github.com/Rhionin/pantry/internal/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestEngine_RegistryValidation(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = app.RunMigrations(db)
	require.NoError(t, err)

	registry := NewRegistry()

	// Create a provider with valid capabilities
	provider := &MockProvider{
		id:         ProviderID("test"),
		caps:       Capabilities{Auth: AuthNone, Delivery: DeliveryServerPush, Confirmation: ConfirmPerRequest, Mutation: MutateAddOnly, Identity: IdentityDerived},
	}

	err = registry.Register(provider)
	assert.NoError(t, err)

	// Try to register duplicate - should fail
	err = registry.Register(provider)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

type MockProvider struct {
	id         ProviderID
	displayName string
	caps       Capabilities
}

func (p *MockProvider) ID() ProviderID             { return p.id }
func (p *MockProvider) DisplayName() string        { return p.displayName }
func (p *MockProvider) Capabilities() Capabilities { return p.caps }
