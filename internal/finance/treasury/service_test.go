package treasury_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/treasury"
)

type mockRepo struct {
	accounts       []treasury.SupplierBankAccount
	policy         treasury.PaymentPolicy
	batches        []treasury.PaymentBatch
	batchItems     []treasury.PaymentBatchItem
	rejectInvoices map[int64]bool
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
		ID:             int64(len(m.batches) + 1),
		CompanyID:      input.CompanyID,
		ReferenceCode:  input.ReferenceCode,
		Currency:       input.Currency,
		ProposedBy:     input.ProposedBy,
		Status:         "DRAFT",
		RevisionNumber: 1,
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
			m.batches[i].Status = "EXPORTED"
			m.batches[i].ExportedFileHash = input.ExportedFileHash
			m.batches[i].ExportedBy = input.ExportedBy
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
	_, _ = repo.UpdatePaymentBatchStatus(ctx, treasury.PaymentBatchStatusUpdate{ID: batch.ID, Status: "APPROVED"})
	payload, err := svc.ExportBatch(ctx, 1, batch.ID, 46, &treasury.CSVEncoder{})
	if err != nil || len(payload) == 0 {
		t.Fatalf("expected export payload, len=%d err=%v", len(payload), err)
	}
}

type mockAP struct{ paid []int64 }

func (m *mockAP) MarkInvoicePaid(_ context.Context, invoiceID, _ int64, _ treasury.Amount) error {
	m.paid = append(m.paid, invoiceID)
	return nil
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
