package jobs

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

// HandleConnectorOutboxSweep returns a handler that processes pending external connector outbox events.
func HandleConnectorOutboxSweep(worker *connectors.OutboxWorker) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		if worker == nil {
			return fmt.Errorf("connector outbox sweep: worker not configured: %w", asynq.SkipRetry)
		}
		// Dispatch up to 100 pending integration commands per run
		return worker.ProcessPending(ctx, 100)
	}
}
