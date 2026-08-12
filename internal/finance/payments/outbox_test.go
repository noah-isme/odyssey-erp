package payments

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

type paymentOutboxStoreFake struct {
	messages   []automation.OutboxMessage
	completed  []int64
	retried    []int64
	deadLetter []int64
}

func (f *paymentOutboxStoreFake) Claim(context.Context, string, int, automation.RetryPolicy) ([]automation.OutboxMessage, error) {
	messages := f.messages
	f.messages = nil
	return messages, nil
}

func (f *paymentOutboxStoreFake) Complete(_ context.Context, id int64, _ string) error {
	f.completed = append(f.completed, id)
	return nil
}

func (f *paymentOutboxStoreFake) Fail(_ context.Context, id int64, _ string, _ error, _ time.Time) (automation.OutboxStatus, error) {
	f.retried = append(f.retried, id)
	return automation.OutboxPending, nil
}

func (f *paymentOutboxStoreFake) DeadLetter(_ context.Context, id int64, _ string, _ error) error {
	f.deadLetter = append(f.deadLetter, id)
	return nil
}

func TestPaymentExecutionOutboxAmbiguousOutcomeDeadLettersAndDoesNotResubmit(t *testing.T) {
	port := &coordinatorPort{
		submitErr:    &automation.ProviderError{Category: automation.ErrorAmbiguous, Operation: "submit", Message: "timeout"},
		lookupResult: settledResult(),
	}
	coordinator, instruction := approvedCoordinator(t, port)
	command := PaymentExecutionCommand{Reference: instruction.Reference, ExecutorID: 303}
	payload, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	store := &paymentOutboxStoreFake{messages: []automation.OutboxMessage{{
		ID: 91, CompanyID: instruction.Reference.Connection.CompanyID,
		Operation: automation.OperationPaymentExecute, AggregateID: instruction.Reference.ObjectID,
		IdempotencyKey: "execute-91", Payload: payload,
	}}}
	dispatcher := automation.NewDispatcher(store, "payment-worker", nil)
	if err := RegisterPaymentExecutionHandlers(dispatcher, coordinator, nil); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.DispatchFinanceAutomation(context.Background(), 1); err == nil {
		t.Fatal("expected ambiguous dispatch error")
	}
	if len(store.deadLetter) != 1 || len(store.retried) != 0 {
		t.Fatalf("dead-letter=%v retries=%v", store.deadLetter, store.retried)
	}
	if port.submitCalls != 1 {
		t.Fatalf("submit calls = %d, want 1", port.submitCalls)
	}

	// A deliberate replay against the same coordinator resolves the persisted
	// ambiguity through Lookup; it never calls Submit a second time.
	if err := coordinator.HandlePaymentExecution(context.Background(), automation.OutboxMessage{
		CompanyID:   instruction.Reference.Connection.CompanyID,
		Operation:   automation.OperationPaymentExecute,
		AggregateID: instruction.Reference.ObjectID,
		Payload:     payload,
	}); err != nil {
		t.Fatal(err)
	}
	if port.submitCalls != 1 || port.lookupCalls != 1 {
		t.Fatalf("submit calls=%d lookup calls=%d", port.submitCalls, port.lookupCalls)
	}
}

func TestPaymentResultImportOutboxRetriesTransientEffect(t *testing.T) {
	effects := &settlementEffectsFake{err: &automation.ProviderError{Category: automation.ErrorTransient, Operation: "effects", Message: "temporary"}}
	service, instruction := settlementServiceFixture(t, effects)
	input := settlementInputFor(instruction, "result-retry", ResultStatusSettled, "125.50")
	outboxInput, err := NewPaymentResultImportOutboxInput(input, input.Correlation, 303)
	if err != nil {
		t.Fatal(err)
	}
	message := automation.OutboxMessage{
		CompanyID:      outboxInput.CompanyID,
		Topic:          outboxInput.Topic,
		AggregateType:  outboxInput.AggregateType,
		AggregateID:    outboxInput.AggregateID,
		Operation:      outboxInput.Operation,
		Correlation:    outboxInput.Correlation,
		IdempotencyKey: outboxInput.IdempotencyKey,
		Payload:        outboxInput.Payload,
		ID:             92,
	}
	store := &paymentOutboxStoreFake{messages: []automation.OutboxMessage{message}}
	dispatcher := automation.NewDispatcher(store, "payment-worker", nil)
	if err := RegisterPaymentExecutionHandlers(dispatcher, nil, service); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.DispatchFinanceAutomation(context.Background(), 1); err == nil {
		t.Fatal("expected transient dispatch error")
	}
	if len(store.retried) != 1 || len(store.deadLetter) != 0 {
		t.Fatalf("retries=%v dead-letter=%v", store.retried, store.deadLetter)
	}
}

func TestPaymentExecutionOutboxRejectsCrossCompanyPayload(t *testing.T) {
	coordinator := NewCoordinator(&coordinatorPort{}, NewMemoryStore(), nil)
	command := PaymentExecutionCommand{Reference: paymentInstruction().Reference, ExecutorID: 303}
	payload, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	err = coordinator.HandlePaymentExecution(context.Background(), automation.OutboxMessage{
		CompanyID: 8, Operation: automation.OperationPaymentExecute, Payload: payload,
	})
	if !errors.Is(err, ErrSettlementResultCompanyMismatch) {
		t.Fatalf("cross-company error = %v", err)
	}
}
