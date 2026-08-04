package jobs

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/odyssey-erp/odyssey-erp/internal/outbox"
)

// HandleOutboxSweep returns a handler that processes pending cross-module outbox events.
func HandleOutboxSweep(dispatcher *outbox.Dispatcher) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		if dispatcher == nil {
			return fmt.Errorf("outbox sweep: dispatcher not configured: %w", asynq.SkipRetry)
		}
		return dispatcher.ProcessPending(ctx, 100)
	}
}
