package procurement

import (
	"context"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/odyssey-erp/odyssey-erp/jobs"
)

type asynqRFQEmailQueue struct{ client *asynq.Client }

func NewAsynqRFQEmailQueue(client *asynq.Client) RFQEmailQueue {
	return asynqRFQEmailQueue{client: client}
}

func (q asynqRFQEmailQueue) EnqueueRFQ(ctx context.Context, recipients []string, subject, body string) error {
	if q.client == nil || len(recipients) == 0 {
		return nil
	}
	for _, recipient := range recipients {
		recipient = strings.TrimSpace(recipient)
		if recipient == "" {
			continue
		}
		task, err := jobs.NewSendEmailTask(jobs.SendEmailPayload{To: recipient, Subject: subject, Body: body, CorrelationID: "rfq-" + strings.ReplaceAll(subject, " ", "-") + "-" + recipient})
		if err != nil {
			return err
		}
		if _, err := q.client.EnqueueContext(ctx, task, asynq.Queue(jobs.QueueDefault), asynq.MaxRetry(5)); err != nil && err != asynq.ErrTaskIDConflict {
			return err
		}
	}
	return nil
}
