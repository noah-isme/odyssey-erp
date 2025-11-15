package variance

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/odyssey-erp/odyssey-erp/jobs"
)

// SnapshotJob processes variance snapshot tasks.
type SnapshotJob struct {
	service *Service
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
		return err
	}
	return nil
}
