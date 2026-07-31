package attendance

import (
	"context"
	"strings"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestImportCSVTracksDuplicatesAndUnknownEmployees(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	service := &Service{pool: db}

	db.ExpectBegin()
	db.ExpectQuery("INSERT INTO hr_attendance_imports").WithArgs(int64(2), "attendance.csv", int64(9), 2, 1, pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(12)))
	db.ExpectQuery("SELECT id FROM hr_employees").WithArgs(int64(2), "E-1").
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(44)))
	db.ExpectExec("INSERT INTO hr_attendance").WithArgs(int64(44), time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC), pgxmock.AnyArg(), pgxmock.AnyArg(), "PRESENT", int64(12)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectExec("UPDATE hr_attendance_imports").WithArgs(int64(12), 1, 1, pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	result, err := service.Import(context.Background(), 2, 9, "attendance.csv", strings.NewReader(
		"employee_number,date,check_in,check_out,status\n"+
			"E-1,2026-07-30,2026-07-30 08:00,2026-07-30 17:00,PRESENT\n"+
			"E-1,2026-07-30,2026-07-30 09:00,2026-07-30 18:00,PRESENT\n"))
	require.NoError(t, err)
	require.Equal(t, ImportResult{ID: 12, Total: 2, Accepted: 1, Rejected: 1, Errors: []string{"line 3: duplicate employee/date row"}}, result)
	require.NoError(t, db.ExpectationsWereMet())
}
