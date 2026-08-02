package payments

import (
	"context"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

var _ ExecutionPort = fakeExecutionPort{}

type fakeExecutionPort struct{}

func (fakeExecutionPort) ValidateConnection(context.Context, automation.ConnectionRef) error {
	return nil
}
func (fakeExecutionPort) Submit(context.Context, automation.ConnectionRef, Instruction) (Submission, error) {
	return Submission{}, nil
}
func (fakeExecutionPort) Lookup(context.Context, automation.ConnectionRef, automation.ExternalReference) (Settlement, error) {
	return Settlement{}, nil
}
func (fakeExecutionPort) Cancel(context.Context, automation.ConnectionRef, automation.ExternalReference) (Settlement, error) {
	return Settlement{}, nil
}
func (fakeExecutionPort) GenerateFile(context.Context, automation.ConnectionRef, []Instruction) (ExportArtifact, error) {
	return ExportArtifact{}, nil
}
