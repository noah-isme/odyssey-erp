package leave

import (
	"context"
	"errors"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestSubmitRejectsInvalidDateRangeBeforeDatabaseAccess(t *testing.T) {
	service := NewService(nil, nil, nil)
	_, err := service.Submit(context.Background(), CreateInput{
		UserID: 1, LeaveTypeID: 1,
		StartDate: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
	})
	require.EqualError(t, err, "hr: invalid leave request")
}

func TestSubmitRejectsMissingIdentityAndLeaveType(t *testing.T) {
	service := NewService(nil, nil, nil)
	for name, input := range map[string]CreateInput{
		"user":       {LeaveTypeID: 1, StartDate: time.Now(), EndDate: time.Now()},
		"leave type": {UserID: 1, StartDate: time.Now(), EndDate: time.Now()},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := service.Submit(context.Background(), input)
			require.EqualError(t, err, "hr: invalid leave request")
		})
	}
}

func TestGetOwnBalancesCalculatesAvailable(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	service := NewService(nil, nil, nil)
	service.pool = db

	db.ExpectQuery("SELECT b.leave_type_id, t.code, t.name, b.year, b.entitled, b.used, b.pending, GREATEST").
		WithArgs(int64(42), 2026).
		WillReturnRows(pgxmock.NewRows([]string{"leave_type_id", "code", "name", "year", "entitled", "used", "pending", "available"}).
			AddRow(int64(1), "ANNUAL", "Annual Leave", 2026, 12.0, 3.0, 2.0, 7.0))

	balances, err := service.GetOwnBalances(context.Background(), 42, 2026)
	require.NoError(t, err)
	require.Len(t, balances, 1)
	require.Equal(t, "ANNUAL", balances[0].TypeCode)
	require.Equal(t, 12.0, balances[0].Entitled)
	require.Equal(t, 7.0, balances[0].Available)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCancelPendingLeaveRequest(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	audit := &leaveAuditFake{}
	service := &Service{pool: db, audit: audit}
	start := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	apprID := int64(101)

	db.ExpectBegin()
	db.ExpectQuery("SELECT r.employee_id, r.leave_type_id, r.days, r.status, r.start_date, r.approval_request_id").
		WithArgs(int64(99), int64(42)).
		WillReturnRows(pgxmock.NewRows([]string{"employee_id", "leave_type_id", "days", "status", "start_date", "approval_request_id"}).
			AddRow(int64(8), int64(1), 3.0, "PENDING", start, &apprID))
	db.ExpectExec("UPDATE hr_leave_requests SET status='CANCELLED'").
		WithArgs(int64(99)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("UPDATE hr_leave_balances SET pending=GREATEST").
		WithArgs(int64(8), int64(1), 2026, 3.0).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("UPDATE approval_requests SET status='CANCELLED'").
		WithArgs(int64(101)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	err = service.Cancel(context.Background(), 99, 42)
	require.NoError(t, err)
	require.Len(t, audit.logs, 1)
	require.Equal(t, "LEAVE_CANCELLED", audit.logs[0].Action)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCancelRejectsNonPendingRequest(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	service := &Service{pool: db}
	start := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)

	db.ExpectBegin()
	db.ExpectQuery("SELECT r.employee_id, r.leave_type_id, r.days, r.status, r.start_date, r.approval_request_id").
		WithArgs(int64(99), int64(42)).
		WillReturnRows(pgxmock.NewRows([]string{"employee_id", "leave_type_id", "days", "status", "start_date", "approval_request_id"}).
			AddRow(int64(8), int64(1), 3.0, "APPROVED", start, nil))
	db.ExpectRollback()

	err = service.Cancel(context.Background(), 99, 42)
	require.EqualError(t, err, "hr: only pending leave requests can be cancelled")
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCreateLeaveType(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	service := &Service{pool: db}
	db.ExpectQuery("INSERT INTO hr_leave_types").
		WithArgs(int64(1), "MATERNITY", "Maternity Leave", 90.0).
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "name", "default_days"}).
			AddRow(int64(5), "MATERNITY", "Maternity Leave", 90.0))

	lt, err := service.CreateType(context.Background(), 1, "MATERNITY", "Maternity Leave", 90.0)
	require.NoError(t, err)
	require.Equal(t, int64(5), lt.ID)
	require.Equal(t, "MATERNITY", lt.Code)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCancelRollbackOnApprovalError(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	service := &Service{pool: db}
	start := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	apprID := int64(101)

	db.ExpectBegin()
	db.ExpectQuery("SELECT r.employee_id, r.leave_type_id, r.days, r.status, r.start_date, r.approval_request_id").
		WithArgs(int64(99), int64(42)).
		WillReturnRows(pgxmock.NewRows([]string{"employee_id", "leave_type_id", "days", "status", "start_date", "approval_request_id"}).
			AddRow(int64(8), int64(1), 3.0, "PENDING", start, &apprID))
	db.ExpectExec("UPDATE hr_leave_requests SET status='CANCELLED'").
		WithArgs(int64(99)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("UPDATE hr_leave_balances SET pending=GREATEST").
		WithArgs(int64(8), int64(1), 2026, 3.0).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("UPDATE approval_requests SET status='CANCELLED'").
		WithArgs(int64(101)).
		WillReturnError(errors.New("db error"))
	db.ExpectRollback()

	err = service.Cancel(context.Background(), 99, 42)
	require.Error(t, err)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestSeedBalance(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()

	service := &Service{pool: db}

	// Valid
	db.ExpectExec("INSERT INTO hr_leave_balances").
		WithArgs(int64(10), int64(2), 2026, 15.0, int64(1)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = service.SeedBalance(context.Background(), 1, 10, 2, 2026, 15.0)
	require.NoError(t, err)

	// Cross-company employee or leave type not found
	db.ExpectExec("INSERT INTO hr_leave_balances").
		WithArgs(int64(99), int64(2), 2026, 15.0, int64(1)).
		WillReturnResult(pgxmock.NewResult("INSERT", 0))

	err = service.SeedBalance(context.Background(), 1, 99, 2, 2026, 15.0)
	require.EqualError(t, err, "hr: employee or leave type not found in company")

	require.NoError(t, db.ExpectationsWereMet())
}
