package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxStatus string

const (
	OutboxPending      OutboxStatus = "PENDING"
	OutboxProcessing   OutboxStatus = "PROCESSING"
	OutboxCompleted    OutboxStatus = "COMPLETED"
	OutboxDeadLettered OutboxStatus = "DEAD_LETTERED"
	OutboxCancelled    OutboxStatus = "CANCELLED"
)

var (
	ErrInvalidOutboxMessage = errors.New("finance automation: invalid outbox message")
	ErrOutboxNotClaimed     = errors.New("finance automation: outbox message is not claimed by worker")
	ErrOutboxNotFound       = errors.New("finance automation: outbox message not found")
)

// OutboxMessage is a durable command. A domain transaction inserts it before
// an asynchronous worker contacts an external provider.
type OutboxMessage struct {
	ID             int64
	CompanyID      int64
	Topic          string
	AggregateType  string
	AggregateID    string
	Operation      string
	Correlation    Correlation
	IdempotencyKey string
	Payload        json.RawMessage
	Status         OutboxStatus
	Attempts       int
	MaxAttempts    int
	AvailableAt    time.Time
	LockedAt       *time.Time
	LockedBy       string
	LastError      string
	CompletedAt    *time.Time
	DeadLetteredAt *time.Time
	ReplayedFromID int64
	CreatedBy      int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (m OutboxMessage) Validate() error {
	if m.CompanyID <= 0 || strings.TrimSpace(m.Topic) == "" || strings.TrimSpace(m.AggregateType) == "" || strings.TrimSpace(m.AggregateID) == "" || strings.TrimSpace(m.Operation) == "" || strings.TrimSpace(m.IdempotencyKey) == "" {
		return ErrInvalidOutboxMessage
	}
	if err := m.Correlation.Validate(); err != nil {
		return err
	}
	if m.MaxAttempts == 0 {
		m.MaxAttempts = DefaultRetryPolicy().MaxAttempts
	}
	if err := (RetryPolicy{MaxAttempts: m.MaxAttempts, Lease: DefaultRetryPolicy().Lease}).Validate(); err != nil {
		return err
	}
	if len(m.Payload) > 0 && !json.Valid(m.Payload) {
		return ErrInvalidOutboxMessage
	}
	return nil
}

type EnqueueInput struct {
	CompanyID      int64
	Topic          string
	AggregateType  string
	AggregateID    string
	Operation      string
	Correlation    Correlation
	IdempotencyKey string
	Payload        json.RawMessage
	MaxAttempts    int
	CreatedBy      int64
}

func (in EnqueueInput) message() OutboxMessage {
	maxAttempts := in.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = DefaultRetryPolicy().MaxAttempts
	}
	payload := in.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	return OutboxMessage{
		CompanyID:      in.CompanyID,
		Topic:          in.Topic,
		AggregateType:  in.AggregateType,
		AggregateID:    in.AggregateID,
		Operation:      in.Operation,
		Correlation:    in.Correlation,
		IdempotencyKey: in.IdempotencyKey,
		Payload:        payload,
		MaxAttempts:    maxAttempts,
		CreatedBy:      in.CreatedBy,
	}
}

type OutboxRepository struct{ pool database }

func NewOutboxRepository(pool *pgxpool.Pool) *OutboxRepository { return &OutboxRepository{pool: pool} }

// Enqueue returns the existing message for a repeated scoped idempotency key.
func (r *OutboxRepository) Enqueue(ctx context.Context, input EnqueueInput) (OutboxMessage, error) {
	message := input.message()
	if err := message.Validate(); err != nil {
		return OutboxMessage{}, err
	}
	return scanOutbox(r.pool.QueryRow(ctx, `
		INSERT INTO finance_automation_outbox (
			company_id, topic, aggregate_type, aggregate_id, operation,
			correlation_id, causation_id, idempotency_key, payload, max_attempts, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10,NULLIF($11,0))
		ON CONFLICT (company_id, operation, idempotency_key) DO UPDATE
		SET id = finance_automation_outbox.id
		RETURNING id, company_id, topic, aggregate_type, aggregate_id, operation,
		          correlation_id, COALESCE(causation_id,''), idempotency_key, payload,
		          status, attempts, max_attempts, available_at, locked_at,
		          COALESCE(locked_by,''), COALESCE(last_error,''), completed_at,
		          dead_lettered_at, COALESCE(replayed_from_id,0), COALESCE(created_by,0),
		          created_at, updated_at`,
		message.CompanyID, message.Topic, message.AggregateType, message.AggregateID,
		message.Operation, message.Correlation.ID, message.Correlation.CausationID,
		message.IdempotencyKey, message.Payload, message.MaxAttempts, message.CreatedBy,
	))
}

// Claim leases pending commands. A stale processing lease is recovered only
// after its retry policy lease expires.
func (r *OutboxRepository) Claim(ctx context.Context, workerID string, limit int, policy RetryPolicy) ([]OutboxMessage, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" || limit < 1 || policy.Validate() != nil {
		return nil, ErrInvalidOutboxMessage
	}
	rows, err := r.pool.Query(ctx, `
		WITH candidates AS (
			SELECT id
			FROM finance_automation_outbox
			WHERE (status = 'PENDING' AND available_at <= NOW())
			   OR (status = 'PROCESSING' AND locked_at < NOW() - ($3 * INTERVAL '1 microsecond'))
			ORDER BY available_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)
		UPDATE finance_automation_outbox o
		SET status = 'PROCESSING', attempts = o.attempts + 1, locked_at = NOW(),
		    locked_by = $1, updated_at = NOW()
		FROM candidates
		WHERE o.id = candidates.id
		RETURNING o.id, o.company_id, o.topic, o.aggregate_type, o.aggregate_id, o.operation,
		          o.correlation_id, COALESCE(o.causation_id,''), o.idempotency_key, o.payload,
		          o.status, o.attempts, o.max_attempts, o.available_at, o.locked_at,
		          COALESCE(o.locked_by,''), COALESCE(o.last_error,''), o.completed_at,
		          o.dead_lettered_at, COALESCE(o.replayed_from_id,0), COALESCE(o.created_by,0),
		          o.created_at, o.updated_at`, workerID, limit, policy.Lease.Microseconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]OutboxMessage, 0, limit)
	for rows.Next() {
		item, err := scanOutbox(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *OutboxRepository) Complete(ctx context.Context, id int64, workerID string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE finance_automation_outbox
		SET status = 'COMPLETED', completed_at = NOW(), locked_at = NULL, locked_by = NULL,
		    last_error = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'PROCESSING' AND locked_by = $2`, id, strings.TrimSpace(workerID))
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrOutboxNotClaimed
	}
	return nil
}

// Fail makes an attempt visible. It dead-letters at max attempts; otherwise
// the caller supplies the next safe retry time.
func (r *OutboxRepository) Fail(ctx context.Context, id int64, workerID string, cause error, retryAt time.Time) (OutboxStatus, error) {
	if cause == nil {
		return "", ErrInvalidOutboxMessage
	}
	var status OutboxStatus
	err := r.pool.QueryRow(ctx, `
		UPDATE finance_automation_outbox
		SET status = CASE WHEN attempts >= max_attempts THEN 'DEAD_LETTERED' ELSE 'PENDING' END,
		    available_at = CASE WHEN attempts >= max_attempts THEN available_at ELSE $3 END,
		    dead_lettered_at = CASE WHEN attempts >= max_attempts THEN NOW() ELSE NULL END,
		    locked_at = NULL, locked_by = NULL, last_error = LEFT($4, 2000), updated_at = NOW()
		WHERE id = $1 AND status = 'PROCESSING' AND locked_by = $2
		RETURNING status`, id, strings.TrimSpace(workerID), retryAt, cause.Error()).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrOutboxNotClaimed
	}
	return status, err
}

// Replay creates a separate durable command from a dead letter. The caller
// supplies a new idempotency suffix so repeated replay requests remain safe.
func (r *OutboxRepository) Replay(ctx context.Context, companyID, id int64, replayKey string, actorID int64) (OutboxMessage, error) {
	if companyID <= 0 || id <= 0 || strings.TrimSpace(replayKey) == "" {
		return OutboxMessage{}, ErrInvalidOutboxMessage
	}
	return scanOutbox(r.pool.QueryRow(ctx, `
		INSERT INTO finance_automation_outbox (
			company_id, topic, aggregate_type, aggregate_id, operation,
			correlation_id, causation_id, idempotency_key, payload, max_attempts,
			replayed_from_id, created_by
		)
		SELECT company_id, topic, aggregate_type, aggregate_id, operation,
		       correlation_id, causation_id, idempotency_key || ':replay:' || $3,
		       payload, max_attempts, id, NULLIF($4,0)
		FROM finance_automation_outbox
		WHERE id = $2 AND company_id = $1 AND status = 'DEAD_LETTERED'
		ON CONFLICT (company_id, operation, idempotency_key) DO UPDATE
		SET id = finance_automation_outbox.id
		RETURNING id, company_id, topic, aggregate_type, aggregate_id, operation,
		          correlation_id, COALESCE(causation_id,''), idempotency_key, payload,
		          status, attempts, max_attempts, available_at, locked_at,
		          COALESCE(locked_by,''), COALESCE(last_error,''), completed_at,
		          dead_lettered_at, COALESCE(replayed_from_id,0), COALESCE(created_by,0),
		          created_at, updated_at`, companyID, id, replayKey, actorID))
}

type outboxRow interface {
	Scan(...any) error
}

func scanOutbox(row outboxRow) (OutboxMessage, error) {
	var message OutboxMessage
	err := row.Scan(
		&message.ID, &message.CompanyID, &message.Topic, &message.AggregateType,
		&message.AggregateID, &message.Operation, &message.Correlation.ID,
		&message.Correlation.CausationID, &message.IdempotencyKey, &message.Payload,
		&message.Status, &message.Attempts, &message.MaxAttempts, &message.AvailableAt,
		&message.LockedAt, &message.LockedBy, &message.LastError, &message.CompletedAt,
		&message.DeadLetteredAt, &message.ReplayedFromID, &message.CreatedBy,
		&message.CreatedAt, &message.UpdatedAt,
	)
	if err != nil {
		return OutboxMessage{}, err
	}
	return message, nil
}

func (m OutboxMessage) String() string {
	return fmt.Sprintf("%s:%d:%s", m.Topic, m.ID, m.Status)
}
