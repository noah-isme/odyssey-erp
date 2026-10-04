package jobs

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

func TestBoundedObjectTaskProducers(t *testing.T) {
	tests := []struct {
		name     string
		enqueue  func(*Client, int64) (*asynq.TaskInfo, error)
		taskID   func(int64) string
		wantID   string
		wantType string
	}{
		{
			name: "variance snapshot",
			enqueue: func(c *Client, id int64) (*asynq.TaskInfo, error) {
				return c.EnqueueVarianceSnapshot(context.Background(), id)
			},
			taskID:   VarianceSnapshotTaskID,
			wantID:   "variance-snapshot:42",
			wantType: TaskVarianceSnapshotProcess,
		},
		{
			name: "board pack",
			enqueue: func(c *Client, id int64) (*asynq.TaskInfo, error) {
				return c.EnqueueBoardPack(context.Background(), id)
			},
			taskID:   BoardPackTaskID,
			wantID:   "board-pack:42",
			wantType: TaskBoardPackGenerate,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mr := miniredis.RunT(t)
			redisOpts := asynq.RedisClientOpt{Addr: mr.Addr()}
			client, err := NewClient(redisOpts)
			require.NoError(t, err)
			t.Cleanup(func() { _ = client.Close() })

			info, err := tc.enqueue(client, 42)
			require.NoError(t, err)
			require.NotNil(t, info)
			require.Equal(t, tc.wantID, info.ID)
			require.Equal(t, tc.wantID, tc.taskID(42))
			require.Equal(t, tc.wantType, info.Type)
			require.Equal(t, QueueDefault, info.Queue)
			require.Equal(t, 5, info.MaxRetry)

			dup, err := tc.enqueue(client, 42)
			require.NoError(t, err, "duplicate enqueue must collapse to nil")
			require.Nil(t, dup)

			inspector := asynq.NewInspector(redisOpts)
			t.Cleanup(func() { _ = inspector.Close() })
			pending, err := inspector.ListPendingTasks(QueueDefault)
			require.NoError(t, err)
			require.Len(t, pending, 1)
			require.Equal(t, 5, pending[0].MaxRetry)
		})
	}
}

// TestWorkerArchivesMissingBoardPackWithoutRetry runs a board-pack task
// produced by EnqueueBoardPack through the worker with a handler that mirrors
// the not-found branch of boardpack.Job.Handle (ErrBoardPackNotFound ->
// asynq.SkipRetry). The real job needs a database-backed service, so this
// pins the queue-side contract: the task archives on its first delivery.
func TestWorkerArchivesMissingBoardPackWithoutRetry(t *testing.T) {
	mr := miniredis.RunT(t)
	redisOpts := asynq.RedisClientOpt{Addr: mr.Addr()}
	var calls atomic.Int32
	worker, err := NewWorker(WorkerConfig{
		RedisOpts:      redisOpts,
		Mailer:         &mailFake{},
		RetryDelayFunc: func(int, error, *asynq.Task) time.Duration { return 10 * time.Millisecond },
		Handlers: []TaskHandler{{
			Type: TaskBoardPackGenerate,
			Handler: func(context.Context, *asynq.Task) error {
				calls.Add(1)
				return fmt.Errorf("board pack not found: %w", asynq.SkipRetry)
			},
		}},
	})
	require.NoError(t, err)

	client, err := NewClient(redisOpts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- worker.Run(ctx) }()

	_, err = client.EnqueueBoardPack(ctx, 77)
	require.NoError(t, err)

	inspector := asynq.NewInspector(redisOpts)
	t.Cleanup(func() { _ = inspector.Close() })
	var info *asynq.TaskInfo
	require.Eventually(t, func() bool {
		info, err = inspector.GetTaskInfo(QueueDefault, BoardPackTaskID(77))
		return err == nil && info.State == asynq.TaskStateArchived
	}, 15*time.Second, 50*time.Millisecond)
	require.Equal(t, 0, info.Retried)
	require.Equal(t, int32(1), calls.Load())

	cancel()
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
}
