package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

const (
	// TaskProcurementReindex refreshes procurement search indexes.
	TaskProcurementReindex = "procurement:reindex"
)

// ProcurementReindexPayload contains options for reindex job.
type ProcurementReindexPayload struct {
	Force bool `json:"force"`
}

// NewProcurementReindexTask builds a new reindex task.
func NewProcurementReindexTask(force bool) (*asynq.Task, error) {
	payload := ProcurementReindexPayload{Force: force}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskProcurementReindex, body, asynq.Queue(QueueDefault)), nil
}

// HandleProcurementReindexTask performs background refresh.
func HandleProcurementReindexTask(ctx context.Context, t *asynq.Task) error {
	var payload ProcurementReindexPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return asynq.SkipRetry
	}
	fmt.Printf("[jobs] procurement reindex force=%v\n", payload.Force)
	return nil
}
