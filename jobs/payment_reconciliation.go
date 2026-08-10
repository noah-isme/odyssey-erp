package jobs

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

type PaymentReconciliationRunner interface {
	Reconcile(context.Context) (connectors.PaymentReconciliationReport, error)
}

func HandlePaymentReconciliation(runner PaymentReconciliationRunner) asynq.HandlerFunc {
	return func(ctx context.Context, _ *asynq.Task) error {
		if runner == nil {
			return fmt.Errorf("payment reconciliation: runner not configured: %w", asynq.SkipRetry)
		}
		_, err := runner.Reconcile(ctx)
		return err
	}
}

type ConnectorDeadLetterAuditor interface {
	AuditDeadLetters(context.Context, int) (int, error)
}

func HandleConnectorDeadLetterAudit(auditor ConnectorDeadLetterAuditor) asynq.HandlerFunc {
	return func(ctx context.Context, _ *asynq.Task) error {
		if auditor == nil {
			return fmt.Errorf("connector dead-letter audit: auditor not configured: %w", asynq.SkipRetry)
		}
		_, err := auditor.AuditDeadLetters(ctx, 100)
		return err
	}
}
