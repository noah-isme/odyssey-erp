package leave

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/approvals"
	"github.com/odyssey-erp/odyssey-erp/internal/hr/schedule"
)

// failingStore embeds the approvals.Store interface so only the methods used
// by Submit need to be implemented.
type failingStore struct {
	approvals.Store
	resolveErr error
	createErr  error
}

func (f failingStore) ResolvePolicy(context.Context, string, *int64, float64) (approvals.Policy, error) {
	if f.resolveErr != nil {
		return approvals.Policy{}, f.resolveErr
	}
	return approvals.Policy{ID: 1, Module: "LEAVE"}, nil
}

func (f failingStore) CreateRequest(context.Context, approvals.Policy, approvals.Submission) (approvals.Request, []int64, error) {
	return approvals.Request{}, nil, f.createErr
}

var (
	submitStart = time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	submitEnd   = time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
)

func submitInput() CreateInput {
	return CreateInput{CompanyID: 7, UserID: 42, LeaveTypeID: 2, StartDate: submitStart, EndDate: submitEnd, Reason: "trip"}
}

func expectSubmitReservation(db pgxmock.PgxPoolIface) {
	db.ExpectBegin()
	// The employee row is locked so concurrent submissions of different leave
	// types serialize and the overlap check cannot race.
	db.ExpectQuery("SELECT e.id,e.company_id,m.user_id(.|\\n)*FOR UPDATE OF e").WithArgs(int64(42), int64(7)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "user_id"}).AddRow(int64(8), int64(7), int64Ptr(5)))
	db.ExpectQuery("FOR UPDATE").WithArgs(int64(8), int64(2), 2026).
		WillReturnRows(pgxmock.NewRows([]string{"available"}).AddRow(10.0))
	db.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM hr_leave_requests WHERE employee_id=$1 AND status IN ('PENDING','APPROVED')")).
		WithArgs(int64(8), submitStart, submitEnd).WillReturnError(errNoRows())
	db.ExpectQuery("INSERT INTO hr_leave_requests").WithArgs(int64(8), int64(2), submitStart, submitEnd, 2.0, false, false, "trip").
		WillReturnRows(pgxmock.NewRows([]string{"id", "employee_id", "leave_type_id", "start_date", "end_date", "days", "start_half_day", "end_half_day", "reason", "status", "type_name"}).
			AddRow(int64(55), int64(8), int64(2), submitStart, submitEnd, 2.0, false, false, "trip", "PENDING", "Annual leave"))
	db.ExpectExec("UPDATE hr_leave_balances SET pending=pending\\+").WithArgs(int64(8), int64(2), 2026, 2.0).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()
}

func TestSubmitLocksBalanceAndCompensatesOnApprovalFailure(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	audit := &leaveAuditFake{}
	svc := &Service{pool: db, workdays: weekdayCalc{}, audit: audit, approvals: approvals.NewService(failingStore{createErr: errors.New("boom")}, nil)}

	expectSubmitReservation(db)
	db.ExpectBegin()
	db.ExpectExec("UPDATE hr_leave_requests SET status='CANCELLED'").WithArgs(int64(55)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("UPDATE hr_leave_balances SET pending=GREATEST").WithArgs(int64(8), int64(2), 2026, 2.0).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	_, err = svc.Submit(context.Background(), submitInput())
	require.EqualError(t, err, "hr: could not submit leave request for approval")
	require.NoError(t, db.ExpectationsWereMet())
	require.Len(t, audit.logs, 1)
	require.Equal(t, "LEAVE_SUBMIT_FAILED", audit.logs[0].Action)
	require.Equal(t, "approval_error", audit.logs[0].Meta["error_class"])
}

func TestSubmitNoPolicyCancelsRequestAndReleasesPending(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	audit := &leaveAuditFake{}
	svc := &Service{pool: db, workdays: weekdayCalc{}, audit: audit, approvals: approvals.NewService(failingStore{resolveErr: approvals.ErrNoPolicy}, nil)}

	expectSubmitReservation(db)
	db.ExpectBegin()
	db.ExpectExec("UPDATE hr_leave_requests SET status='CANCELLED'").WithArgs(int64(55)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("UPDATE hr_leave_balances SET pending=GREATEST\\(0,pending-\\$4\\)").WithArgs(int64(8), int64(2), 2026, 2.0).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	_, err = svc.Submit(context.Background(), submitInput())
	require.EqualError(t, err, "hr: no approval policy configured for leave")
	require.NoError(t, db.ExpectationsWereMet())
	require.Equal(t, "no_policy", audit.logs[0].Meta["error_class"])
}

func TestSubmitCompensationReleasesPendingOnlyWhenItCancelled(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	audit := &leaveAuditFake{}
	svc := &Service{pool: db, workdays: weekdayCalc{}, audit: audit, approvals: approvals.NewService(failingStore{resolveErr: approvals.ErrNoPolicy}, nil)}

	expectSubmitReservation(db)
	db.ExpectBegin()
	// Already cancelled elsewhere: zero rows, so no balance update is expected
	// (an unexpected Exec would fail the test) and the days are not released twice.
	db.ExpectExec("UPDATE hr_leave_requests SET status='CANCELLED'").WithArgs(int64(55)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	db.ExpectCommit()

	_, err = svc.Submit(context.Background(), submitInput())
	require.EqualError(t, err, "hr: no approval policy configured for leave")
	require.NoError(t, db.ExpectationsWereMet())
	require.Equal(t, false, audit.logs[0].Meta["released"])
}

func TestSubmitSuccessLinksApprovalRequest(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	store := approvalsOKStore{}
	svc := &Service{pool: db, workdays: weekdayCalc{}, approvals: approvals.NewService(store, nil)}

	expectSubmitReservation(db)
	db.ExpectExec("UPDATE hr_leave_requests SET approval_request_id").WithArgs(int64(55), int64(900)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	req, err := svc.Submit(context.Background(), submitInput())
	require.NoError(t, err)
	require.Equal(t, "PENDING", req.Status)
	require.Equal(t, "Annual leave", req.TypeName, "the submit response carries the leave type name like the list endpoint")
	require.NotNil(t, req.ApprovalRequestID)
	require.Equal(t, int64(900), *req.ApprovalRequestID)
	require.NoError(t, db.ExpectationsWereMet())
}

type approvalsOKStore struct{ approvals.Store }

func (approvalsOKStore) ResolvePolicy(context.Context, string, *int64, float64) (approvals.Policy, error) {
	return approvals.Policy{ID: 1, Module: "LEAVE"}, nil
}

func (approvalsOKStore) CreateRequest(context.Context, approvals.Policy, approvals.Submission) (approvals.Request, []int64, error) {
	return approvals.Request{ID: 900, Module: "LEAVE", Status: approvals.StatusPending}, nil, nil
}

func TestSubmitRejectsOverlapAndCrossYear(t *testing.T) {
	t.Run("cross year rejected before database access", func(t *testing.T) {
		svc := NewService(nil, nil, nil, nil)
		in := submitInput()
		in.StartDate = time.Date(2026, 12, 30, 0, 0, 0, 0, time.UTC)
		in.EndDate = time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)
		_, err := svc.Submit(context.Background(), in)
		require.EqualError(t, err, "hr: split requests at year end")
	})
	t.Run("overlap rolls back without insert", func(t *testing.T) {
		db, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer db.Close()
		svc := &Service{pool: db, workdays: weekdayCalc{}}
		db.ExpectBegin()
		db.ExpectQuery("SELECT e.id,e.company_id,m.user_id").WithArgs(int64(42), int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "user_id"}).AddRow(int64(8), int64(7), int64Ptr(5)))
		db.ExpectQuery("FOR UPDATE").WithArgs(int64(8), int64(2), 2026).
			WillReturnRows(pgxmock.NewRows([]string{"available"}).AddRow(10.0))
		db.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM hr_leave_requests")).WithArgs(int64(8), submitStart, submitEnd).
			WillReturnRows(pgxmock.NewRows([]string{"x"}).AddRow(1))
		db.ExpectRollback()
		_, err = svc.Submit(context.Background(), submitInput())
		require.EqualError(t, err, "hr: overlaps an existing leave request")
		require.NoError(t, db.ExpectationsWereMet())
	})
}

// weekdayCalc is a WorkdayCalculator fake: Mon-Fri week plus fixed holidays.
type weekdayCalc struct {
	holidays []time.Time
	err      error
}

func (w weekdayCalc) HolidaysBetween(context.Context, int64, time.Time, time.Time) ([]time.Time, error) {
	return w.holidays, w.err
}

func (w weekdayCalc) WorkDaysFor(context.Context, int64, int64, time.Time) ([]time.Weekday, error) {
	return schedule.DefaultWorkDays(), nil
}

func expectEmployeeLookup(db pgxmock.PgxPoolIface) {
	db.ExpectBegin()
	db.ExpectQuery("SELECT e.id,e.company_id,m.user_id").WithArgs(int64(42), int64(7)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "user_id"}).AddRow(int64(8), int64(7), int64Ptr(5)))
}

func TestSubmitCountsWorkingDaysAcrossWeekendAndHoliday(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	// Fri 2026-08-07 .. Mon 2026-08-10 with Monday a holiday => 1 day.
	start := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	svc := &Service{pool: db, workdays: weekdayCalc{holidays: []time.Time{end}}, approvals: approvals.NewService(approvalsOKStore{}, nil)}
	expectEmployeeLookup(db)
	db.ExpectQuery("FOR UPDATE").WithArgs(int64(8), int64(2), 2026).
		WillReturnRows(pgxmock.NewRows([]string{"available"}).AddRow(10.0))
	db.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM hr_leave_requests")).WithArgs(int64(8), start, end).WillReturnError(errNoRows())
	db.ExpectQuery("INSERT INTO hr_leave_requests").WithArgs(int64(8), int64(2), start, end, 1.0, false, false, "trip").
		WillReturnRows(pgxmock.NewRows([]string{"id", "employee_id", "leave_type_id", "start_date", "end_date", "days", "start_half_day", "end_half_day", "reason", "status", "type_name"}).
			AddRow(int64(55), int64(8), int64(2), start, end, 1.0, false, false, "trip", "PENDING", "Annual leave"))
	db.ExpectExec("UPDATE hr_leave_balances SET pending=pending\\+").WithArgs(int64(8), int64(2), 2026, 1.0).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()
	db.ExpectExec("UPDATE hr_leave_requests SET approval_request_id").WithArgs(int64(55), int64(900)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	in := submitInput()
	in.StartDate, in.EndDate = start, end
	req, err := svc.Submit(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, 1.0, req.Days)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestSubmitStoresHalfDayFlags(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	svc := &Service{pool: db, workdays: weekdayCalc{}, approvals: approvals.NewService(approvalsOKStore{}, nil)}
	expectEmployeeLookup(db)
	db.ExpectQuery("FOR UPDATE").WithArgs(int64(8), int64(2), 2026).
		WillReturnRows(pgxmock.NewRows([]string{"available"}).AddRow(10.0))
	db.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM hr_leave_requests")).WithArgs(int64(8), submitStart, submitEnd).WillReturnError(errNoRows())
	db.ExpectQuery("INSERT INTO hr_leave_requests").WithArgs(int64(8), int64(2), submitStart, submitEnd, 1.5, true, false, "trip").
		WillReturnRows(pgxmock.NewRows([]string{"id", "employee_id", "leave_type_id", "start_date", "end_date", "days", "start_half_day", "end_half_day", "reason", "status", "type_name"}).
			AddRow(int64(55), int64(8), int64(2), submitStart, submitEnd, 1.5, true, false, "trip", "PENDING", "Annual leave"))
	db.ExpectExec("UPDATE hr_leave_balances SET pending=pending\\+").WithArgs(int64(8), int64(2), 2026, 1.5).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()
	db.ExpectExec("UPDATE hr_leave_requests SET approval_request_id").WithArgs(int64(55), int64(900)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	in := submitInput()
	in.StartHalfDay = true
	req, err := svc.Submit(context.Background(), in)
	require.NoError(t, err)
	require.True(t, req.StartHalfDay)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestSubmitRejectsZeroWorkingDays(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	// Sat 2026-08-08 .. Sun 2026-08-09: no working days.
	start := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	svc := &Service{pool: db, workdays: weekdayCalc{}}
	expectEmployeeLookup(db)
	db.ExpectRollback()
	in := submitInput()
	in.StartDate, in.EndDate = start, end
	_, err = svc.Submit(context.Background(), in)
	require.EqualError(t, err, "hr: no working days in range")
	require.NoError(t, db.ExpectationsWereMet())
}

func int64Ptr(v int64) *int64 { return &v }

func errNoRows() error { return pgx.ErrNoRows }
