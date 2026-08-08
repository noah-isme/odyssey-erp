package banking

import (
	"context"
	"time"

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

func (r *PGRepository) CreateStatementImportRun(ctx context.Context, arg sqlc.CreateStatementImportRunParams) (sqlc.StatementImportRun, error) {
	return r.queries.CreateStatementImportRun(ctx, arg)
}

func (r *PGRepository) CreateBankStatement(ctx context.Context, arg sqlc.CreateBankStatementParams) (sqlc.BankStatement, error) {
	return r.queries.CreateBankStatement(ctx, arg)
}

func (r *PGRepository) CreateBankStatementLine(ctx context.Context, arg sqlc.CreateBankStatementLineParams) (sqlc.BankStatementLine, error) {
	return r.queries.CreateBankStatementLine(ctx, arg)
}

func (r *PGRepository) FindOpenPeriod(ctx context.Context, companyID int64, date time.Time) (int64, error) {
	var periodID int64
	err := r.pool.QueryRow(ctx, `
		SELECT period_id
		FROM accounting_periods
		WHERE (company_id = $1 OR company_id IS NULL)
		  AND start_date <= $2 AND end_date >= $2 AND status = 'OPEN'
		ORDER BY (company_id = $1) DESC, id DESC
		LIMIT 1`, companyID, date).Scan(&periodID)
	return periodID, err
}

func (r *PGRepository) BankTransactionExists(ctx context.Context, bankAccountID int64, externalRef string, fingerprint string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM bank_transactions
			WHERE bank_account_id = $1 
			  AND ((external_reference = $2 AND external_reference IS NOT NULL AND external_reference != '')
			       OR (fingerprint = $3 AND fingerprint IS NOT NULL AND fingerprint != ''))
		)`, bankAccountID, externalRef, fingerprint).Scan(&exists)
	return exists, err
}
