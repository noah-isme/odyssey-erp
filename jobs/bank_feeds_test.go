package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/bankfeeds"
)

func TestBankFeedsProcessorFailsClosedWhenServiceIsUnavailable(t *testing.T) {
	processor := NewBankFeedsProcessor(nil, nil)

	if err := processor.ProcessSyncTask(context.Background(), asynq.NewTask(TypeBankFeedsSync, []byte(`{"connection_id":1}`))); !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("sync error=%v, want SkipRetry", err)
	}
	if err := processor.ProcessEventTask(context.Background(), asynq.NewTask(TypeBankFeedsEvent, []byte(`{"event_id":1}`))); !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("event error=%v, want SkipRetry", err)
	}
}

func TestNewBankFeedsSyncTaskRequiresConnectionAndCarriesOnlyScopedID(t *testing.T) {
	if _, err := NewBankFeedsSyncTask(0); err == nil {
		t.Fatal("expected invalid connection id error")
	}
	task, err := NewBankFeedsSyncTask(42)
	if err != nil {
		t.Fatal(err)
	}
	if task.Type() != TypeBankFeedsSync {
		t.Fatalf("task type=%q, want %q", task.Type(), TypeBankFeedsSync)
	}
	if got, want := string(task.Payload()), `{"connection_id":42}`; got != want {
		t.Fatalf("task payload=%s, want %s", got, want)
	}
}

func TestEnqueueBankFeedsSyncFailsClosedWithoutQueueClient(t *testing.T) {
	var client *Client
	if _, err := client.EnqueueBankFeedsSync(context.Background(), 42); err == nil {
		t.Fatal("expected missing queue client error")
	}
}

func TestEnqueueBankFeedsEventFailsClosedWithoutQueueClient(t *testing.T) {
	var client *Client
	if _, err := client.EnqueueBankFeedsEvent(context.Background(), 42); err == nil {
		t.Fatal("expected missing queue client error")
	}
}

func TestBankFeedsProcessorRejectsInvalidTasksWithoutRetry(t *testing.T) {
	processor := NewBankFeedsProcessor(&bankfeeds.Service{}, nil)

	cases := []struct {
		name string
		call func() error
	}{
		{name: "sync missing connection", call: func() error {
			return processor.ProcessSyncTask(context.Background(), asynq.NewTask(TypeBankFeedsSync, []byte(`{"connection_id":0}`)))
		}},
		{name: "event missing event", call: func() error {
			return processor.ProcessEventTask(context.Background(), asynq.NewTask(TypeBankFeedsEvent, []byte(`{"event_id":0}`)))
		}},
		{name: "sync nil task", call: func() error {
			return processor.ProcessSyncTask(context.Background(), nil)
		}},
		{name: "event nil task", call: func() error {
			return processor.ProcessEventTask(context.Background(), nil)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, asynq.SkipRetry) {
				t.Fatalf("error=%v, want SkipRetry", err)
			}
		})
	}
}
