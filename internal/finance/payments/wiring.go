package payments

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrSettlementEffectsUnavailable is returned while the AP/GL/tax/FX/bank
// accounting composite is not wired. Returning an error keeps an imported
// result durable with effect_applied=false so a later certified adapter can
// retry it; a no-op success would incorrectly claim that money was posted.
var ErrSettlementEffectsUnavailable = errors.New("finance payments: settlement accounting effects are not configured")

// UnsupportedSettlementEffects is the deliberate v0.11 boundary for worker
// composition. It validates the request but never marks an accounting effect
// as applied and never mutates AP, GL, tax, FX, or bank records.
type UnsupportedSettlementEffects struct{}

var _ SettlementEffectsPort = UnsupportedSettlementEffects{}

func (UnsupportedSettlementEffects) ApplySettlementEffects(_ context.Context, request SettlementEffectRequest) (SettlementEffectOutcome, error) {
	if err := request.Validate(); err != nil {
		return SettlementEffectOutcome{}, err
	}
	return SettlementEffectOutcome{}, ErrSettlementEffectsUnavailable
}

func NewUnsupportedSettlementEffects() SettlementEffectsPort {
	return UnsupportedSettlementEffects{}
}

// NewPostgresSettlementService composes the provider-neutral result-import
// service from the durable execution and result-inbox stores. Callers may
// provide a transaction-scoped accounting-effects applier; omitting it keeps
// the legacy fail-closed behavior for profiles that have not enabled
// settlement posting.
func NewPostgresSettlementService(pool *pgxpool.Pool, appliers ...SettlementEffectsApplier) *SettlementService {
	if pool == nil {
		return nil
	}
	var effects SettlementEffectsPort = NewUnsupportedSettlementEffects()
	if len(appliers) > 0 && appliers[0] != nil {
		effects = NewPostgresSettlementEffectsWithApplier(pool, appliers[0])
	}
	return NewSettlementService(
		NewPostgresStore(pool),
		NewPostgresSettlementResultStore(pool),
		effects,
	)
}
