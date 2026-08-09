package automation

import (
	"context"
	"errors"
	"testing"
	"time"
)

type dispatcherStoreFake struct {
	messages   []OutboxMessage
	completed  []int64
	deadLetter []int64
	failures   []int64
}

func (f *dispatcherStoreFake) Claim(context.Context, string, int, RetryPolicy) ([]OutboxMessage, error) {
	messages := f.messages
	f.messages = nil
	return messages, nil
}
func (f *dispatcherStoreFake) Complete(_ context.Context, id int64, _ string) error {
	f.completed = append(f.completed, id)
	return nil
}
func (f *dispatcherStoreFake) Fail(_ context.Context, id int64, _ string, _ error, _ time.Time) (OutboxStatus, error) {
	f.failures = append(f.failures, id)
	return OutboxPending, nil
}
func (f *dispatcherStoreFake) DeadLetter(_ context.Context, id int64, _ string, _ error) error {
	f.deadLetter = append(f.deadLetter, id)
	return nil
}

func TestDispatcherCompletesIdempotentOperation(t *testing.T) {
	store := &dispatcherStoreFake{messages: []OutboxMessage{{ID: 11, CompanyID: 7, Operation: "payment.submit", IdempotencyKey: "pay-11"}}}
	dispatcher := NewDispatcher(store, "worker-1", nil)
	var received OutboxMessage
	if err := dispatcher.Register("payment.submit", func(_ context.Context, message OutboxMessage) error {
		received = message
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.DispatchFinanceAutomation(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	if received.CompanyID != 7 || received.IdempotencyKey != "pay-11" {
		t.Fatalf("handler received %#v", received)
	}
	if len(store.completed) != 1 || store.completed[0] != 11 {
		t.Fatalf("completed = %v", store.completed)
	}
}

func TestDispatcherDeadLettersNonRetryableProviderOutcome(t *testing.T) {
	store := &dispatcherStoreFake{messages: []OutboxMessage{{ID: 12, Operation: "payment.submit"}}}
	dispatcher := NewDispatcher(store, "worker-1", nil)
	if err := dispatcher.Register("payment.submit", func(context.Context, OutboxMessage) error {
		return &ProviderError{Category: ErrorAmbiguous, Operation: "submit", Message: "provider outcome is unknown"}
	}); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.DispatchFinanceAutomation(context.Background(), 1); err == nil {
		t.Fatal("expected dispatch error")
	}
	if len(store.deadLetter) != 1 || len(store.failures) != 0 {
		t.Fatalf("dead-letter=%v failures=%v", store.deadLetter, store.failures)
	}
}

func TestDispatcherRetriesTransientFailure(t *testing.T) {
	store := &dispatcherStoreFake{messages: []OutboxMessage{{ID: 13, Attempts: 1, Operation: "payment.submit"}}}
	dispatcher := NewDispatcher(store, "worker-1", nil)
	if err := dispatcher.Register("payment.submit", func(context.Context, OutboxMessage) error {
		return &ProviderError{Category: ErrorTransient, Operation: "submit", Err: errors.New("timeout")}
	}); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.DispatchFinanceAutomation(context.Background(), 1); err == nil {
		t.Fatal("expected dispatch error")
	}
	if len(store.failures) != 1 || len(store.deadLetter) != 0 {
		t.Fatalf("dead-letter=%v failures=%v", store.deadLetter, store.failures)
	}
}
