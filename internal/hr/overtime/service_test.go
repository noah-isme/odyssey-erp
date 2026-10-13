package overtime

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/approvals"
	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

const (
	companyA   = int64(7)
	companyB   = int64(8)
	userID     = int64(42)
	managerID  = int64(5)
	employeeID = int64(11)
	requestID  = int64(55)
	approvalID = int64(900)
	actorID    = int64(99)
)

var (
	workedOn        = time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	createdAt       = time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	requestColNames = []string{"id", "company_id", "employee_id", "worked_on", "minutes", "reason", "status", "approval_request_id", "apply_error", "decided_by", "decided_at", "created_at", "updated_at"}
	lockedCols      = []string{"company_id", "worked_on", "minutes", "status", "user_id", "approval_request_id"}
	lockSQL         = regexp.QuoteMeta(payrollLockSQL)
)

type auditFake struct{ logs []shared.AuditLog }

func (a *auditFake) RecordTx(_ context.Context, _ pgx.Tx, log shared.AuditLog) error {
	a.logs = append(a.logs, log)
	return nil
}

func (a *auditFake) actions() []string {
	out := make([]string, 0, len(a.logs))
	for _, l := range a.logs {
		out = append(out, l.Action)
	}
	return out
}

type notifierFake struct{ msgs []notifications.Message }

func (n *notifierFake) Dispatch(_ context.Context, m notifications.Message) error {
	n.msgs = append(n.msgs, m)
	return nil
}

// approvalsStore embeds approvals.Store so only Submit's methods are needed.
type approvalsStore struct {
	approvals.Store
	resolveErr error
	createErr  error
	onCreate   func(approvals.Submission)
}

func (f *approvalsStore) ResolvePolicy(context.Context, string, *int64, float64) (approvals.Policy, error) {
	if f.resolveErr != nil {
		return approvals.Policy{}, f.resolveErr
	}
	return approvals.Policy{ID: 1, Module: Module}, nil
}

func (f *approvalsStore) CreateRequest(_ context.Context, _ approvals.Policy, in approvals.Submission) (approvals.Request, []int64, error) {
	if f.onCreate != nil {
		f.onCreate(in)
	}
	if f.createErr != nil {
		return approvals.Request{}, nil, f.createErr
	}
	return approvals.Request{ID: approvalID, Module: Module, Status: approvals.StatusPending}, nil, nil
}

func newService(t *testing.T, store approvals.Store) (pgxmock.PgxPoolIface, *Service, *auditFake, *notifierFake) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(db.Close)
	audit := &auditFake{}
	notifier := &notifierFake{}
	svc := &Service{pool: db, audit: audit, notifier: notifier}
	if store != nil {
		svc.approvals = approvals.NewService(store, nil)
	}
	return db, svc, audit, notifier
}

func ptr(v int64) *int64 { return &v }

func submitInput() SubmitInput {
	return SubmitInput{UserID: userID, CompanyID: companyA, WorkedOn: workedOn.Add(15 * time.Hour), Minutes: 90, Reason: " month-end close "}
}

func expectEmployeeAndLock(db pgxmock.PgxPoolIface, lock string) {
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta("SELECT e.id, m.user_id FROM hr_employees e LEFT JOIN hr_employees m ON m.id=e.manager_id AND m.company_id=e.company_id WHERE e.user_id=$1 AND e.company_id=$2")).
		WithArgs(userID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id"}).AddRow(employeeID, ptr(managerID)))
	q := db.ExpectQuery(lockSQL).WithArgs(companyA, workedOn)
	if lock == "" {
		q.WillReturnError(pgx.ErrNoRows)
	} else {
		q.WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow(lock))
	}
}

func expectInsert(db pgxmock.PgxPoolIface) {
	db.ExpectQuery("INSERT INTO hr_overtime_requests").
		WithArgs(companyA, employeeID, workedOn, 90, "month-end close").
		WillReturnRows(pgxmock.NewRows(requestColNames).
			AddRow(requestID, companyA, employeeID, workedOn, 90, "month-end close", StatusPending, (*int64)(nil), "", (*int64)(nil), (*time.Time)(nil), createdAt, createdAt))
}

func TestSubmitInsertsPendingThenCallsApprovals(t *testing.T) {
	var got approvals.Submission
	var auditAtApproval []string
	store := &approvalsStore{}
	db, svc, audit, _ := newService(t, store)
	store.onCreate = func(in approvals.Submission) {
		got = in
		auditAtApproval = audit.actions()
		// The PENDING insert must already be committed when approvals runs.
		require.NoError(t, db.ExpectationsWereMet())
		db.ExpectExec(regexp.QuoteMeta("UPDATE hr_overtime_requests SET approval_request_id=$2")).
			WithArgs(requestID, approvalID, companyA).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	}

	expectEmployeeAndLock(db, "")
	expectInsert(db)
	db.ExpectCommit()

	req, err := svc.Submit(context.Background(), submitInput())
	require.NoError(t, err)
	require.Equal(t, StatusPending, req.Status)
	require.Equal(t, approvalID, *req.ApprovalRequestID)
	require.Equal(t, 1.5, req.Hours())
	require.Equal(t, []string{ActionSubmitted}, auditAtApproval)
	require.Equal(t, Module, got.Module)
	require.Equal(t, requestID, got.DocumentID)
	require.Equal(t, userID, got.RequesterID)
	require.Equal(t, companyA, *got.CompanyID)
	require.Equal(t, 1.5, got.Amount)
	require.Equal(t, managerID, got.ManagerID)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestSubmitDuplicateDay(t *testing.T) {
	called := false
	db, svc, audit, _ := newService(t, &approvalsStore{onCreate: func(approvals.Submission) { called = true }})
	expectEmployeeAndLock(db, "")
	db.ExpectQuery("INSERT INTO hr_overtime_requests").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnError(&pgconn.PgError{Code: "23505"})
	db.ExpectRollback()

	_, err := svc.Submit(context.Background(), submitInput())
	require.ErrorIs(t, err, ErrAlreadyRequested)
	require.EqualError(t, err, "hr: overtime already requested for that day")
	require.False(t, called)
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestSubmitRefusesLockedPayrollWithoutInsert(t *testing.T) {
	cases := []struct {
		lock string
		want error
		msg  string
	}{
		{"POSTED", ErrPayrollPosted, "hr: payroll for that date is already posted"},
		{"APPROVAL", ErrPayrollAwaitingApproval, "hr: payroll run awaiting approval"},
	}
	for _, tc := range cases {
		t.Run(tc.lock, func(t *testing.T) {
			db, svc, audit, _ := newService(t, &approvalsStore{})
			expectEmployeeAndLock(db, tc.lock)
			db.ExpectRollback()

			_, err := svc.Submit(context.Background(), submitInput())
			require.ErrorIs(t, err, tc.want)
			require.EqualError(t, err, tc.msg)
			require.Empty(t, audit.logs)
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}

func TestSubmitLockQueryCoversApprovalAndPostedRegularRuns(t *testing.T) {
	require.Contains(t, payrollLockSQL, "r.status IN ('APPROVAL','POSTED')")
	require.Contains(t, payrollLockSQL, "r.run_type='REGULAR'")
	require.Contains(t, payrollLockSQL, "p.company_id=$1")
	require.Contains(t, payrollLockSQL, "$2 BETWEEN p.starts_on AND p.ends_on")
}

func TestSubmitCompensatesOnApprovalFailure(t *testing.T) {
	cases := []struct {
		name  string
		store *approvalsStore
		want  error
		class string
	}{
		{"no policy", &approvalsStore{resolveErr: approvals.ErrNoPolicy}, ErrNoPolicy, "no_policy"},
		{"engine error", &approvalsStore{createErr: errors.New("boom")}, ErrSubmitFailed, "approval_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, svc, audit, _ := newService(t, tc.store)
			expectEmployeeAndLock(db, "")
			expectInsert(db)
			db.ExpectCommit()
			db.ExpectBegin()
			db.ExpectExec(regexp.QuoteMeta("UPDATE hr_overtime_requests SET status='CANCELLED', updated_at=NOW() WHERE id=$1 AND status='PENDING'")).
				WithArgs(requestID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			db.ExpectCommit()

			_, err := svc.Submit(context.Background(), submitInput())
			require.ErrorIs(t, err, tc.want)
			require.Equal(t, []string{ActionSubmitted, ActionSubmitFailed}, audit.actions())
			require.Equal(t, tc.class, audit.logs[1].Meta["error_class"])
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}

func TestSubmitValidation(t *testing.T) {
	svc := &Service{audit: &auditFake{}}
	in := submitInput()
	in.CompanyID = 0
	_, err := svc.Submit(context.Background(), in)
	require.ErrorIs(t, err, ErrCompanyRequired)
	for _, m := range []int{0, -5, 721} {
		in := submitInput()
		in.Minutes = m
		_, err := svc.Submit(context.Background(), in)
		require.ErrorIs(t, err, ErrInvalidMinutes)
	}
}

func TestSubmitRejectsFutureWorkedOn(t *testing.T) {
	// No database expectations: the request is rejected before any query.
	db, svc, audit, _ := newService(t, &approvalsStore{})
	for _, days := range []int{1, 30} {
		in := submitInput()
		in.WorkedOn = shared.BusinessToday().AddDate(0, 0, days)
		_, err := svc.Submit(context.Background(), in)
		require.ErrorIs(t, err, ErrFutureOvertime)
		require.EqualError(t, err, "hr: overtime can only be requested for days already worked")
	}
	// A UTC "today" that is still tomorrow in Jakarta is not in the future there.
	in := submitInput()
	in.WorkedOn = shared.BusinessToday()
	db.ExpectBegin()
	db.ExpectQuery("SELECT e.id, m.user_id FROM hr_employees").WithArgs(userID, companyA).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()
	_, err := svc.Submit(context.Background(), in)
	require.ErrorIs(t, err, ErrNotFound, "today is accepted and proceeds to the employee lookup")
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestSubmitEmployeeNotInCompany(t *testing.T) {
	db, svc, _, _ := newService(t, &approvalsStore{})
	db.ExpectBegin()
	db.ExpectQuery("SELECT e.id, m.user_id FROM hr_employees").WithArgs(userID, companyB).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()
	in := submitInput()
	in.CompanyID = companyB
	_, err := svc.Submit(context.Background(), in)
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, db.ExpectationsWereMet())
}

// ---- FinalizeApproval ------------------------------------------------------

func approvalReq(company int64) approvals.Request {
	return approvals.Request{ID: approvalID, Module: Module, DocumentID: requestID, RequesterID: userID, CompanyID: &company, Status: approvals.StatusApproved}
}

func expectLockedRow(db pgxmock.PgxPoolIface, status string, company int64) {
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta("FROM hr_overtime_requests o JOIN hr_employees e ON e.id=o.employee_id WHERE o.id=$1 FOR UPDATE OF o")).
		WithArgs(requestID).
		WillReturnRows(pgxmock.NewRows(lockedCols).AddRow(company, workedOn, 90, status, ptr(userID), ptr(approvalID)))
}

func expectFinalizeLock(db pgxmock.PgxPoolIface, lock string) {
	q := db.ExpectQuery(lockSQL).WithArgs(companyA, workedOn)
	if lock == "" {
		q.WillReturnError(pgx.ErrNoRows)
	} else {
		q.WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow(lock))
	}
}

func TestFinalizeApprovalApproves(t *testing.T) {
	db, svc, audit, notifier := newService(t, nil)
	expectLockedRow(db, StatusPending, companyA)
	expectFinalizeLock(db, "")
	db.ExpectExec(regexp.QuoteMeta("UPDATE hr_overtime_requests SET status='APPROVED', apply_error='', decided_by=$2, decided_at=NOW()")).
		WithArgs(requestID, actorID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	err := svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, "ok")
	require.NoError(t, err)
	require.Equal(t, []string{ActionApproved}, audit.actions())
	require.Equal(t, actorID, audit.logs[0].ActorID)
	require.Equal(t, "55", audit.logs[0].EntityID)
	require.Empty(t, notifier.msgs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestFinalizeApprovalRejects(t *testing.T) {
	db, svc, audit, _ := newService(t, nil)
	expectLockedRow(db, StatusPending, companyA)
	db.ExpectExec(regexp.QuoteMeta("UPDATE hr_overtime_requests SET status='REJECTED', apply_error='', decided_by=$2")).
		WithArgs(requestID, actorID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	err := svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusRejected, actorID, "no")
	require.NoError(t, err)
	require.Equal(t, []string{ActionRejected}, audit.actions())
	require.NoError(t, db.ExpectationsWereMet())
}

func TestFinalizeApprovalSecondCallIsNoOp(t *testing.T) {
	for _, st := range []string{StatusApproved, StatusRejected, StatusCancelled} {
		t.Run(st, func(t *testing.T) {
			db, svc, audit, notifier := newService(t, nil)
			expectLockedRow(db, st, companyA)
			db.ExpectRollback()

			err := svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, "")
			require.NoError(t, err)
			require.Empty(t, audit.logs)
			require.Empty(t, notifier.msgs)
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}

func TestFinalizeApprovalRejectsCompanyMismatch(t *testing.T) {
	db, svc, audit, _ := newService(t, nil)
	expectLockedRow(db, StatusPending, companyB)
	db.ExpectRollback()

	err := svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, "")
	require.ErrorIs(t, err, ErrCompanyMismatch)
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())

	db2, svc2, _, _ := newService(t, nil)
	expectLockedRow(db2, StatusPending, companyA)
	db2.ExpectRollback()
	err = svc2.FinalizeApproval(context.Background(), approvals.Request{DocumentID: requestID}, approvals.StatusApproved, actorID, "")
	require.ErrorIs(t, err, ErrCompanyMismatch)
	require.NoError(t, db2.ExpectationsWereMet())
}

func TestFinalizeApprovalPostedPeriodBecomesRejected(t *testing.T) {
	db, svc, audit, notifier := newService(t, nil)
	expectLockedRow(db, StatusPending, companyA)
	expectFinalizeLock(db, "POSTED")
	db.ExpectExec(regexp.QuoteMeta("UPDATE hr_overtime_requests SET status='REJECTED', apply_error=$3, decided_by=$2")).
		WithArgs(requestID, actorID, "payroll already posted").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	err := svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, "")
	require.NoError(t, err)
	require.Equal(t, []string{ActionRejectedPostedPeriod}, audit.actions())
	require.Len(t, notifier.msgs, 1)
	msg := notifier.msgs[0]
	require.Equal(t, userID, msg.RecipientID)
	require.Equal(t, notifications.TypeApprovalRejected, msg.Type)
	require.Contains(t, msg.Title, "rejected: payroll locked")
	require.Contains(t, msg.Body, "rejected: payroll locked")
	require.Equal(t, "overtime:55:rejected:payroll_locked", msg.DedupeKey)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestFinalizeApprovalRunAwaitingApprovalStaysPending(t *testing.T) {
	db, svc, audit, notifier := newService(t, nil)
	expectLockedRow(db, StatusPending, companyA)
	expectFinalizeLock(db, "APPROVAL")
	db.ExpectExec(regexp.QuoteMeta("UPDATE hr_overtime_requests SET apply_error=$2, updated_at=NOW() WHERE id=$1")).
		WithArgs(requestID, "payroll run awaiting approval").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	err := svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, "")
	require.ErrorIs(t, err, ErrPayrollAwaitingApproval)
	require.Equal(t, []string{ActionApplyDeferredApproval}, audit.actions())
	require.Empty(t, notifier.msgs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestFinalizeApprovalApplyFailureStoresApplyError(t *testing.T) {
	db, svc, audit, _ := newService(t, nil)
	expectLockedRow(db, StatusPending, companyA)
	expectFinalizeLock(db, "")
	db.ExpectExec("UPDATE hr_overtime_requests SET status='APPROVED'").WithArgs(requestID, actorID).WillReturnError(errors.New("disk full"))
	db.ExpectRollback()
	db.ExpectExec(regexp.QuoteMeta("UPDATE hr_overtime_requests SET apply_error=$2, updated_at=NOW() WHERE id=$1 AND status='PENDING'")).
		WithArgs(requestID, "finalize overtime: approve: disk full").WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err := svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, "")
	require.ErrorContains(t, err, "disk full")
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

// ---- RetryApply ------------------------------------------------------------

var approvalCols = []string{"id", "policy_id", "module", "document_id", "company_id", "amount", "requester_id", "current_step", "status", "submitted_at"}

func expectRetryReads(db pgxmock.PgxPoolIface, approvalStatus string) {
	db.ExpectQuery(regexp.QuoteMeta("SELECT o.status, o.approval_request_id FROM hr_overtime_requests o WHERE o.id=$1 AND o.company_id=$2")).
		WithArgs(requestID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"status", "approval_request_id"}).AddRow(StatusPending, ptr(approvalID)))
	db.ExpectQuery(regexp.QuoteMeta("FROM approval_requests WHERE id=$1 AND module=$2 AND document_id=$3")).
		WithArgs(approvalID, Module, requestID).
		WillReturnRows(pgxmock.NewRows(approvalCols).AddRow(approvalID, int64(1), Module, requestID, ptr(companyA), 1.5, userID, 1, approvalStatus, createdAt))
}

func TestRetryApply(t *testing.T) {
	t.Run("run returned to draft approves", func(t *testing.T) {
		db, svc, audit, notifier := newService(t, nil)
		expectRetryReads(db, approvals.StatusApproved)
		expectLockedRow(db, StatusPending, companyA)
		expectFinalizeLock(db, "")
		db.ExpectExec("UPDATE hr_overtime_requests SET status='APPROVED', apply_error=''").
			WithArgs(requestID, actorID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()

		require.NoError(t, svc.RetryApply(context.Background(), companyA, requestID, actorID))
		require.Equal(t, []string{ActionApproved}, audit.actions())
		require.Empty(t, notifier.msgs)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("run became posted rejects and notifies", func(t *testing.T) {
		db, svc, audit, notifier := newService(t, nil)
		expectRetryReads(db, approvals.StatusApproved)
		expectLockedRow(db, StatusPending, companyA)
		expectFinalizeLock(db, "POSTED")
		db.ExpectExec("UPDATE hr_overtime_requests SET status='REJECTED', apply_error=\\$3").
			WithArgs(requestID, actorID, ApplyErrorPayrollPosted).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()

		require.NoError(t, svc.RetryApply(context.Background(), companyA, requestID, actorID))
		require.Equal(t, []string{ActionRejectedPostedPeriod}, audit.actions())
		require.Len(t, notifier.msgs, 1)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("approval still pending", func(t *testing.T) {
		db, svc, _, _ := newService(t, nil)
		expectRetryReads(db, approvals.StatusPending)
		require.ErrorIs(t, svc.RetryApply(context.Background(), companyA, requestID, actorID), ErrApprovalNotDecided)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("not pending", func(t *testing.T) {
		db, svc, _, _ := newService(t, nil)
		db.ExpectQuery("SELECT o.status, o.approval_request_id").WithArgs(requestID, companyA).
			WillReturnRows(pgxmock.NewRows([]string{"status", "approval_request_id"}).AddRow(StatusApproved, ptr(approvalID)))
		require.ErrorIs(t, svc.RetryApply(context.Background(), companyA, requestID, actorID), ErrNotRetryable)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("cross tenant not found", func(t *testing.T) {
		db, svc, _, _ := newService(t, nil)
		db.ExpectQuery("SELECT o.status, o.approval_request_id").WithArgs(requestID, companyB).WillReturnError(pgx.ErrNoRows)
		err := svc.RetryApply(context.Background(), companyB, requestID, actorID)
		require.ErrorIs(t, err, ErrNotFound)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("company required", func(t *testing.T) {
		require.ErrorIs(t, (&Service{}).RetryApply(context.Background(), 0, requestID, actorID), ErrCompanyRequired)
	})
}

// ---- Cancel ----------------------------------------------------------------

func TestCancelCancelsApprovalAndAssignments(t *testing.T) {
	db, svc, audit, _ := newService(t, nil)
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta("WHERE o.id=$1 AND e.user_id=$2 AND o.company_id=$3 AND e.company_id=$3 FOR UPDATE OF o")).
		WithArgs(requestID, userID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"status", "approval_request_id"}).AddRow(StatusPending, ptr(approvalID)))
	db.ExpectExec(regexp.QuoteMeta("UPDATE hr_overtime_requests SET status='CANCELLED'")).WithArgs(requestID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(regexp.QuoteMeta("UPDATE approval_requests SET status='CANCELLED', updated_at=NOW() WHERE id=$1 AND status='PENDING'")).
		WithArgs(approvalID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(regexp.QuoteMeta("UPDATE approval_assignments SET status='CANCELLED', updated_at=NOW() WHERE request_id=$1 AND status='PENDING'")).
		WithArgs(approvalID).WillReturnResult(pgxmock.NewResult("UPDATE", 2))
	db.ExpectCommit()

	require.NoError(t, svc.Cancel(context.Background(), companyA, requestID, userID))
	require.Equal(t, []string{ActionCancelled}, audit.actions())
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCancelCrossTenantNotFound(t *testing.T) {
	db, svc, audit, _ := newService(t, nil)
	db.ExpectBegin()
	db.ExpectQuery("FOR UPDATE OF o").WithArgs(requestID, userID, companyB).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()

	err := svc.Cancel(context.Background(), companyB, requestID, userID)
	require.ErrorIs(t, err, ErrNotFound)
	require.EqualError(t, err, "hr: not found: overtime request")
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCancelOnlyPending(t *testing.T) {
	db, svc, _, _ := newService(t, nil)
	db.ExpectBegin()
	db.ExpectQuery("FOR UPDATE OF o").WithArgs(requestID, userID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"status", "approval_request_id"}).AddRow(StatusApproved, ptr(approvalID)))
	db.ExpectRollback()
	require.ErrorIs(t, svc.Cancel(context.Background(), companyA, requestID, userID), ErrNotPending)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCancelCompanyRequired(t *testing.T) {
	require.ErrorIs(t, (&Service{}).Cancel(context.Background(), 0, requestID, userID), ErrCompanyRequired)
}

// ---- Lists -----------------------------------------------------------------

var listColNames = append(append([]string{}, requestColNames...), "name", "employee_number", "department")

func TestListAllScopedAndFiltered(t *testing.T) {
	db, svc, _, _ := newService(t, nil)
	emp := int64(employeeID)
	from := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	db.ExpectQuery(regexp.QuoteMeta("WHERE o.company_id=$1 AND o.status=$2 AND o.employee_id=$3 AND o.worked_on>=$4 AND (LOWER(e.name) LIKE $5")).
		WithArgs(companyA, "PENDING", emp, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), "%ann%").
		WillReturnRows(pgxmock.NewRows(listColNames).
			AddRow(requestID, companyA, employeeID, workedOn, 90, "x", StatusPending, ptr(approvalID), "", (*int64)(nil), (*time.Time)(nil), createdAt, createdAt, "Ann", "E-1", "Finance"))

	out, err := svc.ListAll(context.Background(), companyA, Filter{Status: "pending", EmployeeID: &emp, From: &from, Search: "Ann"})
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "Ann", out[0].EmployeeName)
	require.Equal(t, "Finance", out[0].Department)
	require.NoError(t, db.ExpectationsWereMet())

	_, err = svc.ListAll(context.Background(), 0, Filter{})
	require.ErrorIs(t, err, ErrCompanyRequired)
}

func TestListOwnScoped(t *testing.T) {
	db, svc, _, _ := newService(t, nil)
	db.ExpectQuery(regexp.QuoteMeta("WHERE o.company_id=$1 AND e.user_id=$2")).WithArgs(companyA, userID).
		WillReturnRows(pgxmock.NewRows(listColNames))
	out, err := svc.ListOwn(context.Background(), companyA, userID)
	require.NoError(t, err)
	require.Empty(t, out)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestApprovedMinutesBetween(t *testing.T) {
	db, svc, _, _ := newService(t, nil)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	db.ExpectQuery(regexp.QuoteMeta("WHERE o.company_id=$1 AND o.employee_id=$2 AND o.status='APPROVED' AND o.worked_on BETWEEN $3 AND $4")).
		WithArgs(companyA, employeeID, from, to).
		WillReturnRows(pgxmock.NewRows([]string{"sum"}).AddRow(int64(150)))
	got, err := svc.ApprovedMinutesBetween(context.Background(), companyA, employeeID, from, to)
	require.NoError(t, err)
	require.Equal(t, int64(150), got)
	require.NoError(t, db.ExpectationsWereMet())

	_, err = svc.ApprovedMinutesBetween(context.Background(), 0, employeeID, from, to)
	require.ErrorIs(t, err, ErrCompanyRequired)
}

// Compile-time checks for the wiring in cmd/odyssey.
var (
	_ approvals.Finalizer = (*Service)(nil)
	_ Approvals           = (*approvals.Service)(nil)
	_ Notifier            = (*notifications.Dispatcher)(nil)
	_ Audit               = (*shared.AuditLogger)(nil)
)
