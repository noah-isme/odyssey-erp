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
// service from the durable execution and result-inbox stores, plus the
// fail-closed accounting-effects boundary above. It intentionally does not
// create an ExecutionPort or register a live provider: importing a bank result
// is the only v0.11 operation that can be wired without choosing a provider
// adapter or accounting-effects service.
func NewPostgresSettlementService(pool *pgxpool.Pool) *SettlementService {
	if pool == nil {
		return nil
	}
	return NewSettlementService(
		NewPostgresStore(pool),
		NewPostgresSettlementResultStore(pool),
		NewUnsupportedSettlementEffects(),
	)
}
