package jobs

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

func TestFinanceEnqueueMethodsFailClosedWithoutQueueClient(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		call func(*Client) error
	}{
		{
			name: "email",
			call: func(client *Client) error {
				_, err := client.EnqueueSendEmail(ctx, SendEmailPayload{To: "finance@example.com", Subject: "Report"})
				return err
			},
		},
		{
			name: "variance snapshot",
			call: func(client *Client) error {
				_, err := client.EnqueueVarianceSnapshot(ctx, 7)
				return err
			},
		},
		{
			name: "board pack",
			call: func(client *Client) error {
				_, err := client.EnqueueBoardPack(ctx, 7)
				return err
			},
		},
		{
			name: "cash forecast",
			call: func(client *Client) error {
				_, err := EnqueueCashForecastRefresh(ctx, nil, 1, 2)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(nil)
			require.Error(t, err)
			require.Contains(t, err.Error(), "queue client")
		})
	}
}

func TestFinanceEnqueueMethodsDeduplicateResourceTasks(t *testing.T) {
	redis := miniredis.RunT(t)
	redisOpts := asynq.RedisClientOpt{Addr: redis.Addr()}
	client, err := NewClient(redisOpts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	variance, err := client.EnqueueVarianceSnapshot(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, variance)
	duplicateVariance, err := client.EnqueueVarianceSnapshot(context.Background(), 7)
	require.NoError(t, err)
	require.Nil(t, duplicateVariance)

	boardPack, err := client.EnqueueBoardPack(context.Background(), 9)
	require.NoError(t, err)
	require.NotNil(t, boardPack)
	duplicateBoardPack, err := client.EnqueueBoardPack(context.Background(), 9)
	require.NoError(t, err)
	require.Nil(t, duplicateBoardPack)

	forecast, err := EnqueueCashForecastRefresh(context.Background(), client.AsynqClient(), 4, 6)
	require.NoError(t, err)
	require.NotNil(t, forecast)
	duplicateForecast, err := EnqueueCashForecastRefresh(context.Background(), client.AsynqClient(), 4, 6)
	require.NoError(t, err)
	require.Nil(t, duplicateForecast)

	inspector := asynq.NewInspector(redisOpts)
	t.Cleanup(func() { _ = inspector.Close() })
	tasks, err := inspector.ListPendingTasks(QueueDefault)
	require.NoError(t, err)
	require.Len(t, tasks, 3)
	ids := map[string]bool{}
	for _, task := range tasks {
		ids[task.ID] = true
	}
	require.True(t, ids["variance-snapshot:7"])
	require.True(t, ids["board-pack:9"])
	require.True(t, ids["cash-forecast:4:6"])
}
