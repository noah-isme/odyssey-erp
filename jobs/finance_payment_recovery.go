package jobs

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/payments"
)

// TaskFinancePaymentRecovery scans the v0.11 finance payment execution,
// settlement-result, effect, and finance-outbox boundaries for operator work.
// It is profile-gated by cmd/worker and is never registered for v0.10-core.
const TaskFinancePaymentRecovery = "finance:payment_recovery_scan"

type FinancePaymentRecoveryScanner interface {
	Scan(context.Context) (payments.PaymentRecoveryScanReport, error)
}

func NewFinancePaymentRecoveryTask() *asynq.Task {
	return asynq.NewTask(TaskFinancePaymentRecovery, nil)
}

func HandleFinancePaymentRecoveryScan(scanner FinancePaymentRecoveryScanner) asynq.HandlerFunc {
	return func(ctx context.Context, _ *asynq.Task) error {
		if scanner == nil {
			return fmt.Errorf("finance payment recovery: scanner not configured: %w", asynq.SkipRetry)
		}
		_, err := scanner.Scan(ctx)
		return err
	}
}
