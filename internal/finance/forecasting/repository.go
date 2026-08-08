package forecasting

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

func (r *PGRepository) CreateForecastRun(ctx context.Context, arg sqlc.CreateForecastRunParams) (sqlc.ForecastRun, error) {
	return r.queries.CreateForecastRun(ctx, arg)
}

func (r *PGRepository) UpdateForecastRunStatus(ctx context.Context, arg sqlc.UpdateForecastRunStatusParams) error {
	return r.queries.UpdateForecastRunStatus(ctx, arg)
}

func (r *PGRepository) CreateForecastDailyBucket(ctx context.Context, arg sqlc.CreateForecastDailyBucketParams) (sqlc.ForecastDailyBucket, error) {
	return r.queries.CreateForecastDailyBucket(ctx, arg)
}

func (r *PGRepository) CreateForecastSourceLine(ctx context.Context, arg sqlc.CreateForecastSourceLineParams) (sqlc.ForecastSourceLine, error) {
	return r.queries.CreateForecastSourceLine(ctx, arg)
}

func (r *PGRepository) GetLatestForecastRun(ctx context.Context, arg sqlc.GetLatestForecastRunParams) (sqlc.ForecastRun, error) {
	return r.queries.GetLatestForecastRun(ctx, arg)
}

func (r *PGRepository) ListForecastDailyBucketsByRun(ctx context.Context, runID int64) ([]sqlc.ForecastDailyBucket, error) {
	return r.queries.ListForecastDailyBucketsByRun(ctx, runID)
}
