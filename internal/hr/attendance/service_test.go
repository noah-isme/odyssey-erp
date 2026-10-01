package attendance

import (
	"context"
	"strings"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestParseCSVValidatesRows(t *testing.T) {
	rows, problems, err := ParseCSV(strings.NewReader("employee_number,date,check_in,check_out,status\nE-1,2026-07-30,2026-07-30 08:00,2026-07-30 17:00,PRESENT\nE-2,bad,,,PRESENT\n"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, problems, 1)
	require.Equal(t, "E-1", rows[0].EmployeeNumber)
}

func TestParseCSVRejectsDuplicatesAndInvalidStatusOrTimeRange(t *testing.T) {
	input := "employee_number,date,check_in,check_out,status\n" +
		"E-1,2026-07-30,2026-07-30 08:00,2026-07-30 17:00,PRESENT\n" +
		"E-1,2026-07-30,2026-07-30 09:00,2026-07-30 18:00,PRESENT\n" +
		"E-2,2026-07-31,2026-07-31 17:00,2026-07-31 08:00,PRESENT\n" +
		"E-3,2026-07-31,,,HOLIDAY\n"

	rows, problems, err := ParseCSV(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, problems, 3)
	require.Contains(t, problems[0], "duplicate")
}

func TestParseCSVEnforcesDateFormatAndRequiredColumns(t *testing.T) {
	_, _, err := ParseCSV(strings.NewReader("employee_number,check_in\nE-1,2026-07-30 08:00\n"))
	require.ErrorContains(t, err, "missing date column")

	rows, problems, err := ParseCSV(strings.NewReader("employee_number,date\nE-1,2026-02-30\nE-2,2026-12-31\n"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, problems, 1)
	require.Equal(t, "E-2", rows[0].EmployeeNumber)
}

func TestCSVTemplateFormat(t *testing.T) {
	s := &Service{}
	tmpl := s.CSVTemplate()
	require.True(t, strings.HasPrefix(string(tmpl), "employee_number,date,check_in,check_out,status\n"))
	require.Contains(t, string(tmpl), "PRESENT")
}

func TestRecordManualRejectsCheckOutBeforeCheckIn(t *testing.T) {
	s := &Service{}
	in := time.Date(2026, 9, 18, 17, 0, 0, 0, time.UTC)
	out := time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC)
	err := s.RecordManual(context.Background(), 1, ManualRecordInput{
		EmployeeID: 10,
		Date:       time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC),
		CheckIn:    &in,
		CheckOut:   &out,
		Status:     "PRESENT",
	})
	require.EqualError(t, err, "hr: check-out cannot be before check-in")
}

func TestRecordManualValidInput(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	service := &Service{pool: db}
	recDate := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	in := time.Date(2026, 9, 18, 8, 30, 0, 0, time.UTC)
	out := time.Date(2026, 9, 18, 17, 30, 0, 0, time.UTC)

	db.ExpectExec("INSERT INTO hr_attendance").
		WithArgs(int64(10), recDate, &in, &out, "PRESENT", int64(1)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = service.RecordManual(context.Background(), 1, ManualRecordInput{
		EmployeeID: 10,
		Date:       recDate,
		CheckIn:    &in,
		CheckOut:   &out,
		Status:     "PRESENT",
	})
	require.NoError(t, err)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestRecordManualRejectsCrossCompanyEmployee(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	service := &Service{pool: db}
	recDate := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	in := time.Date(2026, 9, 18, 8, 30, 0, 0, time.UTC)
	out := time.Date(2026, 9, 18, 17, 30, 0, 0, time.UTC)

	db.ExpectExec("INSERT INTO hr_attendance").
		WithArgs(int64(99), recDate, &in, &out, "PRESENT", int64(1)).
		WillReturnResult(pgxmock.NewResult("INSERT", 0))

	err = service.RecordManual(context.Background(), 1, ManualRecordInput{
		EmployeeID: 99,
		Date:       recDate,
		CheckIn:    &in,
		CheckOut:   &out,
		Status:     "PRESENT",
	})
	require.EqualError(t, err, "hr: employee not found in current company")
	require.NoError(t, db.ExpectationsWereMet())
}

func TestListRecordsWithDurationAndStats(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	service := &Service{pool: db}
	recDate := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	in := time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC)
	out := time.Date(2026, 9, 18, 17, 0, 0, 0, time.UTC)

	db.ExpectQuery("SELECT a.id, a.employee_id, e.employee_number").
		WithArgs(int64(1), recDate).
		WillReturnRows(pgxmock.NewRows([]string{"id", "employee_id", "employee_number", "name", "department", "attendance_date", "check_in", "check_out", "status", "source", "import_id"}).
			AddRow(int64(1), int64(10), "EMP-001", "Budi", "Engineering", recDate, &in, &out, "PRESENT", "CSV", nil))

	db.ExpectQuery("SELECT COUNT.*FROM hr_employees").
		WithArgs(int64(1)).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(15))
	db.ExpectQuery("SELECT.*COUNT.*FILTER.*FROM hr_attendance").
		WithArgs(int64(1), recDate).
		WillReturnRows(pgxmock.NewRows([]string{"present", "absent", "leave"}).AddRow(12, 2, 1))

	records, stats, err := service.ListRecords(context.Background(), 1, AttendanceFilter{Date: recDate})
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, 8.0, records[0].DurationHours)
	require.Equal(t, 15, stats.TotalActive)
	require.Equal(t, 12, stats.Present)
	require.Equal(t, 2, stats.Absent)
	require.Equal(t, 1, stats.OnLeave)
	require.NoError(t, db.ExpectationsWereMet())
}
