package jobs

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

func TestEnqueueScheduledReportEmailDeduplicatesByScheduleAndPeriod(t *testing.T) {
	redis := miniredis.RunT(t)
	redisOpts := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(redisOpts)
	t.Cleanup(func() { _ = client.Close() })

	first := asynq.NewTask(TypeEmailDelivery, []byte(`{"to":["finance@example.com"],"subject":"P&L"}`))
	second := asynq.NewTask(TypeEmailDelivery, []byte(`{"to":["finance@example.com"],"subject":"P&L"}`))
	require.NoError(t, enqueueScheduledReportEmail(context.Background(), client, first, 7, "2026-08"))
	require.NoError(t, enqueueScheduledReportEmail(context.Background(), client, second, 7, "2026-08"))

	inspector := asynq.NewInspector(redisOpts)
	t.Cleanup(func() { _ = inspector.Close() })
	tasks, err := inspector.ListPendingTasks(QueueDefault)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, "report-schedule:7:2026-08", tasks[0].ID)
}
