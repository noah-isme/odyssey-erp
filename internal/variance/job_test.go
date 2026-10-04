package variance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/jobs"
)

type snapshotProcessorFake struct {
	err   error
	calls atomic.Int32
}

func (f *snapshotProcessorFake) ProcessSnapshot(context.Context, int64) error {
	f.calls.Add(1)
	return f.err
}

func snapshotTask(t *testing.T, id int64) *asynq.Task {
	t.Helper()
	body, err := json.Marshal(jobs.VarianceSnapshotPayload{SnapshotID: id})
	require.NoError(t, err)
	return asynq.NewTask(jobs.TaskVarianceSnapshotProcess, body)
}

func TestSnapshotJobErrorMapping(t *testing.T) {
	transient := errors.New("db down")
	tests := []struct {
		name      string
		task      func(*testing.T) *asynq.Task
		err       error
		wantErr   error
		wantSkip  bool
		wantNoErr bool
		wantCalls int32
	}{
		{name: "processed", task: func(t *testing.T) *asynq.Task { return snapshotTask(t, 3) }, wantNoErr: true, wantCalls: 1},
		{name: "malformed payload skips retry", task: func(*testing.T) *asynq.Task { return asynq.NewTask(jobs.TaskVarianceSnapshotProcess, []byte("{")) }, wantSkip: true},
		{name: "zero snapshot id skips retry", task: func(t *testing.T) *asynq.Task { return snapshotTask(t, 0) }, wantSkip: true},
		{name: "not found skips retry", task: func(t *testing.T) *asynq.Task { return snapshotTask(t, 3) }, err: ErrSnapshotNotFound, wantErr: ErrSnapshotNotFound, wantSkip: true, wantCalls: 1},
		{name: "wrapped not found skips retry", task: func(t *testing.T) *asynq.Task { return snapshotTask(t, 3) }, err: fmt.Errorf("load: %w", ErrSnapshotNotFound), wantErr: ErrSnapshotNotFound, wantSkip: true, wantCalls: 1},
		{name: "transient error retries", task: func(t *testing.T) *asynq.Task { return snapshotTask(t, 3) }, err: transient, wantErr: transient, wantCalls: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &snapshotProcessorFake{err: tc.err}
			job := &SnapshotJob{service: fake}
			err := job.Handle(context.Background(), tc.task(t))
			require.Equal(t, tc.wantCalls, fake.calls.Load())
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

func TestWorkerArchivesMissingVarianceSnapshotWithoutRetry(t *testing.T) {
	mr := miniredis.RunT(t)
	redisOpts := asynq.RedisClientOpt{Addr: mr.Addr()}
	fake := &snapshotProcessorFake{err: ErrSnapshotNotFound}
	job := &SnapshotJob{service: fake}
	worker, err := jobs.NewWorker(jobs.WorkerConfig{
		RedisOpts:      redisOpts,
		RetryDelayFunc: func(int, error, *asynq.Task) time.Duration { return 10 * time.Millisecond },
		Handlers:       []jobs.TaskHandler{{Type: jobs.TaskVarianceSnapshotProcess, Handler: job.Handle}},
	})
	require.NoError(t, err)

	client, err := jobs.NewClient(redisOpts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- worker.Run(ctx) }()

	// A forged snapshot ID enqueued twice collapses to one task.
	_, err = client.EnqueueVarianceSnapshot(ctx, 9_000_001)
	require.NoError(t, err)
	dup, err := client.EnqueueVarianceSnapshot(ctx, 9_000_001)
	require.NoError(t, err)
	require.Nil(t, dup)

	inspector := asynq.NewInspector(redisOpts)
	t.Cleanup(func() { _ = inspector.Close() })
	var info *asynq.TaskInfo
	require.Eventually(t, func() bool {
		info, err = inspector.GetTaskInfo(jobs.QueueDefault, jobs.VarianceSnapshotTaskID(9_000_001))
		return err == nil && info.State == asynq.TaskStateArchived
	}, 15*time.Second, 50*time.Millisecond)
	require.Equal(t, 0, info.Retried)
	require.Equal(t, 5, info.MaxRetry)
	require.Contains(t, info.LastErr, ErrSnapshotNotFound.Error())
	require.Equal(t, int32(1), fake.calls.Load())

	cancel()
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
}
