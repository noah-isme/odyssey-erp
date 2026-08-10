package connectors_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

type reconciliationStoreFake struct {
	candidates  []connectors.PaymentReconciliationCandidate
	transition  connectors.PaymentTransitionResult
	applyErr    error
	runs        []connectors.PaymentReconciliationRun
	issues      []connectors.PaymentReconciliationIssue
	alerted     []connectors.PaymentReconciliationIssue
	resolved    []string
	deadLetters []connectors.ConnectorDeadLetter
	deadAlerted []int64
	replayed    []int64
}

func (f *reconciliationStoreFake) ListPaymentReconciliationCandidates(context.Context, int, time.Time) ([]connectors.PaymentReconciliationCandidate, error) {
	return f.candidates, nil
}
func (f *reconciliationStoreFake) ApplyPaymentIntentEvent(_ context.Context, input connectors.PaymentIntentEventInput) (connectors.PaymentTransitionResult, error) {
	if f.applyErr != nil {
		return connectors.PaymentTransitionResult{}, f.applyErr
	}
	if input.ProviderEventID == "" {
		return connectors.PaymentTransitionResult{}, errors.New("provider event ID missing")
	}
	return f.transition, nil
}
func (f *reconciliationStoreFake) RecordPaymentReconciliationRun(_ context.Context, run connectors.PaymentReconciliationRun) error {
	f.runs = append(f.runs, run)
	return nil
}
func (f *reconciliationStoreFake) UpsertPaymentReconciliationIssue(_ context.Context, issue connectors.PaymentReconciliationIssue) (bool, error) {
	f.issues = append(f.issues, issue)
	return true, nil
}
func (f *reconciliationStoreFake) MarkPaymentReconciliationIssueAlerted(_ context.Context, issue connectors.PaymentReconciliationIssue, _ time.Time) error {
	f.alerted = append(f.alerted, issue)
	return nil
}
func (f *reconciliationStoreFake) ResolvePaymentReconciliationIssues(_ context.Context, _, _ int64, reference string) error {
	f.resolved = append(f.resolved, reference)
	return nil
}
func (f *reconciliationStoreFake) ListConnectorDeadLetters(context.Context, int) ([]connectors.ConnectorDeadLetter, error) {
	return f.deadLetters, nil
}
func (f *reconciliationStoreFake) MarkConnectorDeadLetterAlerted(_ context.Context, id int64, _ time.Time) error {
	f.deadAlerted = append(f.deadAlerted, id)
	return nil
}
func (f *reconciliationStoreFake) ReplayConnectorDeadLetter(_ context.Context, id int64) error {
	f.replayed = append(f.replayed, id)
	return nil
}

type reconciliationAdapterFake struct {
	snapshot connectors.PaymentStatusSnapshot
	err      error
}

func (f *reconciliationAdapterFake) ValidateConnection(context.Context, *connectors.Connection) error {
	return nil
}
func (f *reconciliationAdapterFake) CheckHealth(context.Context, *connectors.Connection) (connectors.ConnectionStatus, error) {
	return connectors.StatusHealthy, nil
}
func (f *reconciliationAdapterFake) RefreshToken(context.Context, *connectors.Connection) error {
	return nil
}
func (f *reconciliationAdapterFake) VerifyCallbackSignature(context.Context, *connectors.Connection, map[string]string, []byte) error {
	return nil
}
func (f *reconciliationAdapterFake) ExecuteCommand(context.Context, *connectors.Connection, *connectors.OutboxCommand) error {
	return nil
}
func (f *reconciliationAdapterFake) TranslateWebhook(context.Context, *connectors.Connection, map[string]string, []byte) ([]*connectors.CanonicalEvent, error) {
	return nil, nil
}
func (f *reconciliationAdapterFake) LookupPaymentStatus(context.Context, *connectors.Connection, string) (connectors.PaymentStatusSnapshot, error) {
	return f.snapshot, f.err
}

type reconciliationAlertFake struct {
	issues      []connectors.PaymentReconciliationIssue
	deadLetters []connectors.ConnectorDeadLetter
}

func (f *reconciliationAlertFake) AlertUnmatchedPayment(_ context.Context, issue connectors.PaymentReconciliationIssue) error {
	f.issues = append(f.issues, issue)
	return nil
}
func (f *reconciliationAlertFake) AlertConnectorDeadLetter(_ context.Context, deadLetter connectors.ConnectorDeadLetter) error {
	f.deadLetters = append(f.deadLetters, deadLetter)
	return nil
}

func TestPaymentReconciliationRecoversProviderStateAndRecordsRun(t *testing.T) {
	store := &reconciliationStoreFake{
		candidates: []connectors.PaymentReconciliationCandidate{{
			Intent:     connectors.PaymentIntent{ID: 9, CompanyID: 7, ConnectionID: 3, ProviderReference: "order-9", Status: connectors.PaymentStatusPending},
			Connection: connectors.Connection{ID: 3, CompanyID: 7, Provider: "cert", Type: "payment"},
		}},
		transition: connectors.PaymentTransitionResult{Applied: true, FromStatus: connectors.PaymentStatusPending, ToStatus: connectors.PaymentStatusSettled},
	}
	registry := connectors.NewRegistry()
	registry.Register("cert", &reconciliationAdapterFake{snapshot: connectors.PaymentStatusSnapshot{
		ProviderReference: "order-9", Status: "settlement", EventType: "payment.settled", OccurredAt: time.Now().UTC(),
	}})
	service := connectors.NewPaymentReconciliationService(store, registry, slog.Default(), nil, nil)

	report, err := service.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != connectors.ReconciliationRunSuccess || report.Scanned != 1 || report.Recovered != 1 {
		t.Fatalf("report = %#v", report)
	}
	if len(store.runs) != 1 || store.runs[0].RecoveredCount != 1 {
		t.Fatalf("runs = %#v", store.runs)
	}
	if len(store.resolved) != 1 || store.resolved[0] != "order-9" {
		t.Fatalf("resolved = %v", store.resolved)
	}
}

func TestPaymentReconciliationAlertsLookupFailure(t *testing.T) {
	store := &reconciliationStoreFake{candidates: []connectors.PaymentReconciliationCandidate{{
		Intent:     connectors.PaymentIntent{ID: 10, CompanyID: 7, ConnectionID: 3, ProviderReference: "order-10", Status: connectors.PaymentStatusCreated},
		Connection: connectors.Connection{ID: 3, CompanyID: 7, Provider: "cert", Type: "payment"},
	}}}
	registry := connectors.NewRegistry()
	registry.Register("cert", &reconciliationAdapterFake{err: errors.New("status endpoint timeout")})
	alerts := &reconciliationAlertFake{}
	service := connectors.NewPaymentReconciliationService(store, registry, slog.Default(), nil, alerts)

	report, err := service.Reconcile(context.Background())
	if err == nil || report.Status != connectors.ReconciliationRunPartial || report.Unmatched != 1 || report.Errors != 1 {
		t.Fatalf("report=%#v err=%v", report, err)
	}
	if len(store.issues) != 1 || store.issues[0].IssueType != connectors.PaymentIssueLookupFailed {
		t.Fatalf("issues = %#v", store.issues)
	}
	if len(alerts.issues) != 1 || len(store.alerted) != 1 {
		t.Fatalf("alerts=%#v persisted=%#v", alerts.issues, store.alerted)
	}
}

func TestPaymentReconciliationAuditsAndReplaysDeadLetter(t *testing.T) {
	store := &reconciliationStoreFake{deadLetters: []connectors.ConnectorDeadLetter{{
		ID: 41, CommandID: 9, CompanyID: 7, ConnectionID: 3, Provider: "midtrans",
		CommandType: "payment.refund", CorrelationID: "refund-9", Attempts: 5,
		ErrorMessage: "provider outcome is unknown", DeadLetteredAt: time.Now().UTC(),
	}}}
	alerts := &reconciliationAlertFake{}
	service := connectors.NewPaymentReconciliationService(store, connectors.NewRegistry(), slog.Default(), nil, alerts)

	count, err := service.AuditDeadLetters(context.Background(), 10)
	if err != nil || count != 1 || len(alerts.deadLetters) != 1 || len(store.deadAlerted) != 1 {
		t.Fatalf("count=%d err=%v alerts=%#v marked=%v", count, err, alerts.deadLetters, store.deadAlerted)
	}
	if err := service.ReplayDeadLetter(context.Background(), 41); err != nil {
		t.Fatal(err)
	}
	if len(store.replayed) != 1 || store.replayed[0] != 41 {
		t.Fatalf("replayed = %v", store.replayed)
	}
}
