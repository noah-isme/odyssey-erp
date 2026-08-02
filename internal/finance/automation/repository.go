package automation

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type database interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Repository persists company-scoped F0 settings. It intentionally does not
// expose provider credentials or implement any finance workflow.
type Repository struct{ pool database }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Settings(ctx context.Context, companyID int64) (Settings, error) {
	if companyID <= 0 {
		return Settings{}, ErrInvalidSettings
	}
	var settings Settings
	err := r.pool.QueryRow(ctx, `
		SELECT company_id, bank_feed_auto_sync_enabled, cash_forecast_enabled,
		       payment_scheduling_enabled, payment_execution_enabled,
		       p2p_auto_post_enabled, asset_operations_enabled,
		       forecast_horizon_weeks, bank_feed_sync_interval_minutes,
		       payment_maker_checker_enabled, payment_executor_separation_enabled,
		       COALESCE(updated_by, 0)
		FROM finance_automation_settings
		WHERE company_id = $1`, companyID).Scan(
		&settings.CompanyID,
		&settings.BankFeedAutoSyncEnabled,
		&settings.CashForecastEnabled,
		&settings.PaymentSchedulingEnabled,
		&settings.PaymentExecutionEnabled,
		&settings.P2PAutoPostEnabled,
		&settings.AssetOperationsEnabled,
		&settings.ForecastHorizonWeeks,
		&settings.BankFeedSyncIntervalMinutes,
		&settings.PaymentMakerCheckerEnabled,
		&settings.PaymentExecutorSeparationEnabled,
		&settings.UpdatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, ErrInvalidSettings
	}
	return settings, err
}

func (r *Repository) SaveSettings(ctx context.Context, settings Settings) (Settings, error) {
	if err := settings.Validate(); err != nil {
		return Settings{}, err
	}
	var saved Settings
	err := r.pool.QueryRow(ctx, `
		UPDATE finance_automation_settings
		SET bank_feed_auto_sync_enabled = $2,
		    cash_forecast_enabled = $3,
		    payment_scheduling_enabled = $4,
		    payment_execution_enabled = $5,
		    p2p_auto_post_enabled = $6,
		    asset_operations_enabled = $7,
		    forecast_horizon_weeks = $8,
		    bank_feed_sync_interval_minutes = $9,
		    payment_maker_checker_enabled = $10,
		    payment_executor_separation_enabled = $11,
		    updated_by = NULLIF($12, 0),
		    updated_at = NOW()
		WHERE company_id = $1
		RETURNING company_id, bank_feed_auto_sync_enabled, cash_forecast_enabled,
		          payment_scheduling_enabled, payment_execution_enabled,
		          p2p_auto_post_enabled, asset_operations_enabled,
		          forecast_horizon_weeks, bank_feed_sync_interval_minutes,
		          payment_maker_checker_enabled, payment_executor_separation_enabled,
		          COALESCE(updated_by, 0)`,
		settings.CompanyID,
		settings.BankFeedAutoSyncEnabled,
		settings.CashForecastEnabled,
		settings.PaymentSchedulingEnabled,
		settings.PaymentExecutionEnabled,
		settings.P2PAutoPostEnabled,
		settings.AssetOperationsEnabled,
		settings.ForecastHorizonWeeks,
		settings.BankFeedSyncIntervalMinutes,
		settings.PaymentMakerCheckerEnabled,
		settings.PaymentExecutorSeparationEnabled,
		settings.UpdatedBy,
	).Scan(
		&saved.CompanyID,
		&saved.BankFeedAutoSyncEnabled,
		&saved.CashForecastEnabled,
		&saved.PaymentSchedulingEnabled,
		&saved.PaymentExecutionEnabled,
		&saved.P2PAutoPostEnabled,
		&saved.AssetOperationsEnabled,
		&saved.ForecastHorizonWeeks,
		&saved.BankFeedSyncIntervalMinutes,
		&saved.PaymentMakerCheckerEnabled,
		&saved.PaymentExecutorSeparationEnabled,
		&saved.UpdatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, ErrInvalidSettings
	}
	return saved, err
}
