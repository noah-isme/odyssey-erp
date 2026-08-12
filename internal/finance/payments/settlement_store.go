package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresSettlementResultStore is the durable result inbox. It stores the
// normalized result payload and immutable fingerprint; the unique company and
// result key makes callback/file replay safe across worker processes.
type PostgresSettlementResultStore struct {
	db database
}

var _ SettlementResultStore = (*PostgresSettlementResultStore)(nil)

func NewPostgresSettlementResultStore(db database) *PostgresSettlementResultStore {
	return &PostgresSettlementResultStore{db: db}
}

// NewPGSettlementResultStore is the pool-typed constructor used by app wiring.
func NewPGSettlementResultStore(pool *pgxpool.Pool) *PostgresSettlementResultStore {
	return NewPostgresSettlementResultStore(pool)
}

const getSettlementResultSQL = `
SELECT payload, fingerprint, effect_applied, recorded_at
FROM payment_settlement_results
WHERE company_id = $1 AND result_id = $2`

const insertSettlementResultSQL = `
INSERT INTO payment_settlement_results (
    company_id, result_id, connection_id, provider, instruction_type,
    instruction_id, provider_object_type, provider_object_id, status, state,
    effect_applied, fingerprint, payload, recorded_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
ON CONFLICT (company_id, result_id) DO NOTHING`

func (s *PostgresSettlementResultStore) GetSettlementResult(ctx context.Context, companyID int64, resultID string) (SettlementResultRecord, error) {
	if s == nil || s.db == nil || companyID <= 0 || strings.TrimSpace(resultID) == "" {
		return SettlementResultRecord{}, ErrInvalidSettlementResult
	}
	var (
		payload       []byte
		fingerprint   string
		effectApplied bool
		recordedAt    time.Time
	)
	err := s.db.QueryRow(ctx, getSettlementResultSQL, companyID, resultID).Scan(&payload, &fingerprint, &effectApplied, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SettlementResultRecord{}, ErrSettlementResultNotFound
	}
	if err != nil {
		return SettlementResultRecord{}, err
	}
	var result SettlementResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return SettlementResultRecord{}, fmt.Errorf("%w: decode result: %v", ErrInvalidSettlementResult, err)
	}
	result.EffectApplied = effectApplied
	record := SettlementResultRecord{
		Result:        result,
		Fingerprint:   fingerprint,
		EffectApplied: effectApplied,
		RecordedAt:    recordedAt,
	}
	if err := record.Validate(); err != nil {
		return SettlementResultRecord{}, err
	}
	return record, nil
}

func (s *PostgresSettlementResultStore) PutSettlementResult(ctx context.Context, record SettlementResultRecord) error {
	if s == nil || s.db == nil {
		return ErrInvalidSettlementResult
	}
	if err := record.Validate(); err != nil {
		return err
	}
	if record.RecordedAt.IsZero() {
		record.RecordedAt = time.Now().UTC()
	}
	record.Result.RecordedAt = record.RecordedAt
	payload, err := json.Marshal(record.Result)
	if err != nil {
		return fmt.Errorf("%w: encode result: %v", ErrInvalidSettlementResult, err)
	}
	ref := record.Result.InstructionReference
	providerType, providerID := "", ""
	if !isZeroReference(record.Result.ProviderReference) {
		providerType = record.Result.ProviderReference.ObjectType
		providerID = record.Result.ProviderReference.ObjectID
	}
	commandTag, err := s.db.Exec(ctx, insertSettlementResultSQL,
		record.Result.CompanyID,
		record.Result.ResultID,
		ref.Connection.ConnectionID,
		ref.Connection.Provider,
		ref.ObjectType,
		ref.ObjectID,
		providerType,
		providerID,
		record.Result.Status,
		string(record.Result.State),
		record.EffectApplied,
		record.Fingerprint,
		payload,
		record.RecordedAt,
	)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return ErrDuplicateSettlementResult
	}
	return nil
}

func (s *PostgresSettlementResultStore) MarkSettlementEffectApplied(ctx context.Context, companyID int64, resultID string) error {
	if s == nil || s.db == nil || companyID <= 0 || strings.TrimSpace(resultID) == "" {
		return ErrInvalidSettlementResult
	}
	commandTag, err := s.db.Exec(ctx, `
UPDATE payment_settlement_results
SET effect_applied = TRUE,
    payload = jsonb_set(payload, '{effect_applied}', 'true'::jsonb, TRUE)
WHERE company_id = $1 AND result_id = $2 AND effect_applied = FALSE`, companyID, resultID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() > 0 {
		return nil
	}
	record, err := s.GetSettlementResult(ctx, companyID, resultID)
	if err != nil {
		return err
	}
	if record.EffectApplied {
		return nil
	}
	return ErrSettlementResultNotFound
}

// Keep the compile-time dependency explicit for database adapters used by
// pgxmock and pgxpool. The common interface itself is declared with the
// payment execution store.
var _ interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
} = (*pgxpool.Pool)(nil)
