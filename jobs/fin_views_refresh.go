package jobs

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RefreshFinancialViews refreshes GL-related materialized views.
func RefreshFinancialViews(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	if pool == nil {
		return nil
	}
	if _, err := pool.Exec(ctx, `REFRESH MATERIALIZED VIEW CONCURRENTLY gl_balances`); err != nil {
		if logger != nil {
			logger.Error("refresh gl_balances", slog.Any("error", err))
		}
		return err
	}
	if logger != nil {
		logger.Info("refreshed gl_balances", slog.String("job", "fin_views_refresh"))
	}
	return nil
}
