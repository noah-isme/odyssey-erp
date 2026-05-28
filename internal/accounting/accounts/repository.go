package accounts

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/reports"
)

type Repository interface {
	List(ctx context.Context) ([]Account, error)
	ListBalances(ctx context.Context) ([]reports.AccountBalance, error)
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) List(ctx context.Context) ([]Account, error) {
	rows, err := r.db.Query(ctx, `SELECT id, code, name, type, parent_id, is_active, created_at, updated_at FROM accounts ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []Account
	for rows.Next() {
		var a Account
		err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.Type, &a.ParentID, &a.IsActive, &a.CreatedAt, &a.UpdatedAt)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (r *repository) ListBalances(ctx context.Context) ([]reports.AccountBalance, error) {
	// Simple aggregation of journal lines. In a production app, we'd use a summary table or MV.
	rows, err := r.db.Query(ctx, `
		SELECT a.id, a.code, a.name, a.type,
		       0.0 AS opening,
		       COALESCE(SUM(jl.debit), 0)::double precision AS debit,
		       COALESCE(SUM(jl.credit), 0)::double precision AS credit
		FROM accounts a
		LEFT JOIN journal_lines jl ON a.id = jl.account_id
		GROUP BY a.id, a.code, a.name, a.type
		ORDER BY a.code
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var balances []reports.AccountBalance
	for rows.Next() {
		var b reports.AccountBalance
		err := rows.Scan(&b.ID, &b.Code, &b.Name, &b.Type, &b.Opening, &b.Debit, &b.Credit)
		if err != nil {
			return nil, err
		}
		balances = append(balances, b)
	}
	return balances, rows.Err()
}
