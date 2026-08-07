package connectors

import (
	"fmt"
	"sync"
)

// DefaultRegistry is an in-memory thread-safe registry of provider adapters.
type DefaultRegistry struct {
	mu       sync.RWMutex
	adapters map[string]ProviderAdapter
}

// NewRegistry creates an empty DefaultRegistry.
func NewRegistry() *DefaultRegistry {
	return &DefaultRegistry{
		adapters: make(map[string]ProviderAdapter),
	}
}

// Register adds a provider adapter to the registry under the given name.
func (r *DefaultRegistry) Register(name string, adapter ProviderAdapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[name] = adapter
}

// GetAdapter retrieves a registered provider adapter.
func (r *DefaultRegistry) GetAdapter(provider string) (ProviderAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	adapter, exists := r.adapters[provider]
	if !exists {
		return nil, fmt.Errorf("connectors: adapter not found for provider %q", provider)
	}
	return adapter, nil
}
