package payments

import (
	"context"
	"fmt"
	"strings"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

// ErrProviderUnavailable means that a connection's provider has no execution
// adapter in the current process. It is intentionally distinct from a
// provider error so an outbox worker can dead-letter configuration mistakes
// without retrying a remote request.
var ErrProviderUnavailable = fmt.Errorf("%w: payment execution provider is unavailable", ErrInvalidCoordinator)

// ProviderRouter dispatches the provider-neutral execution port by the
// company-owned connection reference. It keeps the coordinator independent
// from connector registration and makes it impossible for a provider adapter
// to receive a reference for a different provider.
type ProviderRouter struct {
	ports map[string]ExecutionPort
}

// NewProviderRouter creates a router from provider names to execution ports.
// Names are normalized to lower case and '-' is treated as '_', which allows
// the persisted Midtrans Iris names to remain compatible across migrations.
func NewProviderRouter(ports map[string]ExecutionPort) *ProviderRouter {
	router := &ProviderRouter{ports: make(map[string]ExecutionPort, len(ports))}
	for name, port := range ports {
		if port == nil {
			continue
		}
		router.ports[normalizeProviderName(name)] = port
	}
	return router
}

func (r *ProviderRouter) port(ref automation.ConnectionRef) (ExecutionPort, error) {
	if r == nil || len(r.ports) == 0 {
		return nil, ErrProviderUnavailable
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	port := r.ports[normalizeProviderName(ref.Provider)]
	if port == nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderUnavailable, ref.Provider)
	}
	return port, nil
}

func (r *ProviderRouter) ValidateConnection(ctx context.Context, ref automation.ConnectionRef) error {
	port, err := r.port(ref)
	if err != nil {
		return err
	}
	return port.ValidateConnection(ctx, ref)
}

func (r *ProviderRouter) Submit(ctx context.Context, ref automation.ConnectionRef, instruction Instruction) (Submission, error) {
	port, err := r.port(ref)
	if err != nil {
		return Submission{}, err
	}
	return port.Submit(ctx, ref, instruction)
}

func (r *ProviderRouter) Lookup(ctx context.Context, ref automation.ConnectionRef, payoutRef automation.ExternalReference) (Settlement, error) {
	port, err := r.port(ref)
	if err != nil {
		return Settlement{}, err
	}
	return port.Lookup(ctx, ref, payoutRef)
}

// LookupWithInstruction forwards the optional durable-reference extension to
// a provider adapter. Adapters without the extension retain the legacy local
// reference behavior.
func (r *ProviderRouter) LookupWithInstruction(ctx context.Context, ref automation.ConnectionRef, instructionRef, payoutRef automation.ExternalReference) (Settlement, error) {
	port, err := r.port(ref)
	if err != nil {
		return Settlement{}, err
	}
	if durable, ok := port.(InstructionReferenceLookup); ok {
		return durable.LookupWithInstruction(ctx, ref, instructionRef, payoutRef)
	}
	return port.Lookup(ctx, ref, instructionRef)
}

func (r *ProviderRouter) Cancel(ctx context.Context, ref automation.ConnectionRef, payoutRef automation.ExternalReference) (Settlement, error) {
	port, err := r.port(ref)
	if err != nil {
		return Settlement{}, err
	}
	return port.Cancel(ctx, ref, payoutRef)
}

// CancelWithInstruction forwards the optional durable-reference cancellation
// extension to a provider adapter.
func (r *ProviderRouter) CancelWithInstruction(ctx context.Context, ref automation.ConnectionRef, instructionRef, payoutRef automation.ExternalReference) (Settlement, error) {
	port, err := r.port(ref)
	if err != nil {
		return Settlement{}, err
	}
	if durable, ok := port.(InstructionReferenceCanceller); ok {
		return durable.CancelWithInstruction(ctx, ref, instructionRef, payoutRef)
	}
	return port.Cancel(ctx, ref, instructionRef)
}

func (r *ProviderRouter) GenerateFile(ctx context.Context, ref automation.ConnectionRef, instructions []Instruction) (ExportArtifact, error) {
	port, err := r.port(ref)
	if err != nil {
		return ExportArtifact{}, err
	}
	return port.GenerateFile(ctx, ref, instructions)
}

func normalizeProviderName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	return value
}
