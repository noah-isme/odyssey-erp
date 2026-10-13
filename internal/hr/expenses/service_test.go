package expenses

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
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

const (
	companyA   = int64(7)
	companyB   = int64(8)
	userID     = int64(42)
	managerID  = int64(5)
	employeeID = int64(11)
	claimID    = int64(55)
	documentID = int64(300)
	approvalID = int64(900)
	actorID    = int64(99)
)

var (
	claimDate     = time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	createdAt     = time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	claimColNames = []string{"id", "company_id", "employee_id", "claim_date", "category", "description", "amount", "currency", "document_id", "status", "approval_request_id", "apply_error", "decided_by", "decided_at", "created_at", "updated_at"}
	lockedCols    = []string{"company_id", "status", "claim_date", "amount", "currency"}
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

func newService(t *testing.T, store approvals.Store) (pgxmock.PgxPoolIface, *Service, *auditFake) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(db.Close)
	audit := &auditFake{}
	svc := &Service{pool: db, audit: audit}
	if store != nil {
		svc.approvals = approvals.NewService(store, nil)
	}
	return db, svc, audit
}

func ptr(v int64) *int64 { return &v }

func submitInput() SubmitInput {
	return SubmitInput{UserID: userID, CompanyID: companyA, ClaimDate: claimDate.Add(15 * time.Hour), Category: " Travel ", Description: " taxi to client ", Amount: " 125000.50 ", DocumentID: ptr(documentID)}
}

func expectEmployee(db pgxmock.PgxPoolIface, company int64) {
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta("SELECT e.id, m.user_id FROM hr_employees e LEFT JOIN hr_employees m ON m.id=e.manager_id AND m.company_id=e.company_id WHERE e.user_id=$1 AND e.company_id=$2 AND e.status='ACTIVE'")).
		WithArgs(userID, company).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id"}).AddRow(employeeID, ptr(managerID)))
}

const documentSQL = "SELECT d.id FROM documents d WHERE d.id=$1 AND d.company_id=$2"

func expectDocument(db pgxmock.PgxPoolIface) {
	db.ExpectQuery(regexp.QuoteMeta(documentSQL)).WithArgs(documentID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(documentID))
}

func expectInsert(db pgxmock.PgxPoolIface) {
	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO hr_expense_claims(company_id, employee_id, claim_date, category, description, amount, document_id, status) VALUES($1,$2,$3,$4,$5,$6::numeric,$7,'PENDING') RETURNING id, company_id, employee_id, claim_date, category, description, amount::text")).
		WithArgs(companyA, employeeID, claimDate, "Travel", "taxi to client", "125000.50", ptr(documentID)).
		WillReturnRows(pgxmock.NewRows(claimColNames).
			AddRow(claimID, companyA, employeeID, claimDate, "Travel", "taxi to client", "125000.50", "IDR", ptr(documentID), StatusPending, (*int64)(nil), "", (*int64)(nil), (*time.Time)(nil), createdAt, createdAt))
}

func TestSubmitInsertsPendingThenCallsApprovals(t *testing.T) {
	var got approvals.Submission
	var auditAtApproval []string
	store := &approvalsStore{}
	db, svc, audit := newService(t, store)
	store.onCreate = func(in approvals.Submission) {
		got = in
		auditAtApproval = audit.actions()
		// The PENDING insert must already be committed when approvals runs.
		require.NoError(t, db.ExpectationsWereMet())
		db.ExpectExec(regexp.QuoteMeta("UPDATE hr_expense_claims SET approval_request_id=$2, updated_at=NOW() WHERE id=$1 AND company_id=$3")).
			WithArgs(claimID, approvalID, companyA).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	}

	expectEmployee(db, companyA)
	expectDocument(db)
	expectInsert(db)
	db.ExpectCommit()

	claim, err := svc.Submit(context.Background(), submitInput())
	require.NoError(t, err)
	require.Equal(t, StatusPending, claim.Status)
	require.Equal(t, "125000.50", claim.Amount)
	require.Equal(t, "IDR", claim.Currency)
	require.Equal(t, documentID, *claim.DocumentID)
	require.Equal(t, approvalID, *claim.ApprovalRequestID)
	require.Equal(t, []string{ActionSubmitted}, auditAtApproval)
	require.Equal(t, "125000.50", audit.logs[0].Meta["amount"])
	require.Equal(t, documentID, audit.logs[0].Meta["document_id"])
	require.Equal(t, Module, got.Module)
	require.Equal(t, claimID, got.DocumentID)
	require.Equal(t, userID, got.RequesterID)
	require.Equal(t, companyA, *got.CompanyID)
	require.Equal(t, 125000.5, got.Amount)
	require.Equal(t, managerID, got.ManagerID)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestSubmitWithoutDocumentSkipsDocumentCheck(t *testing.T) {
	store := &approvalsStore{}
	db, svc, _ := newService(t, store)
	store.onCreate = func(approvals.Submission) {
		db.ExpectExec("UPDATE hr_expense_claims SET approval_request_id").
			WithArgs(claimID, approvalID, companyA).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	}
	expectEmployee(db, companyA)
	db.ExpectQuery("INSERT INTO hr_expense_claims").
		WithArgs(companyA, employeeID, claimDate, "Meals", "", "75", (*int64)(nil)).
		WillReturnRows(pgxmock.NewRows(claimColNames).
			AddRow(claimID, companyA, employeeID, claimDate, "Meals", "", "75.00", "IDR", (*int64)(nil), StatusPending, (*int64)(nil), "", (*int64)(nil), (*time.Time)(nil), createdAt, createdAt))
	db.ExpectCommit()

	claim, err := svc.Submit(context.Background(), SubmitInput{UserID: userID, CompanyID: companyA, ClaimDate: claimDate, Category: "Meals", Amount: "75"})
	require.NoError(t, err)
	require.Equal(t, "75.00", claim.Amount)
	require.Nil(t, claim.DocumentID)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestSubmitCrossTenantDocumentRejected(t *testing.T) {
	called := false
	db, svc, audit := newService(t, &approvalsStore{onCreate: func(approvals.Submission) { called = true }})
	expectEmployee(db, companyA)
	// Document 300 belongs to company B; the company-scoped lookup finds nothing.
	db.ExpectQuery(regexp.QuoteMeta(documentSQL)).WithArgs(documentID, companyA).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()

	_, err := svc.Submit(context.Background(), submitInput())
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, err, ErrDocumentNotFound)
	require.EqualError(t, err, "hr: not found: document")
	require.False(t, called)
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
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
			db, svc, audit := newService(t, tc.store)
			expectEmployee(db, companyA)
			expectDocument(db)
			expectInsert(db)
			db.ExpectCommit()
			db.ExpectBegin()
			db.ExpectExec(regexp.QuoteMeta("UPDATE hr_expense_claims SET status='CANCELLED', updated_at=NOW() WHERE id=$1 AND status='PENDING'")).
				WithArgs(claimID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			db.ExpectCommit()

			_, err := svc.Submit(context.Background(), submitInput())
			require.ErrorIs(t, err, tc.want)
			require.Equal(t, []string{ActionSubmitted, ActionSubmitFailed}, audit.actions())
			require.Equal(t, tc.class, audit.logs[1].Meta["error_class"])
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}

func TestSubmitEmployeeNotInCompany(t *testing.T) {
	db, svc, _ := newService(t, &approvalsStore{})
	db.ExpectBegin()
	db.ExpectQuery("SELECT e.id, m.user_id FROM hr_employees").WithArgs(userID, companyB).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()
	in := submitInput()
	in.CompanyID = companyB
	_, err := svc.Submit(context.Background(), in)
	require.ErrorIs(t, err, ErrEmployeeNotFound)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestSubmitManagerRequired(t *testing.T) {
	db, svc, _ := newService(t, &approvalsStore{})
	db.ExpectBegin()
	db.ExpectQuery("SELECT e.id, m.user_id FROM hr_employees").WithArgs(userID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id"}).AddRow(employeeID, (*int64)(nil)))
	db.ExpectRollback()
	_, err := svc.Submit(context.Background(), submitInput())
	require.ErrorIs(t, err, ErrManagerRequired)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestSubmitValidation(t *testing.T) {
	svc := &Service{audit: &auditFake{}}
	cases := []struct {
		name string
		mut  func(*SubmitInput)
		want error
	}{
		{"company", func(in *SubmitInput) { in.CompanyID = 0 }, ErrCompanyRequired},
		{"user", func(in *SubmitInput) { in.UserID = 0 }, ErrInvalidRequest},
		{"date", func(in *SubmitInput) { in.ClaimDate = time.Time{} }, ErrInvalidRequest},
		{"document id", func(in *SubmitInput) { in.DocumentID = ptr(0) }, ErrInvalidRequest},
		{"category", func(in *SubmitInput) { in.Category = "  " }, ErrCategoryRequired},
		{"amount", func(in *SubmitInput) { in.Amount = "abc" }, ErrInvalidAmount},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := submitInput()
			tc.mut(&in)
			_, err := svc.Submit(context.Background(), in)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestNormalizeAmount(t *testing.T) {
	for _, ok := range []struct{ in, want string }{
		{"1", "1"}, {" 125000.50 ", "125000.50"}, {"0.01", "0.01"}, {"9999999999999999.99", "9999999999999999.99"}, {"10.5", "10.5"},
	} {
		got, err := NormalizeAmount(ok.in)
		require.NoError(t, err, ok.in)
		require.Equal(t, ok.want, got)
	}
	for _, bad := range []string{"", "0", "0.00", "-1", "+1", "1.234", "1e3", "1,000", ".5", "5.", "NaN", "Inf", "12345678901234567", "1 000", "0x10"} {
		_, err := NormalizeAmount(bad)
		require.ErrorIs(t, err, ErrInvalidAmount, bad)
	}
}

// ---- FinalizeApproval ------------------------------------------------------

func approvalReq(company int64) approvals.Request {
	return approvals.Request{ID: approvalID, Module: Module, DocumentID: claimID, RequesterID: userID, CompanyID: &company, Status: approvals.StatusApproved}
}

func expectLockedRow(db pgxmock.PgxPoolIface, status string, company int64) {
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta("SELECT c.company_id, c.status, c.claim_date, c.amount::text, c.currency FROM hr_expense_claims c WHERE c.id=$1 FOR UPDATE OF c")).
		WithArgs(claimID).
		WillReturnRows(pgxmock.NewRows(lockedCols).AddRow(company, status, claimDate, "125000.50", "IDR"))
}

const decideSQL = "UPDATE hr_expense_claims SET status=$3, apply_error='', decided_by=$2, decided_at=NOW(), updated_at=NOW() WHERE id=$1"

func TestFinalizeApproval(t *testing.T) {
	cases := []struct {
		decision string
		want     string
		action   string
	}{
		{approvals.StatusApproved, StatusApproved, ActionApproved},
		{approvals.StatusRejected, StatusRejected, ActionRejected},
	}
	for _, tc := range cases {
		t.Run(tc.decision, func(t *testing.T) {
			db, svc, audit := newService(t, nil)
			expectLockedRow(db, StatusPending, companyA)
			db.ExpectExec(regexp.QuoteMeta(decideSQL)).WithArgs(claimID, actorID, tc.want).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			db.ExpectCommit()

			require.NoError(t, svc.FinalizeApproval(context.Background(), approvalReq(companyA), tc.decision, actorID, " ok "))
			require.Equal(t, []string{tc.action}, audit.actions())
			require.Equal(t, actorID, audit.logs[0].ActorID)
			require.Equal(t, "55", audit.logs[0].EntityID)
			require.Equal(t, "ok", audit.logs[0].Meta["note"])
			require.Equal(t, "125000.50", audit.logs[0].Meta["amount"])
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}

func TestFinalizeApprovalSecondCallIsNoOp(t *testing.T) {
	for _, st := range []string{StatusApproved, StatusRejected, StatusCancelled} {
		t.Run(st, func(t *testing.T) {
			db, svc, audit := newService(t, nil)
			expectLockedRow(db, st, companyA)
			db.ExpectRollback()

			require.NoError(t, svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, ""))
			require.Empty(t, audit.logs)
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}

func TestFinalizeApprovalRejectsCompanyMismatch(t *testing.T) {
	db, svc, audit := newService(t, nil)
	expectLockedRow(db, StatusPending, companyB)
	db.ExpectRollback()

	err := svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, "")
	require.ErrorIs(t, err, ErrCompanyMismatch)
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())

	db2, svc2, _ := newService(t, nil)
	expectLockedRow(db2, StatusPending, companyA)
	db2.ExpectRollback()
	err = svc2.FinalizeApproval(context.Background(), approvals.Request{DocumentID: claimID}, approvals.StatusApproved, actorID, "")
	require.ErrorIs(t, err, ErrCompanyMismatch)
	require.NoError(t, db2.ExpectationsWereMet())
}

func TestFinalizeApprovalInvalidDecision(t *testing.T) {
	_, svc, _ := newService(t, nil)
	require.ErrorIs(t, svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusPending, actorID, ""), ErrInvalidDecision)
}

func TestFinalizeApprovalApplyFailureStoresApplyError(t *testing.T) {
	db, svc, audit := newService(t, nil)
	expectLockedRow(db, StatusPending, companyA)
	db.ExpectExec(regexp.QuoteMeta(decideSQL)).WithArgs(claimID, actorID, StatusApproved).WillReturnError(errors.New("disk full"))
	db.ExpectRollback()
	db.ExpectExec(regexp.QuoteMeta("UPDATE hr_expense_claims SET apply_error=$2, updated_at=NOW() WHERE id=$1 AND status='PENDING'")).
		WithArgs(claimID, "finalize expense: approved: disk full").WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err := svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, "")
	require.ErrorContains(t, err, "disk full")
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

// ---- RetryApply ------------------------------------------------------------

var approvalCols = []string{"id", "policy_id", "module", "document_id", "company_id", "amount", "requester_id", "current_step", "status", "submitted_at"}

const retryClaimSQL = "SELECT c.status, c.approval_request_id FROM hr_expense_claims c WHERE c.id=$1 AND c.company_id=$2"

func expectRetryReads(db pgxmock.PgxPoolIface, approvalStatus string) {
	db.ExpectQuery(regexp.QuoteMeta(retryClaimSQL)).
		WithArgs(claimID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"status", "approval_request_id"}).AddRow(StatusPending, ptr(approvalID)))
	db.ExpectQuery(regexp.QuoteMeta("FROM approval_requests WHERE id=$1 AND module=$2 AND document_id=$3")).
		WithArgs(approvalID, Module, claimID).
		WillReturnRows(pgxmock.NewRows(approvalCols).AddRow(approvalID, int64(1), Module, claimID, ptr(companyA), 125000.5, userID, 1, approvalStatus, createdAt))
}

func TestRetryApply(t *testing.T) {
	t.Run("approved applies", func(t *testing.T) {
		db, svc, audit := newService(t, nil)
		expectRetryReads(db, approvals.StatusApproved)
		expectLockedRow(db, StatusPending, companyA)
		db.ExpectExec(regexp.QuoteMeta(decideSQL)).WithArgs(claimID, actorID, StatusApproved).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()

		require.NoError(t, svc.RetryApply(context.Background(), companyA, claimID, actorID))
		require.Equal(t, []string{ActionApproved}, audit.actions())
		require.Equal(t, "retry apply", audit.logs[0].Meta["note"])
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("rejected applies", func(t *testing.T) {
		db, svc, audit := newService(t, nil)
		expectRetryReads(db, approvals.StatusRejected)
		expectLockedRow(db, StatusPending, companyA)
		db.ExpectExec(regexp.QuoteMeta(decideSQL)).WithArgs(claimID, actorID, StatusRejected).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()

		require.NoError(t, svc.RetryApply(context.Background(), companyA, claimID, actorID))
		require.Equal(t, []string{ActionRejected}, audit.actions())
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("approval still pending", func(t *testing.T) {
		db, svc, _ := newService(t, nil)
		expectRetryReads(db, approvals.StatusPending)
		require.ErrorIs(t, svc.RetryApply(context.Background(), companyA, claimID, actorID), ErrApprovalNotDecided)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("not pending", func(t *testing.T) {
		db, svc, _ := newService(t, nil)
		db.ExpectQuery(regexp.QuoteMeta(retryClaimSQL)).WithArgs(claimID, companyA).
			WillReturnRows(pgxmock.NewRows([]string{"status", "approval_request_id"}).AddRow(StatusApproved, ptr(approvalID)))
		require.ErrorIs(t, svc.RetryApply(context.Background(), companyA, claimID, actorID), ErrNotRetryable)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("no approval", func(t *testing.T) {
		db, svc, _ := newService(t, nil)
		db.ExpectQuery(regexp.QuoteMeta(retryClaimSQL)).WithArgs(claimID, companyA).
			WillReturnRows(pgxmock.NewRows([]string{"status", "approval_request_id"}).AddRow(StatusPending, (*int64)(nil)))
		require.ErrorIs(t, svc.RetryApply(context.Background(), companyA, claimID, actorID), ErrNoApproval)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("cross tenant not found", func(t *testing.T) {
		db, svc, _ := newService(t, nil)
		db.ExpectQuery(regexp.QuoteMeta(retryClaimSQL)).WithArgs(claimID, companyB).WillReturnError(pgx.ErrNoRows)
		err := svc.RetryApply(context.Background(), companyB, claimID, actorID)
		require.ErrorIs(t, err, ErrNotFound)
		require.EqualError(t, err, "hr: not found: expense claim")
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("company required", func(t *testing.T) {
		require.ErrorIs(t, (&Service{}).RetryApply(context.Background(), 0, claimID, actorID), ErrCompanyRequired)
	})
}

// ---- Cancel ----------------------------------------------------------------

const cancelLockSQL = "SELECT c.status, c.approval_request_id FROM hr_expense_claims c JOIN hr_employees e ON e.id=c.employee_id WHERE c.id=$1 AND e.user_id=$2 AND c.company_id=$3 AND e.company_id=$3 FOR UPDATE OF c"

func TestCancelCancelsApprovalAndAssignments(t *testing.T) {
	db, svc, audit := newService(t, nil)
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta(cancelLockSQL)).
		WithArgs(claimID, userID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"status", "approval_request_id"}).AddRow(StatusPending, ptr(approvalID)))
	db.ExpectExec(regexp.QuoteMeta("UPDATE hr_expense_claims SET status='CANCELLED', updated_at=NOW() WHERE id=$1")).WithArgs(claimID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(regexp.QuoteMeta("UPDATE approval_requests SET status='CANCELLED', updated_at=NOW() WHERE id=$1 AND status='PENDING'")).
		WithArgs(approvalID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(regexp.QuoteMeta("UPDATE approval_assignments SET status='CANCELLED', updated_at=NOW() WHERE request_id=$1 AND status='PENDING'")).
		WithArgs(approvalID).WillReturnResult(pgxmock.NewResult("UPDATE", 2))
	db.ExpectCommit()

	require.NoError(t, svc.Cancel(context.Background(), companyA, claimID, userID))
	require.Equal(t, []string{ActionCancelled}, audit.actions())
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCancelCrossTenantNotFound(t *testing.T) {
	db, svc, audit := newService(t, nil)
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta(cancelLockSQL)).WithArgs(claimID, userID, companyB).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()

	err := svc.Cancel(context.Background(), companyB, claimID, userID)
	require.ErrorIs(t, err, ErrNotFound)
	require.EqualError(t, err, "hr: not found: expense claim")
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCancelOnlyPending(t *testing.T) {
	db, svc, _ := newService(t, nil)
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta(cancelLockSQL)).WithArgs(claimID, userID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"status", "approval_request_id"}).AddRow(StatusApproved, ptr(approvalID)))
	db.ExpectRollback()
	require.ErrorIs(t, svc.Cancel(context.Background(), companyA, claimID, userID), ErrNotPending)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCancelCompanyRequired(t *testing.T) {
	require.ErrorIs(t, (&Service{}).Cancel(context.Background(), 0, claimID, userID), ErrCompanyRequired)
}

// ---- Lists -----------------------------------------------------------------

var listColNames = append(append([]string{}, claimColNames...), "name", "employee_number", "department")

func TestListAllScopedAndFiltered(t *testing.T) {
	db, svc, _ := newService(t, nil)
	emp := employeeID
	from := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	db.ExpectQuery(regexp.QuoteMeta("WHERE c.company_id=$1 AND c.status=$2 AND c.employee_id=$3 AND c.claim_date>=$4 AND c.claim_date<=$5 AND (LOWER(e.name) LIKE $6")).
		WithArgs(companyA, "PENDING", emp, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), to, "%ann%").
		WillReturnRows(pgxmock.NewRows(listColNames).
			AddRow(claimID, companyA, employeeID, claimDate, "Travel", "x", "125000.50", "IDR", ptr(documentID), StatusPending, ptr(approvalID), "", (*int64)(nil), (*time.Time)(nil), createdAt, createdAt, "Ann", "E-1", "Finance"))

	out, err := svc.ListAll(context.Background(), companyA, Filter{Status: "pending", EmployeeID: &emp, From: &from, To: &to, Search: "Ann"})
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "Ann", out[0].EmployeeName)
	require.Equal(t, "Finance", out[0].Department)
	require.Equal(t, "125000.50", out[0].Amount)
	require.NoError(t, db.ExpectationsWereMet())

	_, err = svc.ListAll(context.Background(), 0, Filter{})
	require.ErrorIs(t, err, ErrCompanyRequired)
}

func TestListOwnScoped(t *testing.T) {
	db, svc, _ := newService(t, nil)
	db.ExpectQuery(regexp.QuoteMeta("JOIN hr_employees e ON e.id=c.employee_id AND e.company_id=c.company_id")).
		WithArgs(companyA, userID).
		WillReturnRows(pgxmock.NewRows(listColNames))
	out, err := svc.ListOwn(context.Background(), companyA, userID)
	require.NoError(t, err)
	require.Empty(t, out)
	require.NoError(t, db.ExpectationsWereMet())

	_, err = svc.ListOwn(context.Background(), 0, userID)
	require.ErrorIs(t, err, ErrCompanyRequired)
}

// Compile-time checks for the wiring in cmd/odyssey.
var (
	_ approvals.Finalizer = (*Service)(nil)
	_ Approvals           = (*approvals.Service)(nil)
	_ Audit               = (*shared.AuditLogger)(nil)
)
