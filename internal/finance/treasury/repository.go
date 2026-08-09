package treasury

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// PGRepository adapts generated treasury rows to the storage-neutral treasury port.
type PGRepository struct {
	queries *sqlc.Queries
}

func NewPGRepository(db *pgxpool.Pool) *PGRepository {
	return &PGRepository{queries: sqlc.New(db)}
}

func (r *PGRepository) SupplierBelongsToCompany(ctx context.Context, supplierID, companyID int64) (bool, error) {
	return r.queries.SupplierBelongsToCompany(ctx, sqlc.SupplierBelongsToCompanyParams{
		ID:        supplierID,
		CompanyID: pgtype.Int8{Int64: companyID, Valid: companyID > 0},
	})
}

func (r *PGRepository) CreateSupplierBankAccount(ctx context.Context, input SupplierBankAccountCreate) (SupplierBankAccount, error) {
	row, err := r.queries.CreateTreasurySupplierBankAccount(ctx, sqlc.CreateTreasurySupplierBankAccountParams{
		CompanyID:     input.CompanyID,
		SupplierID:    input.SupplierID,
		BankName:      input.BankName,
		AccountNumber: input.AccountNumber,
		RoutingNumber: optionalText(input.RoutingNumber),
		Currency:      input.Currency,
		EffectiveFrom: pgtype.Timestamptz{Time: input.EffectiveFrom, Valid: !input.EffectiveFrom.IsZero()},
		EffectiveTo:   optionalTime(input.EffectiveTo),
		EvidenceRef:   optionalText(input.EvidenceRef),
		CreatedBy:     input.CreatedBy,
	})
	if err != nil {
		return SupplierBankAccount{}, err
	}
	return mapSupplierBankAccount(row), nil
}

func (r *PGRepository) UpdateSupplierBankAccountVerification(ctx context.Context, input SupplierBankAccountVerificationUpdate) (SupplierBankAccount, error) {
	row, err := r.queries.UpdateTreasurySupplierBankAccountVerification(ctx, sqlc.UpdateTreasurySupplierBankAccountVerificationParams{
		ID:                 input.ID,
		VerificationStatus: input.VerificationStatus,
		HoldPayments:       input.HoldPayments,
		ApprovedBy:         optionalInt(input.ApprovedBy),
	})
	if err != nil {
		return SupplierBankAccount{}, err
	}
	return mapSupplierBankAccount(row), nil
}

func (r *PGRepository) GetSupplierBankAccount(ctx context.Context, id int64) (SupplierBankAccount, error) {
	row, err := r.queries.GetTreasurySupplierBankAccount(ctx, id)
	if err != nil {
		return SupplierBankAccount{}, err
	}
	return mapSupplierBankAccount(row), nil
}

func (r *PGRepository) ListSupplierBankAccounts(ctx context.Context, filter SupplierBankAccountFilter) ([]SupplierBankAccount, error) {
	rows, err := r.queries.ListTreasurySupplierBankAccounts(ctx, sqlc.ListTreasurySupplierBankAccountsParams{
		SupplierID: filter.SupplierID,
		CompanyID:  filter.CompanyID,
	})
	if err != nil {
		return nil, err
	}
	accounts := make([]SupplierBankAccount, 0, len(rows))
	for _, row := range rows {
		accounts = append(accounts, mapSupplierBankAccount(row))
	}
	return accounts, nil
}

func (r *PGRepository) GetPaymentPolicy(ctx context.Context, companyID int64) (PaymentPolicy, error) {
	row, err := r.queries.GetTreasuryPaymentPolicy(ctx, companyID)
	if err != nil {
		return PaymentPolicy{}, err
	}
	return PaymentPolicy{RequiresMakerChecker: row.RequiresMakerChecker}, nil
}

func (r *PGRepository) APInvoiceEligibleForPayment(ctx context.Context, invoiceID, supplierID, companyID int64, currency string) (bool, error) {
	return r.queries.APInvoiceEligibleForTreasuryPayment(ctx, sqlc.APInvoiceEligibleForTreasuryPaymentParams{
		ID:         invoiceID,
		SupplierID: supplierID,
		CompanyID:  pgtype.Int8{Int64: companyID, Valid: companyID > 0},
		Currency:   currency,
	})
}

func (r *PGRepository) CreatePaymentBatch(ctx context.Context, input PaymentBatchCreate) (PaymentBatch, error) {
	row, err := r.queries.CreateTreasuryPaymentBatch(ctx, sqlc.CreateTreasuryPaymentBatchParams{
		CompanyID:     input.CompanyID,
		ReferenceCode: input.ReferenceCode,
		Currency:      input.Currency,
		ProposedBy:    input.ProposedBy,
	})
	if err != nil {
		return PaymentBatch{}, err
	}
	return mapPaymentBatch(row)
}

func (r *PGRepository) GetPaymentBatch(ctx context.Context, id int64) (PaymentBatch, error) {
	row, err := r.queries.GetTreasuryPaymentBatch(ctx, id)
	if err != nil {
		return PaymentBatch{}, err
	}
	return mapPaymentBatch(row)
}

func (r *PGRepository) UpdatePaymentBatchStatus(ctx context.Context, input PaymentBatchStatusUpdate) (PaymentBatch, error) {
	row, err := r.queries.UpdateTreasuryPaymentBatchStatus(ctx, sqlc.UpdateTreasuryPaymentBatchStatusParams{
		ID:         input.ID,
		Status:     input.Status,
		ApprovedBy: optionalInt(input.ApprovedBy),
		ApprovedAt: optionalTime(input.ApprovedAt),
	})
	if err != nil {
		return PaymentBatch{}, err
	}
	return mapPaymentBatch(row)
}

func (r *PGRepository) UpdatePaymentBatchRevision(ctx context.Context, input PaymentBatchRevisionUpdate) (PaymentBatch, error) {
	row, err := r.queries.UpdateTreasuryPaymentBatchRevision(ctx, sqlc.UpdateTreasuryPaymentBatchRevisionParams{
		BatchID:     input.ID,
		TotalAmount: numericOf(input.TotalAmount),
	})
	if err != nil {
		return PaymentBatch{}, err
	}
	return mapPaymentBatch(row)
}

func (r *PGRepository) UpdatePaymentBatchTotal(ctx context.Context, input PaymentBatchTotalUpdate) (PaymentBatch, error) {
	row, err := r.queries.UpdateTreasuryPaymentBatchTotal(ctx, sqlc.UpdateTreasuryPaymentBatchTotalParams{
		BatchID:     input.ID,
		TotalAmount: numericOf(input.TotalAmount),
	})
	if err != nil {
		return PaymentBatch{}, err
	}
	return mapPaymentBatch(row)
}

func (r *PGRepository) UpdatePaymentBatchExport(ctx context.Context, input PaymentBatchExportUpdate) (PaymentBatch, error) {
	row, err := r.queries.UpdateTreasuryPaymentBatchExport(ctx, sqlc.UpdateTreasuryPaymentBatchExportParams{
		ID:               input.ID,
		ExportedFileHash: optionalText(input.ExportedFileHash),
		ExportedBy:       optionalInt(input.ExportedBy),
	})
	if err != nil {
		return PaymentBatch{}, err
	}
	return mapPaymentBatch(row)
}

func (r *PGRepository) UpdatePaymentBatchSettlement(ctx context.Context, input PaymentBatchSettlementUpdate) (PaymentBatch, error) {
	row, err := r.queries.UpdateTreasuryPaymentBatchSettlement(ctx, sqlc.UpdateTreasuryPaymentBatchSettlementParams{
		ID:        input.ID,
		SettledBy: optionalInt(input.SettledBy),
	})
	if err != nil {
		return PaymentBatch{}, err
	}
	return mapPaymentBatch(row)
}

func (r *PGRepository) CreatePaymentBatchItem(ctx context.Context, input PaymentBatchItemCreate) (PaymentBatchItem, error) {
	row, err := r.queries.CreateTreasuryPaymentBatchItem(ctx, sqlc.CreateTreasuryPaymentBatchItemParams{
		BatchID:       input.BatchID,
		SupplierID:    input.SupplierID,
		BankAccountID: input.BankAccountID,
		Amount:        numericOf(input.Amount),
		ApInvoiceID:   optionalInt(input.APInvoiceID),
	})
	if err != nil {
		return PaymentBatchItem{}, err
	}
	return mapPaymentBatchItem(row)
}

func (r *PGRepository) ListPaymentBatchItems(ctx context.Context, batchID int64) ([]PaymentBatchItem, error) {
	rows, err := r.queries.ListTreasuryPaymentBatchItems(ctx, batchID)
	if err != nil {
		return nil, err
	}
	items := make([]PaymentBatchItem, 0, len(rows))
	for _, row := range rows {
		item, err := mapPaymentBatchItem(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *PGRepository) RemovePaymentBatchItem(ctx context.Context, id int64) error {
	return r.queries.RemoveTreasuryPaymentBatchItem(ctx, id)
}

func mapSupplierBankAccount(row sqlc.TreasurySupplierBankAccount) SupplierBankAccount {
	account := SupplierBankAccount{
		ID:                 row.ID,
		CompanyID:          row.CompanyID,
		SupplierID:         row.SupplierID,
		BankName:           row.BankName,
		AccountNumber:      row.AccountNumber,
		RoutingNumber:      row.RoutingNumber.String,
		Currency:           row.Currency,
		EffectiveFrom:      row.EffectiveFrom.Time,
		VerificationStatus: row.VerificationStatus,
		EvidenceRef:        row.EvidenceRef.String,
		HoldPayments:       row.HoldPayments,
		CreatedBy:          row.CreatedBy,
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
	}
	if row.EffectiveTo.Valid {
		account.EffectiveTo = timePtr(row.EffectiveTo.Time)
	}
	if row.ApprovedBy.Valid {
		account.ApprovedBy = int64Ptr(row.ApprovedBy.Int64)
	}
	return account
}

func mapPaymentBatch(row sqlc.TreasuryPaymentBatch) (PaymentBatch, error) {
	amount, err := numericFloat(row.TotalAmount)
	if err != nil {
		return PaymentBatch{}, err
	}
	batch := PaymentBatch{
		ID:               row.ID,
		CompanyID:        row.CompanyID,
		ReferenceCode:    row.ReferenceCode,
		Status:           row.Status,
		Currency:         row.Currency,
		TotalAmount:      amount,
		RevisionNumber:   row.RevisionNumber,
		ProposedBy:       row.ProposedBy,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
		ExportedFileHash: row.ExportedFileHash.String,
	}
	if row.ApprovedBy.Valid {
		batch.ApprovedBy = int64Ptr(row.ApprovedBy.Int64)
	}
	if row.ApprovedAt.Valid {
		batch.ApprovedAt = timePtr(row.ApprovedAt.Time)
	}
	if row.ExportedAt.Valid {
		batch.ExportedAt = timePtr(row.ExportedAt.Time)
	}
	if row.ExportedBy.Valid {
		batch.ExportedBy = int64Ptr(row.ExportedBy.Int64)
	}
	if row.SettledAt.Valid {
		batch.SettledAt = timePtr(row.SettledAt.Time)
	}
	if row.SettledBy.Valid {
		batch.SettledBy = int64Ptr(row.SettledBy.Int64)
	}
	return batch, nil
}

func mapPaymentBatchItem(row sqlc.TreasuryPaymentBatchItem) (PaymentBatchItem, error) {
	amount, err := numericFloat(row.Amount)
	if err != nil {
		return PaymentBatchItem{}, err
	}
	item := PaymentBatchItem{
		ID:            row.ID,
		BatchID:       row.BatchID,
		SupplierID:    row.SupplierID,
		BankAccountID: row.BankAccountID,
		Amount:        amount,
		Status:        row.Status,
		CreatedAt:     row.CreatedAt.Time,
	}
	if row.ApInvoiceID.Valid {
		item.APInvoiceID = int64Ptr(row.ApInvoiceID.Int64)
	}
	return item, nil
}

func numericOf(value float64) pgtype.Numeric {
	var number pgtype.Numeric
	_ = number.Scan(fmt.Sprintf("%.6f", value))
	return number
}

func numericFloat(value pgtype.Numeric) (float64, error) {
	converted, err := value.Float64Value()
	if err != nil || !converted.Valid {
		return 0, fmt.Errorf("invalid numeric value")
	}
	return converted.Float64, nil
}

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func optionalTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func optionalInt(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}
