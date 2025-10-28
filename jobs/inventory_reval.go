package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

const (
	// TaskInventoryRevaluation triggers nightly inventory revaluation.
	TaskInventoryRevaluation = "inventory:revaluation"
)

// InventoryRevaluationPayload carries scheduling metadata.
type InventoryRevaluationPayload struct {
	ScheduledFor time.Time `json:"scheduled_for"`
}

// NewInventoryRevaluationTask constructs an Asynq task for inventory revaluation.
func NewInventoryRevaluationTask(at time.Time) (*asynq.Task, error) {
	payload := InventoryRevaluationPayload{ScheduledFor: at}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskInventoryRevaluation, body, asynq.Queue(QueueDefault)), nil
}

// HandleInventoryRevaluationTask performs a light-weight check.
func HandleInventoryRevaluationTask(ctx context.Context, t *asynq.Task) error {
	var payload InventoryRevaluationPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return asynq.SkipRetry
	}
	// Placeholder: in future iterate products and recalc moving-average snapshots.
	fmt.Printf("[jobs] inventory revaluation scheduled_for=%s\n", payload.ScheduledFor.Format(time.RFC3339))
	return nil
}
