package treasury

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Repository is the treasury persistence port. Generated SQL types stay inside
// the PostgreSQL adapter.
type Repository interface {
	SupplierBelongsToCompany(ctx context.Context, supplierID, companyID int64) (bool, error)
	CreateSupplierBankAccount(ctx context.Context, input SupplierBankAccountCreate) (SupplierBankAccount, error)
	UpdateSupplierBankAccountVerification(ctx context.Context, input SupplierBankAccountVerificationUpdate) (SupplierBankAccount, error)
	GetSupplierBankAccount(ctx context.Context, id int64) (SupplierBankAccount, error)
	ListSupplierBankAccounts(ctx context.Context, filter SupplierBankAccountFilter) ([]SupplierBankAccount, error)
	GetPaymentPolicy(ctx context.Context, companyID int64) (PaymentPolicy, error)
	APInvoiceEligibleForPayment(ctx context.Context, invoiceID, supplierID, companyID int64, currency string) (bool, error)

	CreatePaymentBatch(ctx context.Context, input PaymentBatchCreate) (PaymentBatch, error)
	GetPaymentBatch(ctx context.Context, id int64) (PaymentBatch, error)
	UpdatePaymentBatchStatus(ctx context.Context, input PaymentBatchStatusUpdate) (PaymentBatch, error)
	UpdatePaymentBatchRevision(ctx context.Context, input PaymentBatchRevisionUpdate) (PaymentBatch, error)
	UpdatePaymentBatchTotal(ctx context.Context, input PaymentBatchTotalUpdate) (PaymentBatch, error)
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

var (
	errCompanyScopeRequired = errors.New("treasury: company scope is required")
	errCompanyScopeMismatch = errors.New("treasury: resource does not belong to active company")
)

func NewService(repo Repository, apSvc APService, logger *slog.Logger) *Service {
	return &Service{repo: repo, apSvc: apSvc, logger: logger}
}

// AddBankAccount securely registers a new beneficiary account on hold.
func (s *Service) AddBankAccount(ctx context.Context, companyID, supplierID, actorID int64, bankName, accountNumber, routingNumber, currency, evidenceRef string) (SupplierBankAccount, error) {
	if companyID <= 0 || supplierID <= 0 || actorID <= 0 {
		return SupplierBankAccount{}, errCompanyScopeRequired
	}
	if strings.TrimSpace(bankName) == "" || strings.TrimSpace(accountNumber) == "" {
		return SupplierBankAccount{}, errors.New("bank name and account number are required")
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 {
		return SupplierBankAccount{}, errors.New("currency must be a three-letter code")
	}
	belongs, err := s.repo.SupplierBelongsToCompany(ctx, supplierID, companyID)
	if err != nil {
		return SupplierBankAccount{}, fmt.Errorf("failed to validate supplier ownership: %w", err)
	}
	if !belongs {
		return SupplierBankAccount{}, errCompanyScopeMismatch
	}
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
func (s *Service) ApproveBankAccount(ctx context.Context, companyID, accountID, approverID int64) (SupplierBankAccount, error) {
	if companyID <= 0 || accountID <= 0 || approverID <= 0 {
		return SupplierBankAccount{}, errCompanyScopeRequired
	}
	account, err := s.repo.GetSupplierBankAccount(ctx, accountID)
	if err != nil {
		return SupplierBankAccount{}, fmt.Errorf("failed to get account: %w", err)
	}
	if account.CompanyID != companyID {
		return SupplierBankAccount{}, errCompanyScopeMismatch
	}

	policy, err := s.repo.GetPaymentPolicy(ctx, account.CompanyID)
	if err != nil {
		return SupplierBankAccount{}, fmt.Errorf("failed to get payment policy: %w", err)
	}
	if policy.RequiresMakerChecker && account.CreatedBy == approverID {
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
	if companyID <= 0 || supplierID <= 0 {
		return false, errCompanyScopeRequired
	}
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

// ListBankAccounts returns only accounts belonging to the active tenant and
// supplier. The company filter is applied in the repository as well as at the
// handler boundary.
func (s *Service) ListBankAccounts(ctx context.Context, companyID, supplierID int64) ([]SupplierBankAccount, error) {
	if companyID <= 0 || supplierID <= 0 {
		return nil, errCompanyScopeRequired
	}
	return s.repo.ListSupplierBankAccounts(ctx, SupplierBankAccountFilter{CompanyID: companyID, SupplierID: supplierID})
}

func (s *Service) CreatePaymentBatch(ctx context.Context, companyID int64, refCode, currency string, actorID int64) (PaymentBatch, error) {
	if companyID <= 0 || actorID <= 0 {
		return PaymentBatch{}, errCompanyScopeRequired
	}
	if strings.TrimSpace(refCode) == "" {
		return PaymentBatch{}, errors.New("reference code is required")
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 {
		return PaymentBatch{}, errors.New("currency must be a three-letter code")
	}
	return s.repo.CreatePaymentBatch(ctx, PaymentBatchCreate{CompanyID: companyID, ReferenceCode: refCode, Currency: currency, ProposedBy: actorID})
}

// AddBatchItem adds an item and revises the batch.
func (s *Service) AddBatchItem(ctx context.Context, companyID, batchID, supplierID, bankAccountID int64, amount float64, apInvoiceID int64) (PaymentBatchItem, error) {
	if companyID <= 0 || batchID <= 0 || supplierID <= 0 || bankAccountID <= 0 {
		return PaymentBatchItem{}, errCompanyScopeRequired
	}
	if amount <= 0 {
		return PaymentBatchItem{}, errors.New("payment amount must be greater than zero")
	}
	batch, err := s.repo.GetPaymentBatch(ctx, batchID)
	if err != nil {
		return PaymentBatchItem{}, err
	}
	if batch.CompanyID != companyID {
		return PaymentBatchItem{}, errCompanyScopeMismatch
	}
	if batch.Status == "EXPORTED" || batch.Status == "SETTLED" || batch.Status == "CANCELLED" {
		return PaymentBatchItem{}, errors.New("cannot edit batch in terminal or exported state")
	}
	canPay, err := s.CanPaySupplier(ctx, batch.CompanyID, supplierID)
	if err != nil {
		return PaymentBatchItem{}, fmt.Errorf("failed to validate supplier payment controls: %w", err)
	}
	if !canPay {
		return PaymentBatchItem{}, errors.New("supplier cannot be paid (unverified or on hold)")
	}
	account, err := s.repo.GetSupplierBankAccount(ctx, bankAccountID)
	if err != nil {
		return PaymentBatchItem{}, fmt.Errorf("failed to get beneficiary account: %w", err)
	}
	if !payableAccount(account, companyID, supplierID, batch.Currency, time.Now()) {
		return PaymentBatchItem{}, errCompanyScopeMismatch
	}
	if invoiceID := optionalID(apInvoiceID); invoiceID != nil {
		eligible, err := s.repo.APInvoiceEligibleForPayment(ctx, *invoiceID, supplierID, companyID, batch.Currency)
		if err != nil {
			return PaymentBatchItem{}, fmt.Errorf("failed to validate AP invoice: %w", err)
		}
		if !eligible {
			return PaymentBatchItem{}, errors.New("AP invoice is not payable for this supplier, company, or currency")
		}
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
	// The database recomputes the aggregate from ACTIVE rows. The service does
	// not re-sum float64 values and then race the authoritative SQL total.
	_, err = s.repo.UpdatePaymentBatchRevision(ctx, PaymentBatchRevisionUpdate{ID: batchID})
	return item, err
}

func (s *Service) ApproveBatch(ctx context.Context, companyID, batchID, approverID int64) (PaymentBatch, error) {
	if companyID <= 0 || batchID <= 0 || approverID <= 0 {
		return PaymentBatch{}, errCompanyScopeRequired
	}
	batch, err := s.repo.GetPaymentBatch(ctx, batchID)
	if err != nil {
		return PaymentBatch{}, err
	}
	if batch.CompanyID != companyID {
		return PaymentBatch{}, errCompanyScopeMismatch
	}
	if batch.Status != "PENDING_APPROVAL" {
		return PaymentBatch{}, errors.New("batch is not pending approval")
	}
	policy, err := s.repo.GetPaymentPolicy(ctx, batch.CompanyID)
	if err != nil {
		return PaymentBatch{}, fmt.Errorf("failed to get payment policy: %w", err)
	}
	if policy.RequiresMakerChecker && batch.ProposedBy == approverID {
		return PaymentBatch{}, errors.New("maker checker violation: proposer cannot approve")
	}

	items, err := s.repo.ListPaymentBatchItems(ctx, batchID)
	if err != nil {
		return PaymentBatch{}, err
	}
	if len(items) == 0 {
		return PaymentBatch{}, errors.New("cannot approve an empty payment batch")
	}
	// Reconcile the total on every approval attempt. The repository query uses
	// SUM(amount) over ACTIVE items, making approval safe after retries or
	// concurrent item changes.
	batch, err = s.repo.UpdatePaymentBatchTotal(ctx, PaymentBatchTotalUpdate{ID: batchID})
	if err != nil {
		return PaymentBatch{}, fmt.Errorf("failed to reconcile batch total: %w", err)
	}
	for _, item := range items {
		canPay, err := s.CanPaySupplier(ctx, batch.CompanyID, item.SupplierID)
		if err != nil {
			return PaymentBatch{}, fmt.Errorf("failed to validate item %d supplier controls: %w", item.ID, err)
		}
		if !canPay {
			return PaymentBatch{}, fmt.Errorf("item %d supplier %d cannot be paid", item.ID, item.SupplierID)
		}
		account, err := s.repo.GetSupplierBankAccount(ctx, item.BankAccountID)
		if err != nil {
			return PaymentBatch{}, fmt.Errorf("failed to validate item %d beneficiary: %w", item.ID, err)
		}
		if !payableAccount(account, batch.CompanyID, item.SupplierID, batch.Currency, time.Now()) {
			return PaymentBatch{}, fmt.Errorf("item %d beneficiary is no longer payable", item.ID)
		}
		if item.APInvoiceID != nil {
			eligible, err := s.repo.APInvoiceEligibleForPayment(ctx, *item.APInvoiceID, item.SupplierID, batch.CompanyID, batch.Currency)
			if err != nil {
				return PaymentBatch{}, fmt.Errorf("failed to validate item %d AP invoice: %w", item.ID, err)
			}
			if !eligible {
				return PaymentBatch{}, fmt.Errorf("item %d AP invoice is no longer payable", item.ID)
			}
		}
	}

	return s.repo.UpdatePaymentBatchStatus(ctx, PaymentBatchStatusUpdate{
		ID:         batchID,
		Status:     "APPROVED",
		ApprovedBy: int64Ptr(approverID),
		ApprovedAt: timePtr(time.Now()),
	})
}

// RemoveBatchItem removes an active item only when it belongs to the scoped
// batch, then recomputes the aggregate total from the remaining active items.
func (s *Service) RemoveBatchItem(ctx context.Context, companyID, batchID, itemID int64) error {
	if companyID <= 0 || batchID <= 0 || itemID <= 0 {
		return errCompanyScopeRequired
	}
	batch, err := s.repo.GetPaymentBatch(ctx, batchID)
	if err != nil {
		return err
	}
	if batch.CompanyID != companyID {
		return errCompanyScopeMismatch
	}
	if batch.Status == "EXPORTED" || batch.Status == "SETTLED" || batch.Status == "CANCELLED" {
		return errors.New("cannot edit batch in terminal or exported state")
	}
	items, err := s.repo.ListPaymentBatchItems(ctx, batchID)
	if err != nil {
		return err
	}
	found := false
	for _, item := range items {
		if item.ID == itemID {
			found = true
			break
		}
	}
	if !found {
		return errors.New("batch item not found")
	}
	if err := s.repo.RemovePaymentBatchItem(ctx, itemID); err != nil {
		return err
	}
	remaining := make([]PaymentBatchItem, 0, len(items)-1)
	for _, item := range items {
		if item.ID != itemID {
			remaining = append(remaining, item)
		}
	}
	_, err = s.repo.UpdatePaymentBatchRevision(ctx, PaymentBatchRevisionUpdate{ID: batchID})
	return err
}

// ExportBatch generates a bank format representation and marks the batch exported.
func (s *Service) ExportBatch(ctx context.Context, companyID, batchID, actorID int64, encoder BankFormatEncoder) ([]byte, error) {
	if companyID <= 0 || batchID <= 0 || actorID <= 0 {
		return nil, errCompanyScopeRequired
	}
	batch, err := s.repo.GetPaymentBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if batch.CompanyID != companyID {
		return nil, errCompanyScopeMismatch
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
func (s *Service) SettleBatch(ctx context.Context, companyID, batchID, actorID int64) (PaymentBatch, error) {
	if companyID <= 0 || batchID <= 0 || actorID <= 0 {
		return PaymentBatch{}, errCompanyScopeRequired
	}
	batch, err := s.repo.GetPaymentBatch(ctx, batchID)
	if err != nil {
		return PaymentBatch{}, err
	}
	if batch.CompanyID != companyID {
		return PaymentBatch{}, errCompanyScopeMismatch
	}
	if batch.Status == "SETTLED" {
		return batch, nil
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

func payableAccount(account SupplierBankAccount, companyID, supplierID int64, currency string, now time.Time) bool {
	if account.CompanyID != companyID || account.SupplierID != supplierID {
		return false
	}
	if !strings.EqualFold(account.Currency, currency) {
		return false
	}
	if now.Before(account.EffectiveFrom) || (account.EffectiveTo != nil && now.After(*account.EffectiveTo)) {
		return false
	}
	return account.VerificationStatus == "VERIFIED" && !account.HoldPayments
}

func (s *Service) logError(message string, err error) {
	if s.logger != nil {
		s.logger.Error(message, slog.Any("error", err))
	}
}
