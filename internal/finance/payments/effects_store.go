package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresSettlementEffects is the durable idempotency adapter for a
// transaction-backed accounting effects service. It records the effect key
// before/within the same transaction as AP/GL/bank writes in a production
// composite adapter. This small adapter is useful when those domain services
// are wired separately; it never calls a provider API.
type PostgresSettlementEffects struct {
	db database
}

var _ SettlementEffectsPort = (*PostgresSettlementEffects)(nil)

func NewPostgresSettlementEffects(db database) *PostgresSettlementEffects {
	return &PostgresSettlementEffects{db: db}
}

// NewPGSettlementEffects is the pool-typed constructor used by app wiring.
func NewPGSettlementEffects(pool *pgxpool.Pool) *PostgresSettlementEffects {
	return NewPostgresSettlementEffects(pool)
}

const insertSettlementEffectSQL = `
INSERT INTO payment_settlement_effects (
    company_id, effect_key, result_id, state, fingerprint, payload
) VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (company_id, effect_key) DO NOTHING`

func (s *PostgresSettlementEffects) ApplySettlementEffects(ctx context.Context, request SettlementEffectRequest) (SettlementEffectOutcome, error) {
	if s == nil || s.db == nil {
		return SettlementEffectOutcome{}, ErrInvalidSettlementResult
	}
	if err := request.Validate(); err != nil {
		return SettlementEffectOutcome{}, err
	}
	payload, err := json.Marshal(request.Result)
	if err != nil {
		return SettlementEffectOutcome{}, fmt.Errorf("%w: encode effect: %v", ErrInvalidSettlementResult, err)
	}
	fingerprint := settlementFingerprint(request.Result)
	commandTag, err := s.db.Exec(ctx, insertSettlementEffectSQL,
		request.CompanyID,
		request.EffectKey,
		request.Result.ResultID,
		string(request.Result.State),
		fingerprint,
		payload,
	)
	if err != nil {
		return SettlementEffectOutcome{}, err
	}
	if commandTag.RowsAffected() > 0 {
		return SettlementEffectOutcome{Applied: true}, nil
	}
	var previousFingerprint string
	err = s.db.QueryRow(ctx, `
SELECT fingerprint
FROM payment_settlement_effects
WHERE company_id = $1 AND effect_key = $2`, request.CompanyID, request.EffectKey).Scan(&previousFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return SettlementEffectOutcome{}, ErrSettlementEffectConflict
	}
	if err != nil {
		return SettlementEffectOutcome{}, err
	}
	if previousFingerprint != fingerprint {
		return SettlementEffectOutcome{}, ErrSettlementEffectConflict
	}
	return SettlementEffectOutcome{AlreadyApplied: true}, nil
}

func (s *PostgresSettlementEffects) Apply(ctx context.Context, request SettlementEffectRequest) (SettlementEffectOutcome, error) {
	return s.ApplySettlementEffects(ctx, request)
}
