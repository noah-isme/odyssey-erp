package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

type reconciliationRunnerFake struct {
	report connectors.PaymentReconciliationReport
	err    error
	calls  int
}

func (f *reconciliationRunnerFake) Reconcile(context.Context) (connectors.PaymentReconciliationReport, error) {
	f.calls++
	return f.report, f.err
}

type deadLetterAuditorFake struct {
	count int
	err   error
	calls int
}

func (f *deadLetterAuditorFake) AuditDeadLetters(context.Context, int) (int, error) {
	f.calls++
	return f.count, f.err
}

func TestPaymentReconciliationHandlerRunsService(t *testing.T) {
	runner := &reconciliationRunnerFake{}
	if err := HandlePaymentReconciliation(runner)(context.Background(), asynq.NewTask(TaskPaymentReconciliation, nil)); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Fatalf("calls = %d", runner.calls)
	}
}

func TestPaymentReconciliationHandlerReturnsRetryableFailure(t *testing.T) {
	want := errors.New("database unavailable")
	runner := &reconciliationRunnerFake{err: want}
	if err := HandlePaymentReconciliation(runner)(context.Background(), asynq.NewTask(TaskPaymentReconciliation, nil)); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeadLetterAuditHandlerRunsService(t *testing.T) {
	auditor := &deadLetterAuditorFake{count: 2}
	if err := HandleConnectorDeadLetterAudit(auditor)(context.Background(), asynq.NewTask(TaskConnectorDeadLetterAudit, nil)); err != nil {
		t.Fatal(err)
	}
	if auditor.calls != 1 {
		t.Fatalf("calls = %d", auditor.calls)
	}
}
