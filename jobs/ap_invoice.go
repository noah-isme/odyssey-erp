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

const (
	// APInvoiceProcessTimeout is the asynq timeout of ap:invoice_process. The
	// per-invoice advisory lock transaction bounds its own idle time just above
	// this value (internal/ap processingLockIdleTimeout, 6m).
	APInvoiceProcessTimeout = 5 * time.Minute

	// APInvoiceProcessMaxRetry is sized so the retries of a task that finds the
	// per-invoice lock busy outlast an orphaned lock.
	//
	// ProcessInvoice returns the retryable ap.ErrInvoiceProcessingBusy when the
	// lock is held. If the holder is an orphaned session of a crashed worker,
	// PostgreSQL terminates its backend at most processingLockIdleTimeout (6m =
	// 360s) after the lock was taken, because the lock transaction sets
	// idle_in_transaction_session_timeout. The task must still be retrying then.
	//
	// The worker uses asynq v0.25.1 DefaultRetryDelayFunc (cmd/worker does not
	// set RetryDelayFunc): delay(n) = n^4 + 15 + rand[0,29]*(n+1) seconds, with n
	// the number of retries already used, so the minimum is n^4 + 15 s.
	// A task fails busy, is retried after delay(0), delay(1), ... and is
	// archived by the failure of delivery number MaxRetry+1.
	//
	//	MaxRetry 3 (rc.8 value): 15+16+31             =  62s  (max 236s)  < 360s
	//	MaxRetry 4:              15+16+31+96          = 158s              < 360s
	//	MaxRetry 5:              15+16+31+96+271      = 429s  (max 864s) > 360s
	//
	// MaxRetry 5 is the smallest value whose guaranteed (minimum, jitter-free)
	// window exceeds the 360s orphan bound, with 69s (19%) margin before
	// counting crash detection. Detection adds at least 55s when the holder
	// crashed: the lease lasts 30s and the recoverer only reclaims tasks whose
	// lease expired 30s ago, then applies delay(Retried) before redelivery. A
	// task that already used k retries for unrelated failures has the window
	// sum(n=k..4) of the minimums: 398s for k=2 and 367s for k=3, both above
	// 360s; with k=4 it can exhaust earlier. In every case an exhausted task
	// is archived with the busy error in LastErr (visible), never completed.
	// TestAPInvoiceBusyRetryWindowOutlastsOrphanLock enforces this arithmetic.
	APInvoiceProcessMaxRetry = 5
)

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
		asynq.MaxRetry(APInvoiceProcessMaxRetry),
		asynq.Timeout(APInvoiceProcessTimeout),
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
// asynq.SkipRetry so they archive on first delivery. Every other error,
// including ap.ErrInvoiceProcessingBusy (the per-invoice lock is held), is
// returned as is so asynq retries it.
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
