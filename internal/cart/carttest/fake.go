// Package carttest provides test infrastructure for cart providers.
// It follows the net/http/httptest pattern: a non-test package that can be
// imported by both internal/cart and internal/cart/kroger tests.
package carttest

import (
	"context"
	"sync"
	"time"

	"github.com/Rhionin/pantry/internal/cart"
)

// Script queues per-request dispositions, per-line results for per_line,
// identity lookups, and token-exchange outcomes, all in memory.
type Script struct {
	mu             sync.Mutex
	dispositions   []cart.ResultDisposition
	lineResults    map[cart.ProductIdentity]bool
	identityLookups map[string]cart.ProductIdentity
	tokenExchanges  []TokenExchangeResult
}

// TokenExchangeResult records the result of a token exchange.
type TokenExchangeResult struct {
	TokenSet cart.TokenSet
	Err      error
}

// NewScript creates a new empty script.
func NewScript() *Script {
	return &Script{
		lineResults:     make(map[cart.ProductIdentity]bool),
		identityLookups: make(map[string]cart.ProductIdentity),
	}
}

// WithDispositions sets the dispositions for successive Add calls.
func (s *Script) WithDispositions(disps ...cart.ResultDisposition) *Script {
	s.dispositions = disps
	return s
}

// WithLineResults sets the per-line results for per_line confirmation.
func (s *Script) WithLineResults(results map[cart.ProductIdentity]bool) *Script {
	s.lineResults = results
	return s
}

// WithIdentityLookup records what identity should be returned for a barcode.
func (s *Script) WithIdentityLookup(barcode string, identity cart.ProductIdentity) *Script {
	s.identityLookups[barcode] = identity
	return s
}

// WithTokenExchangeSuccess records a successful token exchange result.
func (s *Script) WithTokenExchangeSuccess() *Script {
	s.tokenExchanges = append(s.tokenExchanges, TokenExchangeResult{
		TokenSet: cart.TokenSet{
			AccessToken:  "test-access-token",
			RefreshToken: "test-refresh-token",
			ExpiresIn:    3600 * time.Second,
			ReceiptAt:    time.Now(),
		},
		Err: nil,
	})
	return s
}

// WithTokenExchangeError records an error token exchange result.
func (s *Script) WithTokenExchangeError(err error) *Script {
	s.tokenExchanges = append(s.tokenExchanges, TokenExchangeResult{
		TokenSet: cart.TokenSet{},
		Err:      err,
	})
	return s
}

// FakeProvider implements cart.Provider with behavior controlled by a Script.
type FakeProvider struct {
	id           cart.ProviderID
	displayName  string
	caps         cart.Capabilities
	script       *Script
	calls        []string
	callsMu      sync.Mutex
}

// NewFake returns a provider declaring exactly caps, implementing exactly the
// narrow interfaces those capabilities require — and no others.
func NewFake(caps cart.Capabilities, script *Script) cart.Provider {
	return &FakeProvider{
		id:          cart.ProviderID("test-" + string(caps.Auth) + "-" + string(caps.Delivery)),
		displayName: "Test Provider",
		caps:        caps,
		script:      script,
	}
}

func (f *FakeProvider) ID() cart.ProviderID {
	return f.id
}

func (f *FakeProvider) DisplayName() string {
	return f.displayName
}

func (f *FakeProvider) Capabilities() cart.Capabilities {
	return f.caps
}

func (f *FakeProvider) recordCall(name string) {
	f.callsMu.Lock()
	defer f.callsMu.Unlock()
	f.calls = append(f.calls, name)
}

func (f *FakeProvider) Calls() []string {
	f.callsMu.Lock()
	defer f.callsMu.Unlock()
	// Return a copy to prevent mutation
	result := make([]string, len(f.calls))
	copy(result, f.calls)
	return result
}

// FakeOAuthFlow implements cart.OAuthFlow for testing.
type FakeOAuthFlow struct {
	script *Script
}

func (f *FakeOAuthFlow) AuthorizationScope() string {
	return "test:scope"
}

func (f *FakeOAuthFlow) AuthorizationURL(state string) (string, error) {
	return "https://example.com/auth?" + state, nil
}

func (f *FakeOAuthFlow) ExchangeCode(ctx context.Context, code string) (cart.TokenSet, error) {
	f.script.mu.Lock()
	defer f.script.mu.Unlock()
	if len(f.script.tokenExchanges) == 0 {
		return cart.TokenSet{}, nil
	}
	result := f.script.tokenExchanges[0]
	f.script.tokenExchanges = f.script.tokenExchanges[1:]
	return result.TokenSet, result.Err
}

func (f *FakeOAuthFlow) RefreshAccessToken(ctx context.Context, refreshToken string) (cart.TokenSet, error) {
	f.script.mu.Lock()
	defer f.script.mu.Unlock()
	if len(f.script.tokenExchanges) == 0 {
		return cart.TokenSet{}, nil
	}
	result := f.script.tokenExchanges[0]
	f.script.tokenExchanges = f.script.tokenExchanges[1:]
	return result.TokenSet, result.Err
}

// FakeStaticCredential implements cart.StaticCredential for testing.
type FakeStaticCredential struct {
	cred cart.Credential
}

func (f *FakeStaticCredential) Credential() cart.Credential {
	return f.cred
}

// FakeServerPush implements cart.ServerPush for testing.
type FakeServerPush struct {
	script *Script
}

func (f *FakeServerPush) Add(ctx context.Context, cred cart.Credential, req cart.ProvisionRequest) (cart.ProvisionResult, error) {
	f.script.mu.Lock()
	defer f.script.mu.Unlock()
	
	var disposition cart.ResultDisposition
	if len(f.script.dispositions) > 0 {
		disposition = f.script.dispositions[0]
		f.script.dispositions = f.script.dispositions[1:]
	} else {
		disposition = cart.DispositionAccepted
	}

	return cart.ProvisionResult{
		Disposition: disposition,
		PerLine:     f.script.lineResults,
		Status:      200,
	}, nil
}

// FakeHandoffBuilder implements cart.HandoffBuilder for testing.
type FakeHandoffBuilder struct {
	script *Script
}

func (f *FakeHandoffBuilder) BuildHandoff(req cart.ProvisionRequest) (cart.HandoffArtifact, error) {
	return cart.HandoffArtifact{
		URL:  "https://example.com/handoff",
		Data: map[string]string{"key": "value"},
	}, nil
}

// FakeDerivedIdentity implements cart.DerivedIdentity for testing.
type FakeDerivedIdentity struct {
	script *Script
}

func (f *FakeDerivedIdentity) DeriveIdentity(barcode string) (cart.ProductIdentity, bool) {
	identity, ok := f.script.identityLookups[barcode]
	return identity, ok
}

// FakeLookedUpIdentity implements cart.LookedUpIdentity for testing.
type FakeLookedUpIdentity struct {
	script *Script
}

func (f *FakeLookedUpIdentity) LookUpIdentity(ctx context.Context, cred cart.Credential, barcode string) (cart.ProductIdentity, bool, error) {
	identity, ok := f.script.identityLookups[barcode]
	return identity, ok, nil
}

// FakeLineUpdater implements cart.LineUpdater for testing.
type FakeLineUpdater struct {
	script *Script
}

func (f *FakeLineUpdater) UpdateLine(ctx context.Context, cred cart.Credential, line cart.ProvisionLine) (cart.ProvisionResult, error) {
	return cart.ProvisionResult{Disposition: cart.DispositionAccepted, Status: 200}, nil
}

// FakeLineRemover implements cart.LineRemover for testing.
type FakeLineRemover struct{}

func (f *FakeLineRemover) RemoveLine(ctx context.Context, cred cart.Credential, identity cart.ProductIdentity) (cart.ProvisionResult, error) {
	return cart.ProvisionResult{Disposition: cart.DispositionAccepted, Status: 200}, nil
}
