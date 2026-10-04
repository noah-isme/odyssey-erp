package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

// Stand-ins for ap.ErrInvoiceNotFound / ap.ErrActorMismatch; jobs cannot
// import internal/ap (import cycle), so the worker passes the real sentinels.
var (
	errTestInvoiceNotFound = errors.New("invoice not found")
	errTestActorMismatch   = errors.New("ap: task actor does not match invoice creator")
)

type apProcessFake struct {
	mu    sync.Mutex
	err   error
	calls int
}

func (f *apProcessFake) Process(context.Context, int64, int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.err
}

func (f *apProcessFake) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func apInvoiceTask(t *testing.T, invoiceID, createdBy int64) *asynq.Task {
	t.Helper()
	payload, err := json.Marshal(ProcessAPInvoicePayload{InvoiceID: invoiceID, CreatedBy: createdBy})
	require.NoError(t, err)
	return asynq.NewTask(TaskProcessAPInvoice, payload)
}

func TestHandleProcessAPInvoiceErrorMapping(t *testing.T) {
	transient := errors.New("db down")
	tests := []struct {
		name      string
		task      func(t *testing.T) *asynq.Task
		err       error
		wantErr   error
		wantSkip  bool
		wantNoErr bool
		wantCalls int
	}{
		{name: "processed", task: func(t *testing.T) *asynq.Task { return apInvoiceTask(t, 5, 1) }, wantNoErr: true, wantCalls: 1},
		{name: "malformed payload skips retry", task: func(*testing.T) *asynq.Task { return asynq.NewTask(TaskProcessAPInvoice, []byte("{")) }, wantSkip: true},
		{name: "zero invoice id skips retry", task: func(t *testing.T) *asynq.Task { return apInvoiceTask(t, 0, 1) }, wantSkip: true},
		{name: "negative invoice id skips retry", task: func(t *testing.T) *asynq.Task { return apInvoiceTask(t, -3, 1) }, wantSkip: true},
		{name: "not found skips retry", task: func(t *testing.T) *asynq.Task { return apInvoiceTask(t, 5, 1) }, err: fmt.Errorf("%w: id 5", errTestInvoiceNotFound), wantErr: errTestInvoiceNotFound, wantSkip: true, wantCalls: 1},
		{name: "actor mismatch skips retry", task: func(t *testing.T) *asynq.Task { return apInvoiceTask(t, 5, 999) }, err: fmt.Errorf("%w: invoice 5", errTestActorMismatch), wantErr: errTestActorMismatch, wantSkip: true, wantCalls: 1},
		{name: "transient error retries", task: func(t *testing.T) *asynq.Task { return apInvoiceTask(t, 5, 1) }, err: transient, wantErr: transient, wantCalls: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &apProcessFake{err: tc.err}
			err := HandleProcessAPInvoice(fake.Process, errTestInvoiceNotFound, errTestActorMismatch)(context.Background(), tc.task(t))
			require.Equal(t, tc.wantCalls, fake.Calls())
			if tc.wantNoErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
			require.Equal(t, tc.wantSkip, errors.Is(err, asynq.SkipRetry))
		})
	}
}

func TestEnqueueProcessAPInvoiceUsesStableTaskID(t *testing.T) {
	mr := miniredis.RunT(t)
	redisOpts := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(redisOpts)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, EnqueueProcessAPInvoice(client, 42, 7))
	require.NoError(t, EnqueueProcessAPInvoice(client, 42, 7), "duplicate enqueue must collapse to nil")

	inspector := asynq.NewInspector(redisOpts)
	t.Cleanup(func() { _ = inspector.Close() })
	info, err := inspector.GetTaskInfo(QueueDefault, APInvoiceProcessTaskID(42))
	require.NoError(t, err)
	require.Equal(t, "ap-invoice-process:42", info.ID)
	require.Equal(t, 3, info.MaxRetry)
	require.Equal(t, 5*time.Minute, info.Timeout)
	pending, err := inspector.ListPendingTasks(QueueDefault)
	require.NoError(t, err)
	require.Len(t, pending, 1)
}

func TestWorkerArchivesForgedAPActorWithoutRetry(t *testing.T) {
	mr := miniredis.RunT(t)
	redisOpts := asynq.RedisClientOpt{Addr: mr.Addr()}
	fake := &apProcessFake{err: fmt.Errorf("%w: invoice 9", errTestActorMismatch)}
	worker, err := NewWorker(WorkerConfig{
		RedisOpts:      redisOpts,
		Mailer:         &mailFake{},
		RetryDelayFunc: func(int, error, *asynq.Task) time.Duration { return 10 * time.Millisecond },
		Handlers: []TaskHandler{{
			Type:    TaskProcessAPInvoice,
			Handler: HandleProcessAPInvoice(fake.Process, errTestInvoiceNotFound, errTestActorMismatch),
		}},
	})
	require.NoError(t, err)

	client := asynq.NewClient(redisOpts)
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- worker.Run(ctx) }()

	require.NoError(t, EnqueueProcessAPInvoice(client, 9, 999))

	inspector := asynq.NewInspector(redisOpts)
	t.Cleanup(func() { _ = inspector.Close() })
	var info *asynq.TaskInfo
	require.Eventually(t, func() bool {
		info, err = inspector.GetTaskInfo(QueueDefault, APInvoiceProcessTaskID(9))
		return err == nil && info.State == asynq.TaskStateArchived
	}, 15*time.Second, 50*time.Millisecond)
	require.Equal(t, 0, info.Retried)
	require.Contains(t, info.LastErr, errTestActorMismatch.Error())
	require.Equal(t, 1, fake.Calls())

	cancel()
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
}
