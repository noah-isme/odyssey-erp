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
	errTestBusy            = errors.New("ap: invoice processing is held by another session")
)

type apProcessFake struct {
	mu    sync.Mutex
	err   error
	seq   []error // consumed one per call before falling back to err
	calls int
}

func (f *apProcessFake) Process(context.Context, int64, int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if len(f.seq) > 0 {
		next := f.seq[0]
		f.seq = f.seq[1:]
		return next
	}
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
		{name: "busy lock is retryable, not skipped", task: func(t *testing.T) *asynq.Task { return apInvoiceTask(t, 5, 1) }, err: fmt.Errorf("%w: invoice 5", errTestBusy), wantErr: errTestBusy, wantCalls: 1},
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
	require.Equal(t, APInvoiceProcessMaxRetry, info.MaxRetry)
	require.Equal(t, 5, info.MaxRetry, "retry budget is sized to outlast the orphaned-lock bound; see APInvoiceProcessMaxRetry")
	require.Equal(t, APInvoiceProcessTimeout, info.Timeout)
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

// runAPWorker starts an asynq server wired like the worker's ap:invoice_process
// handler, with a 10ms retry delay and a 50ms retry-forwarder interval (the
// asynq default of 5s per retry would make multi-retry tests slow), and
// enqueues the invoice through EnqueueProcessAPInvoice so the production
// MaxRetry and timeout apply. The server is shut down at test cleanup.
func runAPWorker(t *testing.T, fake *apProcessFake, invoiceID int64) *asynq.Inspector {
	t.Helper()
	mr := miniredis.RunT(t)
	redisOpts := asynq.RedisClientOpt{Addr: mr.Addr()}
	srv := asynq.NewServer(redisOpts, asynq.Config{
		Concurrency:              1,
		DelayedTaskCheckInterval: 50 * time.Millisecond,
		RetryDelayFunc:           func(int, error, *asynq.Task) time.Duration { return 10 * time.Millisecond },
		Queues:                   map[string]int{QueueDefault: 1},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskProcessAPInvoice, HandleProcessAPInvoice(fake.Process, errTestInvoiceNotFound, errTestActorMismatch))
	require.NoError(t, srv.Start(mux))
	t.Cleanup(srv.Shutdown)

	client := asynq.NewClient(redisOpts)
	t.Cleanup(func() { _ = client.Close() })
	require.NoError(t, EnqueueProcessAPInvoice(client, invoiceID, 1))

	inspector := asynq.NewInspector(redisOpts)
	t.Cleanup(func() { _ = inspector.Close() })
	return inspector
}

func TestWorkerRetriesBusyAPInvoiceUntilHolderFinishes(t *testing.T) {
	busy := fmt.Errorf("%w: invoice 11", errTestBusy)
	fake := &apProcessFake{seq: []error{busy, busy, busy}}
	inspector := runAPWorker(t, fake, 11)

	// A completed task is deleted (no retention), so the task disappears from
	// the inspector once the fourth delivery returned nil.
	require.Eventually(t, func() bool {
		_, err := inspector.GetTaskInfo(QueueDefault, APInvoiceProcessTaskID(11))
		return errors.Is(err, asynq.ErrTaskNotFound)
	}, 15*time.Second, 20*time.Millisecond)
	require.Equal(t, 4, fake.Calls(), "three busy deliveries are retried, the fourth converges")
	archived, err := inspector.ListArchivedTasks(QueueDefault)
	require.NoError(t, err)
	require.Empty(t, archived, "a busy lock that clears must not archive the task")
}

func TestWorkerArchivesAPInvoiceWhenBusyLockNeverClears(t *testing.T) {
	busy := fmt.Errorf("%w: invoice 12", errTestBusy)
	fake := &apProcessFake{err: busy}
	inspector := runAPWorker(t, fake, 12)

	var info *asynq.TaskInfo
	var err error
	require.Eventually(t, func() bool {
		info, err = inspector.GetTaskInfo(QueueDefault, APInvoiceProcessTaskID(12))
		return err == nil && info.State == asynq.TaskStateArchived
	}, 15*time.Second, 20*time.Millisecond)
	require.Equal(t, APInvoiceProcessMaxRetry, info.Retried, "every retry is used before archiving")
	require.Equal(t, APInvoiceProcessMaxRetry+1, fake.Calls())
	require.Contains(t, info.LastErr, errTestBusy.Error(), "the archive must show why the task failed")
}
