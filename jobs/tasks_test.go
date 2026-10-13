package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/stretchr/testify/require"
)

type mailFake struct {
	to, subject, body string
	calls             int
}

func (m *mailFake) SendEmail(_ context.Context, to, subject, body string, _ *shared.Attachment) error {
	m.to, m.subject, m.body = to, subject, body
	m.calls++
	return nil
}

func TestHandleSendEmailTaskUsesInjectedMailer(t *testing.T) {
	mailer := &mailFake{}
	task, err := NewSendEmailTask(SendEmailPayload{To: "user@example.com", Subject: "Invoice issued", Body: "Ready"})
	require.NoError(t, err)
	require.NoError(t, HandleSendEmailTask(mailer)(context.Background(), task))
	require.Equal(t, 1, mailer.calls)
	require.Equal(t, "user@example.com", mailer.to)
	require.Equal(t, "Invoice issued", mailer.subject)
}

func TestSendEmailTaskCarriesNotificationCorrelationID(t *testing.T) {
	task, err := NewSendEmailTask(SendEmailPayload{To: "user@example.com", Subject: "Invoice issued", Body: "Ready", CorrelationID: "notification-email-42"})
	require.NoError(t, err)
	require.Contains(t, string(task.Payload()), `"correlation_id":"notification-email-42"`)
}

func TestNewPayrollPayslipTask(t *testing.T) {
	task, err := NewPayrollPayslipTask(77)
	require.NoError(t, err)
	require.Equal(t, TaskPayrollPayslipEmail, task.Type())
	require.JSONEq(t, `{"payslip_id":77}`, string(task.Payload()))
}

func TestNewDocumentOCRTask(t *testing.T) {
	task, err := NewDocumentOCRTask(77)
	require.NoError(t, err)
	require.Equal(t, TaskDocumentOCR, task.Type())
	require.JSONEq(t, `{"job_id":77}`, string(task.Payload()))
}

func TestDocumentOCRTaskRejectsMalformedPayload(t *testing.T) {
	err := HandleDocumentOCR(nil)(context.Background(), asynq.NewTask(TaskDocumentOCR, []byte(`{"job_id":77}`)))
	require.ErrorIs(t, err, asynq.SkipRetry)
}

func TestHandleSendEmailTaskRejectsMalformedPayload(t *testing.T) {
	err := HandleSendEmailTask(&mailFake{})(context.Background(), asynq.NewTask(TaskTypeSendEmail, []byte("{")))
	require.ErrorIs(t, err, asynq.SkipRetry)
}

type taxCaptureFake struct{ calls, limit int }

func (f *taxCaptureFake) ProcessPending(_ context.Context, limit int) error {
	f.calls++
	f.limit = limit
	return nil
}

func TestHandleTaxCaptureDispatchProcessesOutbox(t *testing.T) {
	fake := &taxCaptureFake{}
	require.NoError(t, HandleTaxCaptureDispatch(fake)(context.Background(), asynq.NewTask(TaskTaxCaptureDispatch, nil)))
	require.Equal(t, 1, fake.calls)
	require.Equal(t, 100, fake.limit)
}

type crmReminderFake struct{ limit int }

func (f *crmReminderFake) DispatchReminders(_ context.Context, limit int) error {
	f.limit = limit
	return nil
}

func TestHandleCRMReminderDispatchProcessesDueActivities(t *testing.T) {
	fake := &crmReminderFake{}
	require.NoError(t, HandleCRMReminderDispatch(fake)(context.Background(), asynq.NewTask(TaskCRMReminderDispatch, nil)))
	require.Equal(t, 100, fake.limit)
}

type financeAutomationFake struct{ limit int }

func (f *financeAutomationFake) DispatchFinanceAutomation(_ context.Context, limit int) error {
	f.limit = limit
	return nil
}

func TestHandleFinanceAutomationDispatchUsesSharedOutboxBoundary(t *testing.T) {
	fake := &financeAutomationFake{}
	require.NoError(t, HandleFinanceAutomationDispatch(fake)(context.Background(), asynq.NewTask(TaskFinanceAutomationDispatch, nil)))
	require.Equal(t, 100, fake.limit)
	require.ErrorIs(t, HandleFinanceAutomationDispatch(nil)(context.Background(), asynq.NewTask(TaskFinanceAutomationDispatch, nil)), asynq.SkipRetry)
}

type hrChecklistReminderFake struct{ limit int }

func (f *hrChecklistReminderFake) DispatchOverdueReminders(_ context.Context, limit int) error {
	f.limit = limit
	return nil
}

func TestHandleHRChecklistRemindersUsesCap(t *testing.T) {
	fake := &hrChecklistReminderFake{}
	require.NoError(t, HandleHRChecklistReminders(fake)(context.Background(), asynq.NewTask(TaskHRChecklistReminders, nil)))
	require.Equal(t, 200, fake.limit)
}

type hrJobChangeFake struct {
	asOf time.Time
	n    int
	err  error
}

func (f *hrJobChangeFake) ApplyDueJobChanges(_ context.Context, asOf time.Time) (int, error) {
	f.asOf = asOf
	return f.n, f.err
}

func TestHandleHRApplyJobChangesUsesBusinessDate(t *testing.T) {
	// 20:00 UTC on 31 May is already 1 June in Jakarta (UTC+7).
	now := time.Date(2026, 5, 31, 20, 0, 0, 0, time.UTC)
	prev := hrJobChangeClock
	hrJobChangeClock = func() time.Time { return now }
	t.Cleanup(func() { hrJobChangeClock = prev })

	jakarta, err := time.LoadLocation("Asia/Jakarta")
	require.NoError(t, err)
	task := asynq.NewTask(TaskHRApplyJobChanges, nil)
	for name, loc := range map[string]*time.Location{"explicit": jakarta, "default": nil} {
		fake := &hrJobChangeFake{n: 2}
		require.NoError(t, HandleHRApplyJobChanges(fake, loc)(context.Background(), task), name)
		y, m, d := fake.asOf.Date()
		require.Equal(t, []int{2026, 6, 1}, []int{y, int(m), d}, name)
	}

	fake := &hrJobChangeFake{}
	require.NoError(t, HandleHRApplyJobChanges(fake, time.UTC)(context.Background(), task))
	y, m, d := fake.asOf.Date()
	require.Equal(t, []int{2026, 5, 31}, []int{y, int(m), d}, "an explicit UTC location is honoured")

	boom := errors.New("boom")
	fake.err = boom
	require.ErrorIs(t, HandleHRApplyJobChanges(fake, jakarta)(context.Background(), task), boom, "errors are returned so asynq retries")
	require.ErrorIs(t, HandleHRApplyJobChanges(nil, jakarta)(context.Background(), task), asynq.SkipRetry)
}
