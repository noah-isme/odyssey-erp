package treasury

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Repository is the treasury persistence port. Generated SQL types stay inside
// the PostgreSQL adapter.
type Repository interface {
	CreateSupplierBankAccount(ctx context.Context, input SupplierBankAccountCreate) (SupplierBankAccount, error)
	UpdateSupplierBankAccountVerification(ctx context.Context, input SupplierBankAccountVerificationUpdate) (SupplierBankAccount, error)
	GetSupplierBankAccount(ctx context.Context, id int64) (SupplierBankAccount, error)
	ListSupplierBankAccounts(ctx context.Context, filter SupplierBankAccountFilter) ([]SupplierBankAccount, error)
	GetPaymentPolicy(ctx context.Context, companyID int64) (PaymentPolicy, error)

	CreatePaymentBatch(ctx context.Context, input PaymentBatchCreate) (PaymentBatch, error)
	GetPaymentBatch(ctx context.Context, id int64) (PaymentBatch, error)
	UpdatePaymentBatchStatus(ctx context.Context, input PaymentBatchStatusUpdate) (PaymentBatch, error)
	UpdatePaymentBatchRevision(ctx context.Context, input PaymentBatchRevisionUpdate) (PaymentBatch, error)
	UpdatePaymentBatchExport(ctx context.Context, input PaymentBatchExportUpdate) (PaymentBatch, error)
	UpdatePaymentBatchSettlement(ctx context.Context, input PaymentBatchSettlementUpdate) (PaymentBatch, error)
	CreatePaymentBatchItem(ctx context.Context, input PaymentBatchItemCreate) (PaymentBatchItem, error)
	ListPaymentBatchItems(ctx context.Context, batchID int64) ([]PaymentBatchItem, error)
	RemovePaymentBatchItem(ctx context.Context, id int64) error
}

// APService defines the required interoperability hooks into Accounts Payable.
type APService interface {
	MarkInvoicePaid(ctx context.Context, invoiceID, batchID int64, amount float64) error
}

type Service struct {
	repo   Repository
	apSvc  APService
	logger *slog.Logger
}

func NewService(repo Repository, apSvc APService, logger *slog.Logger) *Service {
	return &Service{repo: repo, apSvc: apSvc, logger: logger}
}

// AddBankAccount securely registers a new beneficiary account on hold.
func (s *Service) AddBankAccount(ctx context.Context, companyID, supplierID, actorID int64, bankName, accountNumber, routingNumber, currency, evidenceRef string) (SupplierBankAccount, error) {
	return s.repo.CreateSupplierBankAccount(ctx, SupplierBankAccountCreate{
		CompanyID:     companyID,
		SupplierID:    supplierID,
		BankName:      bankName,
		AccountNumber: accountNumber,
		RoutingNumber: routingNumber,
		Currency:      currency,
		EffectiveFrom: time.Now(),
		EvidenceRef:   evidenceRef,
		CreatedBy:     actorID,
	})
}

// ApproveBankAccount performs Maker/Checker verification.
func (s *Service) ApproveBankAccount(ctx context.Context, accountID, approverID int64) (SupplierBankAccount, error) {
	account, err := s.repo.GetSupplierBankAccount(ctx, accountID)
	if err != nil {
		return SupplierBankAccount{}, fmt.Errorf("failed to get account: %w", err)
	}

	policy, err := s.repo.GetPaymentPolicy(ctx, account.CompanyID)
	if err == nil && policy.RequiresMakerChecker && account.CreatedBy == approverID {
		return SupplierBankAccount{}, errors.New("maker checker violation: creator cannot approve")
	}
	if account.VerificationStatus != "PENDING_APPROVAL" {
		return SupplierBankAccount{}, errors.New("account is not pending approval")
	}

	return s.repo.UpdateSupplierBankAccountVerification(ctx, SupplierBankAccountVerificationUpdate{
		ID:                 accountID,
		VerificationStatus: "VERIFIED",
		HoldPayments:       false,
		ApprovedBy:         int64Ptr(approverID),
	})
}

// CanPaySupplier determines if the supplier has a valid, verified account.
func (s *Service) CanPaySupplier(ctx context.Context, companyID, supplierID int64) (bool, error) {
	accounts, err := s.repo.ListSupplierBankAccounts(ctx, SupplierBankAccountFilter{SupplierID: supplierID, CompanyID: companyID})
	if err != nil {
		return false, err
	}
	now := time.Now()
	for _, account := range accounts {
		if now.Before(account.EffectiveFrom) {
			continue
		}
		if account.EffectiveTo != nil && now.After(*account.EffectiveTo) {
			continue
		}
		if account.VerificationStatus == "VERIFIED" && !account.HoldPayments {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) CreatePaymentBatch(ctx context.Context, companyID int64, refCode, currency string, actorID int64) (PaymentBatch, error) {
	return s.repo.CreatePaymentBatch(ctx, PaymentBatchCreate{CompanyID: companyID, ReferenceCode: refCode, Currency: currency, ProposedBy: actorID})
}

// AddBatchItem adds an item and revises the batch.
func (s *Service) AddBatchItem(ctx context.Context, batchID, supplierID, bankAccountID int64, amount float64, apInvoiceID int64) (PaymentBatchItem, error) {
	batch, err := s.repo.GetPaymentBatch(ctx, batchID)
	if err != nil {
		return PaymentBatchItem{}, err
	}
	if batch.Status == "EXPORTED" || batch.Status == "SETTLED" || batch.Status == "CANCELLED" {
		return PaymentBatchItem{}, errors.New("cannot edit batch in terminal or exported state")
	}
	canPay, err := s.CanPaySupplier(ctx, batch.CompanyID, supplierID)
	if err != nil || !canPay {
		return PaymentBatchItem{}, errors.New("supplier cannot be paid (unverified or on hold)")
	}

	item, err := s.repo.CreatePaymentBatchItem(ctx, PaymentBatchItemCreate{
		BatchID:       batchID,
		SupplierID:    supplierID,
		BankAccountID: bankAccountID,
		Amount:        amount,
		APInvoiceID:   optionalID(apInvoiceID),
	})
	if err != nil {
		return PaymentBatchItem{}, err
	}
	_, _ = s.repo.ListPaymentBatchItems(ctx, batchID)
	_, err = s.repo.UpdatePaymentBatchRevision(ctx, PaymentBatchRevisionUpdate{ID: batchID, TotalAmount: amount})
	return item, err
}

func (s *Service) ApproveBatch(ctx context.Context, batchID, approverID int64) (PaymentBatch, error) {
	batch, err := s.repo.GetPaymentBatch(ctx, batchID)
	if err != nil {
		return PaymentBatch{}, err
	}
	if batch.Status != "PENDING_APPROVAL" {
		return PaymentBatch{}, errors.New("batch is not pending approval")
	}
	policy, err := s.repo.GetPaymentPolicy(ctx, batch.CompanyID)
	if err == nil && policy.RequiresMakerChecker && batch.ProposedBy == approverID {
		return PaymentBatch{}, errors.New("maker checker violation: proposer cannot approve")
	}

	items, err := s.repo.ListPaymentBatchItems(ctx, batchID)
	if err != nil {
		return PaymentBatch{}, err
	}
	for _, item := range items {
		canPay, err := s.CanPaySupplier(ctx, batch.CompanyID, item.SupplierID)
		if err != nil || !canPay {
			return PaymentBatch{}, fmt.Errorf("item %d supplier %d cannot be paid", item.ID, item.SupplierID)
		}
	}

	return s.repo.UpdatePaymentBatchStatus(ctx, PaymentBatchStatusUpdate{
		ID:         batchID,
		Status:     "APPROVED",
		ApprovedBy: int64Ptr(approverID),
		ApprovedAt: timePtr(time.Now()),
	})
}

// ExportBatch generates a bank format representation and marks the batch exported.
func (s *Service) ExportBatch(ctx context.Context, batchID, actorID int64, encoder BankFormatEncoder) ([]byte, error) {
	batch, err := s.repo.GetPaymentBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if batch.Status != "APPROVED" {
		return nil, errors.New("only approved batches can be exported")
	}
	items, err := s.repo.ListPaymentBatchItems(ctx, batchID)
	if err != nil {
		return nil, err
	}
	payload, hash, err := encoder.Encode(batch, items)
	if err != nil {
		return nil, fmt.Errorf("failed to encode batch: %w", err)
	}
	_, err = s.repo.UpdatePaymentBatchExport(ctx, PaymentBatchExportUpdate{ID: batchID, ExportedFileHash: hash, ExportedBy: int64Ptr(actorID)})
	if err != nil {
		return nil, fmt.Errorf("failed to mark batch as exported: %w", err)
	}
	return payload, nil
}

// SettleBatch confirms the bank settlement and allocates AP invoices.
func (s *Service) SettleBatch(ctx context.Context, batchID, actorID int64) (PaymentBatch, error) {
	batch, err := s.repo.GetPaymentBatch(ctx, batchID)
	if err != nil {
		return PaymentBatch{}, err
	}
	if batch.Status != "EXPORTED" {
		return PaymentBatch{}, errors.New("only exported batches can be settled")
	}
	items, err := s.repo.ListPaymentBatchItems(ctx, batchID)
	if err != nil {
		return PaymentBatch{}, err
	}
	if s.apSvc != nil {
		for _, item := range items {
			if item.APInvoiceID == nil {
				continue
			}
			if err := s.apSvc.MarkInvoicePaid(ctx, *item.APInvoiceID, batchID, item.Amount); err != nil {
				s.logError("failed to mark AP invoice as paid", err)
				return PaymentBatch{}, fmt.Errorf("failed to allocate AP payment for item %d: %w", item.ID, err)
			}
		}
	}
	return s.repo.UpdatePaymentBatchSettlement(ctx, PaymentBatchSettlementUpdate{ID: batchID, SettledBy: int64Ptr(actorID)})
}

func int64Ptr(value int64) *int64        { return &value }
func timePtr(value time.Time) *time.Time { return &value }
func optionalID(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return int64Ptr(value)
}

func (s *Service) logError(message string, err error) {
	if s.logger != nil {
		s.logger.Error(message, slog.Any("error", err))
	}
}
