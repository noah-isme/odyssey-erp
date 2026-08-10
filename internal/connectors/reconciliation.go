package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	ReconciliationRunSuccess = "SUCCESS"
	ReconciliationRunPartial = "PARTIAL"
	ReconciliationRunFailed  = "FAILED"

	PaymentIssueProviderUnavailable = "PROVIDER_UNAVAILABLE"
	PaymentIssueLookupFailed        = "LOOKUP_FAILED"
	PaymentIssueUnknownStatus       = "UNKNOWN_PROVIDER_STATUS"
	PaymentIssueMissingIntent       = "MISSING_LOCAL_INTENT"
	PaymentIssueStateMismatch       = "STATE_MISMATCH"
)

// PaymentReconciliationCandidate is a local payment that can be checked
// against a provider status endpoint. The connection is included so the
// worker can resolve credentials without widening the provider interface.
type PaymentReconciliationCandidate struct {
	Intent     PaymentIntent
	Connection Connection
}

type PaymentReconciliationIssue struct {
	CompanyID         int64
	ConnectionID      int64
	PaymentIntentID   int64
	Provider          string
	ProviderReference string
	IssueType         string
	ExpectedStatus    PaymentStatus
	ObservedStatus    string
	Details           string
}

type PaymentReconciliationRun struct {
	StartedAt        time.Time
	FinishedAt       time.Time
	Status           string
	ScannedCount     int
	RecoveredCount   int
	MatchedCount     int
	UnmatchedCount   int
	UnsupportedCount int
	ErrorCount       int
	RefundsPersisted int
	DeadLetterCount  int
	ErrorMessage     string
}

type PaymentReconciliationReport struct {
	StartedAt        time.Time
	FinishedAt       time.Time
	Status           string
	Scanned          int
	Recovered        int
	Matched          int
	Unmatched        int
	Unsupported      int
	Errors           int
	RefundsPersisted int
	DeadLetters      int
}

type ConnectorDeadLetter struct {
	ID             int64
	CommandID      int64
	CompanyID      int64
	ConnectionID   int64
	Provider       string
	CommandType    string
	CorrelationID  string
	Attempts       int
	ErrorMessage   string
	DeadLetteredAt time.Time
	AlertedAt      *time.Time
	ReplayedAt     *time.Time
}

// PaymentReconciliationStore is the durable boundary for scheduled provider
// checks, issues, run metrics, and connector dead letters.
type PaymentReconciliationStore interface {
	ListPaymentReconciliationCandidates(context.Context, int, time.Time) ([]PaymentReconciliationCandidate, error)
	ApplyPaymentIntentEvent(context.Context, PaymentIntentEventInput) (PaymentTransitionResult, error)
	RecordPaymentReconciliationRun(context.Context, PaymentReconciliationRun) error
	UpsertPaymentReconciliationIssue(context.Context, PaymentReconciliationIssue) (bool, error)
	MarkPaymentReconciliationIssueAlerted(context.Context, PaymentReconciliationIssue, time.Time) error
	ResolvePaymentReconciliationIssues(context.Context, int64, int64, string) error
	ListConnectorDeadLetters(context.Context, int) ([]ConnectorDeadLetter, error)
	MarkConnectorDeadLetterAlerted(context.Context, int64, time.Time) error
	ReplayConnectorDeadLetter(context.Context, int64) error
}

// PaymentReconciliationAlertSink delivers durable operator alerts. The
// PostgreSQL issue/dead-letter records remain the source of truth even when a
// notification channel is unavailable.
type PaymentReconciliationAlertSink interface {
	AlertUnmatchedPayment(context.Context, PaymentReconciliationIssue) error
	AlertConnectorDeadLetter(context.Context, ConnectorDeadLetter) error
}

// PaymentRecoveryMetrics is intentionally small so the reconciliation service
// can be instrumented without depending on a particular metrics backend.
type PaymentRecoveryMetrics interface {
	ObserveReconciliationRun(status string)
	ObserveCandidate(provider, localStatus string)
	ObserveRecovery(provider, fromStatus, toStatus string)
	ObserveIssue(provider, issueType string)
	ObserveRefund(provider, status string)
	ObserveDeadLetter(commandType string)
}

// PaymentReconciliationService periodically resolves local payment intents
// through provider status APIs. It treats a provider lookup as another
// canonical payment event, so timeout recovery and webhook processing share
// the same monotonic reducer and refund persistence path.
type PaymentReconciliationService struct {
	store      PaymentReconciliationStore
	registry   ProviderRegistry
	logger     *slog.Logger
	metrics    PaymentRecoveryMetrics
	alerts     PaymentReconciliationAlertSink
	staleAfter time.Duration
	limit      int
	now        func() time.Time
}

func NewPaymentReconciliationService(
	store PaymentReconciliationStore,
	registry ProviderRegistry,
	logger *slog.Logger,
	metrics PaymentRecoveryMetrics,
	alerts PaymentReconciliationAlertSink,
) *PaymentReconciliationService {
	if logger == nil {
		logger = slog.Default()
	}
	return &PaymentReconciliationService{
		store:      store,
		registry:   registry,
		logger:     logger,
		metrics:    metrics,
		alerts:     alerts,
		staleAfter: 2 * time.Minute,
		limit:      100,
		now:        time.Now,
	}
}

// Reconcile performs one bounded reconciliation pass. Provider failures are
// recorded as open issues and returned after the pass so one bad connection
// cannot starve every other tenant's recovery work.
func (s *PaymentReconciliationService) Reconcile(ctx context.Context) (PaymentReconciliationReport, error) {
	if s == nil || s.store == nil || s.registry == nil {
		return PaymentReconciliationReport{}, errors.New("connectors: payment reconciliation is not configured")
	}
	started := s.now().UTC()
	report := PaymentReconciliationReport{StartedAt: started}
	var failures []error

	candidates, err := s.store.ListPaymentReconciliationCandidates(ctx, s.limit, started.Add(-s.staleAfter))
	if err != nil {
		report.Status = ReconciliationRunFailed
		report.FinishedAt = s.now().UTC()
		_ = s.recordRun(ctx, report, err)
		return report, fmt.Errorf("connectors: list payment reconciliation candidates: %w", err)
	}

	for _, candidate := range candidates {
		report.Scanned++
		if s.metrics != nil {
			s.metrics.ObserveCandidate(candidate.Connection.Provider, string(candidate.Intent.Status))
		}

		adapter, err := s.registry.GetAdapter(candidate.Connection.Provider)
		if err != nil {
			report.Unsupported++
			failures = append(failures, s.openIssue(ctx, PaymentReconciliationIssue{
				CompanyID: candidate.Intent.CompanyID, ConnectionID: candidate.Intent.ConnectionID,
				PaymentIntentID: candidate.Intent.ID, Provider: candidate.Connection.Provider,
				ProviderReference: candidate.Intent.ProviderReference, IssueType: PaymentIssueProviderUnavailable,
				ExpectedStatus: candidate.Intent.Status, Details: err.Error(),
			})...)
			failures = append(failures, fmt.Errorf("provider %q unavailable: %w", candidate.Connection.Provider, err))
			continue
		}
		lookup, ok := adapter.(PaymentStatusLookup)
		if !ok {
			report.Unsupported++
			failures = append(failures, s.openIssue(ctx, PaymentReconciliationIssue{
				CompanyID: candidate.Intent.CompanyID, ConnectionID: candidate.Intent.ConnectionID,
				PaymentIntentID: candidate.Intent.ID, Provider: candidate.Connection.Provider,
				ProviderReference: candidate.Intent.ProviderReference, IssueType: PaymentIssueProviderUnavailable,
				ExpectedStatus: candidate.Intent.Status, Details: "provider adapter does not implement payment status lookup",
			})...)
			failures = append(failures, fmt.Errorf("provider %q does not support payment status lookup", candidate.Connection.Provider))
			continue
		}

		snapshot, err := lookup.LookupPaymentStatus(ctx, &candidate.Connection, candidate.Intent.ProviderReference)
		if err != nil {
			report.Errors++
			report.Unmatched++
			failures = append(failures, s.openIssue(ctx, PaymentReconciliationIssue{
				CompanyID: candidate.Intent.CompanyID, ConnectionID: candidate.Intent.ConnectionID,
				PaymentIntentID: candidate.Intent.ID, Provider: candidate.Connection.Provider,
				ProviderReference: candidate.Intent.ProviderReference, IssueType: PaymentIssueLookupFailed,
				ExpectedStatus: candidate.Intent.Status, Details: err.Error(),
			})...)
			failures = append(failures, fmt.Errorf("payment status lookup for %s: %w", candidate.Intent.ProviderReference, err))
			continue
		}

		providerReference := strings.TrimSpace(snapshot.ProviderReference)
		if providerReference == "" {
			providerReference = candidate.Intent.ProviderReference
		}
		if strings.TrimSpace(snapshot.EventType) == "" {
			report.Errors++
			report.Unmatched++
			failures = append(failures, s.openIssue(ctx, PaymentReconciliationIssue{
				CompanyID: candidate.Intent.CompanyID, ConnectionID: candidate.Intent.ConnectionID,
				PaymentIntentID: candidate.Intent.ID, Provider: candidate.Connection.Provider,
				ProviderReference: providerReference, IssueType: PaymentIssueUnknownStatus,
				ExpectedStatus: candidate.Intent.Status, ObservedStatus: snapshot.Status,
				Details: "provider status did not map to a canonical payment event",
			})...)
			failures = append(failures, fmt.Errorf("provider status %q for %s has no canonical event", snapshot.Status, providerReference))
			continue
		}

		eventID := "status-" + providerReference + "-" + snapshot.EventType
		var rawPayload []byte
		if strings.EqualFold(snapshot.EventType, "payment.refunded") || strings.EqualFold(snapshot.EventType, "payment.partially_refunded") {
			rawPayload, _ = json.Marshal(struct {
				RefundKey    string `json:"refund_key,omitempty"`
				RefundAmount string `json:"refund_amount,omitempty"`
				Reason       string `json:"reason,omitempty"`
			}{RefundKey: snapshot.RefundKey, RefundAmount: snapshot.RefundAmount, Reason: snapshot.RefundReason})
		}
		transition, err := s.store.ApplyPaymentIntentEvent(ctx, PaymentIntentEventInput{
			CompanyID: candidate.Intent.CompanyID, ConnectionID: candidate.Intent.ConnectionID,
			ProviderReference: providerReference, EventType: snapshot.EventType,
			ProviderEventID: eventID, OccurredAt: snapshot.OccurredAt, RawPayload: rawPayload,
		})
		if err != nil {
			report.Errors++
			issueType := PaymentIssueStateMismatch
			if errors.Is(err, pgx.ErrNoRows) {
				issueType = PaymentIssueMissingIntent
			}
			failures = append(failures, s.openIssue(ctx, PaymentReconciliationIssue{
				CompanyID: candidate.Intent.CompanyID, ConnectionID: candidate.Intent.ConnectionID,
				PaymentIntentID: candidate.Intent.ID, Provider: candidate.Connection.Provider,
				ProviderReference: providerReference, IssueType: issueType,
				ExpectedStatus: candidate.Intent.Status, ObservedStatus: snapshot.Status,
				Details: err.Error(),
			})...)
			failures = append(failures, fmt.Errorf("apply payment reconciliation event for %s: %w", providerReference, err))
			continue
		}

		if transition.Applied {
			report.Recovered++
			if _, isRefund := PaymentStatusForEvent(snapshot.EventType); isRefund && (strings.EqualFold(snapshot.EventType, "payment.refunded") || strings.EqualFold(snapshot.EventType, "payment.partially_refunded")) {
				report.RefundsPersisted++
				if s.metrics != nil {
					s.metrics.ObserveRefund(candidate.Connection.Provider, string(transition.ToStatus))
				}
			}
			if s.metrics != nil {
				s.metrics.ObserveRecovery(candidate.Connection.Provider, string(transition.FromStatus), string(transition.ToStatus))
			}
			if err := s.store.ResolvePaymentReconciliationIssues(ctx, candidate.Intent.CompanyID, candidate.Intent.ConnectionID, providerReference); err != nil {
				failures = append(failures, err)
			}
			continue
		}

		if transition.Duplicate || transition.OutOfOrder {
			report.Matched++
			if err := s.store.ResolvePaymentReconciliationIssues(ctx, candidate.Intent.CompanyID, candidate.Intent.ConnectionID, providerReference); err != nil {
				failures = append(failures, err)
			}
			continue
		}

		report.Unmatched++
		failures = append(failures, s.openIssue(ctx, PaymentReconciliationIssue{
			CompanyID: candidate.Intent.CompanyID, ConnectionID: candidate.Intent.ConnectionID,
			PaymentIntentID: candidate.Intent.ID, Provider: candidate.Connection.Provider,
			ProviderReference: providerReference, IssueType: PaymentIssueStateMismatch,
			ExpectedStatus: candidate.Intent.Status, ObservedStatus: snapshot.Status,
			Details: transition.IgnoredReason,
		})...)
		failures = append(failures, fmt.Errorf("payment reconciliation mismatch for %s: %s", providerReference, transition.IgnoredReason))
	}

	report.FinishedAt = s.now().UTC()
	report.Status = ReconciliationRunSuccess
	if len(failures) > 0 || report.Unmatched > 0 || report.Errors > 0 {
		report.Status = ReconciliationRunPartial
	}
	if err := s.recordRun(ctx, report, errors.Join(failures...)); err != nil {
		failures = append(failures, err)
	}
	if s.metrics != nil {
		s.metrics.ObserveReconciliationRun(report.Status)
	}
	return report, errors.Join(failures...)
}

func (s *PaymentReconciliationService) recordRun(ctx context.Context, report PaymentReconciliationReport, cause error) error {
	if s == nil || s.store == nil {
		return nil
	}
	run := PaymentReconciliationRun{
		StartedAt: report.StartedAt, FinishedAt: report.FinishedAt, Status: report.Status,
		ScannedCount: report.Scanned, RecoveredCount: report.Recovered, MatchedCount: report.Matched,
		UnmatchedCount: report.Unmatched, UnsupportedCount: report.Unsupported, ErrorCount: report.Errors,
		RefundsPersisted: report.RefundsPersisted, DeadLetterCount: report.DeadLetters,
	}
	if cause != nil {
		run.ErrorMessage = cause.Error()
	}
	return s.store.RecordPaymentReconciliationRun(ctx, run)
}

func (s *PaymentReconciliationService) openIssue(ctx context.Context, issue PaymentReconciliationIssue) []error {
	if s.metrics != nil {
		s.metrics.ObserveIssue(issue.Provider, issue.IssueType)
	}
	due, err := s.store.UpsertPaymentReconciliationIssue(ctx, issue)
	if err != nil {
		return []error{err}
	}
	if !due || s.alerts == nil {
		return nil
	}
	if err := s.alerts.AlertUnmatchedPayment(ctx, issue); err != nil {
		return []error{err}
	}
	if err := s.store.MarkPaymentReconciliationIssueAlerted(ctx, issue, s.now().UTC()); err != nil {
		return []error{err}
	}
	return nil
}

// AuditDeadLetters alerts on connector commands that exhausted their retry
// budget. Replays are explicit and remain available through ReplayDeadLetter.
func (s *PaymentReconciliationService) AuditDeadLetters(ctx context.Context, limit int) (int, error) {
	if s == nil || s.store == nil {
		return 0, errors.New("connectors: dead-letter audit is not configured")
	}
	if limit <= 0 {
		limit = 100
	}
	deadLetters, err := s.store.ListConnectorDeadLetters(ctx, limit)
	if err != nil {
		return 0, err
	}
	var failures []error
	alertBefore := s.now().UTC().Add(-time.Hour)
	for _, deadLetter := range deadLetters {
		if s.metrics != nil {
			s.metrics.ObserveDeadLetter(deadLetter.CommandType)
		}
		if s.alerts == nil || (deadLetter.AlertedAt != nil && deadLetter.AlertedAt.After(alertBefore)) {
			continue
		}
		if err := s.alerts.AlertConnectorDeadLetter(ctx, deadLetter); err != nil {
			failures = append(failures, err)
			continue
		}
		if err := s.store.MarkConnectorDeadLetterAlerted(ctx, deadLetter.ID, s.now().UTC()); err != nil {
			failures = append(failures, err)
		}
	}
	return len(deadLetters), errors.Join(failures...)
}

// ReplayDeadLetter requeues one operator-selected dead letter. It does not
// automatically replay financial commands after a timeout or ambiguous result.
func (s *PaymentReconciliationService) ReplayDeadLetter(ctx context.Context, deadLetterID int64) error {
	if s == nil || s.store == nil || deadLetterID <= 0 {
		return errors.New("connectors: dead-letter ID is required")
	}
	return s.store.ReplayConnectorDeadLetter(ctx, deadLetterID)
}
