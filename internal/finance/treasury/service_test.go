package treasury_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/treasury"
)

type mockRepo struct {
	accounts             []treasury.SupplierBankAccount
	policy               treasury.PaymentPolicy
	batches              []treasury.PaymentBatch
	batchItems           []treasury.PaymentBatchItem
	rejectInvoices       map[int64]bool
	bumpRevisionOnExport bool
}

// allocationAwareRepo models the PostgreSQL amount-aware reservation seam for
// the service-level over-allocation tests without changing the legacy mock
// repository used by the rest of this package.
type allocationAwareRepo struct {
	*mockRepo
	invoiceBalances map[int64]treasury.Amount
}

func (m *allocationAwareRepo) APInvoiceAllocationAvailable(_ context.Context, invoiceID, _ int64, _ int64, _ string, batchID int64, amount treasury.Amount) (bool, error) {
	balance, ok := m.invoiceBalances[invoiceID]
	if !ok {
		return true, nil
	}
	reserved := treasury.MustParseAmount("0")
	for _, item := range m.batchItems {
		if item.APInvoiceID == nil || *item.APInvoiceID != invoiceID || item.BatchID == batchID || item.Status != "ACTIVE" {
			continue
		}
		var batch *treasury.PaymentBatch
		for i := range m.batches {
			if m.batches[i].ID == item.BatchID {
				batch = &m.batches[i]
				break
			}
		}
		if batch == nil {
			continue
		}
		switch batch.Status {
		case "DRAFT", "PENDING_APPROVAL", "APPROVED", "EXPORTED", "PROCESSING":
			var err error
			reserved, err = reserved.Add(item.Amount)
			if err != nil {
				return false, err
			}
		}
	}
	total, err := reserved.Add(amount)
	if err != nil {
		return false, err
	}
	cmp, err := total.Cmp(balance)
	return cmp <= 0, err
}

func (m *mockRepo) SupplierBelongsToCompany(_ context.Context, supplierID, companyID int64) (bool, error) {
	// The test repository models the supplier fixture used by the treasury
	// tests without importing the supplier module's persistence types.
	return supplierID == 100 && companyID == 1, nil
}

func (m *mockRepo) CreateSupplierBankAccount(_ context.Context, input treasury.SupplierBankAccountCreate) (treasury.SupplierBankAccount, error) {
	account := treasury.SupplierBankAccount{
		ID:                 int64(len(m.accounts) + 1),
		CompanyID:          input.CompanyID,
		SupplierID:         input.SupplierID,
		BankName:           input.BankName,
		AccountNumber:      input.AccountNumber,
		RoutingNumber:      input.RoutingNumber,
		Currency:           input.Currency,
		EffectiveFrom:      input.EffectiveFrom,
		EffectiveTo:        input.EffectiveTo,
		VerificationStatus: "PENDING_APPROVAL",
		EvidenceRef:        input.EvidenceRef,
		HoldPayments:       true,
		CreatedBy:          input.CreatedBy,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	m.accounts = append(m.accounts, account)
	return account, nil
}

func (m *mockRepo) UpdateSupplierBankAccountVerification(_ context.Context, input treasury.SupplierBankAccountVerificationUpdate) (treasury.SupplierBankAccount, error) {
	for i, account := range m.accounts {
		if account.ID == input.ID {
			m.accounts[i].VerificationStatus = input.VerificationStatus
			m.accounts[i].HoldPayments = input.HoldPayments
			m.accounts[i].ApprovedBy = input.ApprovedBy
			return m.accounts[i], nil
		}
	}
	return treasury.SupplierBankAccount{}, nil
}

func (m *mockRepo) GetSupplierBankAccount(_ context.Context, id int64) (treasury.SupplierBankAccount, error) {
	for _, account := range m.accounts {
		if account.ID == id {
			return account, nil
		}
	}
	return treasury.SupplierBankAccount{}, nil
}

func (m *mockRepo) ListSupplierBankAccounts(_ context.Context, filter treasury.SupplierBankAccountFilter) ([]treasury.SupplierBankAccount, error) {
	var result []treasury.SupplierBankAccount
	for _, account := range m.accounts {
		if account.SupplierID == filter.SupplierID && account.CompanyID == filter.CompanyID {
			result = append(result, account)
		}
	}
	return result, nil
}

func (m *mockRepo) GetPaymentPolicy(context.Context, int64) (treasury.PaymentPolicy, error) {
	return m.policy, nil
}

func (m *mockRepo) APInvoiceEligibleForPayment(_ context.Context, invoiceID, _ int64, _ int64, _ string) (bool, error) {
	return !m.rejectInvoices[invoiceID], nil
}

func (m *mockRepo) CreatePaymentBatch(_ context.Context, input treasury.PaymentBatchCreate) (treasury.PaymentBatch, error) {
	batch := treasury.PaymentBatch{
		ID:                  int64(len(m.batches) + 1),
		CompanyID:           input.CompanyID,
		ReferenceCode:       input.ReferenceCode,
		Currency:            input.Currency,
		ProposedBy:          input.ProposedBy,
		PaymentConnectionID: input.PaymentConnectionID,
		SourceBankAccountID: input.SourceBankAccountID,
		Status:              "DRAFT",
		RevisionNumber:      1,
	}
	m.batches = append(m.batches, batch)
	return batch, nil
}

func (m *mockRepo) GetPaymentBatch(_ context.Context, id int64) (treasury.PaymentBatch, error) {
	for _, batch := range m.batches {
		if batch.ID == id {
			return batch, nil
		}
	}
	return treasury.PaymentBatch{}, nil
}

func (m *mockRepo) UpdatePaymentBatchStatus(_ context.Context, input treasury.PaymentBatchStatusUpdate) (treasury.PaymentBatch, error) {
	for i, batch := range m.batches {
		if batch.ID == input.ID {
			m.batches[i].Status = input.Status
			m.batches[i].ApprovedBy = input.ApprovedBy
			m.batches[i].ApprovedAt = input.ApprovedAt
			return m.batches[i], nil
		}
	}
	return treasury.PaymentBatch{}, nil
}

func (m *mockRepo) UpdatePaymentBatchRevision(_ context.Context, input treasury.PaymentBatchRevisionUpdate) (treasury.PaymentBatch, error) {
	for i, batch := range m.batches {
		if batch.ID == input.ID {
			m.batches[i].RevisionNumber++
			m.batches[i].Status = "DRAFT"
			m.batches[i].ApprovedBy = nil
			m.batches[i].ApprovedAt = nil
			m.batches[i].ExportedFileHash = ""
			m.batches[i].ExportedBy = nil
			m.batches[i].TotalAmount = batchTotal(m.batchItems, input.ID)
			return m.batches[i], nil
		}
	}
	return treasury.PaymentBatch{}, nil
}

func (m *mockRepo) UpdatePaymentBatchTotal(_ context.Context, input treasury.PaymentBatchTotalUpdate) (treasury.PaymentBatch, error) {
	for i, batch := range m.batches {
		if batch.ID == input.ID {
			m.batches[i].TotalAmount = batchTotal(m.batchItems, input.ID)
			return m.batches[i], nil
		}
	}
	return treasury.PaymentBatch{}, nil
}

func batchTotal(items []treasury.PaymentBatchItem, batchID int64) treasury.Amount {
	total := treasury.MustParseAmount("0")
	for _, item := range items {
		if item.BatchID == batchID && item.Status == "ACTIVE" {
			var err error
			total, err = total.Add(item.Amount)
			if err != nil {
				return treasury.Amount("")
			}
		}
	}
	return total
}

func (m *mockRepo) UpdatePaymentBatchExport(_ context.Context, input treasury.PaymentBatchExportUpdate) (treasury.PaymentBatch, error) {
	for i, batch := range m.batches {
		if batch.ID == input.ID {
			if m.bumpRevisionOnExport {
				m.batches[i].RevisionNumber++
				m.bumpRevisionOnExport = false
				return treasury.PaymentBatch{}, treasury.ErrPaymentBatchRevisionConflict
			}
			if input.ExpectedRevisionNumber <= 0 || input.ExpectedRevisionNumber != batch.RevisionNumber {
				return treasury.PaymentBatch{}, treasury.ErrPaymentBatchRevisionConflict
			}
			m.batches[i].Status = "EXPORTED"
			m.batches[i].ExportedFileHash = input.ExportedFileHash
			m.batches[i].ExportedBy = input.ExportedBy
			m.batches[i].RevisionNumber = input.ExpectedRevisionNumber
			return m.batches[i], nil
		}
	}
	return treasury.PaymentBatch{}, nil
}

func (m *mockRepo) UpdatePaymentBatchSettlement(_ context.Context, input treasury.PaymentBatchSettlementUpdate) (treasury.PaymentBatch, error) {
	for i, batch := range m.batches {
		if batch.ID == input.ID {
			m.batches[i].Status = "SETTLED"
			m.batches[i].SettledBy = input.SettledBy
			return m.batches[i], nil
		}
	}
	return treasury.PaymentBatch{}, nil
}

func (m *mockRepo) CreatePaymentBatchItem(_ context.Context, input treasury.PaymentBatchItemCreate) (treasury.PaymentBatchItem, error) {
	item := treasury.PaymentBatchItem{
		ID:            int64(len(m.batchItems) + 1),
		BatchID:       input.BatchID,
		SupplierID:    input.SupplierID,
		BankAccountID: input.BankAccountID,
		Amount:        input.Amount,
		APInvoiceID:   input.APInvoiceID,
		Status:        "ACTIVE",
	}
	m.batchItems = append(m.batchItems, item)
	return item, nil
}

func (m *mockRepo) ListPaymentBatchItems(_ context.Context, batchID int64) ([]treasury.PaymentBatchItem, error) {
	var result []treasury.PaymentBatchItem
	for _, item := range m.batchItems {
		if item.BatchID == batchID && item.Status == "ACTIVE" {
			result = append(result, item)
		}
	}
	return result, nil
}

func (m *mockRepo) RemovePaymentBatchItem(_ context.Context, id int64) error {
	for i, item := range m.batchItems {
		if item.ID == id {
			m.batchItems[i].Status = "REMOVED"
		}
	}
	return nil
}

func TestServiceApproveBankAccount(t *testing.T) {
	repo := &mockRepo{policy: treasury.PaymentPolicy{RequiresMakerChecker: true}}
	svc := treasury.NewService(repo, nil, slog.Default())
	ctx := context.Background()

	account, err := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123456", "021000021", "USD", "doc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if account.VerificationStatus != "PENDING_APPROVAL" || !account.HoldPayments {
		t.Fatalf("new account should be pending and held: %+v", account)
	}
	if _, err = svc.ApproveBankAccount(ctx, 1, account.ID, 42); err == nil || err.Error() != "maker checker violation: creator cannot approve" {
		t.Fatalf("expected maker checker violation, got %v", err)
	}
	approved, err := svc.ApproveBankAccount(ctx, 1, account.ID, 43)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if approved.VerificationStatus != "VERIFIED" || approved.HoldPayments {
		t.Fatalf("account should be verified and released: %+v", approved)
	}
	canPay, err := svc.CanPaySupplier(ctx, 1, 100)
	if err != nil || !canPay {
		t.Fatalf("expected supplier to be payable, canPay=%v err=%v", canPay, err)
	}
}

func TestServiceCanPaySupplierHolds(t *testing.T) {
	repo := &mockRepo{}
	svc := treasury.NewService(repo, nil, slog.Default())
	_, err := svc.AddBankAccount(context.Background(), 1, 100, 42, "Chase", "123456", "021000021", "USD", "doc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	canPay, err := svc.CanPaySupplier(context.Background(), 1, 100)
	if err != nil || canPay {
		t.Fatalf("expected supplier not to be payable, canPay=%v err=%v", canPay, err)
	}
}

func TestServiceBatchApproval(t *testing.T) {
	repo := &mockRepo{policy: treasury.PaymentPolicy{RequiresMakerChecker: true}}
	svc := treasury.NewService(repo, nil, slog.Default())
	ctx := context.Background()
	account, _ := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123", "021", "USD", "doc")
	_, _ = svc.ApproveBankAccount(ctx, 1, account.ID, 43)
	batch, _ := svc.CreatePaymentBatch(ctx, 1, "BATCH-001", "USD", 44)
	if _, err := svc.AddBatchItem(ctx, 1, batch.ID, 100, account.ID, "100.50", 10); err != nil {
		t.Fatalf("unexpected error adding item: %v", err)
	}
	if _, err := svc.ApproveBatch(ctx, 1, batch.ID, 45); err == nil || err.Error() != "batch is not pending approval" {
		t.Fatalf("expected batch not pending approval error, got %v", err)
	}
	_, _ = repo.UpdatePaymentBatchStatus(ctx, treasury.PaymentBatchStatusUpdate{ID: batch.ID, Status: "PENDING_APPROVAL"})
	if _, err := svc.ApproveBatch(ctx, 1, batch.ID, 44); err == nil || err.Error() != "maker checker violation: proposer cannot approve" {
		t.Fatalf("expected maker checker violation, got %v", err)
	}
	approved, err := svc.ApproveBatch(ctx, 1, batch.ID, 45)
	if err != nil || approved.Status != "APPROVED" {
		t.Fatalf("expected approved batch, batch=%+v err=%v", approved, err)
	}
}

func TestServiceBatchApprovalEnforcesConfiguredPaymentLimits(t *testing.T) {
	repo := &mockRepo{
		policy: treasury.PaymentPolicy{
			MaxItemAmount:  treasury.MustParseAmount("100"),
			MaxBatchAmount: treasury.MustParseAmount("150"),
		},
	}
	svc := treasury.NewService(repo, nil, slog.Default())
	ctx := context.Background()
	account, err := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123", "021", "USD", "evidence-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApproveBankAccount(ctx, 1, account.ID, 43); err != nil {
		t.Fatal(err)
	}
	batch, err := svc.CreatePaymentBatch(ctx, 1, "BATCH-LIMIT", "USD", 44)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddBatchItem(ctx, 1, batch.ID, 100, account.ID, "100.01", 0); err != nil {
		t.Fatal(err)
	}
	_, _ = repo.UpdatePaymentBatchStatus(ctx, treasury.PaymentBatchStatusUpdate{ID: batch.ID, Status: "PENDING_APPROVAL"})
	if _, err := svc.ApproveBatch(ctx, 1, batch.ID, 45); !errors.Is(err, treasury.ErrPaymentItemLimit) {
		t.Fatalf("ApproveBatch() error = %v, want item-limit rejection", err)
	}

	// A second batch exercises the batch-level threshold after each item is
	// individually within the configured limit.
	batch, err = svc.CreatePaymentBatch(ctx, 1, "BATCH-LIMIT-TOTAL", "USD", 44)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddBatchItem(ctx, 1, batch.ID, 100, account.ID, "100", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddBatchItem(ctx, 1, batch.ID, 100, account.ID, "51", 0); err != nil {
		t.Fatal(err)
	}
	_, _ = repo.UpdatePaymentBatchStatus(ctx, treasury.PaymentBatchStatusUpdate{ID: batch.ID, Status: "PENDING_APPROVAL"})
	if _, err := svc.ApproveBatch(ctx, 1, batch.ID, 45); !errors.Is(err, treasury.ErrPaymentBatchLimit) {
		t.Fatalf("ApproveBatch() error = %v, want batch-limit rejection", err)
	}
}

func TestServiceBatchApprovalRejectsOverAllocatedInvoiceReservations(t *testing.T) {
	base := &mockRepo{}
	repo := &allocationAwareRepo{mockRepo: base, invoiceBalances: map[int64]treasury.Amount{10: treasury.MustParseAmount("100")}}
	svc := treasury.NewService(repo, nil, slog.Default())
	ctx := context.Background()
	account, err := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123", "021", "USD", "evidence")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApproveBankAccount(ctx, 1, account.ID, 43); err != nil {
		t.Fatal(err)
	}
	first, err := svc.CreatePaymentBatch(ctx, 1, "BATCH-ALLOC-1", "USD", 44)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.CreatePaymentBatch(ctx, 1, "BATCH-ALLOC-2", "USD", 45)
	if err != nil {
		t.Fatal(err)
	}
	invoiceID := int64(10)
	if _, err := svc.AddBatchItem(ctx, 1, first.ID, 100, account.ID, "60", 10); err != nil {
		t.Fatal(err)
	}
	// Simulate a draft item written by a legacy producer before the
	// amount-aware reservation capability was introduced. Approval must still
	// count that active draft rather than silently allowing an over-allocation.
	repo.batchItems = append(repo.batchItems, treasury.PaymentBatchItem{
		ID:            2,
		BatchID:       second.ID,
		SupplierID:    100,
		BankAccountID: account.ID,
		Amount:        treasury.MustParseAmount("60"),
		APInvoiceID:   &invoiceID,
		Status:        "ACTIVE",
	})
	_, _ = repo.UpdatePaymentBatchStatus(ctx, treasury.PaymentBatchStatusUpdate{ID: first.ID, Status: "PENDING_APPROVAL"})
	_, _ = repo.UpdatePaymentBatchStatus(ctx, treasury.PaymentBatchStatusUpdate{ID: second.ID, Status: "PENDING_APPROVAL"})
	if _, err := svc.ApproveBatch(ctx, 1, first.ID, 46); !errors.Is(err, treasury.ErrPaymentInvoiceAllocationLimit) {
		t.Fatalf("ApproveBatch() error = %v, want invoice allocation rejection", err)
	}
}

func TestServiceRequiresEvidenceBeforeBeneficiaryApproval(t *testing.T) {
	repo := &mockRepo{policy: treasury.PaymentPolicy{RequiresMakerChecker: true}}
	svc := treasury.NewService(repo, nil, slog.Default())
	ctx := context.Background()
	if _, err := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123", "021", "USD", " "); !errors.Is(err, treasury.ErrPaymentEvidenceRequired) {
		t.Fatalf("AddBankAccount() error = %v, want evidence requirement", err)
	}
	repo.accounts = append(repo.accounts, treasury.SupplierBankAccount{
		ID: 1, CompanyID: 1, SupplierID: 100, BankName: "Chase", AccountNumber: "123",
		Currency: "USD", EffectiveFrom: time.Now().Add(-time.Hour), VerificationStatus: "PENDING_APPROVAL",
		HoldPayments: true, CreatedBy: 42,
	})
	if _, err := svc.ApproveBankAccount(ctx, 1, 1, 43); !errors.Is(err, treasury.ErrPaymentEvidenceRequired) {
		t.Fatalf("ApproveBankAccount() error = %v, want evidence requirement", err)
	}
}

func TestServiceExportRevalidatesPolicyAndDuties(t *testing.T) {
	repo := &mockRepo{policy: treasury.PaymentPolicy{BankFormat: "iso-20022"}}
	settings := automation.DefaultSettings(1)
	settings.PaymentMakerCheckerEnabled = true
	settings.PaymentExecutorSeparationEnabled = true
	settingsReader := &automationSettingsReaderFake{settings: settings}
	svc := treasury.NewService(repo, nil, slog.Default(), settingsReader)
	ctx := context.Background()
	account, err := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123", "021", "USD", "evidence-2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApproveBankAccount(ctx, 1, account.ID, 43); err != nil {
		t.Fatal(err)
	}
	batch, err := svc.CreatePaymentBatch(ctx, 1, "BATCH-EXPORT-CONTROLS", "USD", 44)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddBatchItem(ctx, 1, batch.ID, 100, account.ID, "25", 0); err != nil {
		t.Fatal(err)
	}
	approvedBy := int64(45)
	_, _ = repo.UpdatePaymentBatchStatus(ctx, treasury.PaymentBatchStatusUpdate{ID: batch.ID, Status: "APPROVED", ApprovedBy: &approvedBy})
	if _, err := svc.ExportBatch(ctx, 1, batch.ID, 44, &treasury.CSVEncoder{}); !errors.Is(err, automation.ErrIncompatiblePaymentDuties) {
		t.Fatalf("ExportBatch() error = %v, want executor separation rejection", err)
	}
	if _, err := svc.ExportBatch(ctx, 1, batch.ID, 46, &treasury.CSVEncoder{}); !errors.Is(err, treasury.ErrBankFormatMismatch) {
		t.Fatalf("ExportBatch() error = %v, want bank format rejection", err)
	}
}

func TestPaymentPolicyValidationRejectsInvalidCutoff(t *testing.T) {
	cutoff := 24 * time.Hour
	if err := (treasury.PaymentPolicy{CutOffTime: &cutoff}).Validate(); !errors.Is(err, treasury.ErrInvalidPaymentPolicy) {
		t.Fatalf("Validate() error = %v, want invalid cutoff", err)
	}
}

func TestServiceBatchTotalsUseAllActiveItems(t *testing.T) {
	repo := &mockRepo{}
	svc := treasury.NewService(repo, nil, slog.Default())
	ctx := context.Background()
	account, _ := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123", "021", "USD", "doc")
	_, _ = svc.ApproveBankAccount(ctx, 1, account.ID, 43)
	batch, _ := svc.CreatePaymentBatch(ctx, 1, "BATCH-MULTI", "USD", 44)
	if _, err := svc.AddBatchItem(ctx, 1, batch.ID, 100, account.ID, "100", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddBatchItem(ctx, 1, batch.ID, 100, account.ID, "25", 0); err != nil {
		t.Fatal(err)
	}
	if repo.batches[0].TotalAmount.String() != "125" {
		t.Fatalf("batch total after two items = %v, want 125", repo.batches[0].TotalAmount)
	}
	_, _ = repo.UpdatePaymentBatchStatus(ctx, treasury.PaymentBatchStatusUpdate{ID: batch.ID, Status: "PENDING_APPROVAL"})
	approved, err := svc.ApproveBatch(ctx, 1, batch.ID, 45)
	if err != nil {
		t.Fatal(err)
	}
	if approved.TotalAmount.String() != "125" || approved.Status != "APPROVED" {
		t.Fatalf("approved batch = %+v, want total 125 and APPROVED", approved)
	}
}

func TestServiceBatchRevisionClearsPriorApprovalSnapshot(t *testing.T) {
	repo := &mockRepo{}
	svc := treasury.NewService(repo, nil, slog.Default())
	ctx := context.Background()
	account, err := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123", "021", "USD", "evidence")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApproveBankAccount(ctx, 1, account.ID, 43); err != nil {
		t.Fatal(err)
	}
	batch, err := svc.CreatePaymentBatch(ctx, 1, "BATCH-REVISION", "USD", 44)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddBatchItem(ctx, 1, batch.ID, 100, account.ID, "25", 0); err != nil {
		t.Fatal(err)
	}
	approvedBy := int64(45)
	_, _ = repo.UpdatePaymentBatchStatus(ctx, treasury.PaymentBatchStatusUpdate{ID: batch.ID, Status: "APPROVED", ApprovedBy: &approvedBy})
	if repo.batches[0].ApprovedBy == nil {
		t.Fatal("fixture approval was not recorded")
	}
	if _, err := svc.AddBatchItem(ctx, 1, batch.ID, 100, account.ID, "10", 0); err != nil {
		t.Fatal(err)
	}
	if repo.batches[0].Status != "DRAFT" || repo.batches[0].ApprovedBy != nil || repo.batches[0].ApprovedAt != nil {
		t.Fatalf("revised batch retained approval: %+v", repo.batches[0])
	}
}

func TestServiceSubmitBatchRequiresProposerAndItems(t *testing.T) {
	repo := &mockRepo{}
	svc := treasury.NewService(repo, nil, slog.Default())
	ctx := context.Background()
	batch, err := svc.CreatePaymentBatch(ctx, 1, "BATCH-SUBMIT", "USD", 44)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SubmitBatch(ctx, 1, batch.ID, 45); err == nil {
		t.Fatal("expected a non-proposer to be rejected")
	}
	if _, err := svc.SubmitBatch(ctx, 1, batch.ID, 44); err == nil {
		t.Fatal("expected an empty batch to be rejected")
	}
	account, err := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123", "021", "USD", "evidence")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApproveBankAccount(ctx, 1, account.ID, 43); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddBatchItem(ctx, 1, batch.ID, 100, account.ID, "25", 0); err != nil {
		t.Fatal(err)
	}
	submitted, err := svc.SubmitBatch(ctx, 1, batch.ID, 44)
	if err != nil {
		t.Fatal(err)
	}
	if submitted.Status != "PENDING_APPROVAL" || submitted.TotalAmount.String() != "25" {
		t.Fatalf("submitted batch = %+v, want pending total 25", submitted)
	}
}

func TestServiceRejectsForeignOrPaidAPInvoice(t *testing.T) {
	repo := &mockRepo{rejectInvoices: map[int64]bool{77: true}}
	svc := treasury.NewService(repo, nil, slog.Default())
	ctx := context.Background()
	account, _ := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123", "021", "USD", "doc")
	_, _ = svc.ApproveBankAccount(ctx, 1, account.ID, 43)
	batch, _ := svc.CreatePaymentBatch(ctx, 1, "BATCH-INVOICE", "USD", 44)
	if _, err := svc.AddBatchItem(ctx, 1, batch.ID, 100, account.ID, "25", 77); err == nil {
		t.Fatal("expected ineligible AP invoice to be rejected")
	}
}

func TestServiceExportBatch(t *testing.T) {
	repo := &mockRepo{}
	svc := treasury.NewService(repo, nil, slog.Default())
	ctx := context.Background()
	account, _ := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123", "021", "USD", "doc")
	_, _ = svc.ApproveBankAccount(ctx, 1, account.ID, 43)
	batch, _ := svc.CreatePaymentBatch(ctx, 1, "BATCH-002", "USD", 44)
	_, _ = svc.AddBatchItem(ctx, 1, batch.ID, 100, account.ID, "250", 11)
	approvedBy := int64(45)
	_, _ = repo.UpdatePaymentBatchStatus(ctx, treasury.PaymentBatchStatusUpdate{ID: batch.ID, Status: "APPROVED", ApprovedBy: &approvedBy})
	payload, err := svc.ExportBatch(ctx, 1, batch.ID, 46, &treasury.CSVEncoder{})
	if err != nil || len(payload) == 0 {
		t.Fatalf("expected export payload, len=%d err=%v", len(payload), err)
	}
}

func TestServiceExportBatchRejectsRevisionChangedDuringEncoding(t *testing.T) {
	repo := &mockRepo{bumpRevisionOnExport: true}
	svc := treasury.NewService(repo, nil, slog.Default())
	ctx := context.Background()
	account, err := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123", "021", "USD", "evidence")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApproveBankAccount(ctx, 1, account.ID, 43); err != nil {
		t.Fatal(err)
	}
	batch, err := svc.CreatePaymentBatch(ctx, 1, "BATCH-CAS", "USD", 44)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddBatchItem(ctx, 1, batch.ID, 100, account.ID, "25", 0); err != nil {
		t.Fatal(err)
	}
	approvedBy := int64(45)
	if _, err := repo.UpdatePaymentBatchStatus(ctx, treasury.PaymentBatchStatusUpdate{ID: batch.ID, Status: "APPROVED", ApprovedBy: &approvedBy}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExportBatch(ctx, 1, batch.ID, 46, &treasury.CSVEncoder{}); !errors.Is(err, treasury.ErrPaymentBatchRevisionConflict) {
		t.Fatalf("ExportBatch() error = %v, want revision conflict", err)
	}
	current, err := repo.GetPaymentBatch(ctx, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status == "EXPORTED" {
		t.Fatalf("stale export changed batch state: %+v", current)
	}
}

type mockAP struct{ paid []int64 }

func (m *mockAP) MarkInvoicePaid(_ context.Context, invoiceID, _ int64, _ treasury.Amount) error {
	m.paid = append(m.paid, invoiceID)
	return nil
}

type executionEnqueuerFake struct {
	called bool
}

func (f *executionEnqueuerFake) EnqueueBatchExecution(_ context.Context, companyID, batchID, _ int64) (treasury.ExecutionBatchResult, error) {
	f.called = true
	return treasury.ExecutionBatchResult{BatchID: batchID, CommandCount: 1, CommandIDs: []int64{companyID}}, nil
}

type automationSettingsReaderFake struct {
	settings automation.Settings
	err      error
	company  int64
}

func (f *automationSettingsReaderFake) Settings(_ context.Context, companyID int64) (automation.Settings, error) {
	f.company = companyID
	if f.err != nil {
		return automation.Settings{}, f.err
	}
	return f.settings, nil
}

func TestServiceSettleBatch(t *testing.T) {
	repo := &mockRepo{}
	ap := &mockAP{}
	svc := treasury.NewService(repo, ap, slog.Default())
	ctx := context.Background()
	account, _ := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123", "021", "USD", "doc")
	_, _ = svc.ApproveBankAccount(ctx, 1, account.ID, 43)
	batch, _ := svc.CreatePaymentBatch(ctx, 1, "BATCH-003", "USD", 44)
	_, _ = svc.AddBatchItem(ctx, 1, batch.ID, 100, account.ID, "300", 99)
	_, _ = repo.UpdatePaymentBatchStatus(ctx, treasury.PaymentBatchStatusUpdate{ID: batch.ID, Status: "EXPORTED"})

	settled, err := svc.SettleBatch(ctx, 1, batch.ID, 47)
	if err != nil || settled.Status != "SETTLED" {
		t.Fatalf("expected settled batch, batch=%+v err=%v", settled, err)
	}
	if len(ap.paid) != 1 || ap.paid[0] != 99 {
		t.Fatalf("expected AP invoice 99 to be marked paid, got %v", ap.paid)
	}
}

func TestValidateLiveExecutionBoundaryRequiresProviderAndSourceAccount(t *testing.T) {
	svc := treasury.NewService(&mockRepo{}, nil, slog.Default())
	base := treasury.PaymentBatch{ID: 1, CompanyID: 7, Status: "APPROVED"}
	if err := svc.ValidateLiveExecutionBoundary(base); err == nil {
		t.Fatal("expected missing provider/source references to block execution")
	}
	connectionID, bankID := int64(11), int64(12)
	base.PaymentConnectionID = &connectionID
	base.SourceBankAccountID = &bankID
	if err := svc.ValidateLiveExecutionBoundary(base); err != nil {
		t.Fatalf("configured approved batch rejected: %v", err)
	}
	base.Status = "DRAFT"
	if err := svc.ValidateLiveExecutionBoundary(base); err == nil {
		t.Fatal("expected non-approved batch to block execution")
	}
}

func TestServiceExecuteBatchEnqueuesAndMarksProcessing(t *testing.T) {
	repo := &mockRepo{}
	connectionID, bankID := int64(11), int64(12)
	batch, _ := repo.CreatePaymentBatch(context.Background(), treasury.PaymentBatchCreate{
		CompanyID: 1, ReferenceCode: "EXEC-1", Currency: "USD", ProposedBy: 4,
		PaymentConnectionID: &connectionID, SourceBankAccountID: &bankID,
	})
	_, _ = repo.UpdatePaymentBatchStatus(context.Background(), treasury.PaymentBatchStatusUpdate{ID: batch.ID, Status: "APPROVED"})
	enqueuer := &executionEnqueuerFake{}
	settings := automation.DefaultSettings(1)
	settings.PaymentSchedulingEnabled = true
	settings.PaymentExecutionEnabled = true
	settingsReader := &automationSettingsReaderFake{settings: settings}
	svc := treasury.NewService(repo, nil, slog.Default(), settingsReader)
	svc.SetExecutionEnqueuer(enqueuer)
	result, err := svc.ExecuteBatch(context.Background(), 1, batch.ID, 9)
	if err != nil {
		t.Fatalf("ExecuteBatch() error = %v", err)
	}
	if !enqueuer.called || result.CommandCount != 1 {
		t.Fatalf("unexpected execution result: %+v called=%v", result, enqueuer.called)
	}
	updated, _ := repo.GetPaymentBatch(context.Background(), batch.ID)
	if updated.Status != "PROCESSING" {
		t.Fatalf("batch status = %q, want PROCESSING", updated.Status)
	}
	if settingsReader.company != 1 {
		t.Fatalf("settings loaded for company %d, want 1", settingsReader.company)
	}
}

func TestServiceExecuteBatchRejectsDisabledPaymentExecution(t *testing.T) {
	repo := &mockRepo{}
	connectionID, bankID := int64(11), int64(12)
	batch, _ := repo.CreatePaymentBatch(context.Background(), treasury.PaymentBatchCreate{
		CompanyID: 1, ReferenceCode: "EXEC-DISABLED", Currency: "USD", ProposedBy: 4,
		PaymentConnectionID: &connectionID, SourceBankAccountID: &bankID,
	})
	_, _ = repo.UpdatePaymentBatchStatus(context.Background(), treasury.PaymentBatchStatusUpdate{ID: batch.ID, Status: "APPROVED"})
	enqueuer := &executionEnqueuerFake{}
	svc := treasury.NewService(repo, nil, slog.Default(), &automationSettingsReaderFake{settings: automation.DefaultSettings(1)})
	svc.SetExecutionEnqueuer(enqueuer)
	if _, err := svc.ExecuteBatch(context.Background(), 1, batch.ID, 9); !errors.Is(err, automation.ErrPaymentExecutionDisabled) {
		t.Fatalf("ExecuteBatch() error = %v, want disabled execution", err)
	}
	if enqueuer.called {
		t.Fatal("disabled payment execution must not enqueue a provider command")
	}
	updated, _ := repo.GetPaymentBatch(context.Background(), batch.ID)
	if updated.Status != "APPROVED" {
		t.Fatalf("batch status = %q, want APPROVED", updated.Status)
	}
}
