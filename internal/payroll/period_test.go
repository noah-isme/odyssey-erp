package payroll

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func periodInput() CreatePeriodInput {
	d := func(day int) time.Time { return time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC) }
	return CreatePeriodInput{CompanyID: 1, Code: "2026-10", StartsOn: d(1), EndsOn: d(31), PayDate: d(28)}
}

func TestCreatePeriodRejectsOverlap(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	in := periodInput()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT 1 FROM payroll_periods`).WithArgs(in.CompanyID, in.StartsOn, in.EndsOn).
		WillReturnRows(pgxmock.NewRows([]string{"?column?"}).AddRow(1))
	mock.ExpectRollback()
	_, err = createPeriod(context.Background(), mock, in)
	require.ErrorIs(t, err, ErrPeriodOverlap)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreatePeriodInsertsWhenFree(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	in := periodInput()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT 1 FROM payroll_periods`).WithArgs(in.CompanyID, in.StartsOn, in.EndsOn).WillReturnError(pgx.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO payroll_periods`).WithArgs(in.CompanyID, in.Code, in.StartsOn, in.EndsOn, in.PayDate).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(7)))
	mock.ExpectCommit()
	id, err := createPeriod(context.Background(), mock, in)
	require.NoError(t, err)
	require.Equal(t, int64(7), id)
	require.NoError(t, mock.ExpectationsWereMet())
}
