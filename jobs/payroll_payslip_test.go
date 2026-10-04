package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/payroll"
)

type payslipSenderFake struct {
	mu    sync.Mutex
	err   error
	calls int
}

func (s *payslipSenderFake) DeliverPayslip(context.Context, int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.err
}

func (s *payslipSenderFake) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestHandlePayrollPayslipEmailErrorMapping(t *testing.T) {
	transient := errors.New("smtp down")
	tests := []struct {
		name      string
		err       error
		wantErr   error
		wantSkip  bool
		wantNoErr bool
	}{
		{name: "delivered or already delivered", wantNoErr: true},
		{name: "not found skips retry", err: payroll.ErrPayslipNotFound, wantErr: payroll.ErrPayslipNotFound, wantSkip: true},
		{name: "transient error retries", err: transient, wantErr: transient},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task, err := NewPayrollPayslipTask(77)
			require.NoError(t, err)
			err = HandlePayrollPayslipEmail(&payslipSenderFake{err: tc.err})(context.Background(), task)
			if tc.wantNoErr {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.wantSkip, errors.Is(err, asynq.SkipRetry))
		})
	}
}

func TestWorkerArchivesNotFoundPayslipWithoutRetry(t *testing.T) {
	mr := miniredis.RunT(t)
	redisOpts := asynq.RedisClientOpt{Addr: mr.Addr()}
	sender := &payslipSenderFake{err: payroll.ErrPayslipNotFound}
	worker, err := NewWorker(WorkerConfig{
		RedisOpts:      redisOpts,
		Mailer:         &mailFake{},
		RetryDelayFunc: func(int, error, *asynq.Task) time.Duration { return 10 * time.Millisecond },
		Handlers:       []TaskHandler{{Type: TaskPayrollPayslipEmail, Handler: HandlePayrollPayslipEmail(sender)}},
	})
	require.NoError(t, err)

	client, err := NewClient(redisOpts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- worker.Run(ctx) }()

	task, err := NewPayrollPayslipTask(404)
	require.NoError(t, err)
	_, err = client.AsynqClient().Enqueue(task, asynq.Queue(QueueDefault), asynq.MaxRetry(5), asynq.Timeout(3*time.Minute), asynq.TaskID("payroll-payslip-404"))
	require.NoError(t, err)

	inspector := asynq.NewInspector(redisOpts)
	t.Cleanup(func() { _ = inspector.Close() })
	var info *asynq.TaskInfo
	require.Eventually(t, func() bool {
		info, err = inspector.GetTaskInfo(QueueDefault, "payroll-payslip-404")
		return err == nil && info.State == asynq.TaskStateArchived
	}, 15*time.Second, 50*time.Millisecond)
	require.Equal(t, 0, info.Retried)
	require.Contains(t, info.LastErr, payroll.ErrPayslipNotFound.Error())
	require.Equal(t, 1, sender.Calls())

	cancel()
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
}
