package payments

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

type settlementEffectsFake struct {
	calls int
	err   error
	last  SettlementEffectRequest
}

func (f *settlementEffectsFake) ApplySettlementEffects(_ context.Context, request SettlementEffectRequest) (SettlementEffectOutcome, error) {
	f.calls++
	f.last = request
	if f.err != nil {
		return SettlementEffectOutcome{}, f.err
	}
	return SettlementEffectOutcome{Applied: true}, nil
}

func settlementInputFor(instruction Instruction, resultID, status, amount string) SettlementResultInput {
	settledAmount := automation.ExactAmount{}
	if amount != "" {
		settledAmount = automation.MustParseExact(amount)
	}
	return SettlementResultInput{
		CompanyID:            instruction.Reference.Connection.CompanyID,
		InstructionReference: instruction.Reference,
		ProviderReference:    providerReference(),
		ResultID:             resultID,
		Status:               status,
		SettledAmount:        settledAmount,
		SettledAt:            time.Date(2026, time.August, 12, 9, 5, 0, 0, time.UTC),
		EndToEndReference:    instruction.EndToEndReference,
		Correlation:          instruction.Correlation,
	}
}

func settlementServiceFixture(t *testing.T, effects SettlementEffectsPort) (*SettlementService, Instruction) {
	t.Helper()
	instruction := paymentInstruction()
	store := NewMemoryStore()
	execution := PaymentExecution{
		Instruction: instruction,
		State:       StateExported,
		ExportArtifact: &ExportArtifact{
			Reference: automation.ExternalReference{
				Connection: instruction.Reference.Connection,
				ObjectType: "bank_file",
				ObjectID:   "file-1",
			},
			Checksum: "sha256:file-1",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.Save(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	return NewSettlementService(store, NewMemorySettlementResultStore(), effects), instruction
}

func TestSettlementServiceDeduplicatesIdenticalResultWithoutRepeatingEffects(t *testing.T) {
	effects := &settlementEffectsFake{}
	service, instruction := settlementServiceFixture(t, effects)
	input := settlementInputFor(instruction, "result-1", ResultStatusPartial, "50.00")

	first, err := service.ImportResult(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ImportResult(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if first.State != StatePartiallySettled || second.State != StatePartiallySettled {
		t.Fatalf("states = %s, %s", first.State, second.State)
	}
	if effects.calls != 1 {
		t.Fatalf("effect calls = %d, want 1", effects.calls)
	}
	if effects.last.EffectKey != "result-1" || effects.last.CompanyID != instruction.Reference.Connection.CompanyID {
		t.Fatalf("effect request = %#v", effects.last)
	}
}

func TestSettlementServiceRejectsChangedDuplicateResult(t *testing.T) {
	service, instruction := settlementServiceFixture(t, &settlementEffectsFake{})
	input := settlementInputFor(instruction, "result-duplicate", ResultStatusSettled, "125.50")
	if _, err := service.ImportResult(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	input.SettledAmount = automation.MustParseExact("125.49")
	if _, err := service.ImportResult(context.Background(), input); !errors.Is(err, ErrSettlementResultConflict) {
		t.Fatalf("changed duplicate error = %v, want %v", err, ErrSettlementResultConflict)
	}
}

func TestSettlementServiceHandlesFailedResultWithoutFinancialEffects(t *testing.T) {
	effects := &settlementEffectsFake{}
	service, instruction := settlementServiceFixture(t, effects)
	input := settlementInputFor(instruction, "result-failed", ResultStatusFailed, "")
	input.SettledAmount = automation.ExactAmount{}
	result, err := service.ImportResult(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != StateFailed {
		t.Fatalf("state = %s, want %s", result.State, StateFailed)
	}
	if effects.calls != 0 {
		t.Fatalf("effect calls = %d, want 0", effects.calls)
	}
}

func TestSettlementServiceHandlesSettledResultAndEffectRetry(t *testing.T) {
	effects := &settlementEffectsFake{err: &automation.ProviderError{Category: automation.ErrorTransient, Operation: "settlement-effect", Message: "temporary"}}
	service, instruction := settlementServiceFixture(t, effects)
	input := settlementInputFor(instruction, "result-settled", ResultStatusSettled, "125.50")
	if _, err := service.ImportResult(context.Background(), input); err == nil {
		t.Fatal("expected effect failure")
	}
	if effects.calls != 1 {
		t.Fatalf("effect calls after first attempt = %d, want 1", effects.calls)
	}
	effects.err = nil
	result, err := service.ImportResult(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != StateSettled || !result.EffectApplied {
		t.Fatalf("result = %#v", result)
	}
	if effects.calls != 2 {
		t.Fatalf("effect calls after retry = %d, want 2", effects.calls)
	}
}

func TestSettlementServiceDoesNotApplyEffectsForInvalidTerminalTransition(t *testing.T) {
	effects := &settlementEffectsFake{}
	service, instruction := settlementServiceFixture(t, effects)
	failed := settlementInputFor(instruction, "result-terminal-failed", ResultStatusFailed, "")
	failed.SettledAmount = automation.ExactAmount{}
	if _, err := service.ImportResult(context.Background(), failed); err != nil {
		t.Fatal(err)
	}
	settled := settlementInputFor(instruction, "result-after-failed", ResultStatusSettled, "125.50")
	if _, err := service.ImportResult(context.Background(), settled); err == nil {
		t.Fatal("expected terminal transition error")
	}
	if effects.calls != 0 {
		t.Fatalf("effect calls = %d, want 0", effects.calls)
	}
}

func TestSettlementResultValidationRejectsCrossCompanyAndReference(t *testing.T) {
	instruction := paymentInstruction()
	input := settlementInputFor(instruction, "result-invalid", ResultStatusSettled, "125.50")
	input.CompanyID = 8
	if !errors.Is(input.Validate(), ErrSettlementResultCompanyMismatch) {
		t.Fatalf("company error = %v", input.Validate())
	}
	input = settlementInputFor(instruction, "result-invalid-ref", ResultStatusSettled, "125.50")
	input.ProviderReference.Connection.CompanyID = 8
	if !errors.Is(input.Validate(), ErrSettlementResultReferenceMismatch) {
		t.Fatalf("reference error = %v", input.Validate())
	}
	input = settlementInputFor(instruction, "result-invalid-status", "PENDING", "125.50")
	if !errors.Is(input.Validate(), ErrUnsupportedSettlementResult) {
		t.Fatalf("status error = %v", input.Validate())
	}
}

func TestSettlementEffectsMemoryPortRejectsConflictingEffectKey(t *testing.T) {
	port := NewMemorySettlementEffects()
	service, instruction := settlementServiceFixture(t, port)
	input := settlementInputFor(instruction, "effect-1", ResultStatusSettled, "125.50")
	result, err := service.ImportResult(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	request := SettlementEffectRequest{CompanyID: result.CompanyID, EffectKey: "effect-1", Result: result}
	request.Result.SettledAmount = automation.MustParseExact("124.00")
	if _, err := port.ApplySettlementEffects(context.Background(), request); !errors.Is(err, ErrSettlementEffectConflict) {
		t.Fatalf("conflicting effect error = %v", err)
	}
}

func TestResultImportOutboxPayloadRoundTrips(t *testing.T) {
	instruction := paymentInstruction()
	input := settlementInputFor(instruction, "result-payload", ResultStatusSettled, "125.50")
	messageInput, err := NewPaymentResultImportOutboxInput(input, input.Correlation, 303)
	if err != nil {
		t.Fatal(err)
	}
	var decoded SettlementResultInput
	if err := json.Unmarshal(messageInput.Payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.resultID() != input.resultID() || messageInput.IdempotencyKey != input.resultID() {
		t.Fatalf("message input = %#v decoded = %#v", messageInput, decoded)
	}
}
