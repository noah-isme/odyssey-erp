package bankfeeds

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

// ErrProviderUnavailable means that a bank connection has no provider
// adapter in the current process. It is deliberately distinct from a remote
// provider failure: callers must surface this as a configuration gap and must
// not retry or pretend that a sync succeeded.
var ErrProviderUnavailable = errors.New("bankfeeds: provider adapter unavailable")

// ErrInvalidProviderRegistration means the process supplied an ambiguous
// provider registry. A normalized-name collision is never resolved by map
// iteration order; the router fails closed until configuration is corrected.
var ErrInvalidProviderRegistration = errors.New("bankfeeds: invalid provider registration")

// ProviderRouter resolves provider-neutral bank-feed ports by the
// company-scoped connection reference. The application currently constructs
// an empty router because no live bank adapter has been approved; an empty
// router therefore fails closed for every sync and webhook operation.
type ProviderRouter struct {
	ports           map[string]FeedPort
	registrationErr error
}

// NewProviderRouter creates a router from explicitly registered provider
// names. Names are normalized to lower case and '-' is treated as '_', while
// nil adapters and blank names are ignored so an accidental registration can
// never create a success path without an implementation.
func NewProviderRouter(ports map[string]FeedPort) *ProviderRouter {
	router := &ProviderRouter{ports: make(map[string]FeedPort, len(ports))}
	names := make([]string, 0, len(ports))
	for name := range ports {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		port := ports[name]
		name = normalizeProviderName(name)
		if name == "" || port == nil {
			continue
		}
		if _, exists := router.ports[name]; exists {
			router.registrationErr = errors.Join(router.registrationErr, fmt.Errorf("%w: duplicate normalized provider %q", ErrInvalidProviderRegistration, name))
			continue
		}
		router.ports[name] = port
	}
	return router
}

// Providers returns the normalized provider names registered in the router.
// It is intended for startup diagnostics and tests; callers cannot mutate the
// underlying registry.
func (r *ProviderRouter) Providers() []string {
	if r == nil || r.registrationErr != nil {
		return nil
	}
	providers := make([]string, 0, len(r.ports))
	for name := range r.ports {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	return providers
}

func (r *ProviderRouter) port(ref automation.ConnectionRef) (FeedPort, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if r == nil || r.registrationErr != nil || len(r.ports) == 0 {
		if r != nil && r.registrationErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrProviderUnavailable, r.registrationErr)
		}
		return nil, ErrProviderUnavailable
	}
	port := r.ports[normalizeProviderName(ref.Provider)]
	if port == nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderUnavailable, ref.Provider)
	}
	return port, nil
}

// ValidateConnection forwards the provider health/credential check through
// the same normalized lookup used by sync and webhook operations. Keeping the
// check on the router prevents callers from accidentally bypassing provider
// registration and invoking an adapter with an invalid company reference.
func (r *ProviderRouter) ValidateConnection(ctx context.Context, ref automation.ConnectionRef) error {
	port, err := r.port(ref)
	if err != nil {
		return err
	}
	return port.ValidateConnection(ctx, ref)
}

func normalizeProviderName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.ReplaceAll(value, "-", "_")
}
