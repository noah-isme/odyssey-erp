package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/hibiken/asynq"
)

type ProcessAPInvoicePayload struct {
	InvoiceID int64 `json:"invoice_id"`
	CreatedBy int64 `json:"created_by"`
}

func EnqueueProcessAPInvoice(client *asynq.Client, invoiceID, createdBy int64) error {
	if client == nil {
		return errors.New("AP invoice queue client not configured")
	}
	if invoiceID <= 0 {
		return errors.New("AP invoice id required")
	}
	if createdBy <= 0 {
		return errors.New("AP invoice creator required")
	}
	payload, err := json.Marshal(ProcessAPInvoicePayload{
		InvoiceID: invoiceID,
		CreatedBy: createdBy,
	})
	if err != nil {
		return err
	}
	// Processing is invoice-scoped: a repeated enqueue must not create a
	// second matching/posting run for the same invoice while the first is
	// pending or being retried.
	task := asynq.NewTask(TaskProcessAPInvoice, payload, asynq.MaxRetry(3), asynq.Timeout(5*time.Minute))
	_, err = client.Enqueue(task,
		asynq.Queue(QueueDefault),
		asynq.TaskID("ap-invoice:"+strconv.FormatInt(invoiceID, 10)),
	)
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

func HandleProcessAPInvoice(processFn func(ctx context.Context, invoiceID, createdBy int64) error) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		if processFn == nil {
			return fmt.Errorf("AP invoice processor not configured: %w", asynq.SkipRetry)
		}
		if t == nil {
			return fmt.Errorf("AP invoice task not configured: %w", asynq.SkipRetry)
		}
		var p ProcessAPInvoicePayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
		}
		if p.InvoiceID <= 0 || p.CreatedBy <= 0 {
			return fmt.Errorf("invalid AP invoice task: invoice id and creator are required: %w", asynq.SkipRetry)
		}
		return processFn(ctx, p.InvoiceID, p.CreatedBy)
	}
}
