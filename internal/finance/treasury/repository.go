package treasury

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

type PGRepository struct {
	db      *pgxpool.Pool
	queries *sqlc.Queries
}

func NewPGRepository(db *pgxpool.Pool) *PGRepository {
	return &PGRepository{
		db:      db,
		queries: sqlc.New(db),
	}
}

func (r *PGRepository) CreateSupplierBankAccount(ctx context.Context, arg sqlc.CreateTreasurySupplierBankAccountParams) (sqlc.TreasurySupplierBankAccount, error) {
	return r.queries.CreateTreasurySupplierBankAccount(ctx, arg)
}

func (r *PGRepository) UpdateSupplierBankAccountVerification(ctx context.Context, arg sqlc.UpdateTreasurySupplierBankAccountVerificationParams) (sqlc.TreasurySupplierBankAccount, error) {
	return r.queries.UpdateTreasurySupplierBankAccountVerification(ctx, arg)
}

func (r *PGRepository) GetSupplierBankAccount(ctx context.Context, id int64) (sqlc.TreasurySupplierBankAccount, error) {
	return r.queries.GetTreasurySupplierBankAccount(ctx, id)
}

func (r *PGRepository) ListSupplierBankAccounts(ctx context.Context, arg sqlc.ListTreasurySupplierBankAccountsParams) ([]sqlc.TreasurySupplierBankAccount, error) {
	return r.queries.ListTreasurySupplierBankAccounts(ctx, arg)
}

func (r *PGRepository) GetPaymentPolicy(ctx context.Context, companyID int64) (sqlc.TreasuryPaymentPolicy, error) {
	return r.queries.GetTreasuryPaymentPolicy(ctx, companyID)
}

func (r *PGRepository) CreatePaymentBatch(ctx context.Context, arg sqlc.CreateTreasuryPaymentBatchParams) (sqlc.TreasuryPaymentBatch, error) {
	return r.queries.CreateTreasuryPaymentBatch(ctx, arg)
}

func (r *PGRepository) GetPaymentBatch(ctx context.Context, id int64) (sqlc.TreasuryPaymentBatch, error) {
	return r.queries.GetTreasuryPaymentBatch(ctx, id)
}

func (r *PGRepository) UpdatePaymentBatchStatus(ctx context.Context, arg sqlc.UpdateTreasuryPaymentBatchStatusParams) (sqlc.TreasuryPaymentBatch, error) {
	return r.queries.UpdateTreasuryPaymentBatchStatus(ctx, arg)
}

func (r *PGRepository) UpdatePaymentBatchRevision(ctx context.Context, arg sqlc.UpdateTreasuryPaymentBatchRevisionParams) (sqlc.TreasuryPaymentBatch, error) {
	return r.queries.UpdateTreasuryPaymentBatchRevision(ctx, arg)
}

func (r *PGRepository) UpdatePaymentBatchExport(ctx context.Context, arg sqlc.UpdateTreasuryPaymentBatchExportParams) (sqlc.TreasuryPaymentBatch, error) {
	return r.queries.UpdateTreasuryPaymentBatchExport(ctx, arg)
}

func (r *PGRepository) UpdatePaymentBatchSettlement(ctx context.Context, arg sqlc.UpdateTreasuryPaymentBatchSettlementParams) (sqlc.TreasuryPaymentBatch, error) {
	return r.queries.UpdateTreasuryPaymentBatchSettlement(ctx, arg)
}

func (r *PGRepository) CreatePaymentBatchItem(ctx context.Context, arg sqlc.CreateTreasuryPaymentBatchItemParams) (sqlc.TreasuryPaymentBatchItem, error) {
	return r.queries.CreateTreasuryPaymentBatchItem(ctx, arg)
}

func (r *PGRepository) ListPaymentBatchItems(ctx context.Context, batchID int64) ([]sqlc.TreasuryPaymentBatchItem, error) {
	return r.queries.ListTreasuryPaymentBatchItems(ctx, batchID)
}

func (r *PGRepository) RemovePaymentBatchItem(ctx context.Context, id int64) error {
	return r.queries.RemoveTreasuryPaymentBatchItem(ctx, id)
}
