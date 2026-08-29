package sqlc

import (
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestGetLatestForecastRunReturnsNewestRunRegardlessOfStatus(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	createdAt := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	db.ExpectQuery(`WHERE company_id = \$1 AND scenario_id = \$2\s+ORDER BY created_at DESC, id DESC`).
		WithArgs(int64(7), int64(3)).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "company_id", "scenario_id", "status", "fx_snapshot", "completed_at", "error_details", "created_at",
		}).AddRow(int64(41), int64(7), int64(3), "INCOMPLETE", []byte(`{}`), nil, "source unavailable", createdAt))

	run, err := New(db).GetLatestForecastRun(context.Background(), GetLatestForecastRunParams{
		CompanyID:  7,
		ScenarioID: 3,
	})
	require.NoError(t, err)
	require.Equal(t, int64(41), run.ID)
	require.Equal(t, "INCOMPLETE", run.Status)
	require.NoError(t, db.ExpectationsWereMet())
}
