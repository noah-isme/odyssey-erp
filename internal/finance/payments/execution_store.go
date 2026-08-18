package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

var (
	// ErrInvalidExecution means that a snapshot cannot be safely persisted.
	ErrInvalidExecution = errors.New("finance payments: invalid execution")
	// ErrCrossCompanyReference means that a nested provider reference does not
	// belong to the instruction's company-owned connection.
	ErrCrossCompanyReference = errors.New("finance payments: cross-company reference")
	// ErrExecutionConflict means that another coordinator instance created or
	// advanced this execution after the caller read it.
	ErrExecutionConflict = errors.New("finance payments: execution version conflict")
	// ErrConcurrentExecution is a descriptive alias for callers that prefer
	// optimistic-concurrency terminology.
	ErrConcurrentExecution = ErrExecutionConflict
)

type database interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// transactionDatabase is intentionally kept separate from database so the
// existing lightweight adapters and fakes remain source compatible. A pool
// (and pgxmock pool) implements Begin; callers that only expose the small
// database interface still get the legacy idempotency-only behavior.
type transactionDatabase interface {
	database
	Begin(context.Context) (pgx.Tx, error)
}

// PostgresStore stores the complete provider-neutral execution snapshot in
// JSONB while keeping the canonical reference and workflow state queryable.
// Save uses one conditional statement, so an insert/update is atomic and a
// stale coordinator cannot overwrite a newer snapshot.
type PostgresStore struct {
	db database
}

var _ ExecutionStore = (*PostgresStore)(nil)

// NewPostgresStore accepts the small database interface used throughout the
// finance repositories. *pgxpool.Pool and pgxmock pools both satisfy it.
func NewPostgresStore(db database) *PostgresStore {
	return &PostgresStore{db: db}
}

const getPaymentExecutionSQL = `
SELECT payload, state, version, created_at, updated_at
FROM payment_executions
WHERE company_id = $1
  AND connection_id = $2
  AND provider = $3
  AND object_type = $4
  AND object_id = $5`

// The candidate CTE makes the two Save paths mutually exclusive. A version-0
// snapshot may only insert; a versioned snapshot may only update the exact
// version it was read with. A zero-row result is therefore either a duplicate
// initial insert or an optimistic-concurrency conflict.
const savePaymentExecutionSQL = `
WITH candidate AS (
    SELECT
        $1::bigint AS company_id,
        $2::bigint AS connection_id,
        $3::text AS provider,
        $4::text AS object_type,
        $5::text AS object_id,
        $6::text AS state,
        $7::bigint AS expected_version,
        $8::jsonb AS payload,
        $9::timestamptz AS created_at,
        $10::timestamptz AS updated_at
),
inserted AS (
    INSERT INTO payment_executions (
        company_id, connection_id, provider, object_type, object_id,
        state, version, payload, created_at, updated_at
    )
    SELECT company_id, connection_id, provider, object_type, object_id,
           state, 1, payload, created_at, updated_at
    FROM candidate
    WHERE expected_version = 0
    ON CONFLICT (company_id, connection_id, provider, object_type, object_id)
    DO NOTHING
    RETURNING version
),
updated AS (
    UPDATE payment_executions AS p
    SET state = candidate.state,
        version = p.version + 1,
        payload = candidate.payload,
        updated_at = candidate.updated_at
    FROM candidate
    WHERE candidate.expected_version > 0
      AND p.company_id = candidate.company_id
      AND p.connection_id = candidate.connection_id
      AND p.provider = candidate.provider
      AND p.object_type = candidate.object_type
      AND p.object_id = candidate.object_id
      AND p.version = candidate.expected_version
    RETURNING p.version
)
SELECT version FROM inserted
UNION ALL
SELECT version FROM updated`

func (s *PostgresStore) Get(ctx context.Context, reference automation.ExternalReference) (PaymentExecution, error) {
	if s == nil || s.db == nil {
		return PaymentExecution{}, ErrInvalidCoordinator
	}
	if err := reference.Validate(); err != nil {
		return PaymentExecution{}, err
	}

	var (
		payload              []byte
		state                string
		version              int64
		createdAt, updatedAt time.Time
	)
	err := s.db.QueryRow(ctx, getPaymentExecutionSQL,
		reference.Connection.CompanyID,
		reference.Connection.ConnectionID,
		reference.Connection.Provider,
		reference.ObjectType,
		reference.ObjectID,
	).Scan(&payload, &state, &version, &createdAt, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentExecution{}, ErrExecutionNotFound
	}
	if err != nil {
		return PaymentExecution{}, err
	}

	var execution PaymentExecution
	if err := json.Unmarshal(payload, &execution); err != nil {
		return PaymentExecution{}, fmt.Errorf("%w: decode payload: %v", ErrInvalidExecution, err)
	}
	// Canonical columns are authoritative for fields used by the lookup and
	// optimistic lock. They also prevent a stale or hand-edited JSON payload
	// from smuggling a different state/version into the coordinator.
	execution.State = ExecutionState(state)
	execution.Version = version
	execution.CreatedAt = createdAt
	execution.UpdatedAt = updatedAt
	if err := validatePaymentExecution(execution); err != nil {
		return PaymentExecution{}, err
	}
	if execution.Instruction.Reference != reference {
		return PaymentExecution{}, fmt.Errorf("%w: payload reference does not match canonical reference", ErrInvalidExecution)
	}
	return cloneExecution(execution), nil
}

func (s *PostgresStore) Save(ctx context.Context, execution PaymentExecution) error {
	if s == nil || s.db == nil {
		return ErrInvalidCoordinator
	}
	if execution.Version < 0 {
		return fmt.Errorf("%w: negative version", ErrInvalidExecution)
	}
	if err := validatePaymentExecution(execution); err != nil {
		return err
	}

	prepared := cloneExecution(execution)
	if prepared.CreatedAt.IsZero() {
		prepared.CreatedAt = time.Now().UTC()
	}
	if prepared.UpdatedAt.IsZero() {
		prepared.UpdatedAt = prepared.CreatedAt
	}
	if execution.Version == int64(^uint64(0)>>1) {
		return fmt.Errorf("%w: version overflow", ErrInvalidExecution)
	}
	prepared.Version = execution.Version + 1
	payload, err := json.Marshal(prepared)
	if err != nil {
		return fmt.Errorf("%w: encode payload: %v", ErrInvalidExecution, err)
	}

	var storedVersion int64
	err = s.db.QueryRow(ctx, savePaymentExecutionSQL,
		prepared.Instruction.Reference.Connection.CompanyID,
		prepared.Instruction.Reference.Connection.ConnectionID,
		prepared.Instruction.Reference.Connection.Provider,
		prepared.Instruction.Reference.ObjectType,
		prepared.Instruction.Reference.ObjectID,
		string(prepared.State),
		execution.Version,
		payload,
		prepared.CreatedAt,
		prepared.UpdatedAt,
	).Scan(&storedVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrExecutionConflict
	}
	return err
}

func validatePaymentExecution(execution PaymentExecution) error {
	if err := execution.Instruction.Validate(); err != nil {
		return err
	}
	if !validExecutionState(execution.State) {
		return fmt.Errorf("%w: unsupported state %q", ErrInvalidExecution, execution.State)
	}

	canonical := execution.Instruction.Reference
	if execution.Submission != nil {
		if err := validateExecutionReference(canonical, execution.Submission.Reference, "submission reference"); err != nil {
			return err
		}
	}
	if execution.Settlement != nil {
		if !isZeroReference(execution.Settlement.Instruction) {
			if err := validateExecutionReference(canonical, execution.Settlement.Instruction, "settlement instruction"); err != nil {
				return err
			}
			if execution.Settlement.Instruction != canonical {
				return fmt.Errorf("%w: settlement instruction does not match instruction", ErrInvalidExecution)
			}
		}
		if err := validateExecutionReference(canonical, execution.Settlement.Reference, "settlement reference"); err != nil {
			return err
		}
		if hasExactAmount(execution.Settlement.SettledAmount) {
			if err := execution.Settlement.SettledAmount.Validate(); err != nil {
				return fmt.Errorf("%w: settled amount: %v", ErrInvalidExecution, err)
			}
		}
		if hasExactAmount(execution.Settlement.ProviderFee) {
			if err := execution.Settlement.ProviderFee.Validate(); err != nil {
				return fmt.Errorf("%w: provider fee: %v", ErrInvalidExecution, err)
			}
		}
	}
	if execution.ExportArtifact != nil {
		if err := validateExecutionReference(canonical, execution.ExportArtifact.Reference, "export artifact reference"); err != nil {
			return err
		}
	}
	for _, transition := range execution.Transitions {
		if !validExecutionState(transition.From) || !validExecutionState(transition.To) {
			return fmt.Errorf("%w: invalid transition %q -> %q", ErrInvalidExecution, transition.From, transition.To)
		}
	}
	return nil
}

func validateExecutionReference(canonical automation.ExternalReference, reference automation.ExternalReference, name string) error {
	if isZeroReference(reference) {
		return nil
	}
	if err := reference.Validate(); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrInvalidExecution, name, err)
	}
	if reference.Connection != canonical.Connection {
		return fmt.Errorf("%w: %s does not belong to instruction connection", ErrCrossCompanyReference, name)
	}
	return nil
}

func validExecutionState(state ExecutionState) bool {
	switch state {
	case StateProposed, StateApproved, StateSubmitted, StateExported,
		StateAmbiguous, StatePartiallySettled, StateSettled, StateCancelled,
		StateFailed:
		return true
	default:
		return false
	}
}
