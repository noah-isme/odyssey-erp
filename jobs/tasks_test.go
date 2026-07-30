package jobs

import (
	"context"
	"testing"

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

func TestHandleSendEmailTaskRejectsMalformedPayload(t *testing.T) {
	err := HandleSendEmailTask(&mailFake{})(context.Background(), asynq.NewTask(TaskTypeSendEmail, []byte("{")))
	require.ErrorIs(t, err, asynq.SkipRetry)
}
