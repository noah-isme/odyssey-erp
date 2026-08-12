package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

// PaymentExecutionCommand is the provider-neutral payload for a payment
// execution outbox message. The coordinator decides whether this means a
// first submission, a provider lookup, or an already-completed no-op.
type PaymentExecutionCommand struct {
	Reference  automation.ExternalReference `json:"reference"`
	ExecutorID int64                        `json:"executor_id"`
}

func (c PaymentExecutionCommand) Validate(companyID int64) error {
	if err := c.Reference.Validate(); err != nil {
		return fmt.Errorf("%w: execution reference: %v", ErrInvalidInstruction, err)
	}
	if companyID <= 0 || c.Reference.Connection.CompanyID != companyID {
		return ErrSettlementResultCompanyMismatch
	}
	if c.ExecutorID <= 0 {
		return ErrUnauthorized
	}
	return nil
}

// NewPaymentExecutionOutboxInput creates an idempotent command for the shared
// finance outbox. The worker may redeliver it; Coordinator.Submit is safe for
// repeated approved/submitted states and resolves AMBIGUOUS through Lookup.
func NewPaymentExecutionOutboxInput(command PaymentExecutionCommand, correlation automation.Correlation, idempotencyKey string, createdBy int64) (automation.EnqueueInput, error) {
	if err := command.Validate(command.Reference.Connection.CompanyID); err != nil {
		return automation.EnqueueInput{}, err
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		return automation.EnqueueInput{}, automation.ErrInvalidOutboxMessage
	}
	if err := correlation.Validate(); err != nil {
		return automation.EnqueueInput{}, err
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return automation.EnqueueInput{}, err
	}
	return automation.EnqueueInput{
		CompanyID:      command.Reference.Connection.CompanyID,
		Topic:          "finance.payment",
		AggregateType:  "payment_instruction",
		AggregateID:    command.Reference.ObjectID,
		Operation:      automation.OperationPaymentExecute,
		Correlation:    correlation,
		IdempotencyKey: idempotencyKey,
		Payload:        payload,
		CreatedBy:      createdBy,
	}, nil
}

// NewPaymentResultImportOutboxInput creates the durable result-import command.
// The outbox idempotency key is the immutable ResultID, so duplicate provider
// callbacks and duplicate file rows converge on the same command.
func NewPaymentResultImportOutboxInput(input SettlementResultInput, correlation automation.Correlation, createdBy int64) (automation.EnqueueInput, error) {
	if err := input.Validate(); err != nil {
		return automation.EnqueueInput{}, err
	}
	if err := correlation.Validate(); err != nil {
		return automation.EnqueueInput{}, err
	}
	input.Correlation = correlation
	payload, err := json.Marshal(input)
	if err != nil {
		return automation.EnqueueInput{}, err
	}
	ref, _ := input.instructionRef()
	return automation.EnqueueInput{
		CompanyID:      input.CompanyID,
		Topic:          "finance.payment",
		AggregateType:  "payment_instruction",
		AggregateID:    ref.ObjectID,
		Operation:      automation.OperationPaymentResultImport,
		Correlation:    correlation,
		IdempotencyKey: input.resultID(),
		Payload:        payload,
		CreatedBy:      createdBy,
	}, nil
}

// HandlePaymentExecution is the only registered execution operation. It
// delegates to Coordinator.Submit, which checks persisted state and performs
// Lookup first when a previous attempt was ambiguous. This method never calls
// the provider directly and therefore cannot blindly resubmit on redelivery.
func (c *Coordinator) HandlePaymentExecution(ctx context.Context, message automation.OutboxMessage) error {
	if c == nil || message.CompanyID <= 0 || (message.Operation != automation.OperationPaymentExecute && message.Operation != automation.OperationPaymentSubmit) {
		return ErrInvalidCoordinator
	}
	var command PaymentExecutionCommand
	if len(message.Payload) == 0 || json.Unmarshal(message.Payload, &command) != nil {
		return fmt.Errorf("%w: invalid execution payload", ErrInvalidCoordinator)
	}
	if err := command.Validate(message.CompanyID); err != nil {
		return err
	}
	if strings.TrimSpace(message.AggregateID) != "" && message.AggregateID != command.Reference.ObjectID {
		return ErrSettlementResultReferenceMismatch
	}
	_, err := c.Submit(ctx, command.Reference, command.ExecutorID)
	if err != nil {
		if errors.Is(err, ErrAmbiguousSubmission) {
			// The automation dispatcher treats this sentinel as terminal for
			// retries. A deliberate replay invokes Submit again, which resolves
			// the persisted AMBIGUOUS state through Lookup before any submit.
			return fmt.Errorf("%w: %v", automation.ErrAmbiguousOutcome, err)
		}
		return err
	}
	return nil
}

// OutboxRegistrar is satisfied by automation.Dispatcher and keeps payment
// registration independent from worker construction.
type OutboxRegistrar interface {
	Register(string, automation.OperationHandler) error
}

// RegisterPaymentExecutionHandlers wires both payment operations into the
// shared finance outbox. Passing nil for either service simply omits that
// operation, which lets applications roll out result import independently of
// execution adapters.
func RegisterPaymentExecutionHandlers(registrar OutboxRegistrar, coordinator *Coordinator, settlementService *SettlementService) error {
	if registrar == nil {
		return automation.ErrInvalidOutboxMessage
	}
	if coordinator != nil {
		if err := registrar.Register(automation.OperationPaymentExecute, coordinator.HandlePaymentExecution); err != nil {
			return err
		}
		// Keep existing producers on the legacy operation while they migrate to
		// payment.execute. Both paths share the same idempotent coordinator.
		if err := registrar.Register(automation.OperationPaymentSubmit, coordinator.HandlePaymentExecution); err != nil {
			return err
		}
	}
	if settlementService != nil {
		if err := registrar.Register(automation.OperationPaymentResultImport, settlementService.HandleResultImport); err != nil {
			return err
		}
	}
	return nil
}
