package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

type ProcessAPInvoicePayload struct {
	InvoiceID int64 `json:"invoice_id"`
	CreatedBy int64 `json:"created_by"`
}

func EnqueueProcessAPInvoice(client *asynq.Client, invoiceID, createdBy int64) error {
	payload, err := json.Marshal(ProcessAPInvoicePayload{
		InvoiceID: invoiceID,
		CreatedBy: createdBy,
	})
	if err != nil {
		return err
	}
	task := asynq.NewTask(TaskProcessAPInvoice, payload, asynq.MaxRetry(3), asynq.Timeout(5*time.Minute))
	_, err = client.Enqueue(task)
	return err
}

func HandleProcessAPInvoice(processFn func(ctx context.Context, invoiceID, createdBy int64) error) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		var p ProcessAPInvoicePayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
		}
		return processFn(ctx, p.InvoiceID, p.CreatedBy)
	}
}
