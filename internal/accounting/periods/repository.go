package periods

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/shared"
)

type Repository interface {
	FindOpenPeriodByDate(ctx context.Context, date time.Time) (Period, error)
	// Additional methods can be added as needed
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

// FindOpenPeriodByDate returns the open period covering the supplied date.
func (r *repository) FindOpenPeriodByDate(ctx context.Context, date time.Time) (Period, error) {
	var period Period
	err := r.db.QueryRow(ctx, `SELECT id, code, start_date, end_date, status, closed_at, locked_by, created_at, updated_at
FROM periods WHERE status='OPEN' AND $1 BETWEEN start_date AND end_date ORDER BY start_date LIMIT 1`, date).
		Scan(&period.ID, &period.Code, &period.StartDate, &period.EndDate, &period.Status, &period.ClosedAt, &period.LockedBy, &period.CreatedAt, &period.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Period{}, shared.ErrInvalidPeriod
		}
		return Period{}, err
	}
	return period, nil
}
