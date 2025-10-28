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
