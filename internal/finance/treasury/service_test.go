package treasury_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/treasury"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

type mockRepo struct {
	accounts    []sqlc.TreasurySupplierBankAccount
	policy      sqlc.TreasuryPaymentPolicy
	batches     []sqlc.TreasuryPaymentBatch
	batchItems  []sqlc.TreasuryPaymentBatchItem
}

func (m *mockRepo) CreateSupplierBankAccount(ctx context.Context, arg sqlc.CreateTreasurySupplierBankAccountParams) (sqlc.TreasurySupplierBankAccount, error) {
	acc := sqlc.TreasurySupplierBankAccount{
		ID:                 int64(len(m.accounts) + 1),
		CompanyID:          arg.CompanyID,
		SupplierID:         arg.SupplierID,
		BankName:           arg.BankName,
		AccountNumber:      arg.AccountNumber,
		RoutingNumber:      arg.RoutingNumber,
		Currency:           arg.Currency,
		EffectiveFrom:      arg.EffectiveFrom,
		EffectiveTo:        arg.EffectiveTo,
		VerificationStatus: "PENDING_APPROVAL",
		EvidenceRef:        arg.EvidenceRef,
		HoldPayments:       true,
		CreatedBy:          arg.CreatedBy,
		CreatedAt:          pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UpdatedAt:          pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	m.accounts = append(m.accounts, acc)
	return acc, nil
}

func (m *mockRepo) UpdateSupplierBankAccountVerification(ctx context.Context, arg sqlc.UpdateTreasurySupplierBankAccountVerificationParams) (sqlc.TreasurySupplierBankAccount, error) {
	for i, acc := range m.accounts {
		if acc.ID == arg.ID {
			m.accounts[i].VerificationStatus = arg.VerificationStatus
			m.accounts[i].HoldPayments = arg.HoldPayments
			m.accounts[i].ApprovedBy = arg.ApprovedBy
			return m.accounts[i], nil
		}
	}
	return sqlc.TreasurySupplierBankAccount{}, nil
}

func (m *mockRepo) GetSupplierBankAccount(ctx context.Context, id int64) (sqlc.TreasurySupplierBankAccount, error) {
	for _, acc := range m.accounts {
		if acc.ID == id {
			return acc, nil
		}
	}
	return sqlc.TreasurySupplierBankAccount{}, nil
}

func (m *mockRepo) ListSupplierBankAccounts(ctx context.Context, arg sqlc.ListTreasurySupplierBankAccountsParams) ([]sqlc.TreasurySupplierBankAccount, error) {
	var res []sqlc.TreasurySupplierBankAccount
	for _, acc := range m.accounts {
		if acc.SupplierID == arg.SupplierID && acc.CompanyID == arg.CompanyID {
			res = append(res, acc)
		}
	}
	return res, nil
}

func (m *mockRepo) GetPaymentPolicy(ctx context.Context, companyID int64) (sqlc.TreasuryPaymentPolicy, error) {
	return m.policy, nil
}

func (m *mockRepo) CreatePaymentBatch(ctx context.Context, arg sqlc.CreateTreasuryPaymentBatchParams) (sqlc.TreasuryPaymentBatch, error) {
	batch := sqlc.TreasuryPaymentBatch{
		ID:            int64(len(m.batches) + 1),
		CompanyID:     arg.CompanyID,
		ReferenceCode: arg.ReferenceCode,
		Currency:      arg.Currency,
		ProposedBy:    arg.ProposedBy,
		Status:        "DRAFT",
		RevisionNumber: 1,
	}
	m.batches = append(m.batches, batch)
	return batch, nil
}

func (m *mockRepo) GetPaymentBatch(ctx context.Context, id int64) (sqlc.TreasuryPaymentBatch, error) {
	for _, b := range m.batches {
		if b.ID == id {
			return b, nil
		}
	}
	return sqlc.TreasuryPaymentBatch{}, nil
}

func (m *mockRepo) UpdatePaymentBatchStatus(ctx context.Context, arg sqlc.UpdateTreasuryPaymentBatchStatusParams) (sqlc.TreasuryPaymentBatch, error) {
	for i, b := range m.batches {
		if b.ID == arg.ID {
			m.batches[i].Status = arg.Status
			m.batches[i].ApprovedBy = arg.ApprovedBy
			m.batches[i].ApprovedAt = arg.ApprovedAt
			return m.batches[i], nil
		}
	}
	return sqlc.TreasuryPaymentBatch{}, nil
}

func (m *mockRepo) UpdatePaymentBatchRevision(ctx context.Context, arg sqlc.UpdateTreasuryPaymentBatchRevisionParams) (sqlc.TreasuryPaymentBatch, error) {
	for i, b := range m.batches {
		if b.ID == arg.ID {
			m.batches[i].RevisionNumber++
			m.batches[i].Status = "DRAFT"
			m.batches[i].TotalAmount = arg.TotalAmount
			return m.batches[i], nil
		}
	}
	return sqlc.TreasuryPaymentBatch{}, nil
}

func (m *mockRepo) UpdatePaymentBatchExport(ctx context.Context, arg sqlc.UpdateTreasuryPaymentBatchExportParams) (sqlc.TreasuryPaymentBatch, error) {
	for i, b := range m.batches {
		if b.ID == arg.ID {
			m.batches[i].Status = "EXPORTED"
			m.batches[i].ExportedFileHash = arg.ExportedFileHash
			m.batches[i].ExportedBy = arg.ExportedBy
			return m.batches[i], nil
		}
	}
	return sqlc.TreasuryPaymentBatch{}, nil
}

func (m *mockRepo) UpdatePaymentBatchSettlement(ctx context.Context, arg sqlc.UpdateTreasuryPaymentBatchSettlementParams) (sqlc.TreasuryPaymentBatch, error) {
	for i, b := range m.batches {
		if b.ID == arg.ID {
			m.batches[i].Status = "SETTLED"
			m.batches[i].SettledBy = arg.SettledBy
			return m.batches[i], nil
		}
	}
	return sqlc.TreasuryPaymentBatch{}, nil
}

func (m *mockRepo) CreatePaymentBatchItem(ctx context.Context, arg sqlc.CreateTreasuryPaymentBatchItemParams) (sqlc.TreasuryPaymentBatchItem, error) {
	item := sqlc.TreasuryPaymentBatchItem{
		ID:            int64(len(m.batchItems) + 1),
		BatchID:       arg.BatchID,
		SupplierID:    arg.SupplierID,
		BankAccountID: arg.BankAccountID,
		Amount:        arg.Amount,
		ApInvoiceID:   arg.ApInvoiceID,
		Status:        "ACTIVE",
	}
	m.batchItems = append(m.batchItems, item)
	return item, nil
}

func (m *mockRepo) ListPaymentBatchItems(ctx context.Context, batchID int64) ([]sqlc.TreasuryPaymentBatchItem, error) {
	var res []sqlc.TreasuryPaymentBatchItem
	for _, it := range m.batchItems {
		if it.BatchID == batchID && it.Status == "ACTIVE" {
			res = append(res, it)
		}
	}
	return res, nil
}

func (m *mockRepo) RemovePaymentBatchItem(ctx context.Context, id int64) error {
	for i, it := range m.batchItems {
		if it.ID == id {
			m.batchItems[i].Status = "REMOVED"
			return nil
		}
	}
	return nil
}

func TestService_ApproveBankAccount(t *testing.T) {
	repo := &mockRepo{
		policy: sqlc.TreasuryPaymentPolicy{
			RequiresMakerChecker: true,
		},
	}
	svc := treasury.NewService(repo, nil, slog.Default())
	ctx := context.Background()

	acc, err := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123456", "021000021", "USD", "doc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc.VerificationStatus != "PENDING_APPROVAL" {
		t.Errorf("expected PENDING_APPROVAL, got %s", acc.VerificationStatus)
	}
	if !acc.HoldPayments {
		t.Errorf("expected payments to be held on new account")
	}

	// Maker attempts to approve
	_, err = svc.ApproveBankAccount(ctx, acc.ID, 42)
	if err == nil || err.Error() != "maker checker violation: creator cannot approve" {
		t.Errorf("expected maker checker violation, got %v", err)
	}

	// Checker attempts to approve
	approvedAcc, err := svc.ApproveBankAccount(ctx, acc.ID, 43)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if approvedAcc.VerificationStatus != "VERIFIED" {
		t.Errorf("expected VERIFIED, got %s", approvedAcc.VerificationStatus)
	}
	if approvedAcc.HoldPayments {
		t.Errorf("expected hold payments to be false")
	}

	canPay, err := svc.CanPaySupplier(ctx, 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !canPay {
		t.Errorf("expected supplier to be payable")
	}
}

func TestService_CanPaySupplier_Holds(t *testing.T) {
	repo := &mockRepo{}
	svc := treasury.NewService(repo, nil, slog.Default())
	ctx := context.Background()

	_, err := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123456", "021000021", "USD", "doc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Without approval
	canPay, err := svc.CanPaySupplier(ctx, 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if canPay {
		t.Errorf("expected supplier NOT to be payable")
	}
}

func TestService_BatchApproval(t *testing.T) {
	repo := &mockRepo{
		policy: sqlc.TreasuryPaymentPolicy{
			RequiresMakerChecker: true,
		},
	}
	svc := treasury.NewService(repo, nil, slog.Default())
	ctx := context.Background()

	// Setup valid supplier
	acc, _ := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123", "021", "USD", "doc")
	_, _ = svc.ApproveBankAccount(ctx, acc.ID, 43)

	batch, _ := svc.CreatePaymentBatch(ctx, 1, "BATCH-001", "USD", 44)

	var num pgtype.Numeric
	_ = num.Scan("100.50")
	_, err := svc.AddBatchItem(ctx, batch.ID, 100, acc.ID, num, 10)
	if err != nil {
		t.Fatalf("unexpected error adding item: %v", err)
	}

	// Try to approve without moving to PENDING_APPROVAL
	_, err = svc.ApproveBatch(ctx, batch.ID, 45)
	if err == nil || err.Error() != "batch is not pending approval" {
		t.Errorf("expected batch not pending approval error")
	}

	// Move to pending approval
	_, _ = repo.UpdatePaymentBatchStatus(ctx, sqlc.UpdateTreasuryPaymentBatchStatusParams{
		ID:     batch.ID,
		Status: "PENDING_APPROVAL",
	})

	// Maker attempts to approve
	_, err = svc.ApproveBatch(ctx, batch.ID, 44)
	if err == nil || err.Error() != "maker checker violation: proposer cannot approve" {
		t.Errorf("expected maker checker violation, got %v", err)
	}

	// Valid approval
	approvedBatch, err := svc.ApproveBatch(ctx, batch.ID, 45)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if approvedBatch.Status != "APPROVED" {
		t.Errorf("expected APPROVED status")
	}
}

func TestService_ExportBatch(t *testing.T) {
	repo := &mockRepo{}
	svc := treasury.NewService(repo, nil, slog.Default())
	ctx := context.Background()

	// Setup valid supplier
	acc, _ := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123", "021", "USD", "doc")
	_, _ = svc.ApproveBankAccount(ctx, acc.ID, 43)

	batch, _ := svc.CreatePaymentBatch(ctx, 1, "BATCH-002", "USD", 44)

	var num pgtype.Numeric
	_ = num.Scan("250.00")
	_, _ = svc.AddBatchItem(ctx, batch.ID, 100, acc.ID, num, 11)

	_, _ = repo.UpdatePaymentBatchStatus(ctx, sqlc.UpdateTreasuryPaymentBatchStatusParams{
		ID:     batch.ID,
		Status: "APPROVED",
	})

	encoder := &treasury.CSVEncoder{}
	payload, err := svc.ExportBatch(ctx, batch.ID, 46, encoder)
	if err != nil {
		t.Fatalf("unexpected error exporting batch: %v", err)
	}

	if len(payload) == 0 {
		t.Errorf("expected payload")
	}
}

type mockAP struct {
	paid []int64
}

func (m *mockAP) MarkInvoicePaid(ctx context.Context, invoiceID, batchID int64, amount pgtype.Numeric) error {
	m.paid = append(m.paid, invoiceID)
	return nil
}

func TestService_SettleBatch(t *testing.T) {
	repo := &mockRepo{}
	ap := &mockAP{}
	svc := treasury.NewService(repo, ap, slog.Default())
	ctx := context.Background()

	acc, _ := svc.AddBankAccount(ctx, 1, 100, 42, "Chase", "123", "021", "USD", "doc")
	_, _ = svc.ApproveBankAccount(ctx, acc.ID, 43)

	batch, _ := svc.CreatePaymentBatch(ctx, 1, "BATCH-003", "USD", 44)

	var num pgtype.Numeric
	_ = num.Scan("300.00")
	_, _ = svc.AddBatchItem(ctx, batch.ID, 100, acc.ID, num, 99)

	_, _ = repo.UpdatePaymentBatchStatus(ctx, sqlc.UpdateTreasuryPaymentBatchStatusParams{
		ID:     batch.ID,
		Status: "EXPORTED",
	})

	settled, err := svc.SettleBatch(ctx, batch.ID, 47)
	if err != nil {
		t.Fatalf("unexpected error settling batch: %v", err)
	}

	if settled.Status != "SETTLED" {
		t.Errorf("expected SETTLED status")
	}

	if len(ap.paid) != 1 || ap.paid[0] != 99 {
		t.Errorf("expected AP invoice 99 to be marked paid")
	}
}
