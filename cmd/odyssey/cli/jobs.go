package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/odyssey-erp/odyssey-erp/jobs"
)

// JobsCLI wraps manual management helpers for Asynq jobs.
type JobsCLI struct {
	client    *asynq.Client
	inspector *asynq.Inspector
}

// NewJobsCLI initialises the CLI helpers using the provided Redis address.
func NewJobsCLI(redisAddr string) (*JobsCLI, error) {
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: redisAddr})
	return &JobsCLI{client: client, inspector: inspector}, nil
}

// Close releases underlying resources.
func (c *JobsCLI) Close() error {
	var err error
	if c.inspector != nil {
		if closeErr := c.inspector.Close(); closeErr != nil {
			err = closeErr
		}
	}
	if c.client != nil {
		if closeErr := c.client.Close(); closeErr != nil {
			err = closeErr
		}
	}
	return err
}

// Trigger enqueues a supported job by name with default payload.
func (c *JobsCLI) Trigger(ctx context.Context, name string) (*asynq.TaskInfo, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("jobs cli: client not configured")
	}
	var task *asynq.Task
	var err error
	switch name {
	case jobs.TaskAnalyticsInsightsWarmup:
		task, err = jobs.NewInsightsWarmupTask("active")
	case jobs.TaskAnalyticsAnomalyScan:
		task, err = jobs.NewAnomalyScanTask(12, 2.5)
	default:
		return nil, fmt.Errorf("jobs cli: unsupported job %s", name)
	}
	if err != nil {
		return nil, err
	}
	return c.client.EnqueueContext(ctx, task, asynq.MaxRetry(3))
}

// QueueStats summarises the current queue state.
type QueueStats struct {
	Queue     string
	Pending   int
	Active    int
	Scheduled int
	Retry     int
}

// InspectQueue reports the queue metrics for the default queue.
func (c *JobsCLI) InspectQueue(ctx context.Context) (QueueStats, error) {
	if c == nil || c.inspector == nil {
		return QueueStats{}, errors.New("jobs cli: inspector not configured")
	}
	info, err := c.inspector.GetQueueInfo(jobs.QueueDefault)
	if err != nil {
		return QueueStats{}, err
	}
	stats := QueueStats{Queue: jobs.QueueDefault}
	if info != nil {
		stats.Pending = int(info.Pending)
		stats.Active = int(info.Active)
		stats.Scheduled = int(info.Scheduled)
		stats.Retry = int(info.Retry)
	}
	return stats, nil
}

// ListScheduled returns scheduled task infos for observability.
func (c *JobsCLI) ListScheduled(ctx context.Context, size int) ([]*asynq.TaskInfo, error) {
	if c == nil || c.inspector == nil {
		return nil, errors.New("jobs cli: inspector not configured")
	}
	if size <= 0 {
		size = 10
	}
	return c.inspector.ListScheduledTasks(jobs.QueueDefault, asynq.PageSize(size), asynq.Page(1))
}
