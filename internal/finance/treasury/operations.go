package treasury

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/payments"
)

// OperationsFilter controls the company-scoped operations queue. State is
// deliberately a small allow-list; callers never interpolate user supplied
// SQL into the repository query.
type OperationsFilter struct {
	State string
	Query string
	Limit int
}

// PaymentOperation is a safe, provider-neutral view of one instruction. Raw
// coordinator/provider JSON is intentionally not exposed to templates or HTTP
// callers. Beneficiary and reference values are masked at the repository
// boundary before they become part of this read model.
type PaymentOperation struct {
	InstructionID        string
	CompanyID            int64
	ConnectionID         int64
	Provider             string
	ObjectType           string
	State                string
	BeneficiaryRef       string
	BeneficiaryName      string
	Amount               string
	Currency             string
	EndToEndReference    string
	SubmissionReference  string
	SettlementReference  string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	ErrorSummary         string
	EffectStatus         string
	ReconciliationStatus string
	Results              []PaymentOperationResult
	Effects              []PaymentOperationEffect
	Outbox               []PaymentOperationOutbox
	reconciliationRef    string
}

type PaymentOperationResult struct {
	ResultID      string
	Status        string
	State         string
	EffectApplied bool
	SettledAmount string
	Currency      string
	RecordedAt    time.Time
}

type PaymentOperationEffect struct {
	EffectKey string
	ResultID  string
	State     string
	LinkCount int
	AppliedAt time.Time
}

type PaymentOperationOutbox struct {
	ID             int64
	Operation      string
	ResultID       string
	Status         string
	Attempts       int
	MaxAttempts    int
	LastError      string
	DeadLetteredAt *time.Time
	ReplayedFromID int64
	CreatedAt      time.Time
}

var (
	ErrOperationsNotFound       = errors.New("treasury operations: operation not found")
	ErrRecoveryNotAllowed       = errors.New("treasury operations: recovery is not allowed for this state")
	ErrRecoveryNotAvailable     = errors.New("treasury operations: no recoverable outbox command")
	ErrEffectsRetryNotAllowed   = errors.New("treasury operations: settlement effects do not require a retry")
	ErrEffectsRetryNotAvailable = errors.New("treasury operations: no recoverable result-import command")
)

// OperationsReader is the storage-neutral read/recovery boundary used by the
// SSR workbench. The command replay method is intentionally separate from the
// query methods so a read-only implementation cannot accidentally mutate.
type OperationsReader interface {
	ListOperations(context.Context, int64, OperationsFilter) ([]PaymentOperation, error)
	GetOperation(context.Context, int64, string) (PaymentOperation, error)
}

type OperationsReplayer interface {
	Replay(context.Context, int64, int64, string, int64) (automation.OutboxMessage, error)
}

// OperationsService contains the guarded operator actions. It only asks the
// shared finance outbox to replay dead letters; provider calls and financial
// effects remain owned by their existing worker/coordinator handlers.
type OperationsService struct {
	reader OperationsReader
	replay OperationsReplayer
	now    func() time.Time
}

func NewOperationsService(reader OperationsReader, replayer OperationsReplayer) *OperationsService {
	return &OperationsService{reader: reader, replay: replayer, now: func() time.Time { return time.Now().UTC() }}
}

func (s *OperationsService) List(ctx context.Context, companyID int64, filter OperationsFilter) ([]PaymentOperation, error) {
	if s == nil || s.reader == nil || companyID <= 0 {
		return nil, errCompanyScopeRequired
	}
	return s.reader.ListOperations(ctx, companyID, filter)
}

func (s *OperationsService) Get(ctx context.Context, companyID int64, instructionID string) (PaymentOperation, error) {
	if s == nil || s.reader == nil || companyID <= 0 || strings.TrimSpace(instructionID) == "" {
		return PaymentOperation{}, errCompanyScopeRequired
	}
	return s.reader.GetOperation(ctx, companyID, instructionID)
}

// Recover replays a dead-lettered execution command for an AMBIGUOUS
// instruction. Coordinator.Submit sees the persisted AMBIGUOUS state and
// performs Lookup before it can ever submit again.
func (s *OperationsService) Recover(ctx context.Context, companyID int64, instructionID string, actorID int64) error {
	if s == nil || s.reader == nil || s.replay == nil || companyID <= 0 || actorID <= 0 || strings.TrimSpace(instructionID) == "" {
		return errCompanyScopeRequired
	}
	op, err := s.reader.GetOperation(ctx, companyID, instructionID)
	if err != nil {
		return err
	}
	if op.State != string(payments.StateAmbiguous) {
		return ErrRecoveryNotAllowed
	}
	for _, command := range op.Outbox {
		if command.Operation != automation.OperationPaymentExecute && command.Operation != automation.OperationPaymentSubmit {
			continue
		}
		if command.Status == string(automation.OutboxDeadLettered) {
			key := fmt.Sprintf("operator-%d-%d", actorID, s.now().UnixNano())
			_, err := s.replay.Replay(ctx, companyID, command.ID, key, actorID)
			return err
		}
		if command.Status == string(automation.OutboxPending) || command.Status == string(automation.OutboxProcessing) {
			return nil
		}
	}
	return ErrRecoveryNotAvailable
}

// RetryEffects replays a dead-lettered result-import command. SettlementService
// loads the immutable result and retries its idempotent effects claim.
func (s *OperationsService) RetryEffects(ctx context.Context, companyID int64, resultID string, actorID int64) error {
	if s == nil || s.reader == nil || s.replay == nil || companyID <= 0 || actorID <= 0 || strings.TrimSpace(resultID) == "" {
		return errCompanyScopeRequired
	}
	// Result IDs are not globally addressable in the workbench. Resolve the
	// instruction through the company-scoped repository query first, then use
	// the exact result row from that operation.
	op, err := s.getByResult(ctx, companyID, resultID)
	if err != nil {
		return err
	}
	for _, result := range op.Results {
		if result.ResultID != resultID {
			continue
		}
		if result.EffectApplied || (result.State != string(payments.StateSettled) && result.State != string(payments.StatePartiallySettled)) {
			return ErrEffectsRetryNotAllowed
		}
		for _, command := range op.Outbox {
			if command.Operation != automation.OperationPaymentResultImport || command.ResultID != resultID {
				continue
			}
			if command.Status == string(automation.OutboxDeadLettered) {
				key := fmt.Sprintf("operator-effects-%d-%d", actorID, s.now().UnixNano())
				_, err := s.replay.Replay(ctx, companyID, command.ID, key, actorID)
				return err
			}
			if command.Status == string(automation.OutboxPending) || command.Status == string(automation.OutboxProcessing) {
				return nil
			}
		}
		return ErrEffectsRetryNotAvailable
	}
	return ErrEffectsRetryNotAllowed
}

// GetOperationByResult is kept on the reader as an optional extension. The
// default repository implements it; simple read-only adapters can omit it and
// receive a clear unsupported error from RetryEffects.
type resultOperationReader interface {
	GetOperationByResult(context.Context, int64, string) (PaymentOperation, error)
}

func (s *OperationsService) readerByResult(ctx context.Context, companyID int64, resultID string) (PaymentOperation, error) {
	reader, ok := s.reader.(resultOperationReader)
	if !ok {
		return PaymentOperation{}, ErrEffectsRetryNotAvailable
	}
	return reader.GetOperationByResult(ctx, companyID, resultID)
}

// GetOperationByResult is implemented as a method on OperationsReader through
// a small helper to keep the public interface backward compatible.
func (s *OperationsService) getByResult(ctx context.Context, companyID int64, resultID string) (PaymentOperation, error) {
	return s.readerByResult(ctx, companyID, resultID)
}

// operationDatabase is the minimal query interface implemented by pgx pools
// and pgxmock pools. It keeps the workbench repository independent of sqlc.
type operationDatabase interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// OperationsRepository is the PostgreSQL adapter for the operations reader.
// It reads canonical execution columns and only the selected JSON payload
// needed to display masked provider-neutral fields.
type OperationsRepository struct{ db operationDatabase }

func NewOperationsRepository(pool operationDatabase) *OperationsRepository {
	return &OperationsRepository{db: pool}
}

var _ OperationsReader = (*OperationsRepository)(nil)

const operationsExecutionSelect = `
SELECT id, company_id, connection_id, provider, object_type, object_id,
       state, payload, created_at, updated_at
FROM payment_executions
WHERE company_id = $1`

func (r *OperationsRepository) ListOperations(ctx context.Context, companyID int64, filter OperationsFilter) ([]PaymentOperation, error) {
	if r == nil || r.db == nil || companyID <= 0 {
		return nil, errCompanyScopeRequired
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	args := []any{companyID}
	query := operationsExecutionSelect
	if state := strings.ToUpper(strings.TrimSpace(filter.State)); state != "" {
		if !validOperationState(state) {
			return nil, fmt.Errorf("invalid operations state %q", state)
		}
		args = append(args, state)
		query += fmt.Sprintf(" AND state = $%d", len(args))
	}
	if search := strings.TrimSpace(filter.Query); search != "" {
		args = append(args, "%"+search+"%")
		query += fmt.Sprintf(" AND object_id ILIKE $%d", len(args))
	}
	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY updated_at DESC, id DESC LIMIT $%d", len(args))
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	operations := make([]PaymentOperation, 0)
	for rows.Next() {
		op, err := r.scanOperation(ctx, rows)
		if err != nil {
			return nil, err
		}
		operations = append(operations, op)
	}
	return operations, rows.Err()
}

func (r *OperationsRepository) GetOperation(ctx context.Context, companyID int64, instructionID string) (PaymentOperation, error) {
	if r == nil || r.db == nil || companyID <= 0 || strings.TrimSpace(instructionID) == "" {
		return PaymentOperation{}, errCompanyScopeRequired
	}
	row := r.db.QueryRow(ctx, operationsExecutionSelect+" AND object_id = $2 LIMIT 1", companyID, instructionID)
	op, err := r.scanOperation(ctx, row)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentOperation{}, ErrOperationsNotFound
	}
	return op, err
}

func (r *OperationsRepository) GetOperationByResult(ctx context.Context, companyID int64, resultID string) (PaymentOperation, error) {
	if r == nil || r.db == nil || companyID <= 0 || strings.TrimSpace(resultID) == "" {
		return PaymentOperation{}, errCompanyScopeRequired
	}
	row := r.db.QueryRow(ctx, operationsExecutionSelect+` AND EXISTS (
  SELECT 1 FROM payment_settlement_results sr
  WHERE sr.company_id = payment_executions.company_id
    AND sr.connection_id = payment_executions.connection_id
    AND sr.instruction_type = payment_executions.object_type
    AND sr.instruction_id = payment_executions.object_id
    AND sr.result_id = $2
) LIMIT 1`, companyID, resultID)
	op, err := r.scanOperation(ctx, row)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentOperation{}, ErrOperationsNotFound
	}
	return op, err
}

type operationRow interface{ Scan(...any) error }

func (r *OperationsRepository) scanOperation(ctx context.Context, row operationRow) (PaymentOperation, error) {
	var (
		id, companyID, connectionID           int64
		provider, objectType, objectID, state string
		payload                               []byte
		createdAt, updatedAt                  time.Time
	)
	if err := row.Scan(&id, &companyID, &connectionID, &provider, &objectType, &objectID, &state, &payload, &createdAt, &updatedAt); err != nil {
		return PaymentOperation{}, err
	}
	op := PaymentOperation{
		InstructionID:        objectID,
		CompanyID:            companyID,
		ConnectionID:         connectionID,
		Provider:             provider,
		ObjectType:           objectType,
		State:                state,
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
		EffectStatus:         "NOT_APPLICABLE",
		ReconciliationStatus: "UNMATCHED",
	}
	var execution payments.PaymentExecution
	if err := json.Unmarshal(payload, &execution); err == nil {
		op.BeneficiaryRef = MaskSensitive(execution.Instruction.BeneficiaryRef)
		op.BeneficiaryName = MaskName(execution.Instruction.BeneficiaryName)
		op.Amount = execution.Instruction.Amount.Amount.String()
		op.Currency = execution.Instruction.Amount.Currency
		op.EndToEndReference = MaskSensitive(execution.Instruction.EndToEndReference)
		op.reconciliationRef = execution.Instruction.EndToEndReference
		if execution.Submission != nil {
			op.SubmissionReference = MaskSensitive(execution.Submission.Reference.ObjectID)
		}
		if execution.Settlement != nil {
			op.SettlementReference = MaskSensitive(execution.Settlement.Reference.ObjectID)
			if execution.Settlement.EndToEndReference != "" {
				op.EndToEndReference = MaskSensitive(execution.Settlement.EndToEndReference)
				op.reconciliationRef = execution.Settlement.EndToEndReference
			}
		}
	}
	if err := r.loadOperationDetails(ctx, &op); err != nil {
		return PaymentOperation{}, err
	}
	return op, nil
}

func (r *OperationsRepository) loadOperationDetails(ctx context.Context, op *PaymentOperation) error {
	rows, err := r.db.Query(ctx, `
SELECT result_id, status, state, effect_applied,
       COALESCE(payload->'settled_amount'->'Amount'->>'Amount',''),
       COALESCE(payload->'settled_amount'->>'Currency',''), recorded_at
FROM payment_settlement_results
WHERE company_id = $1 AND connection_id = $2
  AND instruction_type = $3 AND instruction_id = $4
ORDER BY recorded_at DESC, id DESC`, op.CompanyID, op.ConnectionID, op.ObjectType, op.InstructionID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var result PaymentOperationResult
		if err := rows.Scan(&result.ResultID, &result.Status, &result.State, &result.EffectApplied, &result.SettledAmount, &result.Currency, &result.RecordedAt); err != nil {
			rows.Close()
			return err
		}
		op.Results = append(op.Results, result)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	rows, err = r.db.Query(ctx, `
SELECT e.effect_key, e.result_id, e.state, e.applied_at,
       COUNT(l.id)::int
FROM payment_settlement_effects e
LEFT JOIN payment_settlement_effect_links l
  ON l.company_id = e.company_id AND l.effect_key = e.effect_key
WHERE e.company_id = $1 AND e.result_id IN (
  SELECT result_id FROM payment_settlement_results
  WHERE company_id = $1 AND connection_id = $2
    AND instruction_type = $3 AND instruction_id = $4
)
GROUP BY e.effect_key, e.result_id, e.state, e.applied_at
ORDER BY e.applied_at DESC`, op.CompanyID, op.ConnectionID, op.ObjectType, op.InstructionID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var effect PaymentOperationEffect
		if err := rows.Scan(&effect.EffectKey, &effect.ResultID, &effect.State, &effect.AppliedAt, &effect.LinkCount); err != nil {
			rows.Close()
			return err
		}
		op.Effects = append(op.Effects, effect)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	rows, err = r.db.Query(ctx, `
SELECT id, operation, status, attempts, max_attempts,
       COALESCE(last_error,''), dead_lettered_at, COALESCE(replayed_from_id,0),
       created_at, payload
FROM finance_automation_outbox
WHERE company_id = $1 AND aggregate_id = $2
ORDER BY created_at DESC, id DESC`, op.CompanyID, op.InstructionID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var command PaymentOperationOutbox
		var payload []byte
		if err := rows.Scan(&command.ID, &command.Operation, &command.Status, &command.Attempts, &command.MaxAttempts, &command.LastError, &command.DeadLetteredAt, &command.ReplayedFromID, &command.CreatedAt, &payload); err != nil {
			rows.Close()
			return err
		}
		var resultPayload struct {
			ResultID        string `json:"result_id"`
			ResultReference string `json:"result_reference"`
			ProviderEventID string `json:"provider_event_id"`
		}
		if json.Unmarshal(payload, &resultPayload) == nil {
			command.ResultID = firstNonBlank(resultPayload.ResultID, resultPayload.ResultReference, resultPayload.ProviderEventID)
		}
		if command.LastError != "" {
			op.ErrorSummary = command.LastError
		}
		op.Outbox = append(op.Outbox, command)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(op.Results) > 0 {
		hasConfirmed, hasPending := false, false
		for _, result := range op.Results {
			if result.State != string(payments.StateSettled) && result.State != string(payments.StatePartiallySettled) {
				continue
			}
			hasConfirmed = true
			if !result.EffectApplied {
				hasPending = true
			}
		}
		switch {
		case hasPending:
			op.EffectStatus = "PENDING"
		case hasConfirmed:
			op.EffectStatus = "APPLIED"
		default:
			op.EffectStatus = "NOT_REQUIRED"
		}
	}
	for _, command := range op.Outbox {
		if command.Status == string(automation.OutboxDeadLettered) && op.ErrorSummary == "" {
			op.ErrorSummary = command.LastError
		}
	}
	if op.reconciliationRef != "" {
		var matched bool
		if err := r.db.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM bank_transactions bt
  JOIN bank_accounts ba ON ba.id = bt.bank_account_id
  WHERE ba.company_id = $1
    AND bt.status = 'RECONCILED'
    AND (bt.external_reference = $2 OR bt.reference = $2)
)`, op.CompanyID, op.reconciliationRef).Scan(&matched); err != nil {
			return err
		}
		if matched {
			op.ReconciliationStatus = "MATCHED"
		}
	} else {
		op.ReconciliationStatus = "NOT_RECORDED"
	}
	return nil
}

func validOperationState(state string) bool {
	switch state {
	case "PROPOSED", "APPROVED", "SUBMITTED", "EXPORTED", "AMBIGUOUS", "PARTIALLY_SETTLED", "SETTLED", "CANCELLED", "FAILED":
		return true
	default:
		return false
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// MaskSensitive keeps the last four characters for operator correlation while
// preventing account/reference values from becoming an SSR data leak.
func MaskSensitive(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return strings.Repeat("*", len(value))
	}
	return strings.Repeat("*", len(value)-4) + value[len(value)-4:]
}

func MaskName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) == 1 {
		return "*"
	}
	return value[:1] + strings.Repeat("*", len(value)-1)
}
