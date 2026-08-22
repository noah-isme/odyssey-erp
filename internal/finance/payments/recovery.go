package payments

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
	"github.com/odyssey-erp/odyssey-erp/internal/observability"
)

// PaymentRecoveryIssue identifies an operational condition that requires a
// finance operator. These values are deliberately small and stable because
// they are also used as bounded metric and notification labels.
type PaymentRecoveryIssue string

const (
	PaymentRecoveryIssueAmbiguous        PaymentRecoveryIssue = "AMBIGUOUS"
	PaymentRecoveryIssuePartial          PaymentRecoveryIssue = "PARTIAL_SETTLEMENT"
	PaymentRecoveryIssueFailed           PaymentRecoveryIssue = "FAILED"
	PaymentRecoveryIssueUnappliedEffects PaymentRecoveryIssue = "UNAPPLIED_EFFECTS"
	PaymentRecoveryIssueUnmatched        PaymentRecoveryIssue = "UNMATCHED_SETTLEMENT"
	PaymentRecoveryIssueDeadLetter       PaymentRecoveryIssue = "DEAD_LETTER"
	PaymentRecoveryIssueStalledOutbox    PaymentRecoveryIssue = "STALLED_OUTBOX"
)

var (
	ErrPaymentRecoveryRepositoryNotConfigured = errors.New("finance payments: recovery repository is not configured")
	ErrPaymentRecoveryCaseInvalid             = errors.New("finance payments: invalid recovery case")
)

// PaymentRecoveryCase is a safe, company-scoped projection of unresolved
// execution/result/outbox state. It intentionally contains no provider
// payload, credential, beneficiary, or raw error text.
type PaymentRecoveryCase struct {
	CompanyID       int64
	ConnectionID    int64
	Provider        string
	InstructionType string
	InstructionID   string
	Issue           PaymentRecoveryIssue
	State           string
	ObservedAt      time.Time
	Details         string
}

func (c PaymentRecoveryCase) Validate() error {
	if c.CompanyID <= 0 || strings.TrimSpace(c.Provider) == "" ||
		strings.TrimSpace(c.InstructionType) == "" || strings.TrimSpace(c.InstructionID) == "" ||
		strings.TrimSpace(string(c.Issue)) == "" || strings.TrimSpace(c.State) == "" {
		return ErrPaymentRecoveryCaseInvalid
	}
	return nil
}

// Key is stable across scans and is safe to use as a notification idempotency
// key. An issue state transition naturally creates a new key and therefore a
// new operator notification.
func (c PaymentRecoveryCase) Key() string {
	return fmt.Sprintf("finance-payment-recovery:%d:%d:%s:%s:%s:%s",
		c.CompanyID,
		c.ConnectionID,
		strings.ToLower(strings.TrimSpace(c.Provider)),
		strings.ToLower(strings.TrimSpace(c.InstructionType)),
		strings.TrimSpace(c.InstructionID),
		strings.ToLower(strings.TrimSpace(string(c.Issue))),
	)
}

// PaymentRecoveryDB is the read-only database surface needed by the scan. It
// is deliberately narrower than the finance outbox repository so the scan
// cannot claim, replay, or otherwise mutate a financial command.
type PaymentRecoveryDB interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type PaymentRecoveryRepository struct {
	db PaymentRecoveryDB
}

func NewPaymentRecoveryRepository(db PaymentRecoveryDB) *PaymentRecoveryRepository {
	return &PaymentRecoveryRepository{db: db}
}

const paymentRecoveryCasesSQL = `
WITH unresolved AS (
    SELECT
        e.company_id,
        e.connection_id,
        e.provider,
        e.object_type AS instruction_type,
        e.object_id AS instruction_id,
        e.state,
        e.updated_at AS observed_at,
        CASE
            WHEN e.state = 'AMBIGUOUS' THEN 'AMBIGUOUS'
            WHEN e.state = 'FAILED' THEN 'FAILED'
            ELSE 'PARTIAL_SETTLEMENT'
        END AS issue,
        CASE
            WHEN e.state = 'AMBIGUOUS' THEN 'Provider outcome is unresolved; lookup is required before any replay.'
            WHEN e.state = 'FAILED' THEN 'Payment execution failed and requires operator review.'
            ELSE 'Payment execution remains partially settled and requires follow-up.'
        END AS details
    FROM payment_executions e
    WHERE e.state IN ('AMBIGUOUS', 'FAILED', 'PARTIALLY_SETTLED')

    UNION ALL

    SELECT
        r.company_id,
        r.connection_id,
        r.provider,
        r.instruction_type,
        r.instruction_id,
        r.state,
        r.recorded_at AS observed_at,
        CASE
            WHEN r.state = 'PARTIALLY_SETTLED' THEN 'PARTIAL_SETTLEMENT'
            ELSE 'UNAPPLIED_EFFECTS'
        END AS issue,
        CASE
            WHEN r.state = 'PARTIALLY_SETTLED' THEN 'Confirmed partial settlement requires operator follow-up.'
            ELSE 'Confirmed settlement has not applied its accounting effects.'
        END AS details
    FROM payment_settlement_results r
    WHERE r.state IN ('PARTIALLY_SETTLED', 'SETTLED')
      AND (r.state = 'PARTIALLY_SETTLED' OR NOT r.effect_applied)

    UNION ALL

    SELECT
        r.company_id,
        r.connection_id,
        r.provider,
        r.instruction_type,
        r.instruction_id,
        r.state,
        r.recorded_at AS observed_at,
        'UNMATCHED_SETTLEMENT' AS issue,
        'Confirmed settlement has no linked bank reconciliation record.' AS details
    FROM payment_settlement_results r
    WHERE r.state IN ('PARTIALLY_SETTLED', 'SETTLED')
      AND r.effect_applied
      AND r.recorded_at < NOW() - ($3 * INTERVAL '1 microsecond')
      AND NOT EXISTS (
          SELECT 1
          FROM payment_settlement_effects e
          JOIN payment_settlement_effect_links l
            ON l.company_id = e.company_id AND l.effect_key = e.effect_key
          WHERE e.company_id = r.company_id
            AND e.result_id = r.result_id
            AND (
                l.entity_type IN ('bank_statement', 'bank_reconciliation')
                OR (
                    l.entity_type = 'bank_transaction'
                    AND EXISTS (
                        SELECT 1
                        FROM bank_transactions bt
                        JOIN bank_accounts ba ON ba.id = bt.bank_account_id
                        WHERE ba.company_id = r.company_id
                          AND bt.id::text = l.entity_id
                          AND bt.status = 'RECONCILED'
                    )
                )
            )
      )

    UNION ALL

    SELECT
        o.company_id,
        NULL::BIGINT AS connection_id,
        'finance-outbox' AS provider,
        o.aggregate_type AS instruction_type,
        o.aggregate_id AS instruction_id,
        o.status AS state,
        COALESCE(o.locked_at, o.updated_at) AS observed_at,
        CASE WHEN o.status = 'DEAD_LETTERED' THEN 'DEAD_LETTER' ELSE 'STALLED_OUTBOX' END AS issue,
        CASE
            WHEN o.status = 'DEAD_LETTERED' THEN 'Payment command exhausted its retry budget; explicit recovery is required.'
            ELSE 'Payment command has held a worker lease beyond the recovery threshold.'
        END AS details
    FROM finance_automation_outbox o
    WHERE o.operation IN ('payment.execute', 'payment.submit', 'payment.result.import')
      AND (
          o.status = 'DEAD_LETTERED'
          OR (o.status = 'PROCESSING' AND o.locked_at IS NOT NULL
              AND o.locked_at < NOW() - ($2 * INTERVAL '1 microsecond'))
      )
)
SELECT company_id, connection_id, provider, instruction_type, instruction_id,
       issue, state, observed_at, details
FROM unresolved
ORDER BY observed_at ASC, company_id ASC, instruction_id ASC
LIMIT $1`

// UnresolvedCases reads all finance payment state needed for operator alerts
// in one bounded query. It never mutates an outbox, execution, result, or
// effect row.
func (r *PaymentRecoveryRepository) UnresolvedCases(ctx context.Context, limit int, staleProcessingAfter, unmatchedAfter time.Duration) ([]PaymentRecoveryCase, error) {
	if r == nil || r.db == nil {
		return nil, ErrPaymentRecoveryRepositoryNotConfigured
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	if staleProcessingAfter <= 0 {
		staleProcessingAfter = 10 * time.Minute
	}
	if unmatchedAfter <= 0 {
		unmatchedAfter = 10 * time.Minute
	}
	rows, err := r.db.Query(ctx, paymentRecoveryCasesSQL, limit, staleProcessingAfter.Microseconds(), unmatchedAfter.Microseconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cases := make([]PaymentRecoveryCase, 0, limit)
	seen := make(map[string]struct{}, limit)
	for rows.Next() {
		var (
			item       PaymentRecoveryCase
			connection any
			issue      string
		)
		if err := rows.Scan(
			&item.CompanyID,
			&connection,
			&item.Provider,
			&item.InstructionType,
			&item.InstructionID,
			&issue,
			&item.State,
			&item.ObservedAt,
			&item.Details,
		); err != nil {
			return nil, err
		}
		switch value := connection.(type) {
		case int64:
			item.ConnectionID = value
		case int32:
			item.ConnectionID = int64(value)
		case nil:
			// Finance outbox alerts do not have a provider connection ID.
		default:
			return nil, fmt.Errorf("%w: invalid connection id type %T", ErrPaymentRecoveryCaseInvalid, connection)
		}
		item.Issue = PaymentRecoveryIssue(issue)
		if err := item.Validate(); err != nil {
			return nil, err
		}
		if _, ok := seen[item.Key()]; ok {
			continue
		}
		seen[item.Key()] = struct{}{}
		cases = append(cases, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cases, nil
}

const paymentRecoveryRecipientsSQL = `
SELECT DISTINCT u.id
FROM users u
JOIN rbac_user_role_assignments a ON a.user_id = u.id
JOIN roles r ON r.id = a.role_id
WHERE u.is_active
  AND a.company_id = $1
  AND COALESCE(a.valid_from, TIMESTAMPTZ '-infinity') <= NOW()
  AND (a.valid_to IS NULL OR a.valid_to > NOW())
  AND LOWER(TRIM(r.name)) IN ('admin', 'administrator', 'finance manager', 'finance user')
ORDER BY u.id`

// Recipients returns active company-scoped finance operators. It does not use
// an object ID from a case and therefore cannot widen notification scope.
func (r *PaymentRecoveryRepository) Recipients(ctx context.Context, companyID int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, ErrPaymentRecoveryRepositoryNotConfigured
	}
	if companyID <= 0 {
		return nil, ErrPaymentRecoveryCaseInvalid
	}
	rows, err := r.db.Query(ctx, paymentRecoveryRecipientsSQL, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0, 4)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id > 0 {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

// PaymentRecoveryScanConfig controls the bounded worker scan. The scan is
// notification-only: it never retries or replays a provider command.
type PaymentRecoveryScanConfig struct {
	Limit                int
	StaleProcessingAfter time.Duration
	UnmatchedAfter       time.Duration
	Now                  func() time.Time
}

func DefaultPaymentRecoveryScanConfig() PaymentRecoveryScanConfig {
	return PaymentRecoveryScanConfig{
		Limit:                100,
		StaleProcessingAfter: 10 * time.Minute,
		UnmatchedAfter:       10 * time.Minute,
		Now:                  time.Now,
	}
}

type PaymentRecoveryNotifier interface {
	Dispatch(context.Context, notifications.Message) error
}

type PaymentRecoveryScanner struct {
	repository *PaymentRecoveryRepository
	notifier   PaymentRecoveryNotifier
	metrics    *observability.PaymentRecoveryMetrics
	config     PaymentRecoveryScanConfig
}

func NewPaymentRecoveryScanner(repository *PaymentRecoveryRepository, notifier PaymentRecoveryNotifier, metrics *observability.PaymentRecoveryMetrics, configs ...PaymentRecoveryScanConfig) *PaymentRecoveryScanner {
	config := DefaultPaymentRecoveryScanConfig()
	if len(configs) > 0 {
		provided := configs[0]
		if provided.Limit != 0 {
			config.Limit = provided.Limit
		}
		if provided.StaleProcessingAfter != 0 {
			config.StaleProcessingAfter = provided.StaleProcessingAfter
		}
		if provided.UnmatchedAfter != 0 {
			config.UnmatchedAfter = provided.UnmatchedAfter
		}
		if provided.Now != nil {
			config.Now = provided.Now
		}
	}
	return &PaymentRecoveryScanner{repository: repository, notifier: notifier, metrics: metrics, config: config}
}

type PaymentRecoveryScanReport struct {
	Cases         int
	Notifications int
	Companies     int
}

// Scan emits deduplicated notifications for unresolved finance payment cases.
// The notification key is stable for the case, so repeated scans and worker
// restarts do not create duplicate operator work. All failures are returned
// after best-effort processing of the remaining cases.
func (s *PaymentRecoveryScanner) Scan(ctx context.Context) (PaymentRecoveryScanReport, error) {
	if s == nil || s.repository == nil {
		return PaymentRecoveryScanReport{}, ErrPaymentRecoveryRepositoryNotConfigured
	}
	if s.config.Now == nil {
		s.config.Now = time.Now
	}
	cases, err := s.repository.UnresolvedCases(ctx, s.config.Limit, s.config.StaleProcessingAfter, s.config.UnmatchedAfter)
	if err != nil {
		return PaymentRecoveryScanReport{}, err
	}
	report := PaymentRecoveryScanReport{Cases: len(cases)}
	companies := make(map[int64]struct{})
	recipientsByCompany := make(map[int64][]int64)
	var failures []error
	ages := make(map[string]time.Duration)
	unapplied := make(map[string]int)
	unmatched := make(map[string]int)

	for _, item := range cases {
		companies[item.CompanyID] = struct{}{}
		provider := metricProvider(item.Provider)
		if s.metrics != nil {
			s.metrics.ObserveFinanceExecution(provider, item.State)
		}
		switch item.Issue {
		case PaymentRecoveryIssueAmbiguous:
			age := s.config.Now().Sub(item.ObservedAt)
			if age < 0 {
				age = 0
			}
			if previous, ok := ages[provider]; !ok || age > previous {
				ages[provider] = age
			}
		case PaymentRecoveryIssueUnappliedEffects:
			unapplied[provider+"\x00"+item.State]++
		case PaymentRecoveryIssueUnmatched:
			unmatched[provider]++
		case PaymentRecoveryIssueDeadLetter:
			if s.metrics != nil {
				s.metrics.ObserveFinanceDeadLetter(item.State)
			}
		}

		if s.notifier == nil {
			continue
		}
		recipients, ok := recipientsByCompany[item.CompanyID]
		if !ok {
			recipients, err = s.repository.Recipients(ctx, item.CompanyID)
			if err != nil {
				failures = append(failures, fmt.Errorf("company %d recipients: %w", item.CompanyID, err))
				recipientsByCompany[item.CompanyID] = nil
				continue
			}
			recipientsByCompany[item.CompanyID] = recipients
		}
		if len(recipients) == 0 {
			if s.metrics != nil {
				s.metrics.ObserveRecoveryAttempt("alert", "no_recipients")
			}
			continue
		}
		for _, recipientID := range recipients {
			if s.metrics != nil {
				s.metrics.ObserveRecoveryAttempt("alert", "attempted")
			}
			if err := s.notifier.Dispatch(ctx, financeRecoveryMessage(recipientID, item)); err != nil {
				if s.metrics != nil {
					s.metrics.ObserveRecoveryAttempt("alert", "failed")
				}
				failures = append(failures, fmt.Errorf("notify company %d payment %s: %w", item.CompanyID, item.InstructionID, err))
				continue
			}
			if s.metrics != nil {
				s.metrics.ObserveRecoverySuccess("alert")
			}
			report.Notifications++
		}
	}

	for provider, age := range ages {
		if s.metrics != nil {
			s.metrics.SetAmbiguousAge(provider, age)
		}
	}
	for key, count := range unapplied {
		parts := strings.SplitN(key, "\x00", 2)
		state := "UNKNOWN"
		if len(parts) == 2 {
			state = parts[1]
		}
		if s.metrics != nil {
			s.metrics.SetUnappliedEffects(parts[0], state, count)
		}
	}
	for provider, count := range unmatched {
		if s.metrics != nil {
			s.metrics.SetUnmatchedSettlements(provider, count)
		}
	}

	report.Companies = len(companies)
	return report, errors.Join(failures...)
}

func financeRecoveryMessage(recipientID int64, item PaymentRecoveryCase) notifications.Message {
	title := "Finance payment requires review"
	switch item.Issue {
	case PaymentRecoveryIssueAmbiguous:
		title = "Finance payment outcome is ambiguous"
	case PaymentRecoveryIssuePartial:
		title = "Finance payment is partially settled"
	case PaymentRecoveryIssueFailed:
		title = "Finance payment execution failed"
	case PaymentRecoveryIssueUnappliedEffects:
		title = "Finance settlement effects require review"
	case PaymentRecoveryIssueUnmatched:
		title = "Finance settlement is unmatched"
	case PaymentRecoveryIssueDeadLetter, PaymentRecoveryIssueStalledOutbox:
		title = "Finance payment worker recovery required"
	}
	body := fmt.Sprintf("Payment %s (%s) is in %s. %s", item.InstructionID, item.Issue, item.State, item.Details)
	return notifications.Message{
		RecipientID: recipientID,
		DedupeKey:   item.Key(),
		Type:        notifications.TypeFinancePaymentRecovery,
		Title:       title,
		Body:        body,
		URL:         "/finance/treasury/operations",
	}
}

// metricProvider prevents database/provider configuration values from
// becoming unbounded Prometheus label cardinality.
func metricProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "midtrans_iris", "midtrans-iris", "midtransiris", "iris":
		return "midtrans_iris"
	case "midtrans":
		return "midtrans"
	case "stripe":
		return "stripe"
	case "bank_file", "bank-file":
		return "bank_file"
	case "finance-outbox":
		return "finance_outbox"
	default:
		return "other"
	}
}

// Sort cases in tests and callers that combine multiple bounded scans.
func SortPaymentRecoveryCases(cases []PaymentRecoveryCase) {
	sort.SliceStable(cases, func(i, j int) bool {
		if cases[i].ObservedAt.Equal(cases[j].ObservedAt) {
			return cases[i].Key() < cases[j].Key()
		}
		return cases[i].ObservedAt.Before(cases[j].ObservedAt)
	})
}
