// Package payments defines provider-neutral supplier payment execution contracts.
package payments

import (
	"context"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

type Instruction struct {
	Reference         automation.ExternalReference
	Correlation       automation.Correlation
	BeneficiaryRef    string
	BeneficiaryName   string
	Amount            automation.ExactAmount
	ScheduledFor      time.Time
	EndToEndReference string
}

type Submission struct {
	Reference  automation.ExternalReference
	Status     string
	OccurredAt time.Time
}

type Settlement struct {
	Reference         automation.ExternalReference
	Instruction       automation.ExternalReference
	Status            string
	SettledAmount     automation.ExactAmount
	SettledAt         time.Time
	ProviderFee       automation.ExactAmount
	EndToEndReference string
}

type ExportArtifact struct {
	Reference automation.ExternalReference
	Checksum  string
	CreatedAt time.Time
}

// ExecutionPort is implemented by a provider adapter or a controlled bank
// file adapter. Ambiguous Submit results must be resolved through Lookup before
// a caller sends the same instruction again.
type ExecutionPort interface {
	ValidateConnection(context.Context, automation.ConnectionRef) error
	Submit(context.Context, automation.ConnectionRef, Instruction) (Submission, error)
	Lookup(context.Context, automation.ConnectionRef, automation.ExternalReference) (Settlement, error)
	Cancel(context.Context, automation.ConnectionRef, automation.ExternalReference) (Settlement, error)
	GenerateFile(context.Context, automation.ConnectionRef, []Instruction) (ExportArtifact, error)
}

// InstructionReferenceLookup is an optional extension for providers whose
// durable submission reference differs from the local instruction reference.
// It lets a coordinator resolve a persisted execution after a process restart
// without relying on an in-memory original-to-provider mapping.
type InstructionReferenceLookup interface {
	LookupWithInstruction(context.Context, automation.ConnectionRef, automation.ExternalReference, automation.ExternalReference) (Settlement, error)
}

// InstructionReferenceCanceller is the durable-reference cancellation
// counterpart to InstructionReferenceLookup.
type InstructionReferenceCanceller interface {
	CancelWithInstruction(context.Context, automation.ConnectionRef, automation.ExternalReference, automation.ExternalReference) (Settlement, error)
}
