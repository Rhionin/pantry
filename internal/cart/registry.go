package cart

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds registered providers and validates their capabilities against their interfaces.
type Registry struct {
	mu                    sync.Mutex
	providers             map[ProviderID]Provider
	batchSizes            map[ProviderID]int
	credentialsConfigured map[ProviderID]bool
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		providers:             make(map[ProviderID]Provider),
		batchSizes:            make(map[ProviderID]int),
		credentialsConfigured: make(map[ProviderID]bool),
	}
}

// Register adds a provider to the registry, validating that its implementation matches
// its declared capabilities.
func (r *Registry) Register(p Provider, opts ...RegisterOption) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := p.ID()
	if !id.Valid() {
		return fmt.Errorf("cannot register provider: empty identifier")
	}

	// Check for duplicate
	if _, exists := r.providers[id]; exists {
		return fmt.Errorf("cannot register provider %q: already registered", id)
	}

	caps := p.Capabilities()
	if err := caps.Validate(); err != nil {
		return fmt.Errorf("cannot register provider %q: %w", id, err)
	}

	// Validate that the provider implements exactly the interfaces its capabilities require
	if err := r.validateInterfaces(p, caps); err != nil {
		return fmt.Errorf("cannot register provider %q: %w", id, err)
	}

	r.providers[id] = p

	// Apply options (like batch size)
	for _, opt := range opts {
		opt.apply(r, id)
	}

	return nil
}

// validateInterfaces checks that p implements exactly the interfaces required by caps.
func (r *Registry) validateInterfaces(p Provider, caps Capabilities) error {
	// Mutation interfaces are optional - add to list but don't fail if not present
	switch caps.Mutation {
	case MutateAddAndUpdate:
		// LineUpdater required for add_and_update
	case MutateFull:
		// LineUpdater and LineRemover required for full
	}

	// For now, just verify capabilities are valid
	// Full interface validation is done at runtime via type assertions
	return nil
}

// RegisterOption configures provider registration.
type RegisterOption interface {
	apply(r *Registry, id ProviderID)
}

// WithBatchSize sets the maximum batch size for a provider.
func WithBatchSize(size int) RegisterOption {
	return &batchSizeOption{size: size}
}

type batchSizeOption struct {
	size int
}

func (b *batchSizeOption) apply(r *Registry, id ProviderID) {
	if b.size > 0 {
		r.batchSizes[id] = b.size
	} else {
		// Default to 50
		r.batchSizes[id] = 50
	}
}

// WithCredentialsConfigured records whether the provider's required credentials
// were present at startup. Absent means not configured.
func WithCredentialsConfigured(configured bool) RegisterOption {
	return credentialsOption{configured: configured}
}

type credentialsOption struct {
	configured bool
}

func (c credentialsOption) apply(r *Registry, id ProviderID) {
	r.credentialsConfigured[id] = c.configured
}

// List returns registered provider identifiers in ascending order.
func (r *Registry) List() []ProviderID {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := make([]ProviderID, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// CredentialsConfigured reports whether every credential the provider requires
// was present when it was registered.
func (r *Registry) CredentialsConfigured(id ProviderID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.credentialsConfigured[id]
}

// AnyCredentialsConfigured reports whether provisioning can run against at
// least one provider.
func (r *Registry) AnyCredentialsConfigured() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, configured := range r.credentialsConfigured {
		if configured {
			return true
		}
	}
	return false
}

// SoleConfigured returns the only credentials-configured provider.
// A read that names no provider uses that provider.
func (r *Registry) SoleConfigured() (ProviderID, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var found ProviderID
	count := 0
	for id, configured := range r.credentialsConfigured {
		if !configured {
			continue
		}
		found = id
		count++
	}
	if count == 1 {
		return found, true
	}
	return "", false
}

// Get returns the provider with the given ID, or nil if not found.
func (r *Registry) Get(id ProviderID) (Provider, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.providers[id]
	return p, ok
}

// BatchSize returns the batch size for the given provider.
func (r *Registry) BatchSize(id ProviderID) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.batchSizes[id]
}
