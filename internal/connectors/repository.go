package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// PGRepository maps connector persistence rows to connector domain values.
type PGRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool, queries: sqlc.New(pool)}
}

func (r *PGRepository) ListConnections(ctx context.Context, companyID int64) ([]Connection, error) {
	rows, err := r.queries.ListConnections(ctx, companyID)
	if err != nil {
		return nil, err
	}
	connections := make([]Connection, len(rows))
	for i, row := range rows {
		connections[i] = mapConnection(row)
	}
	return connections, nil
}

func (r *PGRepository) CreateConnection(ctx context.Context, input ConnectionCreateInput) (Connection, error) {
	row, err := r.queries.CreateConnection(ctx, sqlc.CreateConnectionParams{
		CompanyID:   input.CompanyID,
		Provider:    input.Provider,
		Type:        input.Type,
		Name:        input.Name,
		SecretRef:   input.SecretRef,
		Status:      string(StatusDisabled),
		TokenExpiry: optionalTimestamp(input.TokenExpiry),
	})
	if err != nil {
		return Connection{}, err
	}
	return mapConnection(row), nil
}

func (r *PGRepository) GetConnection(ctx context.Context, companyID, connectionID int64) (Connection, error) {
	row, err := r.queries.GetConnection(ctx, sqlc.GetConnectionParams{ID: connectionID, CompanyID: companyID})
	if err != nil {
		return Connection{}, err
	}
	return mapConnection(row), nil
}

func (r *PGRepository) UpdateConnectionStatus(ctx context.Context, companyID, connectionID int64, status string) (Connection, error) {
	row, err := r.queries.UpdateConnectionStatus(ctx, sqlc.UpdateConnectionStatusParams{
		ID:        connectionID,
		CompanyID: companyID,
		Status:    status,
		LastError: pgtype.Text{},
	})
	if err != nil {
		return Connection{}, err
	}
	return mapConnection(row), nil
}

func (r *PGRepository) CreatePaymentIntent(ctx context.Context, input PaymentIntentInput) (int64, error) {
	row, err := r.queries.CreatePaymentIntent(ctx, sqlc.CreatePaymentIntentParams{
		CompanyID:         input.CompanyID,
		ConnectionID:      input.ConnectionID,
		SourceType:        input.SourceType,
		SourceID:          input.SourceID,
		Amount:            numericOf(input.Amount),
		Currency:          input.Currency,
		Status:            input.Status,
		ProviderReference: optionalText(input.ProviderReference),
		CheckoutUrl:       optionalText(input.CheckoutURL),
	})
	return row.ID, err
}

func (r *PGRepository) GetPaymentIntentByProviderReference(ctx context.Context, companyID, connectionID int64, providerReference string) (PaymentIntent, error) {
	row, err := r.queries.GetPaymentIntentByProviderRef(ctx, sqlc.GetPaymentIntentByProviderRefParams{
		CompanyID:         companyID,
		ConnectionID:      connectionID,
		ProviderReference: optionalText(providerReference),
	})
	if err != nil {
		return PaymentIntent{}, err
	}
	return mapPaymentIntent(row), nil
}

func (r *PGRepository) UpdatePaymentIntentCheckout(ctx context.Context, companyID, connectionID, intentID int64, status, providerReference, checkoutURL string) error {
	_, err := r.queries.UpdatePaymentIntentStatus(ctx, sqlc.UpdatePaymentIntentStatusParams{
		ID:                intentID,
		Status:            status,
		ProviderReference: optionalText(providerReference),
		CheckoutUrl:       optionalText(checkoutURL),
		CompanyID:         companyID,
		ConnectionID:      connectionID,
	})
	return err
}

func (r *PGRepository) RequestPaymentRefund(ctx context.Context, input PaymentRefundRequest) (PaymentRefundRequestResult, error) {
	if r == nil || r.pool == nil {
		return PaymentRefundRequestResult{}, errors.New("connectors: refund database is unavailable")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return PaymentRefundRequestResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var providerReference, currency, status string
	var intentAmount pgtype.Numeric
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(provider_reference, ''), amount, currency, status
		FROM payment_intents
		WHERE id = $1 AND company_id = $2 AND connection_id = $3
		FOR UPDATE`, input.PaymentIntentID, input.CompanyID, input.ConnectionID).Scan(&providerReference, &intentAmount, &currency, &status); err != nil {
		return PaymentRefundRequestResult{}, err
	}
	if strings.TrimSpace(providerReference) == "" {
		return PaymentRefundRequestResult{}, errors.New("connectors: payment intent has no provider reference")
	}
	if input.Currency != "" && !strings.EqualFold(strings.TrimSpace(input.Currency), currency) {
		return PaymentRefundRequestResult{}, fmt.Errorf("connectors: refund currency %q does not match payment currency %q", input.Currency, currency)
	}
	if status != string(PaymentStatusCaptured) && status != string(PaymentStatusSettled) && status != string(PaymentStatusPartiallyRefunded) {
		return PaymentRefundRequestResult{}, fmt.Errorf("connectors: payment intent status %q cannot be refunded", status)
	}

	var alreadyRefunded pgtype.Numeric
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0)
		FROM payment_refunds
		WHERE payment_intent_id = $1 AND status IN ('PARTIALLY_REFUNDED', 'REFUNDED', 'SUCCEEDED')`, input.PaymentIntentID).Scan(&alreadyRefunded); err != nil {
		return PaymentRefundRequestResult{}, err
	}
	remainingAmount := numericToFloat64(intentAmount) - numericToFloat64(alreadyRefunded)
	requestedAmount := input.Amount
	if requestedAmount == 0 {
		requestedAmount = remainingAmount
	}
	if requestedAmount <= 0 || math.IsNaN(requestedAmount) || math.IsInf(requestedAmount, 0) {
		return PaymentRefundRequestResult{}, errors.New("connectors: refund amount must be positive")
	}
	if requestedAmount > remainingAmount+0.0000001 {
		return PaymentRefundRequestResult{}, errors.New("connectors: refund amount exceeds refundable balance")
	}

	var existingRefundID int64
	var existingRefundStatus string
	err = tx.QueryRow(ctx, `
		SELECT id, status
		FROM payment_refunds
		WHERE payment_intent_id = $1 AND provider_reference = $2
		FOR UPDATE`, input.PaymentIntentID, input.RefundKey).Scan(&existingRefundID, &existingRefundStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			INSERT INTO payment_refunds (payment_intent_id, amount, currency, reason, status, provider_reference)
			VALUES ($1, $2, $3, NULLIF($4, ''), 'PENDING', $5)
			RETURNING id`, input.PaymentIntentID, numericOf(requestedAmount), currency, input.Reason, input.RefundKey).Scan(&existingRefundID)
		if err != nil {
			return PaymentRefundRequestResult{}, err
		}
	} else if err != nil {
		return PaymentRefundRequestResult{}, err
	} else if existingRefundStatus == "REFUNDED" || existingRefundStatus == "SUCCEEDED" || existingRefundStatus == "PARTIALLY_REFUNDED" {
		return PaymentRefundRequestResult{
			RefundID: existingRefundID, ProviderReference: input.RefundKey,
		}, tx.Commit(ctx)
	} else if _, err := tx.Exec(ctx, `
		UPDATE payment_refunds
		SET amount = $2, currency = $3, reason = NULLIF($4, ''), status = 'PENDING', updated_at = NOW()
		WHERE id = $1`, existingRefundID, numericOf(requestedAmount), currency, input.Reason); err != nil {
		return PaymentRefundRequestResult{}, err
	}

	commandPayload := map[string]any{
		"order_id":   providerReference,
		"refund_key": input.RefundKey,
		"reason":     input.Reason,
	}
	if strings.EqualFold(currency, "IDR") {
		minor, err := idrMinorUnits(requestedAmount)
		if err != nil {
			return PaymentRefundRequestResult{}, err
		}
		commandPayload["amount"] = minor
	} else {
		commandPayload["amount"] = requestedAmount
	}
	payload, err := json.Marshal(commandPayload)
	if err != nil {
		return PaymentRefundRequestResult{}, err
	}

	var commandID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO connector_outbox_commands (company_id, connection_id, command_type, correlation_id, payload)
		VALUES ($1, $2, 'payment.refund', $3, $4)
		ON CONFLICT (connection_id, correlation_id) DO UPDATE
		SET state = CASE WHEN connector_outbox_commands.state = 'dead_letter' THEN 'pending' ELSE connector_outbox_commands.state END,
		    next_attempt = CASE WHEN connector_outbox_commands.state = 'dead_letter' THEN NOW() ELSE connector_outbox_commands.next_attempt END,
		    updated_at = NOW()
		RETURNING id`, input.CompanyID, input.ConnectionID, input.RefundKey, payload).Scan(&commandID)
	if err != nil {
		return PaymentRefundRequestResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PaymentRefundRequestResult{}, err
	}
	return PaymentRefundRequestResult{RefundID: existingRefundID, OutboxCommandID: commandID, ProviderReference: input.RefundKey}, nil
}

func (r *PGRepository) MarkPaymentRefundProcessing(ctx context.Context, companyID, connectionID int64, refundKey string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE payment_refunds r
		SET status = 'PROCESSING', updated_at = NOW()
		FROM payment_intents i
		WHERE r.payment_intent_id = i.id
		  AND i.company_id = $1 AND i.connection_id = $2
		  AND r.provider_reference = $3
		  AND r.status = 'PENDING'`, companyID, connectionID, refundKey)
	return err
}

func (r *PGRepository) MarkPaymentRefundFailed(ctx context.Context, companyID, connectionID int64, refundKey string, cause error) error {
	if cause == nil {
		return errors.New("connectors: refund failure cause is required")
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE payment_refunds r
		SET status = 'FAILED', reason = COALESCE(NULLIF(reason, ''), LEFT($4, 2000)), updated_at = NOW()
		FROM payment_intents i
		WHERE r.payment_intent_id = i.id
		  AND i.company_id = $1 AND i.connection_id = $2
		  AND r.provider_reference = $3
		  AND r.status NOT IN ('REFUNDED', 'SUCCEEDED', 'PARTIALLY_REFUNDED')`, companyID, connectionID, refundKey, cause.Error())
	return err
}

// ApplyPaymentIntentEvent locks one intent, rejects duplicate/stale provider
// events, and records an accepted transition atomically with the state update.
func (r *PGRepository) ApplyPaymentIntentEvent(ctx context.Context, input PaymentIntentEventInput) (PaymentTransitionResult, error) {
	if r.pool == nil {
		return PaymentTransitionResult{}, fmt.Errorf("connectors: payment lifecycle database is unavailable")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return PaymentTransitionResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var intentID int64
	var current, currency string
	var intentAmount pgtype.Numeric
	var createdAt time.Time
	if err := tx.QueryRow(ctx, `
		SELECT id, status, amount, currency, created_at
		FROM payment_intents
		WHERE company_id = $1 AND connection_id = $2 AND provider_reference = $3
		FOR UPDATE`, input.CompanyID, input.ConnectionID, input.ProviderReference).Scan(&intentID, &current, &intentAmount, &currency, &createdAt); err != nil {
		return PaymentTransitionResult{}, err
	}

	if input.ProviderEventID != "" {
		var duplicate bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM payment_intent_transitions
				WHERE payment_intent_id = $1 AND provider_event_id = $2
			)`, intentID, input.ProviderEventID).Scan(&duplicate); err != nil {
			return PaymentTransitionResult{}, err
		}
		if duplicate {
			return PaymentTransitionResult{Duplicate: true, FromStatus: PaymentStatus(current), ToStatus: PaymentStatus(current), IgnoredReason: "provider event already applied"}, tx.Commit(ctx)
		}
	}

	var lastEventAt time.Time
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(occurred_at), $2)
		FROM payment_intent_transitions
		WHERE payment_intent_id = $1`, intentID, createdAt).Scan(&lastEventAt); err != nil {
		return PaymentTransitionResult{}, err
	}
	transition, err := ApplyPaymentTransition(PaymentStatus(current), lastEventAt, PaymentEvent{
		EventType:       input.EventType,
		ProviderEventID: input.ProviderEventID,
		OccurredAt:      input.OccurredAt,
	})
	if err != nil {
		return PaymentTransitionResult{}, err
	}
	if !transition.Applied {
		_, _ = tx.Exec(ctx, `
			UPDATE payment_intents SET updated_at = NOW()
			WHERE id = $1 AND company_id = $2 AND connection_id = $3`,
			intentID, input.CompanyID, input.ConnectionID)
		return transition, tx.Commit(ctx)
	}

	occurredAt := input.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	rawPayload := input.RawPayload
	if len(rawPayload) == 0 {
		rawPayload = []byte(`{}`)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO payment_intent_transitions
			(payment_intent_id, from_status, to_status, provider_event_id, occurred_at, raw_payload)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)
		ON CONFLICT (payment_intent_id, provider_event_id)
		WHERE provider_event_id IS NOT NULL AND provider_event_id <> '' DO NOTHING`,
		intentID, string(transition.FromStatus), string(transition.ToStatus), input.ProviderEventID, occurredAt, rawPayload)
	if err != nil {
		return PaymentTransitionResult{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE payment_intents
		SET status = $2, updated_at = NOW()
		WHERE id = $1 AND company_id = $3 AND connection_id = $4`,
		intentID, string(transition.ToStatus), input.CompanyID, input.ConnectionID); err != nil {
		return PaymentTransitionResult{}, err
	}
	if transition.ToStatus == PaymentStatusPartiallyRefunded || transition.ToStatus == PaymentStatusRefunded {
		if err := persistRefundEvent(ctx, tx, intentID, intentAmount, currency, input.EventType, input.ProviderEventID, input.RawPayload); err != nil {
			return PaymentTransitionResult{}, err
		}
	}
	return transition, tx.Commit(ctx)
}

func persistRefundEvent(ctx context.Context, tx pgx.Tx, intentID int64, intentAmount pgtype.Numeric, currency, eventType, providerEventID string, rawPayload []byte) error {
	var payload struct {
		RefundKey    string          `json:"refund_key"`
		RefundAmount json.RawMessage `json:"refund_amount"`
		Reason       string          `json:"reason"`
	}
	if len(rawPayload) > 0 {
		if err := json.Unmarshal(rawPayload, &payload); err != nil {
			return fmt.Errorf("connectors: decode refund callback: %w", err)
		}
	}
	providerReference := strings.TrimSpace(payload.RefundKey)
	if providerReference == "" {
		providerReference = strings.TrimSpace(providerEventID)
	}
	if providerReference == "" {
		return errors.New("connectors: refund callback provider reference is required")
	}

	amountText, err := refundAmountText(payload.RefundAmount)
	if err != nil {
		return err
	}
	if amountText == "" {
		fallback := numericToFloat64(intentAmount)
		if fallback <= 0 || math.IsNaN(fallback) || math.IsInf(fallback, 0) {
			return errors.New("connectors: refund callback amount is unavailable")
		}
		amountText = strconv.FormatFloat(fallback, 'f', 2, 64)
	}
	status := string(PaymentStatusRefunded)
	if strings.EqualFold(strings.TrimSpace(eventType), "payment.partially_refunded") {
		status = string(PaymentStatusPartiallyRefunded)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO payment_refunds (payment_intent_id, amount, currency, reason, status, provider_reference)
		VALUES ($1, NULLIF($2, '')::numeric, $3, NULLIF($4, ''), $5, $6)
		ON CONFLICT (payment_intent_id, provider_reference)
		WHERE provider_reference IS NOT NULL AND provider_reference <> ''
		DO UPDATE SET amount = EXCLUDED.amount, currency = EXCLUDED.currency,
		              reason = COALESCE(EXCLUDED.reason, payment_refunds.reason),
		              status = EXCLUDED.status, updated_at = NOW()`,
		intentID, amountText, currency, payload.Reason, status, providerReference)
	return err
}

func refundAmountText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	value := strings.TrimSpace(string(raw))
	if strings.HasPrefix(value, `"`) {
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return "", fmt.Errorf("connectors: decode refund amount: %w", err)
		}
		value = strings.TrimSpace(decoded)
	}
	if value == "" {
		return "", nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 {
		return "", fmt.Errorf("connectors: invalid refund amount %q", value)
	}
	return value, nil
}

func (r *PGRepository) ListPaymentReconciliationCandidates(ctx context.Context, limit int, staleBefore time.Time) ([]PaymentReconciliationCandidate, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.company_id, p.connection_id, p.source_type, p.source_id,
		       p.amount::text, p.currency, p.status, COALESCE(p.provider_reference, ''),
		       COALESCE(p.checkout_url, ''), p.created_at, p.updated_at,
		       c.id, c.company_id, c.provider, c.type, c.name, c.secret_ref, c.status,
		       c.created_at, c.updated_at
		FROM payment_intents p
		JOIN connector_connections c ON c.id = p.connection_id AND c.company_id = p.company_id
		WHERE p.provider_reference IS NOT NULL
		  AND p.status IN ('CREATED', 'PENDING', 'AUTHORIZED', 'CAPTURED', 'SETTLED', 'PARTIALLY_REFUNDED')
		  AND p.updated_at <= $1
		ORDER BY p.updated_at ASC, p.id ASC
		LIMIT $2`, staleBefore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := make([]PaymentReconciliationCandidate, 0, limit)
	for rows.Next() {
		var intent PaymentIntent
		var connection Connection
		var amountText string
		if err := rows.Scan(
			&intent.ID, &intent.CompanyID, &intent.ConnectionID, &intent.SourceType, &intent.SourceID,
			&amountText, &intent.Currency, &intent.Status, &intent.ProviderReference, &intent.CheckoutURL,
			&intent.CreatedAt, &intent.UpdatedAt,
			&connection.ID, &connection.CompanyID, &connection.Provider, &connection.Type,
			&connection.Name, &connection.SecretRef, &connection.Status, &connection.CreatedAt, &connection.UpdatedAt,
		); err != nil {
			return nil, err
		}
		intent.Amount, _ = strconv.ParseFloat(amountText, 64)
		candidates = append(candidates, PaymentReconciliationCandidate{Intent: intent, Connection: connection})
	}
	return candidates, rows.Err()
}

func (r *PGRepository) RecordPaymentReconciliationRun(ctx context.Context, run PaymentReconciliationRun) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO payment_reconciliation_runs (
			started_at, finished_at, status, scanned_count, recovered_count, matched_count,
			unmatched_count, unsupported_count, error_count, refunds_persisted, dead_letter_count, error_message
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''))`,
		run.StartedAt, run.FinishedAt, run.Status, run.ScannedCount, run.RecoveredCount,
		run.MatchedCount, run.UnmatchedCount, run.UnsupportedCount, run.ErrorCount,
		run.RefundsPersisted, run.DeadLetterCount, run.ErrorMessage)
	return err
}

func (r *PGRepository) UpsertPaymentReconciliationIssue(ctx context.Context, issue PaymentReconciliationIssue) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now := time.Now().UTC()
	var issueID int64
	var status string
	var alertedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT id, status, alerted_at
		FROM payment_reconciliation_issues
		WHERE company_id = $1 AND connection_id = $2 AND provider_reference = $3 AND issue_type = $4
		FOR UPDATE`, issue.CompanyID, issue.ConnectionID, issue.ProviderReference, issue.IssueType).Scan(&issueID, &status, &alertedAt)
	due := true
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			INSERT INTO payment_reconciliation_issues (
				company_id, connection_id, payment_intent_id, provider, provider_reference,
				issue_type, expected_status, observed_status, details
			) VALUES ($1,$2,NULLIF($3,0),$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9)
			RETURNING id`, issue.CompanyID, issue.ConnectionID, issue.PaymentIntentID, issue.Provider,
			issue.ProviderReference, issue.IssueType, string(issue.ExpectedStatus), issue.ObservedStatus, issue.Details).Scan(&issueID)
	} else if err == nil {
		due = status == "RESOLVED" || (alertedAt == nil || alertedAt.Before(now.Add(-time.Hour)))
		_, err = tx.Exec(ctx, `
			UPDATE payment_reconciliation_issues
			SET payment_intent_id = NULLIF($3,0), provider = $4, expected_status = NULLIF($5,''),
			    observed_status = NULLIF($6,''), details = $7, status = 'OPEN',
			    last_seen_at = $8, resolved_at = NULL, updated_at = $8
			WHERE company_id = $1 AND connection_id = $2 AND provider_reference = $9 AND issue_type = $10`,
			issue.CompanyID, issue.ConnectionID, issue.PaymentIntentID, issue.Provider,
			string(issue.ExpectedStatus), issue.ObservedStatus, issue.Details, now,
			issue.ProviderReference, issue.IssueType)
	}
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return due, nil
}

func (r *PGRepository) MarkPaymentReconciliationIssueAlerted(ctx context.Context, issue PaymentReconciliationIssue, at time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE payment_reconciliation_issues
		SET alerted_at = $5, updated_at = $5
		WHERE company_id = $1 AND connection_id = $2 AND provider_reference = $3 AND issue_type = $4 AND status = 'OPEN'`,
		issue.CompanyID, issue.ConnectionID, issue.ProviderReference, issue.IssueType, at)
	return err
}

func (r *PGRepository) ResolvePaymentReconciliationIssues(ctx context.Context, companyID, connectionID int64, providerReference string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE payment_reconciliation_issues
		SET status = 'RESOLVED', resolved_at = NOW(), updated_at = NOW()
		WHERE company_id = $1 AND connection_id = $2 AND provider_reference = $3 AND status = 'OPEN'`,
		companyID, connectionID, providerReference)
	return err
}

func (r *PGRepository) RecordConnectorDeadLetter(ctx context.Context, command OutboxCommand, cause error) error {
	if cause == nil {
		return errors.New("connectors: dead-letter cause is required")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO connector_dead_letter_events (
			command_id, company_id, connection_id, command_type, correlation_id, attempts, error_message
		) VALUES ($1,$2,$3,$4,$5,$6,LEFT($7,2000))
		ON CONFLICT (command_id) DO UPDATE
		SET attempts = EXCLUDED.attempts, error_message = EXCLUDED.error_message,
		    dead_lettered_at = NOW(), alerted_at = NULL, replayed_at = NULL, updated_at = NOW()`,
		command.ID, command.CompanyID, command.ConnectionID, command.CommandType,
		command.CorrelationID, command.Attempts+1, cause.Error())
	return err
}

func (r *PGRepository) ListConnectorDeadLetters(ctx context.Context, limit int) ([]ConnectorDeadLetter, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT d.id, d.command_id, d.company_id, d.connection_id, c.provider,
		       d.command_type, d.correlation_id, d.attempts, d.error_message,
		       d.dead_lettered_at, d.alerted_at, d.replayed_at
		FROM connector_dead_letter_events d
		JOIN connector_connections c ON c.id = d.connection_id AND c.company_id = d.company_id
		WHERE d.replayed_at IS NULL
		ORDER BY d.dead_lettered_at ASC, d.id ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ConnectorDeadLetter, 0, limit)
	for rows.Next() {
		var item ConnectorDeadLetter
		if err := rows.Scan(&item.ID, &item.CommandID, &item.CompanyID, &item.ConnectionID,
			&item.Provider, &item.CommandType, &item.CorrelationID, &item.Attempts,
			&item.ErrorMessage, &item.DeadLetteredAt, &item.AlertedAt, &item.ReplayedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PGRepository) MarkConnectorDeadLetterAlerted(ctx context.Context, deadLetterID int64, at time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE connector_dead_letter_events SET alerted_at = $2, updated_at = $2
		WHERE id = $1 AND replayed_at IS NULL`, deadLetterID, at)
	return err
}

func (r *PGRepository) ReplayConnectorDeadLetter(ctx context.Context, deadLetterID int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var commandID int64
	if err := tx.QueryRow(ctx, `
		SELECT command_id FROM connector_dead_letter_events
		WHERE id = $1 AND replayed_at IS NULL
		FOR UPDATE`, deadLetterID).Scan(&commandID); err != nil {
		return err
	}
	var updatedID int64
	if err := tx.QueryRow(ctx, `
		UPDATE connector_outbox_commands
		SET state = 'pending', attempts = 0, next_attempt = NOW(), updated_at = NOW()
		WHERE id = $1 AND state = 'dead_letter'
		RETURNING id`, commandID).Scan(&updatedID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE connector_dead_letter_events SET replayed_at = NOW(), updated_at = NOW() WHERE id = $1`, deadLetterID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PGRepository) EnqueueOutboxCommand(ctx context.Context, input OutboxEnqueueInput) (int64, error) {
	row, err := r.queries.EnqueueOutboxCommand(ctx, sqlc.EnqueueOutboxCommandParams{
		CompanyID:     input.CompanyID,
		ConnectionID:  input.ConnectionID,
		CommandType:   input.CommandType,
		CorrelationID: input.CorrelationID,
		Payload:       input.Payload,
	})
	return row.ID, err
}

func (r *PGRepository) InsertInboxEvent(ctx context.Context, input InboxEventInput) (InboxEvent, error) {
	row, err := r.queries.InsertInboxEvent(ctx, sqlc.InsertInboxEventParams{
		CompanyID:       input.CompanyID,
		ConnectionID:    input.ConnectionID,
		ProviderEventID: input.ProviderEventID,
		RawPayload:      input.RawPayload,
	})
	if err != nil {
		return InboxEvent{}, err
	}
	return mapInboxEvent(row), nil
}

func (r *PGRepository) InsertCanonicalEvent(ctx context.Context, input CanonicalEventInput) (int64, error) {
	row, err := r.queries.InsertCanonicalEvent(ctx, sqlc.InsertCanonicalEventParams{
		CompanyID:     input.CompanyID,
		ConnectionID:  input.ConnectionID,
		EventType:     input.EventType,
		EventTime:     pgtype.Timestamptz{Time: input.EventTime, Valid: true},
		CorrelationID: input.CorrelationID,
		CausationID:   input.CausationID,
		Payload:       input.Payload,
	})
	return row.ID, err
}

func (r *PGRepository) MarkInboxEventProcessed(ctx context.Context, id int64) error {
	return r.queries.MarkInboxEventProcessed(ctx, id)
}

func (r *PGRepository) GetPendingOutboxCommands(ctx context.Context, limit int32) ([]OutboxCommand, error) {
	rows, err := r.queries.GetPendingOutboxCommands(ctx, limit)
	if err != nil {
		return nil, err
	}
	commands := make([]OutboxCommand, len(rows))
	for i, row := range rows {
		commands[i] = mapOutboxCommand(row)
	}
	return commands, nil
}

func (r *PGRepository) UpdateOutboxCommandState(ctx context.Context, update OutboxCommandStateUpdate) error {
	_, err := r.queries.UpdateOutboxCommandState(ctx, sqlc.UpdateOutboxCommandStateParams{
		ID:          update.ID,
		State:       update.State,
		NextAttempt: pgtype.Timestamptz{Time: update.NextAttempt, Valid: true},
	})
	return err
}

func mapConnection(row sqlc.ConnectorConnection) Connection {
	return Connection{
		ID:          row.ID,
		CompanyID:   row.CompanyID,
		Provider:    row.Provider,
		Type:        row.Type,
		Name:        row.Name,
		SecretRef:   row.SecretRef,
		Status:      ConnectionStatus(row.Status),
		LastSync:    timestampPtr(row.LastSync),
		LastError:   textPtr(row.LastError),
		TokenExpiry: timestampPtr(row.TokenExpiry),
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}

func mapInboxEvent(row sqlc.ConnectorInboxEvent) InboxEvent {
	return InboxEvent{
		ID:              row.ID,
		CompanyID:       row.CompanyID,
		ConnectionID:    row.ConnectionID,
		ProviderEventID: row.ProviderEventID,
		RawPayload:      row.RawPayload,
		Processed:       row.Processed,
		CreatedAt:       row.CreatedAt.Time,
		ProcessedAt:     timestampPtr(row.ProcessedAt),
	}
}

func mapOutboxCommand(row sqlc.ConnectorOutboxCommand) OutboxCommand {
	return OutboxCommand{
		ID:            row.ID,
		CompanyID:     row.CompanyID,
		ConnectionID:  row.ConnectionID,
		CommandType:   row.CommandType,
		CorrelationID: row.CorrelationID,
		Payload:       row.Payload,
		State:         row.State,
		Attempts:      int(row.Attempts),
		NextAttempt:   row.NextAttempt.Time,
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}
}

func mapPaymentIntent(row sqlc.PaymentIntent) PaymentIntent {
	return PaymentIntent{
		ID:                row.ID,
		CompanyID:         row.CompanyID,
		ConnectionID:      row.ConnectionID,
		SourceType:        row.SourceType,
		SourceID:          row.SourceID,
		Amount:            numericToFloat64(row.Amount),
		Currency:          row.Currency,
		Status:            PaymentStatus(row.Status),
		ProviderReference: row.ProviderReference.String,
		CheckoutURL:       row.CheckoutUrl.String,
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
	}
}

func numericToFloat64(value pgtype.Numeric) float64 {
	if !value.Valid {
		return 0
	}
	parsed, err := value.Float64Value()
	if err != nil || !parsed.Valid {
		return 0
	}
	return parsed.Float64
}

func numericOf(value float64) pgtype.Numeric {
	var numeric pgtype.Numeric
	_ = numeric.Scan(fmt.Sprintf("%.2f", value))
	return numeric
}

func optionalTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func timestampPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
