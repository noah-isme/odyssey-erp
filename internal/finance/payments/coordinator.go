package payments

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

// ExecutionState is the local, provider-neutral state of an instruction.
// Provider statuses remain on Submission and Settlement; callers should use
// this state for local workflow decisions.
type ExecutionState string

const (
	StateProposed         ExecutionState = "PROPOSED"
	StateApproved         ExecutionState = "APPROVED"
	StateSubmitted        ExecutionState = "SUBMITTED"
	StateExported         ExecutionState = "EXPORTED"
	StateAmbiguous        ExecutionState = "AMBIGUOUS"
	StatePartiallySettled ExecutionState = "PARTIALLY_SETTLED"
	StateSettled          ExecutionState = "SETTLED"
	StateCancelled        ExecutionState = "CANCELLED"
	StateFailed           ExecutionState = "FAILED"
)

// State is kept as a short alias for consumers that prefer the generic name.
type State = ExecutionState

const (
	SubmissionStatusSubmitted = "SUBMITTED"
	SettlementStatusPending   = "PENDING"
	SettlementStatusSettled   = "SETTLED"
	SettlementStatusPartial   = "PARTIALLY_SETTLED"
	SettlementStatusCancelled = "CANCELLED"
	SettlementStatusFailed    = "FAILED"
)

// Action identifies an actor-controlled coordinator operation.
type Action string

const (
	ActionPropose Action = "PROPOSE"
	ActionApprove Action = "APPROVE"
	ActionSubmit  Action = "SUBMIT"
	ActionExport  Action = "EXPORT"
	ActionCancel  Action = "CANCEL"
	ActionSettle  Action = "SETTLE"
)

var (
	ErrInvalidCoordinator     = errors.New("finance payments: invalid coordinator")
	ErrInvalidInstruction     = errors.New("finance payments: invalid instruction")
	ErrExecutionNotFound      = errors.New("finance payments: execution not found")
	ErrInstructionConflict    = errors.New("finance payments: instruction reference already has a different instruction")
	ErrInvalidTransition      = errors.New("finance payments: invalid state transition")
	ErrUnauthorized           = errors.New("finance payments: actor is not authorized")
	ErrAmbiguousSubmission    = errors.New("finance payments: submission result is ambiguous")
	ErrLookupRequired         = errors.New("finance payments: provider lookup is required")
	ErrInvalidProviderResult  = errors.New("finance payments: invalid provider result")
	ErrCancellationPending    = errors.New("finance payments: cancellation is pending provider confirmation")
	ErrInvalidExportArtifact  = errors.New("finance payments: invalid export artifact")
	ErrDuplicateExportBatch   = errors.New("finance payments: export batch contains a duplicate instruction")
	ErrMixedExportConnections = errors.New("finance payments: export batch contains multiple connections")
)

// ErrSeparationOfDuties and ErrIncompatiblePaymentDuties retain the same
// sentinel as the shared automation policy so callers can use errors.Is with
// either package boundary.
var (
	ErrSeparationOfDuties        = automation.ErrIncompatiblePaymentDuties
	ErrIncompatiblePaymentDuties = automation.ErrIncompatiblePaymentDuties
)

// StateTransition records an append-only local state change. A durable store
// can persist these transitions alongside the current execution snapshot.
type StateTransition struct {
	From   ExecutionState
	To     ExecutionState
	At     time.Time
	Reason string
}

// PaymentExecution is the coordinator's provider-neutral execution record.
// Instruction.Reference is the idempotency identity for the entire record.
type PaymentExecution struct {
	Instruction    Instruction
	State          ExecutionState
	Submission     *Submission
	Settlement     *Settlement
	ExportArtifact *ExportArtifact
	ProposedBy     int64
	ApprovedBy     int64
	ExecutorID     int64
	CancelledBy    int64
	Transitions    []StateTransition
	CreatedAt      time.Time
	UpdatedAt      time.Time
	// Version is the optimistic-concurrency version maintained by a durable
	// execution store. A zero version represents a new execution.
	Version int64
}

// Execution is a convenient name for PaymentExecution.
type Execution = PaymentExecution

// FileExport is the result of generating one controlled bank file. The
// artifact is shared by every execution in the batch.
type FileExport struct {
	Artifact   ExportArtifact
	Executions []PaymentExecution
}

// Authorizer is intentionally broader than a maker/checker check. A treasury
// integration can add RBAC, company policy, or audit requirements without
// changing the coordinator's workflow.
type Authorizer interface {
	Authorize(context.Context, Action, PaymentExecution, int64) error
}

// AuthorizerFunc adapts a function to Authorizer.
type AuthorizerFunc func(context.Context, Action, PaymentExecution, int64) error

func (f AuthorizerFunc) Authorize(ctx context.Context, action Action, execution PaymentExecution, actorID int64) error {
	if f == nil {
		return ErrUnauthorized
	}
	return f(ctx, action, execution, actorID)
}

// SeparationAuthorizer enforces the default maker/checker/executor matrix.
// SettingsForCompany is optional; when it is absent, a zero CompanyID uses
// automation.DefaultSettings for the instruction's company.
type SeparationAuthorizer struct {
	Settings           automation.Settings
	SettingsForCompany func(context.Context, int64) (automation.Settings, error)
}

func NewSeparationAuthorizer(settings automation.Settings) SeparationAuthorizer {
	return SeparationAuthorizer{Settings: settings}
}

func (a SeparationAuthorizer) Authorize(ctx context.Context, action Action, execution PaymentExecution, actorID int64) error {
	if actorID <= 0 {
		return ErrUnauthorized
	}
	companyID := execution.Instruction.Reference.Connection.CompanyID
	settings, err := a.settings(ctx, companyID)
	if err != nil {
		return err
	}

	switch action {
	case ActionPropose, ActionCancel, ActionSettle:
		return nil
	case ActionApprove:
		if settings.PaymentMakerCheckerEnabled && execution.ProposedBy == actorID {
			return automation.ErrIncompatiblePaymentDuties
		}
		return nil
	case ActionSubmit, ActionExport:
		if execution.ProposedBy <= 0 || execution.ApprovedBy <= 0 {
			return automation.ErrIncompatiblePaymentDuties
		}
		return automation.ValidatePaymentDutySeparation(settings, execution.ProposedBy, execution.ApprovedBy, actorID)
	default:
		return fmt.Errorf("%w: unknown action %q", ErrUnauthorized, action)
	}
}

func (a SeparationAuthorizer) settings(ctx context.Context, companyID int64) (automation.Settings, error) {
	if companyID <= 0 {
		return automation.Settings{}, ErrInvalidInstruction
	}
	if a.SettingsForCompany != nil {
		return a.SettingsForCompany(ctx, companyID)
	}
	if a.Settings.CompanyID == 0 {
		return automation.DefaultSettings(companyID), nil
	}
	if a.Settings.CompanyID != companyID {
		return automation.Settings{}, fmt.Errorf("%w: settings company mismatch", ErrUnauthorized)
	}
	return a.Settings, nil
}

// ExecutionStore is the persistence seam for the coordinator. Implementations
// should key Get and Save by the complete ExternalReference and make Save
// durable before returning. The coordinator serializes calls on one instance;
// a database implementation should still enforce a unique reference.
type ExecutionStore interface {
	Get(context.Context, automation.ExternalReference) (PaymentExecution, error)
	Save(context.Context, PaymentExecution) error
}

// Store is an alias that reads naturally in constructor signatures.
type Store = ExecutionStore

// MemoryStore is useful for unit tests and small synchronous callers. Treasury
// should replace it with a company-scoped durable implementation.
type MemoryStore struct {
	mu         sync.RWMutex
	executions map[automation.ExternalReference]PaymentExecution
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{executions: make(map[automation.ExternalReference]PaymentExecution)}
}

func (s *MemoryStore) Get(_ context.Context, reference automation.ExternalReference) (PaymentExecution, error) {
	if s == nil {
		return PaymentExecution{}, ErrExecutionNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	execution, ok := s.executions[reference]
	if !ok {
		return PaymentExecution{}, ErrExecutionNotFound
	}
	return cloneExecution(execution), nil
}

func (s *MemoryStore) Save(_ context.Context, execution PaymentExecution) error {
	if s == nil {
		return ErrInvalidCoordinator
	}
	if err := execution.Instruction.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.executions == nil {
		s.executions = make(map[automation.ExternalReference]PaymentExecution)
	}
	s.executions[execution.Instruction.Reference] = cloneExecution(execution)
	return nil
}

// Coordinator owns the workflow and never calls Submit for an instruction
// whose prior outcome is ambiguous. It is safe to use with a durable store
// behind an outbox worker; the in-memory mutex only serializes one instance.
type Coordinator struct {
	port       ExecutionPort
	store      ExecutionStore
	authorizer Authorizer
	mu         sync.Mutex
}

func NewCoordinator(port ExecutionPort, store ExecutionStore, authorizer Authorizer) *Coordinator {
	if store == nil {
		store = NewMemoryStore()
	}
	if authorizer == nil {
		authorizer = SeparationAuthorizer{}
	}
	return &Coordinator{port: port, store: store, authorizer: authorizer}
}

// NewService is an alias for integrations that use service naming elsewhere.
func NewService(port ExecutionPort, store ExecutionStore, authorizer Authorizer) *Coordinator {
	return NewCoordinator(port, store, authorizer)
}

func (c *Coordinator) Get(ctx context.Context, reference automation.ExternalReference) (PaymentExecution, error) {
	if c == nil || c.store == nil {
		return PaymentExecution{}, ErrInvalidCoordinator
	}
	if err := reference.Validate(); err != nil {
		return PaymentExecution{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.store.Get(ctx, reference)
}

// Propose creates the idempotent proposal record. Repeating a proposal with
// the same instruction reference returns the original record; changing a
// material instruction field under that reference is rejected.
func (c *Coordinator) Propose(ctx context.Context, instruction Instruction, proposerID int64) (PaymentExecution, error) {
	if c == nil || c.store == nil || c.authorizer == nil {
		return PaymentExecution{}, ErrInvalidCoordinator
	}
	if err := instruction.Validate(); err != nil {
		return PaymentExecution{}, err
	}
	if proposerID <= 0 {
		return PaymentExecution{}, ErrUnauthorized
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	existing, err := c.store.Get(ctx, instruction.Reference)
	if err == nil {
		if !sameInstruction(existing.Instruction, instruction) {
			return PaymentExecution{}, ErrInstructionConflict
		}
		return existing, nil
	}
	if !errors.Is(err, ErrExecutionNotFound) {
		return PaymentExecution{}, err
	}

	execution := PaymentExecution{
		Instruction: instruction,
		State:       StateProposed,
		ProposedBy:  proposerID,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := c.authorizer.Authorize(ctx, ActionPropose, execution, proposerID); err != nil {
		return PaymentExecution{}, err
	}
	if err := c.store.Save(ctx, execution); err != nil {
		return PaymentExecution{}, err
	}
	return execution, nil
}

// Approve moves a proposal to APPROVED. It is deliberately separate from
// Submit so the checker identity is present before execution authorization.
func (c *Coordinator) Approve(ctx context.Context, reference automation.ExternalReference, approverID int64) (PaymentExecution, error) {
	if c == nil || c.store == nil || c.authorizer == nil {
		return PaymentExecution{}, ErrInvalidCoordinator
	}
	if err := reference.Validate(); err != nil {
		return PaymentExecution{}, err
	}
	if approverID <= 0 {
		return PaymentExecution{}, ErrUnauthorized
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	execution, err := c.store.Get(ctx, reference)
	if err != nil {
		return PaymentExecution{}, err
	}
	if execution.State == StateApproved {
		return execution, nil
	}
	if execution.State != StateProposed {
		return PaymentExecution{}, invalidTransition(execution.State, StateApproved)
	}
	if err := c.authorizer.Authorize(ctx, ActionApprove, execution, approverID); err != nil {
		return PaymentExecution{}, err
	}
	execution.ApprovedBy = approverID
	if err := transition(&execution, StateApproved, time.Now().UTC(), "proposal approved"); err != nil {
		return PaymentExecution{}, err
	}
	if err := c.store.Save(ctx, execution); err != nil {
		return PaymentExecution{}, err
	}
	return execution, nil
}

// Submit submits an approved instruction. A repeated call after SUBMITTED is
// idempotent. A repeated call after AMBIGUOUS performs Lookup first and never
// invokes the provider Submit method until an operator has resolved the state.
func (c *Coordinator) Submit(ctx context.Context, reference automation.ExternalReference, executorID int64) (PaymentExecution, error) {
	if c == nil || c.store == nil || c.authorizer == nil || c.port == nil {
		return PaymentExecution{}, ErrInvalidCoordinator
	}
	if err := reference.Validate(); err != nil {
		return PaymentExecution{}, err
	}
	if executorID <= 0 {
		return PaymentExecution{}, ErrUnauthorized
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	execution, err := c.store.Get(ctx, reference)
	if err != nil {
		return PaymentExecution{}, err
	}
	switch execution.State {
	case StateSubmitted, StateSettled, StatePartiallySettled, StateCancelled, StateFailed, StateExported:
		return execution, nil
	case StateAmbiguous:
		if err := c.authorizer.Authorize(ctx, ActionSubmit, execution, executorID); err != nil {
			return PaymentExecution{}, err
		}
		if execution.ExecutorID == 0 {
			execution.ExecutorID = executorID
		}
		return c.lookupLocked(ctx, execution)
	case StateApproved:
		// Continue below.
	default:
		return PaymentExecution{}, invalidTransition(execution.State, StateSubmitted)
	}

	if err := c.authorizer.Authorize(ctx, ActionSubmit, execution, executorID); err != nil {
		return PaymentExecution{}, err
	}
	if err := c.port.ValidateConnection(ctx, reference.Connection); err != nil {
		return PaymentExecution{}, err
	}
	submission, err := c.port.Submit(ctx, reference.Connection, execution.Instruction)
	if err != nil {
		if isAmbiguousProviderError(err) {
			execution.ExecutorID = executorID
			if transitionErr := transition(&execution, StateAmbiguous, time.Now().UTC(), "provider submit outcome is ambiguous"); transitionErr != nil {
				return PaymentExecution{}, transitionErr
			}
			if saveErr := c.store.Save(ctx, execution); saveErr != nil {
				return PaymentExecution{}, saveErr
			}
			return execution, fmt.Errorf("%w: %w: %w", ErrAmbiguousSubmission, automation.ErrAmbiguousOutcome, err)
		}
		return PaymentExecution{}, err
	}

	submission = normalizeSubmission(submission, reference)
	execution.Submission = &submission
	execution.ExecutorID = executorID
	if err := transition(&execution, StateSubmitted, time.Now().UTC(), "provider accepted submission"); err != nil {
		return PaymentExecution{}, err
	}
	if err := c.store.Save(ctx, execution); err != nil {
		return PaymentExecution{}, err
	}
	return execution, nil
}

// ResolveAmbiguity performs the mandatory provider lookup for an ambiguous
// submission. A pending lookup result clears AMBIGUOUS to SUBMITTED; a
// settlement result advances to the corresponding settlement state.
func (c *Coordinator) ResolveAmbiguity(ctx context.Context, reference automation.ExternalReference) (PaymentExecution, error) {
	if c == nil || c.store == nil || c.port == nil {
		return PaymentExecution{}, ErrInvalidCoordinator
	}
	if err := reference.Validate(); err != nil {
		return PaymentExecution{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	execution, err := c.store.Get(ctx, reference)
	if err != nil {
		return PaymentExecution{}, err
	}
	if execution.State != StateAmbiguous {
		if execution.State == StateSettled || execution.State == StateCancelled || execution.State == StateFailed {
			return execution, nil
		}
		return PaymentExecution{}, invalidTransition(execution.State, StateSubmitted)
	}
	return c.lookupLocked(ctx, execution)
}

// Resolve is a shorter alias for ResolveAmbiguity.
func (c *Coordinator) Resolve(ctx context.Context, reference automation.ExternalReference) (PaymentExecution, error) {
	return c.ResolveAmbiguity(ctx, reference)
}

// Settle looks up the authoritative provider state and records it. It is safe
// to call repeatedly after settlement because terminal states are returned
// without another provider lookup.
func (c *Coordinator) Settle(ctx context.Context, reference automation.ExternalReference) (PaymentExecution, error) {
	if c == nil || c.store == nil || c.port == nil {
		return PaymentExecution{}, ErrInvalidCoordinator
	}
	if err := reference.Validate(); err != nil {
		return PaymentExecution{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	execution, err := c.store.Get(ctx, reference)
	if err != nil {
		return PaymentExecution{}, err
	}
	switch execution.State {
	case StateSettled, StateCancelled, StateFailed:
		return execution, nil
	case StateSubmitted, StateAmbiguous, StateExported, StatePartiallySettled:
		return c.lookupLocked(ctx, execution)
	default:
		return PaymentExecution{}, invalidTransition(execution.State, StateSettled)
	}
}

// Cancel cancels a local proposal before provider execution, or requests
// cancellation for an already submitted instruction. Provider confirmation
// is required for the latter path.
func (c *Coordinator) Cancel(ctx context.Context, reference automation.ExternalReference, actorID int64) (PaymentExecution, error) {
	if c == nil || c.store == nil || c.authorizer == nil {
		return PaymentExecution{}, ErrInvalidCoordinator
	}
	if err := reference.Validate(); err != nil {
		return PaymentExecution{}, err
	}
	if actorID <= 0 {
		return PaymentExecution{}, ErrUnauthorized
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	execution, err := c.store.Get(ctx, reference)
	if err != nil {
		return PaymentExecution{}, err
	}
	if execution.State == StateCancelled {
		return execution, nil
	}
	if execution.State == StateSettled || execution.State == StatePartiallySettled || execution.State == StateFailed {
		return PaymentExecution{}, invalidTransition(execution.State, StateCancelled)
	}
	if err := c.authorizer.Authorize(ctx, ActionCancel, execution, actorID); err != nil {
		return PaymentExecution{}, err
	}

	if execution.State == StateProposed || execution.State == StateApproved || execution.State == StateExported {
		execution.CancelledBy = actorID
		if err := transition(&execution, StateCancelled, time.Now().UTC(), "cancelled locally"); err != nil {
			return PaymentExecution{}, err
		}
		if err := c.store.Save(ctx, execution); err != nil {
			return PaymentExecution{}, err
		}
		return execution, nil
	}
	if execution.State != StateSubmitted && execution.State != StateAmbiguous {
		return PaymentExecution{}, invalidTransition(execution.State, StateCancelled)
	}
	if c.port == nil {
		return PaymentExecution{}, ErrInvalidCoordinator
	}
	if err := c.port.ValidateConnection(ctx, reference.Connection); err != nil {
		return PaymentExecution{}, err
	}
	var settlement Settlement
	if execution.Submission != nil && !isZeroReference(execution.Submission.Reference) {
		if port, ok := c.port.(InstructionReferenceCanceller); ok {
			settlement, err = port.CancelWithInstruction(ctx, reference.Connection, execution.Instruction.Reference, execution.Submission.Reference)
		} else {
			settlement, err = c.port.Cancel(ctx, reference.Connection, reference)
		}
	} else {
		settlement, err = c.port.Cancel(ctx, reference.Connection, reference)
	}
	if err != nil {
		return PaymentExecution{}, err
	}
	settlement, next, err := normalizeSettlement(execution.Instruction, settlement)
	if err != nil {
		return PaymentExecution{}, err
	}
	if next != StateCancelled {
		if next == StateSubmitted || next == StateAmbiguous {
			execution.Settlement = &settlement
			if saveErr := c.store.Save(ctx, execution); saveErr != nil {
				return PaymentExecution{}, saveErr
			}
			return execution, ErrCancellationPending
		}
		return PaymentExecution{}, invalidTransition(execution.State, next)
	}
	execution.Settlement = &settlement
	execution.CancelledBy = actorID
	if err := transition(&execution, StateCancelled, time.Now().UTC(), "provider confirmed cancellation"); err != nil {
		return PaymentExecution{}, err
	}
	if err := c.store.Save(ctx, execution); err != nil {
		return PaymentExecution{}, err
	}
	return execution, nil
}

// ExportFile generates a controlled bank file for one approved instruction.
// It is the single-instruction convenience form of ExportBatch.
func (c *Coordinator) ExportFile(ctx context.Context, reference automation.ExternalReference, executorID int64) (ExportArtifact, error) {
	result, err := c.ExportBatch(ctx, []automation.ExternalReference{reference}, executorID)
	if err != nil {
		return ExportArtifact{}, err
	}
	return result.Artifact, nil
}

// GenerateFile is an alias for ExportFile.
func (c *Coordinator) GenerateFile(ctx context.Context, reference automation.ExternalReference, executorID int64) (ExportArtifact, error) {
	return c.ExportFile(ctx, reference, executorID)
}

// ExportBatch generates one file for a set of approved instructions. All
// references must belong to the same connection; every instruction is moved
// to EXPORTED only after the provider returns a checksummed artifact.
func (c *Coordinator) ExportBatch(ctx context.Context, references []automation.ExternalReference, executorID int64) (FileExport, error) {
	if c == nil || c.store == nil || c.authorizer == nil || c.port == nil {
		return FileExport{}, ErrInvalidCoordinator
	}
	if len(references) == 0 {
		return FileExport{}, ErrInvalidInstruction
	}
	if executorID <= 0 {
		return FileExport{}, ErrUnauthorized
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	executions := make([]PaymentExecution, 0, len(references))
	seen := make(map[automation.ExternalReference]struct{}, len(references))
	for _, reference := range references {
		if err := reference.Validate(); err != nil {
			return FileExport{}, err
		}
		if _, ok := seen[reference]; ok {
			return FileExport{}, ErrDuplicateExportBatch
		}
		seen[reference] = struct{}{}
		execution, err := c.store.Get(ctx, reference)
		if err != nil {
			return FileExport{}, err
		}
		executions = append(executions, execution)
	}

	if allExported(executions) {
		artifact := *executions[0].ExportArtifact
		return FileExport{Artifact: artifact, Executions: executions}, nil
	}
	connection := executions[0].Instruction.Reference.Connection
	for _, execution := range executions {
		if execution.Instruction.Reference.Connection != connection {
			return FileExport{}, ErrMixedExportConnections
		}
		if execution.State != StateApproved {
			return FileExport{}, invalidTransition(execution.State, StateExported)
		}
		if err := c.authorizer.Authorize(ctx, ActionExport, execution, executorID); err != nil {
			return FileExport{}, err
		}
	}
	if err := c.port.ValidateConnection(ctx, connection); err != nil {
		return FileExport{}, err
	}
	instructions := make([]Instruction, 0, len(executions))
	for _, execution := range executions {
		instructions = append(instructions, execution.Instruction)
	}
	artifact, err := c.port.GenerateFile(ctx, connection, instructions)
	if err != nil {
		return FileExport{}, err
	}
	artifact = normalizeArtifact(artifact, executions[0].Instruction.Reference, time.Now().UTC())
	if strings.TrimSpace(artifact.Checksum) == "" {
		return FileExport{}, ErrInvalidExportArtifact
	}

	for i := range executions {
		executions[i].ExportArtifact = &artifact
		executions[i].ExecutorID = executorID
		if err := transition(&executions[i], StateExported, time.Now().UTC(), "controlled bank file generated"); err != nil {
			return FileExport{}, err
		}
		if err := c.store.Save(ctx, executions[i]); err != nil {
			return FileExport{}, err
		}
	}
	return FileExport{Artifact: artifact, Executions: executions}, nil
}

func (c *Coordinator) lookupLocked(ctx context.Context, execution PaymentExecution) (PaymentExecution, error) {
	if err := c.port.ValidateConnection(ctx, execution.Instruction.Reference.Connection); err != nil {
		return execution, err
	}
	var (
		settlement Settlement
		err        error
	)
	if execution.Submission != nil && !isZeroReference(execution.Submission.Reference) {
		if port, ok := c.port.(InstructionReferenceLookup); ok {
			settlement, err = port.LookupWithInstruction(ctx, execution.Instruction.Reference.Connection, execution.Instruction.Reference, execution.Submission.Reference)
		} else {
			settlement, err = c.port.Lookup(ctx, execution.Instruction.Reference.Connection, execution.Instruction.Reference)
		}
	} else {
		settlement, err = c.port.Lookup(ctx, execution.Instruction.Reference.Connection, execution.Instruction.Reference)
	}
	if err != nil {
		return execution, fmt.Errorf("%w: %w", ErrLookupRequired, err)
	}
	settlement, next, err := normalizeSettlement(execution.Instruction, settlement)
	if err != nil {
		return execution, err
	}
	if next == StateSubmitted && (execution.State == StateExported || execution.State == StatePartiallySettled) {
		next = execution.State
	}
	execution.Settlement = &settlement
	if next != execution.State {
		if err := transition(&execution, next, time.Now().UTC(), "provider lookup resolved execution state"); err != nil {
			return execution, err
		}
	}
	if err := c.store.Save(ctx, execution); err != nil {
		return execution, err
	}
	return execution, nil
}

func (i Instruction) Validate() error {
	if err := i.Reference.Validate(); err != nil {
		return fmt.Errorf("%w: reference: %v", ErrInvalidInstruction, err)
	}
	if err := i.Correlation.Validate(); err != nil {
		return fmt.Errorf("%w: correlation: %v", ErrInvalidInstruction, err)
	}
	if err := i.Amount.Validate(); err != nil {
		return fmt.Errorf("%w: amount: %v", ErrInvalidInstruction, err)
	}
	if !i.Amount.IsPositive() {
		return fmt.Errorf("%w: amount must be positive", ErrInvalidInstruction)
	}
	if strings.TrimSpace(i.BeneficiaryRef) == "" {
		return fmt.Errorf("%w: beneficiary reference required", ErrInvalidInstruction)
	}
	return nil
}

func normalizeSubmission(submission Submission, instructionReference automation.ExternalReference) Submission {
	if isZeroReference(submission.Reference) {
		submission.Reference = instructionReference
	}
	if strings.TrimSpace(submission.Status) == "" {
		submission.Status = SubmissionStatusSubmitted
	}
	if submission.OccurredAt.IsZero() {
		submission.OccurredAt = time.Now().UTC()
	}
	return submission
}

func normalizeSettlement(instruction Instruction, settlement Settlement) (Settlement, ExecutionState, error) {
	if isZeroReference(settlement.Instruction) {
		settlement.Instruction = instruction.Reference
	}
	if settlement.Instruction != instruction.Reference {
		return Settlement{}, "", fmt.Errorf("%w: settlement instruction reference mismatch", ErrInvalidProviderResult)
	}
	if !isZeroReference(settlement.Reference) {
		if err := settlement.Reference.Validate(); err != nil {
			return Settlement{}, "", fmt.Errorf("%w: settlement reference: %v", ErrInvalidProviderResult, err)
		}
	}
	if settlement.EndToEndReference != "" && instruction.EndToEndReference != "" && settlement.EndToEndReference != instruction.EndToEndReference {
		return Settlement{}, "", fmt.Errorf("%w: end-to-end reference mismatch", ErrInvalidProviderResult)
	}
	status := normalizeStatus(settlement.Status)
	if status == "" {
		return Settlement{}, "", fmt.Errorf("%w: settlement status required", ErrInvalidProviderResult)
	}
	settlement.Status = status

	switch status {
	case "PENDING", "PROCESSING", "SUBMITTED", "ACCEPTED", "INITIATED", "DUPLICATE":
		return settlement, StateSubmitted, nil
	case "CANCELLED":
		return settlement, StateCancelled, nil
	case "FAILED", "REJECTED", "DECLINED", "EXPIRED":
		return settlement, StateFailed, nil
	case "PARTIALLY_SETTLED", "PARTIAL":
		if err := validateSettlementAmount(instruction, &settlement, false); err != nil {
			return Settlement{}, "", err
		}
		return settlement, StatePartiallySettled, nil
	case "SETTLED", "PAID", "COMPLETED", "SUCCESS", "SUCCEEDED":
		if err := validateSettlementAmount(instruction, &settlement, true); err != nil {
			return Settlement{}, "", err
		}
		if settlement.SettledAmount.Amount.Cmp(instruction.Amount.Amount) < 0 {
			return settlement, StatePartiallySettled, nil
		}
		return settlement, StateSettled, nil
	default:
		return Settlement{}, "", fmt.Errorf("%w: unsupported settlement status %q", ErrInvalidProviderResult, settlement.Status)
	}
}

func validateSettlementAmount(instruction Instruction, settlement *Settlement, full bool) error {
	if isZeroExactAmount(settlement.SettledAmount) {
		if !full {
			return fmt.Errorf("%w: partial settlement amount required", ErrInvalidProviderResult)
		}
		settlement.SettledAmount = instruction.Amount
	}
	if err := settlement.SettledAmount.Validate(); err != nil {
		return fmt.Errorf("%w: settled amount: %v", ErrInvalidProviderResult, err)
	}
	if settlement.SettledAmount.Currency != instruction.Amount.Currency {
		return fmt.Errorf("%w: settlement currency mismatch", ErrInvalidProviderResult)
	}
	if !settlement.SettledAmount.IsPositive() {
		return fmt.Errorf("%w: settled amount must be positive", ErrInvalidProviderResult)
	}
	if settlement.SettledAmount.Amount.Cmp(instruction.Amount.Amount) > 0 {
		return fmt.Errorf("%w: settled amount exceeds instruction amount", ErrInvalidProviderResult)
	}
	if hasExactAmount(settlement.ProviderFee) {
		if err := settlement.ProviderFee.Validate(); err != nil {
			return fmt.Errorf("%w: provider fee: %v", ErrInvalidProviderResult, err)
		}
		if settlement.ProviderFee.Currency != instruction.Amount.Currency {
			return fmt.Errorf("%w: provider fee currency mismatch", ErrInvalidProviderResult)
		}
		zero := instruction.Amount.Amount.Sub(instruction.Amount.Amount)
		if settlement.ProviderFee.Amount.Cmp(zero) < 0 {
			return fmt.Errorf("%w: provider fee must not be negative", ErrInvalidProviderResult)
		}
	}
	if settlement.SettledAt.IsZero() {
		settlement.SettledAt = time.Now().UTC()
	}
	return nil
}

func transition(execution *PaymentExecution, next ExecutionState, at time.Time, reason string) error {
	if execution.State == next {
		return nil
	}
	if !allowedTransition(execution.State, next) {
		return invalidTransition(execution.State, next)
	}
	execution.Transitions = append(execution.Transitions, StateTransition{
		From:   execution.State,
		To:     next,
		At:     at,
		Reason: reason,
	})
	execution.State = next
	execution.UpdatedAt = at
	return nil
}

func allowedTransition(from, to ExecutionState) bool {
	switch from {
	case StateProposed:
		return to == StateApproved || to == StateCancelled
	case StateApproved:
		return to == StateSubmitted || to == StateExported || to == StateAmbiguous || to == StateCancelled
	case StateSubmitted:
		return to == StateAmbiguous || to == StatePartiallySettled || to == StateSettled || to == StateCancelled || to == StateFailed
	case StateExported:
		return to == StatePartiallySettled || to == StateSettled || to == StateCancelled || to == StateFailed
	case StateAmbiguous:
		return to == StateSubmitted || to == StatePartiallySettled || to == StateSettled || to == StateCancelled || to == StateFailed
	case StatePartiallySettled:
		return to == StateSettled
	default:
		return false
	}
}

func invalidTransition(from, to ExecutionState) error {
	return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
}

func normalizeStatus(status string) string {
	status = strings.ToUpper(strings.TrimSpace(status))
	status = strings.NewReplacer("-", "_", " ", "_").Replace(status)
	switch status {
	case "CANCELED":
		return "CANCELLED"
	case "PARTIAL_SETTLEMENT", "PARTIALLY_PAID":
		return "PARTIALLY_SETTLED"
	default:
		return status
	}
}

func isAmbiguousProviderError(err error) bool {
	var providerErr *automation.ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return false
	}
	return providerErr.Category == automation.ErrorAmbiguous || providerErr.Category == automation.ErrorDuplicate
}

func isZeroReference(reference automation.ExternalReference) bool {
	return reference == (automation.ExternalReference{})
}

func isZeroExactAmount(amount automation.ExactAmount) bool {
	return strings.TrimSpace(amount.Currency) == "" && strings.TrimSpace(amount.Amount.Amount) == ""
}

func hasExactAmount(amount automation.ExactAmount) bool {
	return !isZeroExactAmount(amount)
}

func sameInstruction(a, b Instruction) bool {
	return a.Reference == b.Reference &&
		a.BeneficiaryRef == b.BeneficiaryRef &&
		a.BeneficiaryName == b.BeneficiaryName &&
		a.Amount.Currency == b.Amount.Currency &&
		a.Amount.Amount.String() == b.Amount.Amount.String() &&
		a.Amount.Amount.Scale == b.Amount.Amount.Scale &&
		a.ScheduledFor.Equal(b.ScheduledFor) &&
		a.EndToEndReference == b.EndToEndReference
}

func allExported(executions []PaymentExecution) bool {
	if len(executions) == 0 {
		return false
	}
	for _, execution := range executions {
		if execution.State != StateExported || execution.ExportArtifact == nil {
			return false
		}
	}
	return true
}

func normalizeArtifact(artifact ExportArtifact, fallback automation.ExternalReference, now time.Time) ExportArtifact {
	if isZeroReference(artifact.Reference) {
		artifact.Reference = fallback
	}
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = now
	}
	return artifact
}

func cloneExecution(execution PaymentExecution) PaymentExecution {
	if execution.Submission != nil {
		submission := *execution.Submission
		execution.Submission = &submission
	}
	if execution.Settlement != nil {
		settlement := *execution.Settlement
		execution.Settlement = &settlement
	}
	if execution.ExportArtifact != nil {
		artifact := *execution.ExportArtifact
		execution.ExportArtifact = &artifact
	}
	if execution.Transitions != nil {
		execution.Transitions = append([]StateTransition(nil), execution.Transitions...)
	}
	return execution
}
