package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

func TestEnqueueProcessAPInvoiceFailsClosedBeforeQueueUse(t *testing.T) {
	tests := []struct {
		name        string
		client      *asynq.Client
		invoiceID   int64
		createdBy   int64
		wantMessage string
	}{
		{name: "queue client", invoiceID: 7, createdBy: 11, wantMessage: "queue client"},
		{name: "invoice id", client: &asynq.Client{}, createdBy: 11, wantMessage: "invoice id"},
		{name: "creator", client: &asynq.Client{}, invoiceID: 7, wantMessage: "creator"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EnqueueProcessAPInvoice(tt.client, tt.invoiceID, tt.createdBy)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantMessage)
		})
	}
}

func TestEnqueueProcessAPInvoiceDeduplicatesByInvoiceID(t *testing.T) {
	redis := miniredis.RunT(t)
	redisOpts := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(redisOpts)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, EnqueueProcessAPInvoice(client, 7, 11))
	require.NoError(t, EnqueueProcessAPInvoice(client, 7, 12))

	inspector := asynq.NewInspector(redisOpts)
	t.Cleanup(func() { _ = inspector.Close() })
	tasks, err := inspector.ListPendingTasks(QueueDefault)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, "ap-invoice:7", tasks[0].ID)
	require.Equal(t, TaskProcessAPInvoice, tasks[0].Type)
}

func TestHandleProcessAPInvoiceRejectsUnconfiguredOrInvalidTasks(t *testing.T) {
	processor := func(context.Context, int64, int64) error { return nil }
	tests := []struct {
		name string
		fn   func(context.Context, int64, int64) error
		task *asynq.Task
	}{
		{name: "processor", task: asynq.NewTask(TaskProcessAPInvoice, []byte(`{"invoice_id":7,"created_by":11}`))},
		{name: "task", fn: processor},
		{name: "malformed payload", fn: processor, task: asynq.NewTask(TaskProcessAPInvoice, []byte("{"))},
		{name: "missing invoice", fn: processor, task: asynq.NewTask(TaskProcessAPInvoice, []byte(`{"created_by":11}`))},
		{name: "missing creator", fn: processor, task: asynq.NewTask(TaskProcessAPInvoice, []byte(`{"invoice_id":7}`))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := HandleProcessAPInvoice(tt.fn)(context.Background(), tt.task)
			require.Error(t, err)
			require.True(t, errors.Is(err, asynq.SkipRetry), "error = %v", err)
		})
	}
}

func TestHandleProcessAPInvoicePassesValidatedPayload(t *testing.T) {
	var gotInvoiceID, gotCreatedBy int64
	processor := func(_ context.Context, invoiceID, createdBy int64) error {
		gotInvoiceID, gotCreatedBy = invoiceID, createdBy
		return nil
	}
	task := asynq.NewTask(TaskProcessAPInvoice, []byte(`{"invoice_id":7,"created_by":11}`))

	require.NoError(t, HandleProcessAPInvoice(processor)(context.Background(), task))
	require.Equal(t, int64(7), gotInvoiceID)
	require.Equal(t, int64(11), gotCreatedBy)
}
