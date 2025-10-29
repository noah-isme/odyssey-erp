package cli

import (
	"context"
	"errors"

	"github.com/hibiken/asynq"

	"github.com/odyssey-erp/odyssey-erp/jobs"
)

// ConsolOpsCLI exposes helpers for managing consolidation refresh jobs.
type ConsolOpsCLI struct {
	jobs *JobsCLI
}

// NewConsolOpsCLI constructs the helper wired to the provided Redis endpoint.
func NewConsolOpsCLI(redisAddr string) (*ConsolOpsCLI, error) {
	base, err := NewJobsCLI(redisAddr)
	if err != nil {
		return nil, err
	}
	return &ConsolOpsCLI{jobs: base}, nil
}

// Close releases the underlying Asynq resources.
func (c *ConsolOpsCLI) Close() error {
	if c == nil || c.jobs == nil {
		return nil
	}
	return c.jobs.Close()
}

// TriggerRefresh enqueues a consolidate refresh job with the provided scope.
func (c *ConsolOpsCLI) TriggerRefresh(ctx context.Context, groupID, period string) (*asynq.TaskInfo, error) {
	if c == nil || c.jobs == nil {
		return nil, errors.New("consol cli: client not configured")
	}
	task, err := jobs.NewConsolidateRefreshTask(groupID, period)
	if err != nil {
		return nil, err
	}
	return c.jobs.Enqueue(ctx, task, asynq.MaxRetry(3))
}

// InspectQueue proxies queue statistics for observability.
func (c *ConsolOpsCLI) InspectQueue(ctx context.Context) (QueueStats, error) {
	if c == nil || c.jobs == nil {
		return QueueStats{}, errors.New("consol cli: inspector not configured")
	}
	return c.jobs.InspectQueue(ctx)
}

// ListScheduled exposes the upcoming scheduled jobs from the default queue.
func (c *ConsolOpsCLI) ListScheduled(ctx context.Context, size int) ([]*asynq.TaskInfo, error) {
	if c == nil || c.jobs == nil {
		return nil, errors.New("consol cli: inspector not configured")
	}
	return c.jobs.ListScheduled(ctx, size)
}
