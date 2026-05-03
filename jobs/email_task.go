package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Task names
const (
	TypeEmailDelivery = "email:deliver"
)

// EmailDeliveryPayload defines the data needed to send a document email.
type EmailDeliveryPayload struct {
	To       []string `json:"to"`
	Subject  string   `json:"subject"`
	BodyHTML string   `json:"body_html"`
	// Additional metadata for attachments could be added here
	AttachmentURL string `json:"attachment_url,omitempty"`
}

// NewEmailDeliveryTask creates a new Asynq task for email delivery.
func NewEmailDeliveryTask(payload EmailDeliveryPayload) (*asynq.Task, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeEmailDelivery, b), nil
}

// HandleEmailDeliveryTask handles the email delivery task execution.
func HandleEmailDeliveryTask(logger *slog.Logger) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var p EmailDeliveryPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
		}

		logger.Info("processing email delivery", slog.String("subject", p.Subject), slog.Any("to", p.To))

		// Instantiate the mailer (in a real app, this should be injected or configured from env)
		client := shared.NewMailClient(shared.MailConfig{
			Host: "127.0.0.1",
			Port: 1025,
			From: "noreply@odyssey.local",
		})

		var toAddress string
		if len(p.To) > 0 {
			toAddress = p.To[0]
		}

		if err := client.SendEmail(ctx, toAddress, p.Subject, p.BodyHTML, nil); err != nil {
			logger.Error("failed to send email", slog.Any("error", err))
			return fmt.Errorf("mailer.SendEmail failed: %w", err)
		}

		logger.Info("email sent successfully", slog.String("subject", p.Subject))
		return nil
	}
}
