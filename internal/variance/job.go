package variance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/odyssey-erp/odyssey-erp/jobs"
)

// snapshotProcessor is the part of Service the job needs.
type snapshotProcessor interface {
	ProcessSnapshot(ctx context.Context, snapshotID int64) error
}

// SnapshotJob processes variance snapshot tasks.
type SnapshotJob struct {
	service snapshotProcessor
	logger  *slog.Logger
}

// NewSnapshotJob constructs a job handler.
func NewSnapshotJob(service *Service, logger *slog.Logger) *SnapshotJob {
	return &SnapshotJob{service: service, logger: logger}
}

// Handle fulfils the asynq.HandlerFunc contract.
func (j *SnapshotJob) Handle(ctx context.Context, task *asynq.Task) error {
	var payload jobs.VarianceSnapshotPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return asynq.SkipRetry
	}
	if payload.SnapshotID == 0 {
		return asynq.SkipRetry
	}
	if err := j.service.ProcessSnapshot(ctx, payload.SnapshotID); err != nil {
		if j.logger != nil {
			j.logger.Error("variance snapshot", slog.Int64("snapshot_id", payload.SnapshotID), slog.Any("error", err))
		}
		if errors.Is(err, ErrSnapshotNotFound) {
			// A missing (forged or deleted) snapshot never appears on retry.
			return fmt.Errorf("%w: %w", err, asynq.SkipRetry)
		}
		return err
	}
	return nil
}
