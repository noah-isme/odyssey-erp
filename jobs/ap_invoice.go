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

// APInvoiceProcessTaskID is the stable asynq TaskID for an invoice's
// processing task, so duplicate enqueues collapse while the task exists.
func APInvoiceProcessTaskID(invoiceID int64) string {
	return "ap-invoice-process:" + strconv.FormatInt(invoiceID, 10)
}

func EnqueueProcessAPInvoice(client *asynq.Client, invoiceID, createdBy int64) error {
	payload, err := json.Marshal(ProcessAPInvoicePayload{
		InvoiceID: invoiceID,
		CreatedBy: createdBy,
	})
	if err != nil {
		return err
	}
	task := asynq.NewTask(TaskProcessAPInvoice, payload,
		asynq.MaxRetry(3),
		asynq.Timeout(5*time.Minute),
		asynq.TaskID(APInvoiceProcessTaskID(invoiceID)),
	)
	_, err = client.Enqueue(task)
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

// HandleProcessAPInvoice decodes the payload and runs processFn. Malformed
// payloads, non-positive invoice IDs, and any error matching one of
// permanentErrs (for the worker: ap.ErrInvoiceNotFound and ap.ErrActorMismatch;
// jobs cannot import internal/ap because of an import cycle) are wrapped with
// asynq.SkipRetry so they archive on first delivery.
func HandleProcessAPInvoice(processFn func(ctx context.Context, invoiceID, createdBy int64) error, permanentErrs ...error) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		var p ProcessAPInvoicePayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
		}
		if p.InvoiceID <= 0 {
			return fmt.Errorf("ap invoice process: invalid invoice_id %d: %w", p.InvoiceID, asynq.SkipRetry)
		}
		err := processFn(ctx, p.InvoiceID, p.CreatedBy)
		if err == nil {
			return nil
		}
		for _, permanent := range permanentErrs {
			if permanent != nil && errors.Is(err, permanent) {
				return fmt.Errorf("%w: %w", err, asynq.SkipRetry)
			}
		}
		return err
	}
}
