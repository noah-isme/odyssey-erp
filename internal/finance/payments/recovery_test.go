package payments

import (
	"context"
	"errors"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
	"github.com/odyssey-erp/odyssey-erp/internal/observability"
)

func TestPaymentRecoveryCaseKeyIsStableAndCompanyScoped(t *testing.T) {
	item := PaymentRecoveryCase{
		CompanyID:       7,
		ConnectionID:    11,
		Provider:        "Midtrans-Iris",
		InstructionType: "payment_instruction",
		InstructionID:   "instruction-1",
		Issue:           PaymentRecoveryIssueAmbiguous,
		State:           "AMBIGUOUS",
	}
	require.NoError(t, item.Validate())
	require.Equal(t, item.Key(), item.Key())
	otherCompany := item
	otherCompany.CompanyID = 8
	require.NotEqual(t, item.Key(), otherCompany.Key())
}

func TestPaymentRecoveryRepositoryReadsAndDeduplicatesFinanceCases(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	repo := NewPaymentRecoveryRepository(db)
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	db.ExpectQuery("WITH unresolved").WithArgs(int(10), int64((5 * time.Minute).Microseconds()), int64((15 * time.Minute).Microseconds())).WillReturnRows(
		pgxmock.NewRows([]string{"company_id", "connection_id", "provider", "instruction_type", "instruction_id", "issue", "state", "observed_at", "details"}).
			AddRow(int64(7), int64(11), "midtrans_iris", "payment_instruction", "instruction-1", "AMBIGUOUS", "AMBIGUOUS", now.Add(-time.Hour), "lookup required").
			AddRow(int64(7), int64(11), "midtrans_iris", "payment_instruction", "instruction-1", "AMBIGUOUS", "AMBIGUOUS", now.Add(-time.Hour), "duplicate row").
			AddRow(int64(7), nil, "finance-outbox", "payment_instruction", "instruction-2", "DEAD_LETTER", "DEAD_LETTERED", now.Add(-10*time.Minute), "explicit recovery required"),
	)
	cases, err := repo.UnresolvedCases(context.Background(), 10, 5*time.Minute, 15*time.Minute)
	require.NoError(t, err)
	require.Len(t, cases, 2)
	require.Equal(t, int64(0), cases[1].ConnectionID)
	require.Equal(t, PaymentRecoveryIssueDeadLetter, cases[1].Issue)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestPaymentRecoveryRepositoryRecipientsAreCompanyScoped(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	repo := NewPaymentRecoveryRepository(db)
	db.ExpectQuery("SELECT DISTINCT u.id").WithArgs(int64(7)).WillReturnRows(
		pgxmock.NewRows([]string{"id"}).AddRow(int64(101)).AddRow(int64(202)),
	)
	ids, err := repo.Recipients(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, []int64{101, 202}, ids)
	require.NoError(t, db.ExpectationsWereMet())
}

type recoveryNotificationFake struct {
	messages []notifications.Message
	err      error
}

func (f *recoveryNotificationFake) Dispatch(_ context.Context, message notifications.Message) error {
	f.messages = append(f.messages, message)
	return f.err
}

func TestPaymentRecoveryScannerDeduplicatesAlertsAndDoesNotReplayCommands(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	repo := NewPaymentRecoveryRepository(db)
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	db.ExpectQuery("WITH unresolved").WithArgs(int(100), int64((10 * time.Minute).Microseconds()), int64((10 * time.Minute).Microseconds())).WillReturnRows(
		pgxmock.NewRows([]string{"company_id", "connection_id", "provider", "instruction_type", "instruction_id", "issue", "state", "observed_at", "details"}).
			AddRow(int64(7), int64(11), "midtrans_iris", "payment_instruction", "instruction-1", "AMBIGUOUS", "AMBIGUOUS", now.Add(-time.Hour), "lookup required").
			AddRow(int64(7), int64(11), "midtrans_iris", "payment_instruction", "instruction-1", "AMBIGUOUS", "AMBIGUOUS", now.Add(-time.Hour), "duplicate row"),
	)
	db.ExpectQuery("SELECT DISTINCT u.id").WithArgs(int64(7)).WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(101)).AddRow(int64(202)))
	notifier := &recoveryNotificationFake{}
	registry := prometheus.NewRegistry()
	metrics := observability.NewPaymentRecoveryMetrics(registry)
	scanner := NewPaymentRecoveryScanner(repo, notifier, metrics, PaymentRecoveryScanConfig{Now: func() time.Time { return now }})
	report, err := scanner.Scan(context.Background())
	require.NoError(t, err)
	require.Equal(t, PaymentRecoveryScanReport{Cases: 1, Notifications: 2, Companies: 1}, report)
	require.Len(t, notifier.messages, 2)
	require.Equal(t, notifications.TypeFinancePaymentRecovery, notifier.messages[0].Type)
	require.Equal(t, notifier.messages[0].DedupeKey, notifier.messages[1].DedupeKey)
	require.Contains(t, notifier.messages[0].Body, "lookup required")
	require.NoError(t, db.ExpectationsWereMet())
}

func TestPaymentRecoveryScannerReturnsNotificationErrorsAfterBestEffort(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	repo := NewPaymentRecoveryRepository(db)
	db.ExpectQuery("WITH unresolved").WithArgs(int(100), int64((10 * time.Minute).Microseconds()), int64((10 * time.Minute).Microseconds())).WillReturnRows(
		pgxmock.NewRows([]string{"company_id", "connection_id", "provider", "instruction_type", "instruction_id", "issue", "state", "observed_at", "details"}).
			AddRow(int64(7), int64(11), "midtrans_iris", "payment_instruction", "instruction-1", "FAILED", "FAILED", time.Now().UTC(), "failed"),
	)
	db.ExpectQuery("SELECT DISTINCT u.id").WithArgs(int64(7)).WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(101)))
	want := errors.New("notification unavailable")
	report, err := NewPaymentRecoveryScanner(repo, &recoveryNotificationFake{err: want}, nil).Scan(context.Background())
	require.Equal(t, 1, report.Cases)
	require.ErrorIs(t, err, want)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestPaymentRecoveryContractsRemainProviderNeutral(t *testing.T) {
	input, err := NewPaymentExecutionOutboxInput(PaymentExecutionCommand{
		Reference:  paymentInstruction().Reference,
		ExecutorID: 303,
	}, automation.Correlation{ID: "corr-1"}, "instruction-1", 303)
	require.NoError(t, err)
	require.Equal(t, automation.OperationPaymentExecute, input.Operation)
}
