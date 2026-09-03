package accounting

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// BudgetEntry is the storage-neutral budget data consumed by accounting reports.
type BudgetEntry struct {
	ID        int64
	AccountID int64
	Amount    float64
}

// BudgetRepository provides the budget reads needed by the accounting HTTP boundary.
type BudgetRepository interface {
	ListBudgetsByPeriod(ctx context.Context, year, month int32) ([]BudgetEntry, error)
}

// PGBudgetRepository adapts generated SQLC budget rows to the accounting boundary.
type PGBudgetRepository struct {
	queries *sqlc.Queries
}

func NewBudgetRepository(pool *pgxpool.Pool) *PGBudgetRepository {
	return &PGBudgetRepository{queries: sqlc.New(pool)}
}

func (r *PGBudgetRepository) ListBudgetsByPeriod(ctx context.Context, year, month int32) ([]BudgetEntry, error) {
	rows, err := r.queries.ListBudgetsByPeriod(ctx, sqlc.ListBudgetsByPeriodParams{
		PeriodYear:  year,
		PeriodMonth: month,
	})
	if err != nil {
		return nil, err
	}

	entries := make([]BudgetEntry, 0, len(rows))
	for _, row := range rows {
		amount, err := row.Amount.Float64Value()
		if err != nil || !amount.Valid {
			if err == nil {
				err = fmt.Errorf("amount is not a valid number")
			}
			return nil, fmt.Errorf("budget %d: %w", row.ID, err)
		}
		entries = append(entries, BudgetEntry{ID: row.ID, AccountID: row.AccountID, Amount: amount.Float64})
	}
	return entries, nil
}

var _ BudgetRepository = (*PGBudgetRepository)(nil)
