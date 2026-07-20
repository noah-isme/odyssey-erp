package accounts

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/reports"
)

type Repository interface {
	List(ctx context.Context) ([]Account, error)
	ListBalances(ctx context.Context) ([]reports.AccountBalance, error)
	ListBalancesForPeriod(ctx context.Context, year int, month time.Month) ([]reports.AccountBalance, error)
	ListBalancesForPeriodAndDimensions(ctx context.Context, year int, month time.Month, filter reports.DimensionFilter) ([]reports.AccountBalance, error)
}

func (r *repository) ListBalancesForPeriod(ctx context.Context, year int, month time.Month) ([]reports.AccountBalance, error) {
	return r.ListBalancesForPeriodAndDimensions(ctx, year, month, reports.DimensionFilter{})
}

func (r *repository) ListBalancesForPeriodAndDimensions(ctx context.Context, year int, month time.Month, filter reports.DimensionFilter) ([]reports.AccountBalance, error) {
	start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	args := []any{start, end}
	where := "WHERE je.status = 'POSTED' AND je.date >= $1 AND je.date < $2"
	if filter.DepartmentID > 0 {
		args = append(args, filter.DepartmentID)
		where += " AND jl.department_id = $" + fmt.Sprint(len(args))
	}
	if filter.CostCenterID > 0 {
		args = append(args, filter.CostCenterID)
		where += " AND jl.cost_center_id = $" + fmt.Sprint(len(args))
	}
	rows, err := r.db.Query(ctx, `
		SELECT a.id, a.code, a.name, a.type,
		       0.0 AS opening,
		       COALESCE(SUM(jl.debit), 0)::double precision AS debit,
		       COALESCE(SUM(jl.credit), 0)::double precision AS credit
		FROM accounts a
		LEFT JOIN (
			SELECT jl.account_id, jl.debit, jl.credit
			FROM journal_lines jl
			JOIN journal_entries je ON je.id = jl.je_id
		`+where+`
		) jl ON a.id = jl.account_id
		GROUP BY a.id, a.code, a.name, a.type
		ORDER BY a.code`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	balances := make([]reports.AccountBalance, 0)
	for rows.Next() {
		var balance reports.AccountBalance
		if err := rows.Scan(&balance.ID, &balance.Code, &balance.Name, &balance.Type, &balance.Opening, &balance.Debit, &balance.Credit); err != nil {
			return nil, err
		}
		balances = append(balances, balance)
	}
	return balances, rows.Err()
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
