package treasury

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/payments"
)

// PaymentConnectionResolver resolves a company-scoped treasury connection
// without exposing credentials to the treasury service.
type PaymentConnectionResolver func(context.Context, int64, int64) (automation.ConnectionRef, error)

// BatchExecutionEnqueuer creates provider-neutral execution snapshots and
// durable worker commands for an approved treasury batch. It deliberately
// never invokes a provider from the HTTP request path.
type BatchExecutionEnqueuer struct {
	repo                 Repository
	outbox               *automation.OutboxRepository
	coordinator          *payments.Coordinator
	connectionResolver   PaymentConnectionResolver
	beneficiaryRefPrefix string
	now                  func() time.Time
}

// NewBatchExecutionEnqueuer composes the treasury batch boundary with the
// finance payment coordinator and durable automation outbox.
func NewBatchExecutionEnqueuer(repo Repository, outbox *automation.OutboxRepository, coordinator *payments.Coordinator, resolver PaymentConnectionResolver) *BatchExecutionEnqueuer {
	return &BatchExecutionEnqueuer{
		repo:                 repo,
		outbox:               outbox,
		coordinator:          coordinator,
		connectionResolver:   resolver,
		beneficiaryRefPrefix: "bank-account:",
		now:                  func() time.Time { return time.Now().UTC() },
	}
}

var _ ExecutionEnqueuer = (*BatchExecutionEnqueuer)(nil)

func (e *BatchExecutionEnqueuer) EnqueueBatchExecution(ctx context.Context, companyID, batchID, executorID int64) (ExecutionBatchResult, error) {
	if e == nil || e.repo == nil || e.outbox == nil || e.coordinator == nil || e.connectionResolver == nil {
		return ExecutionBatchResult{}, fmt.Errorf("treasury: payment execution enqueuer is not configured")
	}
	if companyID <= 0 || batchID <= 0 || executorID <= 0 {
		return ExecutionBatchResult{}, errCompanyScopeRequired
	}
	batch, err := e.repo.GetPaymentBatch(ctx, batchID)
	if err != nil {
		return ExecutionBatchResult{}, err
	}
	if batch.CompanyID != companyID {
		return ExecutionBatchResult{}, errCompanyScopeMismatch
	}
	if err := validateExecutableBatch(batch); err != nil {
		return ExecutionBatchResult{}, err
	}
	if batch.ProposedBy <= 0 || batch.ApprovedBy == nil || *batch.ApprovedBy <= 0 {
		return ExecutionBatchResult{}, fmt.Errorf("treasury: approved batch is missing maker/checker identities")
	}
	connection, err := e.connectionResolver(ctx, companyID, *batch.PaymentConnectionID)
	if err != nil {
		return ExecutionBatchResult{}, err
	}
	if err := connection.Validate(); err != nil {
		return ExecutionBatchResult{}, fmt.Errorf("treasury: payment connection: %w", err)
	}
	if connection.CompanyID != companyID {
		return ExecutionBatchResult{}, errCompanyScopeMismatch
	}
	items, err := e.repo.ListPaymentBatchItems(ctx, batchID)
	if err != nil {
		return ExecutionBatchResult{}, err
	}
	if len(items) == 0 {
		return ExecutionBatchResult{}, fmt.Errorf("treasury: cannot execute an empty payment batch")
	}

	result := ExecutionBatchResult{BatchID: batchID}
	for _, item := range items {
		instruction, err := e.instruction(batch, connection, item)
		if err != nil {
			return ExecutionBatchResult{}, err
		}
		execution, err := e.coordinator.Propose(ctx, instruction, batch.ProposedBy)
		if err != nil {
			return ExecutionBatchResult{}, fmt.Errorf("treasury: propose payment item %d: %w", item.ID, err)
		}
		if execution.State == payments.StateProposed {
			execution, err = e.coordinator.Approve(ctx, instruction.Reference, *batch.ApprovedBy)
			if err != nil {
				return ExecutionBatchResult{}, fmt.Errorf("treasury: approve payment item %d: %w", item.ID, err)
			}
		}
		if execution.State != payments.StateApproved {
			// A previously submitted or terminal execution is already owned by
			// the worker/result-import flow. Do not enqueue a second command.
			continue
		}
		correlation := instruction.Correlation
		command := payments.PaymentExecutionCommand{Reference: instruction.Reference, ExecutorID: executorID}
		input, err := payments.NewPaymentExecutionOutboxInput(command, correlation, instruction.Reference.ObjectID, executorID)
		if err != nil {
			return ExecutionBatchResult{}, fmt.Errorf("treasury: build payment item %d command: %w", item.ID, err)
		}
		message, err := e.outbox.Enqueue(ctx, input)
		if err != nil {
			return ExecutionBatchResult{}, fmt.Errorf("treasury: enqueue payment item %d: %w", item.ID, err)
		}
		result.CommandCount++
		result.CommandIDs = append(result.CommandIDs, message.ID)
	}
	return result, nil
}

func (e *BatchExecutionEnqueuer) instruction(batch PaymentBatch, connection automation.ConnectionRef, item PaymentBatchItem) (payments.Instruction, error) {
	if item.ID <= 0 || item.BatchID != batch.ID || item.BankAccountID <= 0 || item.Amount == "" || !item.Amount.IsPositive() {
		return payments.Instruction{}, fmt.Errorf("treasury: invalid payment batch item %d", item.ID)
	}
	if item.APInvoiceID == nil || *item.APInvoiceID <= 0 {
		return payments.Instruction{}, fmt.Errorf("treasury: payment batch item %d has no AP invoice", item.ID)
	}
	money, err := exactTreasuryMoney(item.Amount.String())
	if err != nil {
		return payments.Instruction{}, fmt.Errorf("treasury: item %d amount: %w", item.ID, err)
	}
	objectID := fmt.Sprintf("treasury-batch-%d-item-%d", batch.ID, item.ID)
	correlationID := fmt.Sprintf("treasury-batch:%d:item:%d", batch.ID, item.ID)
	prefix := e.beneficiaryRefPrefix
	if strings.TrimSpace(prefix) == "" {
		prefix = "bank-account:"
	}
	return payments.Instruction{
		Reference: automation.ExternalReference{
			Connection: connection,
			ObjectType: "treasury_payment_item",
			ObjectID:   objectID,
		},
		Amount: automation.ExactAmount{
			Amount:   money,
			Currency: strings.ToUpper(strings.TrimSpace(batch.Currency)),
		},
		BeneficiaryRef:    prefix + strconv.FormatInt(item.BankAccountID, 10),
		Correlation:       automation.Correlation{ID: correlationID},
		EndToEndReference: objectID,
		BeneficiaryName:   "",
	}, nil
}

func validateExecutableBatch(batch PaymentBatch) error {
	if batch.ID <= 0 || batch.CompanyID <= 0 {
		return errCompanyScopeRequired
	}
	if batch.PaymentConnectionID == nil || *batch.PaymentConnectionID <= 0 {
		return fmt.Errorf("treasury: payment batch has no provider connection")
	}
	if batch.SourceBankAccountID == nil || *batch.SourceBankAccountID <= 0 {
		return fmt.Errorf("treasury: payment batch has no source bank account")
	}
	if batch.Status != "APPROVED" && batch.Status != "EXPORTED" && batch.Status != "PROCESSING" {
		return fmt.Errorf("treasury: payment batch is not executable")
	}
	return nil
}

func exactTreasuryMoney(value string) (accountingmoney.Money, error) {
	value = strings.TrimSpace(value)
	scale := 0
	if dot := strings.IndexByte(value, '.'); dot >= 0 {
		scale = len(value) - dot - 1
	}
	return accountingmoney.Parse(value, scale)
}
