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
	db      database
	applier SettlementEffectsApplier
}

var _ SettlementEffectsPort = (*PostgresSettlementEffects)(nil)

func NewPostgresSettlementEffects(db database) *PostgresSettlementEffects {
	return &PostgresSettlementEffects{db: db}
}

// SettlementEffectsApplier performs the domain mutations that are owned by
// AP/accounting/banking while the settlement claim is open. It returns the
// durable links that identify those mutations. The adapter invokes it only
// with the caller transaction, so an error rolls back both the claim and all
// financial writes. A production composition layer can implement this port
// without making the payments package depend on AP or accounting packages.
type SettlementEffectsApplier interface {
	ApplySettlementEffectsTx(context.Context, pgx.Tx, SettlementEffectRequest) ([]SettlementEffectLink, error)
}

// SettlementEffectsApplierFunc adapts a function to SettlementEffectsApplier.
type SettlementEffectsApplierFunc func(context.Context, pgx.Tx, SettlementEffectRequest) ([]SettlementEffectLink, error)

func (f SettlementEffectsApplierFunc) ApplySettlementEffectsTx(ctx context.Context, tx pgx.Tx, request SettlementEffectRequest) ([]SettlementEffectLink, error) {
	if f == nil {
		return nil, ErrSettlementEffectsTransactionRequired
	}
	return f(ctx, tx, request)
}

// NewPostgresSettlementEffectsWithApplier enables the transaction-backed
// composite path. NewPostgresSettlementEffects remains available for the
// durable idempotency-only path used by existing callers.
func NewPostgresSettlementEffectsWithApplier(db database, applier SettlementEffectsApplier) *PostgresSettlementEffects {
	return &PostgresSettlementEffects{db: db, applier: applier}
}

// NewPGSettlementEffects is the pool-typed constructor used by app wiring.
func NewPGSettlementEffects(pool *pgxpool.Pool) *PostgresSettlementEffects {
	return NewPostgresSettlementEffects(pool)
}

func NewPGSettlementEffectsWithApplier(pool *pgxpool.Pool, applier SettlementEffectsApplier) *PostgresSettlementEffects {
	return NewPostgresSettlementEffectsWithApplier(pool, applier)
}

const insertSettlementEffectSQL = `
INSERT INTO payment_settlement_effects (
    company_id, effect_key, result_id, state, fingerprint, payload
) VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (company_id, effect_key) DO NOTHING`

const insertSettlementEffectLinkSQL = `
INSERT INTO payment_settlement_effect_links (
    company_id, effect_key, result_id, link_type, entity_type, entity_id,
    amount, currency, metadata
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (company_id, effect_key, link_type, entity_type, entity_id) DO NOTHING`

func (s *PostgresSettlementEffects) ApplySettlementEffects(ctx context.Context, request SettlementEffectRequest) (SettlementEffectOutcome, error) {
	if s == nil || s.db == nil {
		return SettlementEffectOutcome{}, ErrInvalidSettlementResult
	}
	if err := request.Validate(); err != nil {
		return SettlementEffectOutcome{}, err
	}
	if _, transactionCapable := s.db.(transactionDatabase); !transactionCapable && (s.applier != nil || len(request.Links) > 0) {
		return SettlementEffectOutcome{}, ErrSettlementEffectsTransactionRequired
	}
	payload, err := json.Marshal(request.Result)
	if err != nil {
		return SettlementEffectOutcome{}, fmt.Errorf("%w: encode effect: %v", ErrInvalidSettlementResult, err)
	}
	fingerprint := settlementEffectFingerprint(request)

	if txdb, ok := s.db.(transactionDatabase); ok {
		tx, err := txdb.Begin(ctx)
		if err != nil {
			return SettlementEffectOutcome{}, err
		}
		outcome, err := s.applyOn(ctx, tx, request, fingerprint, payload)
		if err != nil {
			_ = tx.Rollback(ctx)
			return SettlementEffectOutcome{}, err
		}
		if outcome.Applied {
			links := append([]SettlementEffectLink(nil), request.Links...)
			if s.applier != nil {
				created, applyErr := s.applier.ApplySettlementEffectsTx(ctx, tx, request)
				if applyErr != nil {
					_ = tx.Rollback(ctx)
					return SettlementEffectOutcome{}, applyErr
				}
				if err := validateSettlementEffectLinks(request.Result, created); err != nil {
					_ = tx.Rollback(ctx)
					return SettlementEffectOutcome{}, err
				}
				links = append(links, created...)
			}
			if err := validateSettlementEffectLinks(request.Result, links); err != nil {
				_ = tx.Rollback(ctx)
				return SettlementEffectOutcome{}, err
			}
			if err := insertSettlementEffectLinks(ctx, tx, request, links); err != nil {
				_ = tx.Rollback(ctx)
				return SettlementEffectOutcome{}, err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return SettlementEffectOutcome{}, err
		}
		return outcome, nil
	}

	return s.applyOn(ctx, s.db, request, fingerprint, payload)
}

func (s *PostgresSettlementEffects) applyOn(ctx context.Context, db database, request SettlementEffectRequest, fingerprint string, payload []byte) (SettlementEffectOutcome, error) {
	commandTag, err := db.Exec(ctx, insertSettlementEffectSQL,
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
	err = db.QueryRow(ctx, `
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

func validateSettlementEffectLinks(result SettlementResult, links []SettlementEffectLink) error {
	seen := make(map[string]struct{}, len(links))
	for _, link := range links {
		if err := link.Validate(result); err != nil {
			return err
		}
		key := link.LinkType + "\x00" + link.EntityType + "\x00" + link.EntityID
		if _, ok := seen[key]; ok {
			return fmt.Errorf("%w: duplicate settlement effect link", ErrSettlementEffectConflict)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func insertSettlementEffectLinks(ctx context.Context, db database, request SettlementEffectRequest, links []SettlementEffectLink) error {
	for _, link := range links {
		var amount, currency any
		if hasExactAmount(link.Amount) {
			amount = link.Amount.Amount.String()
			currency = link.Amount.Currency
		}
		metadata := []byte(`{}`)
		if link.Metadata != nil {
			var err error
			metadata, err = json.Marshal(link.Metadata)
			if err != nil {
				return fmt.Errorf("%w: encode settlement effect link metadata: %v", ErrInvalidSettlementResult, err)
			}
		}
		if _, err := db.Exec(ctx, insertSettlementEffectLinkSQL,
			request.CompanyID,
			request.EffectKey,
			request.Result.ResultID,
			link.LinkType,
			link.EntityType,
			link.EntityID,
			amount,
			currency,
			metadata,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresSettlementEffects) Apply(ctx context.Context, request SettlementEffectRequest) (SettlementEffectOutcome, error) {
	return s.ApplySettlementEffects(ctx, request)
}
