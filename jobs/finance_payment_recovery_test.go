package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/payments"
)

type financePaymentRecoveryScannerFake struct {
	called bool
	err    error
}

func (f *financePaymentRecoveryScannerFake) Scan(context.Context) (payments.PaymentRecoveryScanReport, error) {
	f.called = true
	return payments.PaymentRecoveryScanReport{Cases: 2, Notifications: 2, Companies: 1}, f.err
}

func TestHandleFinancePaymentRecoveryScanInvokesScanner(t *testing.T) {
	fake := &financePaymentRecoveryScannerFake{}
	err := HandleFinancePaymentRecoveryScan(fake)(context.Background(), asynq.NewTask(TaskFinancePaymentRecovery, nil))
	if err != nil || !fake.called {
		t.Fatalf("handler err=%v called=%v", err, fake.called)
	}
}

func TestHandleFinancePaymentRecoveryScanPropagatesError(t *testing.T) {
	want := errors.New("database unavailable")
	err := HandleFinancePaymentRecoveryScan(&financePaymentRecoveryScannerFake{err: want})(context.Background(), asynq.NewTask(TaskFinancePaymentRecovery, nil))
	if !errors.Is(err, want) {
		t.Fatalf("error=%v, want %v", err, want)
	}
}

func TestHandleFinancePaymentRecoveryScanSkipsWhenUnconfigured(t *testing.T) {
	err := HandleFinancePaymentRecoveryScan(nil)(context.Background(), asynq.NewTask(TaskFinancePaymentRecovery, nil))
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("error=%v, want SkipRetry", err)
	}
}
