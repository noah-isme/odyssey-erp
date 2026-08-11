package payments

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

type coordinatorPort struct {
	submitResult Submission
	submitErr    error
	lookupResult Settlement
	lookupErr    error
	cancelResult Settlement
	cancelErr    error
	artifact     ExportArtifact
	fileErr      error

	validateCalls int
	submitCalls   int
	lookupCalls   int
	cancelCalls   int
	fileCalls     int
}

func (p *coordinatorPort) ValidateConnection(context.Context, automation.ConnectionRef) error {
	p.validateCalls++
	return nil
}

func (p *coordinatorPort) Submit(context.Context, automation.ConnectionRef, Instruction) (Submission, error) {
	p.submitCalls++
	return p.submitResult, p.submitErr
}

func (p *coordinatorPort) Lookup(context.Context, automation.ConnectionRef, automation.ExternalReference) (Settlement, error) {
	p.lookupCalls++
	return p.lookupResult, p.lookupErr
}

func (p *coordinatorPort) Cancel(context.Context, automation.ConnectionRef, automation.ExternalReference) (Settlement, error) {
	p.cancelCalls++
	return p.cancelResult, p.cancelErr
}

func (p *coordinatorPort) GenerateFile(context.Context, automation.ConnectionRef, []Instruction) (ExportArtifact, error) {
	p.fileCalls++
	return p.artifact, p.fileErr
}

func paymentInstruction() Instruction {
	connection := automation.ConnectionRef{CompanyID: 7, ConnectionID: 11, Provider: "bank-test"}
	return Instruction{
		Reference:         automation.ExternalReference{Connection: connection, ObjectType: "payment_instruction", ObjectID: "instruction-1"},
		Correlation:       automation.Correlation{ID: "corr-1"},
		BeneficiaryRef:    "supplier-1",
		BeneficiaryName:   "Supplier One",
		Amount:            automation.MustParseExact("125.50"),
		ScheduledFor:      time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC),
		EndToEndReference: "e2e-1",
	}
}

func providerReference() automation.ExternalReference {
	return automation.ExternalReference{
		Connection: automation.ConnectionRef{CompanyID: 7, ConnectionID: 11, Provider: "bank-test"},
		ObjectType: "provider_payment",
		ObjectID:   "provider-1",
	}
}

func settledResult() Settlement {
	instruction := paymentInstruction()
	return Settlement{
		Reference:         providerReference(),
		Instruction:       instruction.Reference,
		Status:            SettlementStatusSettled,
		SettledAmount:     instruction.Amount,
		SettledAt:         time.Date(2026, time.August, 12, 9, 5, 0, 0, time.UTC),
		EndToEndReference: instruction.EndToEndReference,
	}
}

func approvedCoordinator(t *testing.T, port ExecutionPort) (*Coordinator, Instruction) {
	t.Helper()
	coordinator := NewCoordinator(port, NewMemoryStore(), nil)
	instruction := paymentInstruction()
	if _, err := coordinator.Propose(context.Background(), instruction, 101); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Approve(context.Background(), instruction.Reference, 202); err != nil {
		t.Fatal(err)
	}
	return coordinator, instruction
}

func TestCoordinatorSubmitAndSettle(t *testing.T) {
	port := &coordinatorPort{
		submitResult: Submission{Reference: providerReference(), Status: "ACCEPTED"},
		lookupResult: settledResult(),
	}
	coordinator, instruction := approvedCoordinator(t, port)

	submitted, err := coordinator.Submit(context.Background(), instruction.Reference, 303)
	if err != nil {
		t.Fatal(err)
	}
	if submitted.State != StateSubmitted {
		t.Fatalf("submit state = %s, want %s", submitted.State, StateSubmitted)
	}

	settled, err := coordinator.Settle(context.Background(), instruction.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if settled.State != StateSettled {
		t.Fatalf("settle state = %s, want %s", settled.State, StateSettled)
	}
	if settled.Settlement == nil || settled.Settlement.SettledAmount.Amount.String() != "125.50" {
		t.Fatalf("settlement = %#v", settled.Settlement)
	}
	if len(settled.Transitions) != 3 {
		t.Fatalf("transitions = %#v, want proposal, submission, settlement", settled.Transitions)
	}
	if port.submitCalls != 1 || port.lookupCalls != 1 {
		t.Fatalf("submit calls = %d, lookup calls = %d", port.submitCalls, port.lookupCalls)
	}
}

func TestCoordinatorIsIdempotentByInstructionReference(t *testing.T) {
	port := &coordinatorPort{
		submitResult: Submission{Reference: providerReference(), Status: "SUBMITTED"},
	}
	coordinator, instruction := approvedCoordinator(t, port)

	first, err := coordinator.Submit(context.Background(), instruction.Reference, 303)
	if err != nil {
		t.Fatal(err)
	}
	second, err := coordinator.Submit(context.Background(), instruction.Reference, 404)
	if err != nil {
		t.Fatal(err)
	}
	if first.State != StateSubmitted || second.State != StateSubmitted {
		t.Fatalf("states = %s, %s", first.State, second.State)
	}
	if port.submitCalls != 1 {
		t.Fatalf("submit calls = %d, want 1", port.submitCalls)
	}

	duplicate, err := coordinator.Propose(context.Background(), instruction, 999)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.State != StateSubmitted {
		t.Fatalf("duplicate proposal state = %s, want existing submitted state", duplicate.State)
	}

	changed := instruction
	changed.Amount = automation.MustParseExact("125.51")
	if _, err := coordinator.Propose(context.Background(), changed, 101); !errors.Is(err, ErrInstructionConflict) {
		t.Fatalf("changed duplicate error = %v, want %v", err, ErrInstructionConflict)
	}
}

func TestCoordinatorResolvesAmbiguousSubmitThroughLookup(t *testing.T) {
	port := &coordinatorPort{
		submitErr:    &automation.ProviderError{Category: automation.ErrorAmbiguous, Operation: "submit", Message: "request timed out"},
		lookupResult: settledResult(),
	}
	coordinator, instruction := approvedCoordinator(t, port)

	ambiguous, err := coordinator.Submit(context.Background(), instruction.Reference, 303)
	if !errors.Is(err, ErrAmbiguousSubmission) {
		t.Fatalf("ambiguous error = %v", err)
	}
	if ambiguous.State != StateAmbiguous {
		t.Fatalf("ambiguous state = %s, want %s", ambiguous.State, StateAmbiguous)
	}

	recovered, err := coordinator.Submit(context.Background(), instruction.Reference, 303)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State != StateSettled {
		t.Fatalf("recovered state = %s, want %s", recovered.State, StateSettled)
	}
	if port.submitCalls != 1 || port.lookupCalls != 1 {
		t.Fatalf("submit calls = %d, lookup calls = %d; ambiguous result was resubmitted", port.submitCalls, port.lookupCalls)
	}
}

func TestCoordinatorCancelsSubmittedInstruction(t *testing.T) {
	instruction := paymentInstruction()
	port := &coordinatorPort{
		submitResult: Submission{Reference: providerReference(), Status: "ACCEPTED"},
		cancelResult: Settlement{Reference: providerReference(), Instruction: instruction.Reference, Status: "CANCELED"},
	}
	coordinator, _ := approvedCoordinator(t, port)
	if _, err := coordinator.Submit(context.Background(), instruction.Reference, 303); err != nil {
		t.Fatal(err)
	}

	cancelled, err := coordinator.Cancel(context.Background(), instruction.Reference, 303)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.State != StateCancelled {
		t.Fatalf("cancel state = %s, want %s", cancelled.State, StateCancelled)
	}
	if _, err := coordinator.Cancel(context.Background(), instruction.Reference, 404); err != nil {
		t.Fatal(err)
	}
	if port.cancelCalls != 1 {
		t.Fatalf("cancel calls = %d, want 1", port.cancelCalls)
	}
}

func TestCoordinatorExportsControlledBankFile(t *testing.T) {
	port := &coordinatorPort{
		artifact: ExportArtifact{
			Reference: automation.ExternalReference{
				Connection: automation.ConnectionRef{CompanyID: 7, ConnectionID: 11, Provider: "bank-test"},
				ObjectType: "bank_file",
				ObjectID:   "file-1",
			},
			Checksum: "sha256:file-1",
		},
	}
	coordinator, instruction := approvedCoordinator(t, port)

	artifact, err := coordinator.ExportFile(context.Background(), instruction.Reference, 303)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Checksum != "sha256:file-1" {
		t.Fatalf("artifact = %#v", artifact)
	}
	execution, err := coordinator.Get(context.Background(), instruction.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if execution.State != StateExported {
		t.Fatalf("export state = %s, want %s", execution.State, StateExported)
	}
	if port.submitCalls != 0 || port.fileCalls != 1 {
		t.Fatalf("submit calls = %d, file calls = %d", port.submitCalls, port.fileCalls)
	}
	if _, err := coordinator.ExportFile(context.Background(), instruction.Reference, 404); err != nil {
		t.Fatal(err)
	}
	if port.fileCalls != 1 {
		t.Fatalf("duplicate export file calls = %d, want 1", port.fileCalls)
	}
}

func TestCoordinatorRejectsSeparationOfDutiesViolations(t *testing.T) {
	port := &coordinatorPort{submitResult: Submission{Status: "ACCEPTED"}}
	coordinator := NewCoordinator(port, NewMemoryStore(), nil)
	instruction := paymentInstruction()
	if _, err := coordinator.Propose(context.Background(), instruction, 101); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Approve(context.Background(), instruction.Reference, 101); !errors.Is(err, ErrSeparationOfDuties) {
		t.Fatalf("same maker/checker error = %v", err)
	}
	if _, err := coordinator.Approve(context.Background(), instruction.Reference, 202); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Submit(context.Background(), instruction.Reference, 101); !errors.Is(err, ErrSeparationOfDuties) {
		t.Fatalf("maker as executor error = %v", err)
	}
	if _, err := coordinator.Submit(context.Background(), instruction.Reference, 202); !errors.Is(err, ErrSeparationOfDuties) {
		t.Fatalf("checker as executor error = %v", err)
	}
	if port.submitCalls != 0 {
		t.Fatalf("submit calls = %d after separation failures", port.submitCalls)
	}
}
