package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
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
	// TaskPayrollPayslipEmail renders and emails a posted payslip.
	TaskPayrollPayslipEmail = "payroll:payslip_email"
	// TaskPayrollPayslipDispatch re-enqueues undelivered durable payslip records.
	TaskPayrollPayslipDispatch = "payroll:payslip_dispatch"
	// TaskTaxCaptureDispatch retries durable AR/AP tax captures.
	TaskTaxCaptureDispatch = "tax:capture_dispatch"

	// TaskOutboxSweep processes pending cross-module outbox events.
	TaskOutboxSweep = "outbox:sweep"
	// TaskConnectorOutboxSweep processes pending external connector outbox events.
	TaskConnectorOutboxSweep = "connectors:outbox_sweep"
	// TaskCRMReminderDispatch sends due and overdue CRM activity notifications.
	TaskCRMReminderDispatch     = "crm:reminder_dispatch"
	TaskWebhookDeliveryDispatch = "webhook:delivery_dispatch"
	// TaskFinanceAutomationDispatch claims durable finance commands. Individual
	// Phase 1+ handlers decide which provider-neutral topic they can execute.
	TaskFinanceAutomationDispatch = "finance:automation_dispatch"
)

type FinanceAutomationDispatcher interface {
	DispatchFinanceAutomation(context.Context, int) error
}

func HandleFinanceAutomationDispatch(dispatcher FinanceAutomationDispatcher) asynq.HandlerFunc {
	return func(ctx context.Context, _ *asynq.Task) error {
		if dispatcher == nil {
			return fmt.Errorf("finance automation dispatcher not configured: %w", asynq.SkipRetry)
		}
		return dispatcher.DispatchFinanceAutomation(ctx, 100)
	}
}

type WebhookDeliveryDispatcher interface{ DispatchWebhookDeliveries(context.Context) error }

func HandleWebhookDeliveryDispatch(dispatcher WebhookDeliveryDispatcher) asynq.HandlerFunc {
	return func(ctx context.Context, _ *asynq.Task) error {
		if dispatcher == nil {
			return fmt.Errorf("webhook dispatcher not configured: %w", asynq.SkipRetry)
		}
		return dispatcher.DispatchWebhookDeliveries(ctx)
	}
}

type CRMReminderDispatcher interface {
	DispatchReminders(context.Context, int) error
}

func HandleCRMReminderDispatch(dispatcher CRMReminderDispatcher) asynq.HandlerFunc {
	return func(ctx context.Context, _ *asynq.Task) error {
		if dispatcher == nil {
			return fmt.Errorf("CRM reminder dispatcher not configured: %w", asynq.SkipRetry)
		}
		return dispatcher.DispatchReminders(ctx, 100)
	}
}

type TaxCaptureDispatcher interface {
	ProcessPending(context.Context, int) error
}

func HandleTaxCaptureDispatch(dispatcher TaxCaptureDispatcher) asynq.HandlerFunc {
	return func(ctx context.Context, _ *asynq.Task) error {
		if dispatcher == nil {
			return fmt.Errorf("tax capture dispatcher not configured: %w", asynq.SkipRetry)
		}
		return dispatcher.ProcessPending(ctx, 100)
	}
}

type PayslipOutboxDispatcher interface {
	DispatchPending(context.Context) error
}

func HandlePayrollPayslipDispatch(dispatcher PayslipOutboxDispatcher) asynq.HandlerFunc {
	return func(ctx context.Context, _ *asynq.Task) error {
		if dispatcher == nil {
			return fmt.Errorf("payslip outbox dispatcher not configured: %w", asynq.SkipRetry)
		}
		return dispatcher.DispatchPending(ctx)
	}
}

type PayrollPayslipPayload struct {
	PayslipID int64 `json:"payslip_id"`
}

func NewPayrollPayslipTask(payslipID int64) (*asynq.Task, error) {
	if payslipID <= 0 {
		return nil, fmt.Errorf("jobs: payslip id required")
	}
	body, err := json.Marshal(PayrollPayslipPayload{PayslipID: payslipID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskPayrollPayslipEmail, body), nil
}

type PayslipEmailer interface {
	DeliverPayslip(context.Context, int64) error
}

func HandlePayrollPayslipEmail(sender PayslipEmailer) asynq.HandlerFunc {
	return func(ctx context.Context, task *asynq.Task) error {
		if sender == nil {
			return fmt.Errorf("payslip sender not configured: %w", asynq.SkipRetry)
		}
		var payload PayrollPayslipPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil || payload.PayslipID <= 0 {
			return fmt.Errorf("invalid payslip task: %w", asynq.SkipRetry)
		}
		return sender.DeliverPayslip(ctx, payload.PayslipID)
	}
}

// SendEmailPayload describes the information required to send an email.
type SendEmailPayload struct {
	To            string `json:"to"`
	Subject       string `json:"subject"`
	Body          string `json:"body"`
	CorrelationID string `json:"correlation_id,omitempty"`
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
func HandleSendEmailTask(mailer Mailer) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		if mailer == nil {
			return fmt.Errorf("mail client not configured: %w", asynq.SkipRetry)
		}
		var payload SendEmailPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return asynq.SkipRetry
		}
		if payload.To == "" || payload.Subject == "" {
			return fmt.Errorf("invalid email payload: %w", asynq.SkipRetry)
		}
		if err := mailer.SendEmail(ctx, payload.To, payload.Subject, payload.Body, nil); err != nil {
			return fmt.Errorf("send email: %w", err)
		}
		return nil
	}
}

// Mailer is implemented by shared.MailClient and small test fakes.
type Mailer interface {
	SendEmail(context.Context, string, string, string, *shared.Attachment) error
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

// NewConnectorOutboxSweepTask creates a task to sweep the external connectors outbox.
func NewConnectorOutboxSweepTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskConnectorOutboxSweep, nil, asynq.Queue(QueueDefault)), nil
}
