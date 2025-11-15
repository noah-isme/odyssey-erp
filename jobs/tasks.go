package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

const (
	// QueueDefault is the default queue name for background jobs.
	QueueDefault = "default"
	// TaskTypeSendEmail is the task type for sending transactional emails.
	TaskTypeSendEmail = "mail:send"
	// TaskAnalyticsInsightsWarmup pre-warms analytics dashboards caches.
	TaskAnalyticsInsightsWarmup = "analytics:insights_warmup"
	// TaskAnalyticsAnomalyScan scans finance signals for anomalies.
	TaskAnalyticsAnomalyScan = "analytics:anomaly_scan"
	// TaskVarianceSnapshotProcess processes variance snapshots.
	TaskVarianceSnapshotProcess = "variance:snapshot_process"
	// TaskBoardPackGenerate triggers board pack generation.
	TaskBoardPackGenerate = "boardpack:generate"
)

// SendEmailPayload describes the information required to send an email.
type SendEmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// NewSendEmailTask constructs an Asynq task.
func NewSendEmailTask(payload SendEmailPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskTypeSendEmail, data), nil
}

// HandleSendEmailTask processes TaskTypeSendEmail tasks.
func HandleSendEmailTask(ctx context.Context, t *asynq.Task) error {
	var payload SendEmailPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return asynq.SkipRetry
	}
	// Placeholder: integrate with SMTP/Mailpit in phase 2.
	fmt.Printf("[jobs] send email to %s subject=%s\n", payload.To, payload.Subject)
	return nil
}

// InsightsWarmupPayload describes the cache warmup scope.
type InsightsWarmupPayload struct {
	PeriodScope string `json:"period_scope"`
}

// NewInsightsWarmupTask creates a new warmup task.
func NewInsightsWarmupTask(scope string) (*asynq.Task, error) {
	if scope == "" {
		scope = "active"
	}
	body, err := json.Marshal(InsightsWarmupPayload{PeriodScope: scope})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskAnalyticsInsightsWarmup, body, asynq.Queue(QueueDefault)), nil
}

// AnomalyScanPayload configures the anomaly detection job.
type AnomalyScanPayload struct {
	WindowMonths int     `json:"window_months"`
	Z            float64 `json:"z"`
}

// NewAnomalyScanTask constructs an anomaly scan task.
func NewAnomalyScanTask(window int, z float64) (*asynq.Task, error) {
	if window <= 0 {
		window = 6
	}
	if z <= 0 {
		z = 2.0
	}
	body, err := json.Marshal(AnomalyScanPayload{WindowMonths: window, Z: z})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskAnalyticsAnomalyScan, body, asynq.Queue(QueueDefault)), nil
}

// VarianceSnapshotPayload requests a variance snapshot processing.
type VarianceSnapshotPayload struct {
	SnapshotID int64 `json:"snapshot_id"`
}

// BoardPackPayload points to the board pack record that should be generated.
type BoardPackPayload struct {
	BoardPackID int64 `json:"board_pack_id"`
}

// NewVarianceSnapshotTask enqueues a variance snapshot job.
func NewVarianceSnapshotTask(snapshotID int64) (*asynq.Task, error) {
	if snapshotID == 0 {
		return nil, fmt.Errorf("jobs: snapshot id required")
	}
	body, err := json.Marshal(VarianceSnapshotPayload{SnapshotID: snapshotID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskVarianceSnapshotProcess, body, asynq.Queue(QueueDefault)), nil
}

// NewBoardPackTask enqueues a board pack generation job.
func NewBoardPackTask(boardPackID int64) (*asynq.Task, error) {
	if boardPackID == 0 {
		return nil, fmt.Errorf("jobs: board pack id required")
	}
	body, err := json.Marshal(BoardPackPayload{BoardPackID: boardPackID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskBoardPackGenerate, body, asynq.Queue(QueueDefault)), nil
}
