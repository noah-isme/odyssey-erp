package payments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

// Settlement result statuses accepted at the controlled bank-file result
// boundary. Pending/provider-processing statuses are deliberately excluded:
// they are observations, not confirmed accounting events.
const (
	ResultStatusPartial   = SettlementStatusPartial
	ResultStatusSettled   = SettlementStatusSettled
	ResultStatusFailed    = SettlementStatusFailed
	ResultStatusCancelled = SettlementStatusCancelled
)

var (
	ErrInvalidSettlementResult              = errors.New("finance payments: invalid settlement result")
	ErrSettlementResultNotFound             = errors.New("finance payments: settlement result not found")
	ErrDuplicateSettlementResult            = errors.New("finance payments: settlement result already recorded")
	ErrSettlementResultConflict             = errors.New("finance payments: settlement result reference has different content")
	ErrSettlementResultCompanyMismatch      = errors.New("finance payments: settlement result company mismatch")
	ErrSettlementResultReferenceMismatch    = errors.New("finance payments: settlement result reference mismatch")
	ErrUnsupportedSettlementResult          = errors.New("finance payments: unsupported settlement result status")
	ErrSettlementEffectConflict             = errors.New("finance payments: settlement effect key has different content")
	ErrSettlementEffectNotApplied           = errors.New("finance payments: settlement effects were not applied")
	ErrSettlementEffectsTransactionRequired = errors.New("finance payments: settlement effects require a transaction-capable database")
)

// Common aliases make the boundary readable to callers that call the input a
// provider result or an imported result.
var (
	ErrInvalidResultInput = ErrInvalidSettlementResult
	ErrDuplicateResult    = ErrDuplicateSettlementResult
	ErrResultConflict     = ErrSettlementResultConflict
)

// SettlementResultInput is the only input accepted from a bank-file result
// import. It contains references and exact amounts, never provider credentials
// or raw provider payloads. ResultID is the immutable provider/bank event key
// and is the idempotency key for the complete settlement flow.
type SettlementResultInput struct {
	CompanyID            int64                        `json:"company_id"`
	InstructionReference automation.ExternalReference `json:"instruction_reference"`
	// Instruction is a compatibility spelling for adapters that already use
	// the provider-neutral Settlement field name. If both are supplied they
	// must be identical.
	Instruction       automation.ExternalReference `json:"instruction,omitempty"`
	ProviderReference automation.ExternalReference `json:"provider_reference"`
	ResultID          string                       `json:"result_id"`
	// ResultReference and ProviderEventID are accepted as wire aliases. The
	// canonical in-memory identity is ResultID.
	ResultReference   string                 `json:"result_reference,omitempty"`
	ProviderEventID   string                 `json:"provider_event_id,omitempty"`
	Status            string                 `json:"status"`
	SettledAmount     automation.ExactAmount `json:"settled_amount"`
	ProviderFee       automation.ExactAmount `json:"provider_fee,omitempty"`
	SettledAt         time.Time              `json:"settled_at"`
	EndToEndReference string                 `json:"end_to_end_reference,omitempty"`
	Correlation       automation.Correlation `json:"correlation,omitempty"`
}

// ResultInput is a short name for integrations that do not need to spell out
// the transport boundary.
type ResultInput = SettlementResultInput

func (in SettlementResultInput) instructionRef() (automation.ExternalReference, error) {
	if !isZeroReference(in.InstructionReference) && !isZeroReference(in.Instruction) && in.InstructionReference != in.Instruction {
		return automation.ExternalReference{}, fmt.Errorf("%w: instruction aliases differ", ErrSettlementResultReferenceMismatch)
	}
	if !isZeroReference(in.InstructionReference) {
		return in.InstructionReference, nil
	}
	return in.Instruction, nil
}

func (in SettlementResultInput) resultID() string {
	for _, candidate := range []string{in.ResultID, in.ResultReference, in.ProviderEventID} {
		if value := strings.TrimSpace(candidate); value != "" {
			return value
		}
	}
	return ""
}

// Validate checks the transport-level invariants. Amount bounds that depend
// on the original instruction are checked again by SettlementService.Import.
func (in SettlementResultInput) Validate() error {
	if in.CompanyID <= 0 {
		return fmt.Errorf("%w: company is required", ErrInvalidSettlementResult)
	}
	ref, err := in.instructionRef()
	if err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return fmt.Errorf("%w: instruction reference: %v", ErrInvalidSettlementResult, err)
	}
	if ref.Connection.CompanyID != in.CompanyID {
		return ErrSettlementResultCompanyMismatch
	}
	if !isZeroReference(in.ProviderReference) {
		if err := in.ProviderReference.Validate(); err != nil {
			return fmt.Errorf("%w: provider reference: %v", ErrInvalidSettlementResult, err)
		}
		if in.ProviderReference.Connection != ref.Connection {
			return ErrSettlementResultReferenceMismatch
		}
	}
	if in.resultID() == "" {
		return fmt.Errorf("%w: result id is required", ErrInvalidSettlementResult)
	}
	status := normalizeStatus(in.Status)
	switch status {
	case ResultStatusPartial, ResultStatusSettled, ResultStatusFailed, ResultStatusCancelled:
		// Accepted below.
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedSettlementResult, status)
	}
	if hasExactAmount(in.SettledAmount) {
		if err := in.SettledAmount.Validate(); err != nil {
			return fmt.Errorf("%w: settled amount: %v", ErrInvalidSettlementResult, err)
		}
		if !in.SettledAmount.IsPositive() {
			return fmt.Errorf("%w: settled amount must be positive", ErrInvalidSettlementResult)
		}
	}
	if hasExactAmount(in.ProviderFee) {
		if err := in.ProviderFee.Validate(); err != nil {
			return fmt.Errorf("%w: provider fee: %v", ErrInvalidSettlementResult, err)
		}
		zero := in.ProviderFee.Amount.Sub(in.ProviderFee.Amount)
		if in.ProviderFee.Amount.Cmp(zero) < 0 {
			return fmt.Errorf("%w: provider fee must not be negative", ErrInvalidSettlementResult)
		}
	}
	if in.Correlation.ID != "" {
		if err := in.Correlation.Validate(); err != nil {
			return fmt.Errorf("%w: correlation: %v", ErrInvalidSettlementResult, err)
		}
	}
	return nil
}

// SettlementResult is the normalized, persisted observation. State is the
// local workflow state after applying the result; it is not a provider status.
type SettlementResult struct {
	CompanyID            int64                        `json:"company_id"`
	ResultID             string                       `json:"result_id"`
	InstructionReference automation.ExternalReference `json:"instruction_reference"`
	ProviderReference    automation.ExternalReference `json:"provider_reference"`
	Status               string                       `json:"status"`
	State                ExecutionState               `json:"state"`
	SettledAmount        automation.ExactAmount       `json:"settled_amount"`
	ProviderFee          automation.ExactAmount       `json:"provider_fee,omitempty"`
	SettledAt            time.Time                    `json:"settled_at"`
	EndToEndReference    string                       `json:"end_to_end_reference,omitempty"`
	RecordedAt           time.Time                    `json:"recorded_at"`
	EffectApplied        bool                         `json:"effect_applied"`
}

func (r SettlementResult) Validate() error {
	if r.CompanyID <= 0 || strings.TrimSpace(r.ResultID) == "" {
		return ErrInvalidSettlementResult
	}
	if err := r.InstructionReference.Validate(); err != nil {
		return fmt.Errorf("%w: instruction reference: %v", ErrInvalidSettlementResult, err)
	}
	if r.InstructionReference.Connection.CompanyID != r.CompanyID {
		return ErrSettlementResultCompanyMismatch
	}
	if !isZeroReference(r.ProviderReference) {
		if err := r.ProviderReference.Validate(); err != nil {
			return fmt.Errorf("%w: provider reference: %v", ErrInvalidSettlementResult, err)
		}
		if r.ProviderReference.Connection != r.InstructionReference.Connection {
			return ErrSettlementResultReferenceMismatch
		}
	}
	if r.State != StatePartiallySettled && r.State != StateSettled && r.State != StateFailed && r.State != StateCancelled {
		return fmt.Errorf("%w: invalid local state %q", ErrInvalidSettlementResult, r.State)
	}
	if strings.TrimSpace(r.Status) == "" {
		return fmt.Errorf("%w: status is required", ErrInvalidSettlementResult)
	}
	if r.State == StatePartiallySettled || r.State == StateSettled {
		if err := r.SettledAmount.Validate(); err != nil {
			return fmt.Errorf("%w: settled amount: %v", ErrInvalidSettlementResult, err)
		}
		if !r.SettledAmount.IsPositive() {
			return fmt.Errorf("%w: settled amount must be positive", ErrInvalidSettlementResult)
		}
	}
	if hasExactAmount(r.ProviderFee) {
		if err := r.ProviderFee.Validate(); err != nil {
			return fmt.Errorf("%w: provider fee: %v", ErrInvalidSettlementResult, err)
		}
		if !r.ProviderFee.IsPositive() {
			zero := r.ProviderFee.Amount.Sub(r.ProviderFee.Amount)
			if r.ProviderFee.Amount.Cmp(zero) < 0 {
				return fmt.Errorf("%w: provider fee must not be negative", ErrInvalidSettlementResult)
			}
		}
		if hasExactAmount(r.SettledAmount) && r.ProviderFee.Currency != r.SettledAmount.Currency {
			return ErrSettlementResultReferenceMismatch
		}
	}
	return nil
}

func (in SettlementResultInput) normalized(ref automation.ExternalReference, settlement Settlement, state ExecutionState) SettlementResult {
	provider := settlement.Reference
	if isZeroReference(provider) {
		provider = ref
	}
	return SettlementResult{
		CompanyID:            in.CompanyID,
		ResultID:             in.resultID(),
		InstructionReference: ref,
		ProviderReference:    provider,
		Status:               settlement.Status,
		State:                state,
		SettledAmount:        settlement.SettledAmount,
		ProviderFee:          settlement.ProviderFee,
		SettledAt:            settlement.SettledAt,
		EndToEndReference:    settlement.EndToEndReference,
		RecordedAt:           time.Now().UTC(),
	}
}

func (r SettlementResult) settlement() Settlement {
	return Settlement{
		Reference:         r.ProviderReference,
		Instruction:       r.InstructionReference,
		Status:            r.Status,
		SettledAmount:     r.SettledAmount,
		SettledAt:         r.SettledAt,
		ProviderFee:       r.ProviderFee,
		EndToEndReference: r.EndToEndReference,
	}
}

// SettlementEffectRequest is the narrow boundary where confirmed partial or
// full settlements may create AP, bank, GL, tax, reconciliation, and forecast
// effects. Implementations must deduplicate by EffectKey before mutating any
// financial state.
type SettlementEffectRequest struct {
	CompanyID int64            `json:"company_id"`
	EffectKey string           `json:"effect_key"`
	Result    SettlementResult `json:"result"`
	// Links identify durable financial records created by the accounting
	// boundary (for example an AP payment, allocation, journal, or bank
	// transaction). They are optional for compatibility with the original
	// idempotency-only port; when present they are persisted atomically with
	// the effect claim by PostgresSettlementEffects.
	Links []SettlementEffectLink `json:"links,omitempty"`
}

// SettlementEffectLink is a provider-neutral source link. Entity IDs are
// strings because AP, GL, bank, and tax records do not share one identifier
// type. Amount is optional for records whose identity is sufficient (for
// example a tax outbox item), but when supplied it is exact and non-negative.
type SettlementEffectLink struct {
	LinkType   string                 `json:"link_type"`
	EntityType string                 `json:"entity_type"`
	EntityID   string                 `json:"entity_id"`
	Amount     automation.ExactAmount `json:"amount,omitempty"`
	Metadata   map[string]any         `json:"metadata,omitempty"`
}

func (l SettlementEffectLink) Validate(result SettlementResult) error {
	if strings.TrimSpace(l.LinkType) == "" || strings.TrimSpace(l.EntityType) == "" || strings.TrimSpace(l.EntityID) == "" {
		return fmt.Errorf("%w: settlement effect link identity is required", ErrInvalidSettlementResult)
	}
	if !hasExactAmount(l.Amount) {
		return nil
	}
	if err := l.Amount.Validate(); err != nil {
		return fmt.Errorf("%w: settlement effect link amount: %v", ErrInvalidSettlementResult, err)
	}
	if !l.Amount.IsPositive() {
		return fmt.Errorf("%w: settlement effect link amount must be positive", ErrInvalidSettlementResult)
	}
	if hasExactAmount(result.SettledAmount) && l.Amount.Currency != result.SettledAmount.Currency {
		return ErrSettlementResultReferenceMismatch
	}
	return nil
}

// SettlementEffect is a compatibility alias for callers that use the shorter
// domain term.
type SettlementEffect = SettlementEffectRequest

func (e SettlementEffectRequest) Validate() error {
	if e.CompanyID <= 0 || strings.TrimSpace(e.EffectKey) == "" {
		return ErrInvalidSettlementResult
	}
	if err := e.Result.Validate(); err != nil {
		return err
	}
	if e.Result.CompanyID != e.CompanyID {
		return ErrSettlementResultCompanyMismatch
	}
	if e.Result.State != StatePartiallySettled && e.Result.State != StateSettled {
		return fmt.Errorf("%w: effects require confirmed settlement", ErrInvalidSettlementResult)
	}
	seen := make(map[string]struct{}, len(e.Links))
	for _, link := range e.Links {
		if err := link.Validate(e.Result); err != nil {
			return err
		}
		key := strings.Join([]string{link.LinkType, link.EntityType, link.EntityID}, "\x00")
		if _, ok := seen[key]; ok {
			return fmt.Errorf("%w: duplicate settlement effect link", ErrSettlementEffectConflict)
		}
		seen[key] = struct{}{}
	}
	return nil
}

type SettlementEffectOutcome struct {
	Applied        bool
	AlreadyApplied bool
}

// SettlementEffectsPort is implemented by the accounting boundary. It must
// atomically deduplicate EffectKey with the AP/GL/bank/reconciliation writes.
// This package intentionally does not implement those domain mutations.
type SettlementEffectsPort interface {
	ApplySettlementEffects(context.Context, SettlementEffectRequest) (SettlementEffectOutcome, error)
}

type SettlementEffectPort = SettlementEffectsPort

// MemorySettlementEffects is a deterministic idempotency implementation for
// tests and synchronous development. Production wiring should replace it
// with a transaction-backed adapter.
type MemorySettlementEffects struct {
	mu      sync.Mutex
	effects map[string]string
}

func NewMemorySettlementEffects() *MemorySettlementEffects {
	return &MemorySettlementEffects{effects: make(map[string]string)}
}

func (m *MemorySettlementEffects) ApplySettlementEffects(_ context.Context, request SettlementEffectRequest) (SettlementEffectOutcome, error) {
	if m == nil {
		return SettlementEffectOutcome{}, ErrInvalidSettlementResult
	}
	if err := request.Validate(); err != nil {
		return SettlementEffectOutcome{}, err
	}
	fingerprint := settlementEffectFingerprint(request)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.effects == nil {
		m.effects = make(map[string]string)
	}
	if previous, ok := m.effects[effectMapKey(request.CompanyID, request.EffectKey)]; ok {
		if previous != fingerprint {
			return SettlementEffectOutcome{}, ErrSettlementEffectConflict
		}
		return SettlementEffectOutcome{AlreadyApplied: true}, nil
	}
	m.effects[effectMapKey(request.CompanyID, request.EffectKey)] = fingerprint
	return SettlementEffectOutcome{Applied: true}, nil
}

// Apply is a convenience method for direct callers; the port contract remains
// ApplySettlementEffects so the intent is explicit at integration seams.
func (m *MemorySettlementEffects) Apply(ctx context.Context, request SettlementEffectRequest) (SettlementEffectOutcome, error) {
	return m.ApplySettlementEffects(ctx, request)
}

type SettlementResultRecord struct {
	Result        SettlementResult
	Fingerprint   string
	EffectApplied bool
	RecordedAt    time.Time
}

func (r SettlementResultRecord) Validate() error {
	if err := r.Result.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.Fingerprint) == "" {
		return ErrInvalidSettlementResult
	}
	return nil
}

// SettlementResultStore is the durable inbox boundary. Put must be insert-only
// for a company/result key; conflicting content is never overwritten.
type SettlementResultStore interface {
	GetSettlementResult(context.Context, int64, string) (SettlementResultRecord, error)
	PutSettlementResult(context.Context, SettlementResultRecord) error
	MarkSettlementEffectApplied(context.Context, int64, string) error
}

// MemorySettlementResultStore provides an idempotent store for unit tests.
type MemorySettlementResultStore struct {
	mu      sync.RWMutex
	results map[string]SettlementResultRecord
}

func NewMemorySettlementResultStore() *MemorySettlementResultStore {
	return &MemorySettlementResultStore{results: make(map[string]SettlementResultRecord)}
}

func (m *MemorySettlementResultStore) GetSettlementResult(_ context.Context, companyID int64, resultID string) (SettlementResultRecord, error) {
	if m == nil || companyID <= 0 || strings.TrimSpace(resultID) == "" {
		return SettlementResultRecord{}, ErrInvalidSettlementResult
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	record, ok := m.results[resultMapKey(companyID, resultID)]
	if !ok {
		return SettlementResultRecord{}, ErrSettlementResultNotFound
	}
	return record, nil
}

func (m *MemorySettlementResultStore) PutSettlementResult(_ context.Context, record SettlementResultRecord) error {
	if m == nil {
		return ErrInvalidSettlementResult
	}
	if err := record.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.results == nil {
		m.results = make(map[string]SettlementResultRecord)
	}
	key := resultMapKey(record.Result.CompanyID, record.Result.ResultID)
	if _, exists := m.results[key]; exists {
		return ErrDuplicateSettlementResult
	}
	if record.RecordedAt.IsZero() {
		record.RecordedAt = time.Now().UTC()
	}
	m.results[key] = record
	return nil
}

func (m *MemorySettlementResultStore) MarkSettlementEffectApplied(_ context.Context, companyID int64, resultID string) error {
	if m == nil {
		return ErrInvalidSettlementResult
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := resultMapKey(companyID, resultID)
	record, ok := m.results[key]
	if !ok {
		return ErrSettlementResultNotFound
	}
	record.EffectApplied = true
	record.Result.EffectApplied = true
	m.results[key] = record
	return nil
}

type SettlementService struct {
	executions ExecutionStore
	results    SettlementResultStore
	effects    SettlementEffectsPort
	mu         sync.Mutex
}

func NewSettlementService(executions ExecutionStore, results SettlementResultStore, effects SettlementEffectsPort) *SettlementService {
	if executions == nil {
		executions = NewMemoryStore()
	}
	if results == nil {
		results = NewMemorySettlementResultStore()
	}
	if effects == nil {
		effects = NewMemorySettlementEffects()
	}
	return &SettlementService{executions: executions, results: results, effects: effects}
}

// ImportResult validates, durably records, and applies one bank result. A
// repeated result ID with identical content returns the original outcome;
// different content under that ID is rejected. Effects are applied before the
// local execution snapshot advances, and the effects port itself is required
// to be idempotent.
func (s *SettlementService) ImportResult(ctx context.Context, input SettlementResultInput) (SettlementResult, error) {
	if s == nil || s.executions == nil || s.results == nil || s.effects == nil {
		return SettlementResult{}, ErrInvalidSettlementResult
	}
	if err := input.Validate(); err != nil {
		return SettlementResult{}, err
	}
	ref, _ := input.instructionRef()
	resultID := input.resultID()
	fingerprintInput := input
	fingerprintInput.ResultID = resultID
	fingerprint := settlementInputFingerprint(fingerprintInput)

	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.results.GetSettlementResult(ctx, input.CompanyID, resultID)
	if err == nil {
		if record.Fingerprint != fingerprint {
			return SettlementResult{}, ErrSettlementResultConflict
		}
		return s.finishResult(ctx, record)
	}
	if !errors.Is(err, ErrSettlementResultNotFound) {
		return SettlementResult{}, err
	}

	execution, err := s.executions.Get(ctx, ref)
	if err != nil {
		return SettlementResult{}, err
	}
	if execution.Instruction.Reference.Connection.CompanyID != input.CompanyID {
		return SettlementResult{}, ErrSettlementResultCompanyMismatch
	}
	if err := validateResultAgainstInstruction(input, execution.Instruction); err != nil {
		return SettlementResult{}, err
	}
	settlement, state, err := settlementFromInput(input, execution.Instruction)
	if err != nil {
		return SettlementResult{}, err
	}
	if state == StateSubmitted {
		return SettlementResult{}, ErrUnsupportedSettlementResult
	}
	result := input.normalized(ref, settlement, state)
	record = SettlementResultRecord{Result: result, Fingerprint: fingerprint, RecordedAt: result.RecordedAt}
	if err := s.results.PutSettlementResult(ctx, record); err != nil {
		if errors.Is(err, ErrDuplicateSettlementResult) {
			stored, getErr := s.results.GetSettlementResult(ctx, input.CompanyID, resultID)
			if getErr != nil {
				return SettlementResult{}, getErr
			}
			if stored.Fingerprint != fingerprint {
				return SettlementResult{}, ErrSettlementResultConflict
			}
			return s.finishResult(ctx, stored)
		}
		return SettlementResult{}, err
	}
	return s.finishResult(ctx, record)
}

// HandleResultImport is an automation outbox operation handler. The outbox
// company and idempotency key are checked before any financial state is read.
func (s *SettlementService) HandleResultImport(ctx context.Context, message automation.OutboxMessage) error {
	if s == nil || message.CompanyID <= 0 || message.Operation != automation.OperationPaymentResultImport {
		return ErrInvalidSettlementResult
	}
	var input SettlementResultInput
	if len(message.Payload) == 0 || json.Unmarshal(message.Payload, &input) != nil {
		return fmt.Errorf("%w: invalid result payload", ErrInvalidSettlementResult)
	}
	if input.CompanyID == 0 {
		input.CompanyID = message.CompanyID
	}
	if input.Correlation.ID == "" {
		input.Correlation = message.Correlation
	}
	if message.CompanyID != input.CompanyID {
		return ErrSettlementResultCompanyMismatch
	}
	if key := strings.TrimSpace(message.IdempotencyKey); key != "" && key != input.resultID() {
		return ErrSettlementResultConflict
	}
	_, err := s.ImportResult(ctx, input)
	return err
}

func (s *SettlementService) finishResult(ctx context.Context, record SettlementResultRecord) (SettlementResult, error) {
	result := record.Result
	execution, err := s.executions.Get(ctx, result.InstructionReference)
	if err != nil {
		return result, err
	}
	if execution.Instruction.Reference.Connection.CompanyID != result.CompanyID {
		return result, ErrSettlementResultCompanyMismatch
	}
	if err := validateImportedStateTransition(execution.State, result.State); err != nil {
		return result, err
	}

	// A result may be recorded before a worker crashes. Retry effects until the
	// store confirms them. The effects port deduplicates by result ID, so a
	// second result for an already-settled execution still gets its own source
	// link and cannot silently lose fee/settlement metadata.
	if (result.State == StatePartiallySettled || result.State == StateSettled) && !record.EffectApplied {
		effect := SettlementEffectRequest{
			CompanyID: result.CompanyID,
			EffectKey: result.ResultID,
			Result:    result,
		}
		outcome, err := s.effects.ApplySettlementEffects(ctx, effect)
		if err != nil {
			return result, err
		}
		if !outcome.Applied && !outcome.AlreadyApplied {
			return result, ErrSettlementEffectNotApplied
		}
		if err := s.results.MarkSettlementEffectApplied(ctx, result.CompanyID, result.ResultID); err != nil {
			return result, err
		}
		result.EffectApplied = true
		record.EffectApplied = true
	}

	if err := applyImportedState(&execution, result); err != nil {
		return result, err
	}
	if execution.State != result.State || execution.Settlement == nil || execution.Settlement.Status != result.Status {
		// applyImportedState already sets the settlement; this guard is kept for
		// callers implementing a custom store with immutable snapshots.
		return result, ErrInvalidSettlementResult
	}
	if err := s.executions.Save(ctx, execution); err != nil {
		return result, err
	}
	return result, nil
}

func validateResultAgainstInstruction(input SettlementResultInput, instruction Instruction) error {
	if input.CompanyID != instruction.Reference.Connection.CompanyID {
		return ErrSettlementResultCompanyMismatch
	}
	if ref, _ := input.instructionRef(); ref != instruction.Reference {
		return ErrSettlementResultReferenceMismatch
	}
	if !isZeroReference(input.ProviderReference) && input.ProviderReference.Connection != instruction.Reference.Connection {
		return ErrSettlementResultReferenceMismatch
	}
	if input.EndToEndReference != "" && instruction.EndToEndReference != "" && input.EndToEndReference != instruction.EndToEndReference {
		return ErrSettlementResultReferenceMismatch
	}
	if hasExactAmount(input.SettledAmount) && input.SettledAmount.Currency != instruction.Amount.Currency {
		return ErrSettlementResultReferenceMismatch
	}
	if hasExactAmount(input.ProviderFee) && input.ProviderFee.Currency != instruction.Amount.Currency {
		return ErrSettlementResultReferenceMismatch
	}
	return nil
}

func settlementFromInput(input SettlementResultInput, instruction Instruction) (Settlement, ExecutionState, error) {
	settlement := Settlement{
		Reference:         input.ProviderReference,
		Instruction:       instruction.Reference,
		Status:            normalizeStatus(input.Status),
		SettledAmount:     input.SettledAmount,
		SettledAt:         input.SettledAt,
		ProviderFee:       input.ProviderFee,
		EndToEndReference: input.EndToEndReference,
	}
	return normalizeSettlement(instruction, settlement)
}

func applyImportedState(execution *PaymentExecution, result SettlementResult) error {
	if execution == nil {
		return ErrInvalidSettlementResult
	}
	if err := validateImportedStateTransition(execution.State, result.State); err != nil {
		return err
	}
	if execution.State == StateSettled || execution.State == StateFailed || execution.State == StateCancelled {
	} else if execution.State != result.State {
		if err := transition(execution, result.State, result.SettledAt, "bank settlement result imported"); err != nil {
			return err
		}
	}
	settlement := result.settlement()
	execution.Settlement = &settlement
	execution.UpdatedAt = result.RecordedAt
	return nil
}

func validateImportedStateTransition(from, to ExecutionState) error {
	if from == StateSettled || from == StateFailed || from == StateCancelled {
		if from != to {
			return invalidTransition(from, to)
		}
		return nil
	}
	if from != to && !allowedTransition(from, to) {
		return invalidTransition(from, to)
	}
	return nil
}

func settlementInputFingerprint(input SettlementResultInput) string {
	return hashJSON(struct {
		CompanyID         int64
		Instruction       automation.ExternalReference
		Provider          automation.ExternalReference
		ResultID          string
		Status            string
		SettledAmount     automation.ExactAmount
		ProviderFee       automation.ExactAmount
		SettledAt         time.Time
		EndToEndReference string
	}{
		CompanyID:         input.CompanyID,
		Instruction:       mustInstructionRef(input),
		Provider:          input.ProviderReference,
		ResultID:          input.resultID(),
		Status:            normalizeStatus(input.Status),
		SettledAmount:     input.SettledAmount,
		ProviderFee:       input.ProviderFee,
		SettledAt:         input.SettledAt,
		EndToEndReference: input.EndToEndReference,
	})
}

func settlementFingerprint(result SettlementResult) string {
	return hashJSON(result)
}

// settlementEffectFingerprint preserves compatibility with rows written by
// the original effects adapter (which fingerprinted only the result), while
// making caller-supplied durable links part of the idempotency identity.
func settlementEffectFingerprint(request SettlementEffectRequest) string {
	if len(request.Links) == 0 {
		return settlementFingerprint(request.Result)
	}
	return hashJSON(struct {
		Result SettlementResult       `json:"result"`
		Links  []SettlementEffectLink `json:"links"`
	}{Result: request.Result, Links: request.Links})
}

func hashJSON(value any) string {
	payload, _ := json.Marshal(value)
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

func mustInstructionRef(input SettlementResultInput) automation.ExternalReference {
	ref, _ := input.instructionRef()
	return ref
}

func resultMapKey(companyID int64, resultID string) string {
	return fmt.Sprintf("%d:%s", companyID, resultID)
}

func effectMapKey(companyID int64, effectKey string) string {
	return fmt.Sprintf("%d:%s", companyID, effectKey)
}
