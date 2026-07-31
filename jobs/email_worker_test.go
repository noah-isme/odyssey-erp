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

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type retryMailer struct {
	mu        sync.Mutex
	calls     int
	failFirst bool
}

func (m *retryMailer) SendEmail(context.Context, string, string, string, *shared.Attachment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.failFirst && m.calls == 1 {
		return errors.New("smtp unavailable")
	}
	return nil
}

func (m *retryMailer) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func TestEnqueueSendEmailDeduplicatesByCorrelationID(t *testing.T) {
	mr := miniredis.RunT(t)
	redisOpts := asynq.RedisClientOpt{Addr: mr.Addr()}
	client, err := NewClient(redisOpts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	payload := SendEmailPayload{To: "user@example.com", Subject: "Invoice issued", Body: "Ready", CorrelationID: "notif-1"}
	info, err := client.EnqueueSendEmail(context.Background(), payload)
	require.NoError(t, err)
	require.NotNil(t, info)

	duplicate, err := client.EnqueueSendEmail(context.Background(), payload)
	require.NoError(t, err)
	require.Nil(t, duplicate)
}

func TestWorkerRetriesSendEmailAfterSMTPFailure(t *testing.T) {
	mr := miniredis.RunT(t)
	redisOpts := asynq.RedisClientOpt{Addr: mr.Addr()}
	mailer := &retryMailer{failFirst: true}
	worker, err := NewWorker(WorkerConfig{
		RedisOpts: redisOpts,
		Mailer:    mailer,
		RetryDelayFunc: func(int, error, *asynq.Task) time.Duration {
			return 10 * time.Millisecond
		},
	})
	require.NoError(t, err)

	client, err := NewClient(redisOpts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- worker.Run(ctx)
	}()

	_, err = client.EnqueueSendEmail(context.Background(), SendEmailPayload{
		To:            "user@example.com",
		Subject:       "Invoice issued",
		Body:          "Ready",
		CorrelationID: "notif-worker-1",
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool { return mailer.Calls() >= 2 }, 15*time.Second, 100*time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
	require.Equal(t, 2, mailer.Calls())
}
