package banking

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// PGRepository implements Repository using PostgreSQL.
type PGRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewRepository creates a new repository.
func NewRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

func (r *PGRepository) CreateBankAccount(ctx context.Context, arg sqlc.CreateBankAccountParams) (sqlc.BankAccount, error) {
	return r.queries.CreateBankAccount(ctx, arg)
}

func (r *PGRepository) GetBankAccount(ctx context.Context, id int64) (sqlc.BankAccount, error) {
	return r.queries.GetBankAccount(ctx, id)
}

func (r *PGRepository) ListBankAccounts(ctx context.Context, companyID int64) ([]sqlc.BankAccount, error) {
	return r.queries.ListBankAccounts(ctx, companyID)
}

func (r *PGRepository) UpdateBankAccount(ctx context.Context, arg sqlc.UpdateBankAccountParams) error {
	return r.queries.UpdateBankAccount(ctx, arg)
}

func (r *PGRepository) CreateBankTransaction(ctx context.Context, arg sqlc.CreateBankTransactionParams) (sqlc.BankTransaction, error) {
	return r.queries.CreateBankTransaction(ctx, arg)
}

func (r *PGRepository) GetBankTransaction(ctx context.Context, id pgtype.UUID) (sqlc.BankTransaction, error) {
	return r.queries.GetBankTransaction(ctx, id)
}

func (r *PGRepository) ListBankTransactions(ctx context.Context, bankAccountID int64) ([]sqlc.BankTransaction, error) {
	return r.queries.ListBankTransactions(ctx, bankAccountID)
}

func (r *PGRepository) UpdateBankTransactionStatus(ctx context.Context, arg sqlc.UpdateBankTransactionStatusParams) error {
	return r.queries.UpdateBankTransactionStatus(ctx, arg)
}
