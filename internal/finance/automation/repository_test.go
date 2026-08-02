package automation

import (
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestRepositorySettingsScopesReadAndWriteByCompany(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	repo := &Repository{pool: db}

	db.ExpectQuery("SELECT company_id").WithArgs(int64(7)).WillReturnRows(settingsRows(7, false))
	got, err := repo.Settings(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, int64(7), got.CompanyID)

	updated := DefaultSettings(7)
	updated.CashForecastEnabled = true
	updated.UpdatedBy = 99
	db.ExpectQuery("UPDATE finance_automation_settings").WithArgs(
		int64(7), false, true, false, false, false, false, int16(13), 1440, true, true, int64(99),
	).WillReturnRows(settingsRows(7, true))
	saved, err := repo.SaveSettings(context.Background(), updated)
	require.NoError(t, err)
	require.Equal(t, int64(7), saved.CompanyID)
	require.True(t, saved.CashForecastEnabled)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestOutboxEnqueueIsScopedAndIdempotent(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	repo := &OutboxRepository{pool: db}
	now := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)

	db.ExpectQuery("INSERT INTO finance_automation_outbox").WithArgs(
		int64(7), "bankfeed.sync", "bank_connection", "19", "bankfeed.sync", "request-1", "", "connection-19-cursor-1", pgxmock.AnyArg(), 10, int64(11),
	).WillReturnRows(outboxRows(now))
	message, err := repo.Enqueue(context.Background(), EnqueueInput{
		CompanyID:      7,
		Topic:          "bankfeed.sync",
		AggregateType:  "bank_connection",
		AggregateID:    "19",
		Operation:      "bankfeed.sync",
		Correlation:    Correlation{ID: "request-1"},
		IdempotencyKey: "connection-19-cursor-1",
		CreatedBy:      11,
	})
	require.NoError(t, err)
	require.Equal(t, int64(7), message.CompanyID)
	require.Equal(t, OutboxPending, message.Status)
	require.NoError(t, db.ExpectationsWereMet())
}

func settingsRows(companyID int64, forecastEnabled bool) *pgxmock.Rows {
	return pgxmock.NewRows([]string{
		"company_id", "bank_feed_auto_sync_enabled", "cash_forecast_enabled",
		"payment_scheduling_enabled", "payment_execution_enabled", "p2p_auto_post_enabled",
		"asset_operations_enabled", "forecast_horizon_weeks", "bank_feed_sync_interval_minutes",
		"payment_maker_checker_enabled", "payment_executor_separation_enabled", "updated_by",
	}).AddRow(companyID, false, forecastEnabled, false, false, false, false, int16(13), 1440, true, true, int64(0))
}

func outboxRows(now time.Time) *pgxmock.Rows {
	return pgxmock.NewRows([]string{
		"id", "company_id", "topic", "aggregate_type", "aggregate_id", "operation",
		"correlation_id", "causation_id", "idempotency_key", "payload", "status", "attempts",
		"max_attempts", "available_at", "locked_at", "locked_by", "last_error", "completed_at",
		"dead_lettered_at", "replayed_from_id", "created_by", "created_at", "updated_at",
	}).AddRow(int64(25), int64(7), "bankfeed.sync", "bank_connection", "19", "bankfeed.sync",
		"request-1", "", "connection-19-cursor-1", []byte("{}"), "PENDING", 0, 10, now,
		nil, "", "", nil, nil, int64(0), int64(11), now, now)
}
