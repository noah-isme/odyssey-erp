package profilechanges

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/approvals"
	"github.com/odyssey-erp/odyssey-erp/internal/hr/compensation"
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
	contactB   = int64(301)
)

var (
	createdAt       = time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	requestColNames = []string{"id", "company_id", "employee_id", "change_type", "payload", "status", "approval_request_id", "apply_error", "applied_at", "decided_by", "created_at", "updated_at"}
	lockedColNames  = []string{"company_id", "employee_id", "change_type", "payload", "status"}
	assignmentCols  = []string{"id", "employee_id", "base_salary", "ptkp_code", "bpjs_health", "bpjs_employment",
		"bank_code", "bank_account_number", "bank_account_name", "effective_from", "effective_to", "reason", "created_by", "created_at"}
	approvalCols = []string{"id", "policy_id", "module", "document_id", "company_id", "amount", "requester_id", "current_step", "status", "submitted_at"}
)

func q(s string) string { return regexp.QuoteMeta(s) }

func ptr(v int64) *int64 { return &v }

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func ptrTime(t time.Time) *time.Time { return &t }

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

// requireNoPayloadInAudit asserts every package audit entry carries
// change_type and none of the payload keys or values.
func requireNoPayloadInAudit(t *testing.T, logs []shared.AuditLog, payload map[string]string) {
	t.Helper()
	for _, l := range logs {
		if l.Entity != entity {
			continue
		}
		require.Contains(t, l.Meta, "change_type", l.Action)
		for k, v := range l.Meta {
			_, isPayloadKey := payload[k]
			require.False(t, isPayloadKey, "audit %s leaks payload key %s", l.Action, k)
			for _, pv := range payload {
				if pv != "" {
					require.NotEqual(t, pv, fmt.Sprint(v), "audit %s leaks payload value", l.Action)
				}
			}
		}
	}
}

// approvalsStore embeds approvals.Store so only Submit's methods are needed.
type approvalsStore struct {
	approvals.Store
	resolveErr error
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
	return approvals.Request{ID: approvalID, Module: Module, Status: approvals.StatusPending}, nil, nil
}

func newService(t *testing.T, store approvals.Store) (pgxmock.PgxPoolIface, *Service, *auditFake) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(db.Close)
	audit := &auditFake{}
	svc := &Service{pool: db, audit: audit, bank: compensation.NewService(nil, audit)}
	if store != nil {
		svc.approvals = approvals.NewService(store, nil)
	}
	return db, svc, audit
}

func payloadJSON(t *testing.T, p map[string]string) []byte {
	t.Helper()
	b, err := json.Marshal(p)
	require.NoError(t, err)
	return b
}

func approvalReq(company int64) approvals.Request {
	return approvals.Request{ID: approvalID, Module: Module, DocumentID: requestID, CompanyID: ptr(company), RequesterID: userID, Status: approvals.StatusApproved}
}

// ---- NormalizePayload ------------------------------------------------------

func TestNormalizePayload(t *testing.T) {
	cases := []struct {
		name    string
		typ     string
		payload map[string]string
		want    map[string]string
		err     error
	}{
		{"address ok", TypeAddress, map[string]string{"address_line1": " Jl. Merdeka 1 ", "city": "Bandung", "phone": "0812"}, map[string]string{"address_line1": "Jl. Merdeka 1", "city": "Bandung", "phone": "0812"}, nil},
		{"address empty", TypeAddress, map[string]string{}, nil, ErrInvalidPayload},
		{"address blank line1", TypeAddress, map[string]string{"address_line1": " "}, nil, ErrInvalidPayload},
		{"address unknown key", TypeAddress, map[string]string{"address_line1": "x", "national_id": "1"}, nil, ErrInvalidPayload},
		{"bank ok", TypeBank, map[string]string{"bank_code": "BCA", "bank_account_number": "123", "bank_account_name": "Budi"}, map[string]string{"bank_code": "BCA", "bank_account_number": "123", "bank_account_name": "Budi"}, nil},
		{"bank missing number", TypeBank, map[string]string{"bank_code": "BCA"}, nil, ErrInvalidPayload},
		{"bank unknown key", TypeBank, map[string]string{"bank_code": "BCA", "bank_account_number": "1", "base_salary": "9"}, nil, ErrInvalidPayload},
		{"contact ok", TypeEmergencyContact, map[string]string{"name": "Sari", "is_primary": "on", "contact_id": "", "email": "sari@example.com"}, map[string]string{"name": "Sari", "is_primary": "true", "email": "sari@example.com"}, nil},
		{"contact id normalized", TypeEmergencyContact, map[string]string{"name": "Sari", "contact_id": "007"}, map[string]string{"name": "Sari", "contact_id": "7"}, nil},
		{"contact bad id", TypeEmergencyContact, map[string]string{"name": "Sari", "contact_id": "-1"}, nil, ErrInvalidPayload},
		{"contact bad email", TypeEmergencyContact, map[string]string{"name": "Sari", "email": "nope"}, nil, ErrInvalidPayload},
		{"contact no name", TypeEmergencyContact, map[string]string{"phone": "1"}, nil, ErrInvalidPayload},
		{"bad type", "SALARY", map[string]string{"x": "1"}, nil, ErrInvalidChangeType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizePayload(tc.typ, tc.payload)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// ---- Submit ----------------------------------------------------------------

func expectEmployee(db pgxmock.PgxPoolIface, company int64) {
	db.ExpectBegin()
	db.ExpectQuery(q("SELECT e.id, m.user_id FROM hr_employees e LEFT JOIN hr_employees m ON m.id=e.manager_id AND m.company_id=e.company_id WHERE e.user_id=$1 AND e.company_id=$2 AND e.status='ACTIVE'")).
		WithArgs(userID, company).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id"}).AddRow(employeeID, ptr(managerID)))
}

func TestSubmitInsertsPendingThenCallsApprovals(t *testing.T) {
	payload := map[string]string{"bank_code": "BNI", "bank_account_number": "987654", "bank_account_name": "Budi"}
	var got approvals.Submission
	store := &approvalsStore{}
	db, svc, audit := newService(t, store)
	store.onCreate = func(in approvals.Submission) {
		got = in
		require.NoError(t, db.ExpectationsWereMet(), "PENDING insert must be committed before approvals runs")
		db.ExpectExec(q("UPDATE hr_profile_change_requests SET approval_request_id=$2, updated_at=NOW() WHERE id=$1 AND company_id=$3")).
			WithArgs(requestID, approvalID, companyA).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	}
	expectEmployee(db, companyA)
	raw := payloadJSON(t, payload)
	db.ExpectQuery(q("INSERT INTO hr_profile_change_requests(company_id, employee_id, change_type, payload, status) VALUES($1,$2,$3,$4::jsonb,'PENDING')")).
		WithArgs(companyA, employeeID, TypeBank, string(raw)).
		WillReturnRows(pgxmock.NewRows(requestColNames).AddRow(requestID, companyA, employeeID, TypeBank, raw, StatusPending, (*int64)(nil), "", (*time.Time)(nil), (*int64)(nil), createdAt, createdAt))
	db.ExpectCommit()

	req, err := svc.Submit(context.Background(), SubmitInput{UserID: userID, CompanyID: companyA, ChangeType: " bank ", Payload: payload})
	require.NoError(t, err)
	require.Equal(t, StatusPending, req.Status)
	require.Equal(t, payload, req.Payload)
	require.Equal(t, approvalID, *req.ApprovalRequestID)
	require.Equal(t, Module, got.Module)
	require.Equal(t, requestID, got.DocumentID)
	require.Equal(t, companyA, *got.CompanyID)
	require.Equal(t, 0.0, got.Amount)
	require.Equal(t, managerID, got.ManagerID)
	require.Equal(t, []string{ActionSubmitted}, audit.actions())
	require.Equal(t, map[string]any{"change_type": TypeBank}, audit.logs[0].Meta)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestSubmitNoPolicyCompensates(t *testing.T) {
	payload := map[string]string{"address_line1": "Jl. Asia Afrika 8"}
	db, svc, audit := newService(t, &approvalsStore{resolveErr: approvals.ErrNoPolicy})
	expectEmployee(db, companyA)
	raw := payloadJSON(t, payload)
	db.ExpectQuery("INSERT INTO hr_profile_change_requests").
		WithArgs(companyA, employeeID, TypeAddress, string(raw)).
		WillReturnRows(pgxmock.NewRows(requestColNames).AddRow(requestID, companyA, employeeID, TypeAddress, raw, StatusPending, (*int64)(nil), "", (*time.Time)(nil), (*int64)(nil), createdAt, createdAt))
	db.ExpectCommit()
	db.ExpectBegin()
	db.ExpectExec(q("UPDATE hr_profile_change_requests SET status='CANCELLED', updated_at=NOW() WHERE id=$1 AND status='PENDING'")).
		WithArgs(requestID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	_, err := svc.Submit(context.Background(), SubmitInput{UserID: userID, CompanyID: companyA, ChangeType: TypeAddress, Payload: payload})
	require.ErrorIs(t, err, ErrNoPolicy)
	require.EqualError(t, err, "hr: no approval policy configured for profile changes")
	require.Equal(t, []string{ActionSubmitted, ActionSubmitFailed}, audit.actions())
	require.Equal(t, "no_policy", audit.logs[1].Meta["error_class"])
	requireNoPayloadInAudit(t, audit.logs, payload)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestSubmitRejectsCrossTenantContactID(t *testing.T) {
	called := false
	db, svc, audit := newService(t, &approvalsStore{onCreate: func(approvals.Submission) { called = true }})
	expectEmployee(db, companyA)
	// contactB belongs to an employee of company B: the company-scoped join finds nothing.
	db.ExpectQuery(q("SELECT c.id FROM hr_employee_emergency_contacts c JOIN hr_employees e ON e.id=c.employee_id AND e.company_id=$3 WHERE c.id=$1 AND c.employee_id=$2")).
		WithArgs(contactB, employeeID, companyA).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()

	_, err := svc.Submit(context.Background(), SubmitInput{UserID: userID, CompanyID: companyA, ChangeType: TypeEmergencyContact, Payload: map[string]string{"contact_id": fmt.Sprint(contactB), "name": "Sari"}})
	require.ErrorIs(t, err, ErrContactNotFound)
	require.ErrorIs(t, err, ErrNotFound)
	require.Contains(t, err.Error(), "hr: not found")
	require.False(t, called)
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestSubmitValidatesBeforeDB(t *testing.T) {
	db, svc, _ := newService(t, &approvalsStore{})
	_, err := svc.Submit(context.Background(), SubmitInput{UserID: userID, CompanyID: 0, ChangeType: TypeBank})
	require.ErrorIs(t, err, ErrCompanyRequired)
	_, err = svc.Submit(context.Background(), SubmitInput{UserID: userID, CompanyID: companyA, ChangeType: TypeBank, Payload: map[string]string{"bank_code": "BCA"}})
	require.ErrorIs(t, err, ErrInvalidPayload)
	_, err = svc.Submit(context.Background(), SubmitInput{UserID: userID, CompanyID: companyA, ChangeType: "PHOTO", Payload: map[string]string{"x": "1"}})
	require.ErrorIs(t, err, ErrInvalidChangeType)
	require.NoError(t, db.ExpectationsWereMet())
}

// ---- FinalizeApproval ------------------------------------------------------

func expectLocked(t *testing.T, db pgxmock.PgxPoolIface, company int64, changeType string, payload map[string]string, status string) {
	t.Helper()
	db.ExpectBegin()
	db.ExpectQuery(q("SELECT p.company_id, p.employee_id, p.change_type, p.payload, p.status FROM hr_profile_change_requests p WHERE p.id=$1 FOR UPDATE OF p")).
		WithArgs(requestID).
		WillReturnRows(pgxmock.NewRows(lockedColNames).AddRow(company, employeeID, changeType, payloadJSON(t, payload), status))
}

func expectApproved(db pgxmock.PgxPoolIface) {
	db.ExpectExec(q("UPDATE hr_profile_change_requests SET status='APPROVED', apply_error='', applied_at=NOW(), decided_by=$2, updated_at=NOW() WHERE id=$1")).
		WithArgs(requestID, actorID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()
}

func TestFinalizeAddressUpdatesEmployee(t *testing.T) {
	payload := map[string]string{"address_line1": "Jl. Braga 5", "city": "Bandung", "postal_code": "40111"}
	db, svc, audit := newService(t, nil)
	expectLocked(t, db, companyA, TypeAddress, payload, StatusPending)
	db.ExpectExec(q("UPDATE hr_employees SET address_line1=$3, city=$4, postal_code=$5, updated_at=NOW() WHERE id=$1 AND company_id=$2")).
		WithArgs(employeeID, companyA, "Jl. Braga 5", "Bandung", "40111").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	expectApproved(db)

	require.NoError(t, svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, "ok Jl. Braga 5"))
	require.Equal(t, []string{ActionApplied}, audit.actions())
	requireNoPayloadInAudit(t, audit.logs, payload)
	require.NotContains(t, audit.logs[0].Meta, "note")
	require.NoError(t, db.ExpectationsWereMet())
}

func TestFinalizeEmergencyContact(t *testing.T) {
	t.Run("new primary contact", func(t *testing.T) {
		payload := map[string]string{"name": "Sari", "relationship": "Spouse", "phone": "0812", "is_primary": "true"}
		db, svc, audit := newService(t, nil)
		expectLocked(t, db, companyA, TypeEmergencyContact, payload, StatusPending)
		db.ExpectQuery(q("SELECT id FROM hr_employees WHERE id=$1 AND company_id=$2 FOR UPDATE")).
			WithArgs(employeeID, companyA).WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(employeeID))
		db.ExpectQuery(q("INSERT INTO hr_employee_emergency_contacts(company_id,employee_id,name,relationship,phone,email,is_primary)")).
			WithArgs(companyA, employeeID, "Sari", "Spouse", "0812", "", true).WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(70)))
		db.ExpectExec(q("UPDATE hr_employee_emergency_contacts SET is_primary=FALSE, updated_at=NOW() WHERE employee_id=$1 AND id<>$2 AND is_primary")).
			WithArgs(employeeID, int64(70)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		expectApproved(db)

		require.NoError(t, svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, ""))
		require.Equal(t, []string{ActionApplied}, audit.actions())
		requireNoPayloadInAudit(t, audit.logs, payload)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("update existing contact", func(t *testing.T) {
		payload := map[string]string{"contact_id": "70", "name": "Sari W", "is_primary": "false"}
		db, svc, _ := newService(t, nil)
		expectLocked(t, db, companyA, TypeEmergencyContact, payload, StatusPending)
		db.ExpectQuery(q("SELECT id FROM hr_employees WHERE id=$1 AND company_id=$2 FOR UPDATE")).
			WithArgs(employeeID, companyA).WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(employeeID))
		db.ExpectQuery(q("UPDATE hr_employee_emergency_contacts c SET name=$1")+`(.|\n)*`+q("e.company_id=$6 AND c.employee_id=$7 AND c.id=$8 RETURNING c.id")).
			WithArgs("Sari W", "", "", "", false, companyA, employeeID, int64(70)).WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(70)))
		expectApproved(db)

		require.NoError(t, svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, ""))
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("cross tenant contact id stores apply error", func(t *testing.T) {
		payload := map[string]string{"contact_id": fmt.Sprint(contactB), "name": "Sari"}
		db, svc, audit := newService(t, nil)
		expectLocked(t, db, companyA, TypeEmergencyContact, payload, StatusPending)
		db.ExpectQuery(q("SELECT id FROM hr_employees WHERE id=$1 AND company_id=$2 FOR UPDATE")).
			WithArgs(employeeID, companyA).WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(employeeID))
		db.ExpectQuery(q("UPDATE hr_employee_emergency_contacts c SET name=$1")).
			WithArgs("Sari", "", "", "", false, companyA, employeeID, contactB).WillReturnError(pgx.ErrNoRows)
		db.ExpectRollback()
		db.ExpectExec(q("UPDATE hr_profile_change_requests SET apply_error=$2, updated_at=NOW() WHERE id=$1 AND status='PENDING'")).
			WithArgs(requestID, "hr: not found: emergency contact").WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		err := svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, "")
		require.ErrorIs(t, err, ErrContactNotFound)
		require.Empty(t, audit.logs)
		require.NoError(t, db.ExpectationsWereMet())
	})
}

func expectCompLock(db pgxmock.PgxPoolIface, rows *pgxmock.Rows) {
	db.ExpectQuery(q("SELECT e.id FROM hr_employees e WHERE e.id=$2 AND e.company_id=$1 FOR NO KEY UPDATE")).
		WithArgs(companyA, employeeID).WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(employeeID))
	db.ExpectQuery(q("FROM payroll_compensation_assignments ca")+`(.|\n)*`+q("FOR UPDATE OF ca")).
		WithArgs(companyA, employeeID).WillReturnRows(rows)
}

func compRow(rows *pgxmock.Rows, id int64, salary string, bank [3]string, from time.Time, to *time.Time) *pgxmock.Rows {
	return rows.AddRow(id, employeeID, salary, "TK/0", true, true, bank[0], bank[1], bank[2], from, to, "", (*int64)(nil), createdAt)
}

func expectDecisionDate(db pgxmock.PgxPoolIface, d time.Time) {
	db.ExpectQuery(q(decisionDateSQL)).WithArgs(approvalID).WillReturnRows(pgxmock.NewRows([]string{"date"}).AddRow(d))
}

func TestFinalizeBankSplitsAndPropagatesToFutureRaise(t *testing.T) {
	payload := map[string]string{"bank_code": "MANDIRI", "bank_account_number": "222", "bank_account_name": "Budi S"}
	oldBank := [3]string{"BCA", "111", "Budi"}
	decided := day(2026, 5, 1)
	raiseFrom := day(2026, 9, 1)
	db, svc, audit := newService(t, nil)
	expectLocked(t, db, companyA, TypeBank, payload, StatusPending)
	expectDecisionDate(db, decided)
	rows := pgxmock.NewRows(assignmentCols)
	compRow(rows, 10, "8000000.00", oldBank, day(2026, 1, 1), ptrTime(day(2026, 8, 31)))
	compRow(rows, 11, "9000000.00", oldBank, raiseFrom, nil)
	expectCompLock(db, rows)
	db.ExpectExec(q("UPDATE payroll_compensation_assignments SET effective_to=$2 WHERE id=$1")).
		WithArgs(int64(10), day(2026, 4, 30)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	// New row: salary/PTKP/BPJS copied, new bank, effective from the decision date.
	db.ExpectQuery(q("INSERT INTO payroll_compensation_assignments AS ca")).
		WithArgs(employeeID, "8000000.00", "TK/0", true, true, "MANDIRI", "222", "Budi S", decided, ptrTime(day(2026, 8, 31)), BankChangeReason, ptr(actorID)).
		WillReturnRows(compRow(pgxmock.NewRows(assignmentCols), 12, "8000000.00", [3]string{"MANDIRI", "222", "Budi S"}, decided, ptrTime(day(2026, 8, 31))))
	// Recurring components move from 10 onto 12.
	db.ExpectExec(q("INSERT INTO payroll_recurring_components(assignment_id, component_id, amount, effective_from, effective_to)")).
		WithArgs(int64(12), int64(10), decided).WillReturnResult(pgxmock.NewResult("INSERT", 2))
	db.ExpectExec(q("DELETE FROM payroll_recurring_components")).
		WithArgs(int64(10), decided).WillReturnResult(pgxmock.NewResult("DELETE", 0))
	db.ExpectExec(q("UPDATE payroll_recurring_components SET effective_to = $3::date")).
		WithArgs(int64(10), decided, day(2026, 4, 30)).WillReturnResult(pgxmock.NewResult("UPDATE", 2))
	// The future-dated raise (11) receives the new bank fields.
	db.ExpectExec(q("WHERE employee_id=$4 AND effective_from > $5")).
		WithArgs("MANDIRI", "222", "Budi S", employeeID, decided).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	expectApproved(db)

	require.NoError(t, svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, ""))
	require.Equal(t, []string{compensation.ActionBankChanged, ActionApplied}, audit.actions())
	require.Equal(t, BankChangeReason, audit.logs[0].Meta["reason"])
	requireNoPayloadInAudit(t, audit.logs, payload)
	for _, l := range audit.logs {
		for _, v := range l.Meta {
			for _, pv := range payload {
				require.NotEqual(t, pv, fmt.Sprint(v))
			}
		}
	}
	require.NoError(t, db.ExpectationsWereMet())
}

func TestFinalizeBankSameDayUpdatesInPlace(t *testing.T) {
	payload := map[string]string{"bank_code": "BNI", "bank_account_number": "333", "bank_account_name": "Budi"}
	decided := day(2026, 7, 1)
	db, svc, audit := newService(t, nil)
	expectLocked(t, db, companyA, TypeBank, payload, StatusPending)
	expectDecisionDate(db, decided)
	rows := pgxmock.NewRows(assignmentCols)
	compRow(rows, 10, "8000000.00", [3]string{"BCA", "111", "Budi"}, day(2026, 1, 1), ptrTime(day(2026, 6, 30)))
	compRow(rows, 11, "9000000.00", [3]string{"BCA", "111", "Budi"}, decided, nil)
	expectCompLock(db, rows)
	db.ExpectExec(q("SET bank_code=$2, bank_account_number=$3, bank_account_name=$4, reason=$5")).
		WithArgs(int64(11), "BNI", "333", "Budi", BankChangeReason).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(q("WHERE employee_id=$4 AND effective_from > $5")).
		WithArgs("BNI", "333", "Budi", employeeID, decided).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	expectApproved(db)

	require.NoError(t, svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, ""))
	require.Equal(t, []string{compensation.ActionBankChanged, ActionApplied}, audit.actions())
	require.Equal(t, int64(11), audit.logs[0].Meta["assignment_id"], "same-day bank change must not insert a row")
	require.NoError(t, db.ExpectationsWereMet())
}

func TestFinalizeRejectedLeavesDataUntouched(t *testing.T) {
	for _, typ := range []string{TypeAddress, TypeBank, TypeEmergencyContact} {
		t.Run(typ, func(t *testing.T) {
			payload := map[string]string{TypeAddress: "address_line1", TypeBank: "bank_code", TypeEmergencyContact: "name"}
			db, svc, audit := newService(t, nil)
			expectLocked(t, db, companyA, typ, map[string]string{payload[typ]: "x", "bank_account_number": "1"}, StatusPending)
			// Only the request row changes: no hr_employees, contacts, or compensation statements.
			db.ExpectExec(q("UPDATE hr_profile_change_requests SET status='REJECTED', apply_error='', decided_by=$2, updated_at=NOW() WHERE id=$1")).
				WithArgs(requestID, actorID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			db.ExpectCommit()

			a := approvalReq(companyA)
			a.Status = approvals.StatusRejected
			require.NoError(t, svc.FinalizeApproval(context.Background(), a, approvals.StatusRejected, actorID, "no"))
			require.Equal(t, []string{ActionRejected}, audit.actions())
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}

func TestFinalizeIsIdempotent(t *testing.T) {
	payload := map[string]string{"bank_code": "BNI", "bank_account_number": "333"}
	for _, st := range []string{StatusApproved, StatusRejected, StatusCancelled} {
		t.Run(st, func(t *testing.T) {
			db, svc, audit := newService(t, nil)
			expectLocked(t, db, companyA, TypeBank, payload, st)
			db.ExpectRollback()

			require.NoError(t, svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, ""))
			require.Empty(t, audit.logs)
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}

func TestFinalizeCompanyMismatch(t *testing.T) {
	db, svc, audit := newService(t, nil)
	expectLocked(t, db, companyA, TypeAddress, map[string]string{"city": "Bandung"}, StatusPending)
	db.ExpectRollback()

	err := svc.FinalizeApproval(context.Background(), approvalReq(companyB), approvals.StatusApproved, actorID, "")
	require.ErrorIs(t, err, ErrCompanyMismatch)
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestFinalizeBankWithoutAssignmentStoresApplyError(t *testing.T) {
	payload := map[string]string{"bank_code": "BNI", "bank_account_number": "333"}
	db, svc, audit := newService(t, nil)
	expectLocked(t, db, companyA, TypeBank, payload, StatusPending)
	expectDecisionDate(db, day(2026, 7, 1))
	expectCompLock(db, pgxmock.NewRows(assignmentCols))
	db.ExpectRollback()
	db.ExpectExec(q("UPDATE hr_profile_change_requests SET apply_error=$2")).
		WithArgs(requestID, "apply bank change: "+compensation.ErrNoAssignment.Error()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err := svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, "")
	require.ErrorIs(t, err, compensation.ErrNoAssignment)
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestFinalizeInvalidDecision(t *testing.T) {
	require.ErrorIs(t, (&Service{audit: &auditFake{}}).FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusPending, actorID, ""), ErrInvalidDecision)
}

// ---- RetryApply ------------------------------------------------------------

func TestFinalizeAddressKeepsFieldsMissingOrEmptyInPayload(t *testing.T) {
	// The portal form submits every address input; blanks and omissions must not wipe stored values.
	payload := map[string]string{"address_line1": "Jl. Braga 5", "address_line2": "", "city": "  ", "province": "", "phone": "0812"}
	db, svc, _ := newService(t, nil)
	expectLocked(t, db, companyA, TypeAddress, payload, StatusPending)
	db.ExpectExec(q("UPDATE hr_employees SET address_line1=$3, phone=$4, updated_at=NOW() WHERE id=$1 AND company_id=$2")).
		WithArgs(employeeID, companyA, "Jl. Braga 5", "0812").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	expectApproved(db)
	require.NoError(t, svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, ""))
	require.NoError(t, db.ExpectationsWereMet())
}

func TestFinalizeAddressWithOnlyEmptyFieldsIsInvalid(t *testing.T) {
	db, svc, _ := newService(t, nil)
	expectLocked(t, db, companyA, TypeAddress, map[string]string{"city": "", "province": ""}, StatusPending)
	err := svc.FinalizeApproval(context.Background(), approvalReq(companyA), approvals.StatusApproved, actorID, "")
	require.ErrorIs(t, err, ErrInvalidPayload)
}

func TestDecisionDateIsComputedInJakarta(t *testing.T) {
	require.Contains(t, decisionDateSQL, `AT TIME ZONE 'Asia/Jakarta'`)
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN is required for the database check")
	}
	if u, err := url.Parse(dsn); err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		t.Skip("PG_DSN is not a localhost URL; refusing to run")
	}
	conn, err := pgx.Connect(context.Background(), dsn)
	require.NoError(t, err)
	defer conn.Close(context.Background())
	// 18:00 UTC on 30 Sep is 01:00 on 1 Oct in Jakarta.
	var got time.Time
	require.NoError(t, conn.QueryRow(context.Background(),
		`SELECT `+decisionDateExpr+` FROM (SELECT TIMESTAMPTZ '2026-09-30 18:00:00+00' AS completed_at) r`).Scan(&got))
	require.Equal(t, "2026-10-01", got.Format("2006-01-02"))
}

func TestRetryApply(t *testing.T) {
	t.Run("re-runs finalizer", func(t *testing.T) {
		payload := map[string]string{"city": "Bandung"}
		db, svc, audit := newService(t, nil)
		db.ExpectQuery(q("SELECT p.status, p.approval_request_id FROM hr_profile_change_requests p WHERE p.id=$1 AND p.company_id=$2")).
			WithArgs(requestID, companyA).
			WillReturnRows(pgxmock.NewRows([]string{"status", "approval_request_id"}).AddRow(StatusPending, ptr(approvalID)))
		db.ExpectQuery(q("FROM approval_requests WHERE id=$1 AND module=$2 AND document_id=$3")).
			WithArgs(approvalID, Module, requestID).
			WillReturnRows(pgxmock.NewRows(approvalCols).AddRow(approvalID, int64(1), Module, requestID, ptr(companyA), 0.0, userID, 1, approvals.StatusApproved, createdAt))
		expectLocked(t, db, companyA, TypeAddress, payload, StatusPending)
		db.ExpectExec(q("UPDATE hr_employees SET city=$3, updated_at=NOW() WHERE id=$1 AND company_id=$2")).
			WithArgs(employeeID, companyA, "Bandung").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		expectApproved(db)

		require.NoError(t, svc.RetryApply(context.Background(), companyA, requestID, actorID))
		require.Equal(t, []string{ActionApplied}, audit.actions())
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("approval still pending", func(t *testing.T) {
		db, svc, _ := newService(t, nil)
		db.ExpectQuery("SELECT p.status, p.approval_request_id").WithArgs(requestID, companyA).
			WillReturnRows(pgxmock.NewRows([]string{"status", "approval_request_id"}).AddRow(StatusPending, ptr(approvalID)))
		db.ExpectQuery("FROM approval_requests").WithArgs(approvalID, Module, requestID).
			WillReturnRows(pgxmock.NewRows(approvalCols).AddRow(approvalID, int64(1), Module, requestID, ptr(companyA), 0.0, userID, 1, approvals.StatusPending, createdAt))
		require.ErrorIs(t, svc.RetryApply(context.Background(), companyA, requestID, actorID), ErrApprovalNotDecided)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("not pending", func(t *testing.T) {
		db, svc, _ := newService(t, nil)
		db.ExpectQuery("SELECT p.status, p.approval_request_id").WithArgs(requestID, companyA).
			WillReturnRows(pgxmock.NewRows([]string{"status", "approval_request_id"}).AddRow(StatusApproved, ptr(approvalID)))
		require.ErrorIs(t, svc.RetryApply(context.Background(), companyA, requestID, actorID), ErrNotRetryable)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("cross tenant not found", func(t *testing.T) {
		db, svc, _ := newService(t, nil)
		db.ExpectQuery("SELECT p.status, p.approval_request_id").WithArgs(requestID, companyB).WillReturnError(pgx.ErrNoRows)
		err := svc.RetryApply(context.Background(), companyB, requestID, actorID)
		require.ErrorIs(t, err, ErrNotFound)
		require.EqualError(t, err, "hr: not found: profile change request")
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("company required", func(t *testing.T) {
		require.ErrorIs(t, (&Service{}).RetryApply(context.Background(), 0, requestID, actorID), ErrCompanyRequired)
	})
}

// ---- Cancel ----------------------------------------------------------------

func TestCancelCancelsApprovalAndAssignments(t *testing.T) {
	db, svc, audit := newService(t, nil)
	db.ExpectBegin()
	db.ExpectQuery(q("WHERE p.id=$1 AND e.user_id=$2 AND p.company_id=$3 AND e.company_id=$3 FOR UPDATE OF p")).
		WithArgs(requestID, userID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"status", "change_type", "approval_request_id"}).AddRow(StatusPending, TypeBank, ptr(approvalID)))
	db.ExpectExec(q("UPDATE hr_profile_change_requests SET status='CANCELLED', updated_at=NOW() WHERE id=$1")).
		WithArgs(requestID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(q("UPDATE approval_requests SET status='CANCELLED', updated_at=NOW() WHERE id=$1 AND status='PENDING'")).
		WithArgs(approvalID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(q("UPDATE approval_assignments SET status='CANCELLED', updated_at=NOW() WHERE request_id=$1 AND status='PENDING'")).
		WithArgs(approvalID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	require.NoError(t, svc.Cancel(context.Background(), companyA, requestID, userID))
	require.Equal(t, []string{ActionCancelled}, audit.actions())
	require.Equal(t, map[string]any{"change_type": TypeBank}, audit.logs[0].Meta)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCancelErrors(t *testing.T) {
	t.Run("not pending", func(t *testing.T) {
		db, svc, _ := newService(t, nil)
		db.ExpectBegin()
		db.ExpectQuery("FROM hr_profile_change_requests p").WithArgs(requestID, userID, companyA).
			WillReturnRows(pgxmock.NewRows([]string{"status", "change_type", "approval_request_id"}).AddRow(StatusApproved, TypeBank, ptr(approvalID)))
		db.ExpectRollback()
		require.ErrorIs(t, svc.Cancel(context.Background(), companyA, requestID, userID), ErrNotPending)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("cross tenant", func(t *testing.T) {
		db, svc, _ := newService(t, nil)
		db.ExpectBegin()
		db.ExpectQuery("FROM hr_profile_change_requests p").WithArgs(requestID, userID, companyB).WillReturnError(pgx.ErrNoRows)
		db.ExpectRollback()
		require.ErrorIs(t, svc.Cancel(context.Background(), companyB, requestID, userID), ErrNotFound)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("company required", func(t *testing.T) {
		require.ErrorIs(t, (&Service{}).Cancel(context.Background(), 0, requestID, userID), ErrCompanyRequired)
	})
}

// ---- Listing ---------------------------------------------------------------

func listRows(t *testing.T) *pgxmock.Rows {
	cols := append(append([]string{}, requestColNames...), "name", "employee_number")
	rows := pgxmock.NewRows(cols)
	add := func(id int64, typ string, p map[string]string) {
		rows.AddRow(id, companyA, employeeID, typ, payloadJSON(t, p), StatusPending, ptr(approvalID), "", (*time.Time)(nil), (*int64)(nil), createdAt, createdAt, "Budi", "E-001")
	}
	add(1, TypeBank, map[string]string{"bank_code": "BCA", "bank_account_number": "1234567890", "bank_account_name": "Budi"})
	add(2, TypeAddress, map[string]string{"address_line1": "Jl. Braga 5", "city": "Bandung", "phone": "0812"})
	add(3, TypeEmergencyContact, map[string]string{"name": "Sari", "phone": "0813"})
	return rows
}

func TestListAllMasksWithoutSensitivePermission(t *testing.T) {
	db, svc, _ := newService(t, nil)
	db.ExpectQuery(q("FROM hr_profile_change_requests p JOIN hr_employees e ON e.id=p.employee_id AND e.company_id=p.company_id WHERE p.company_id=$1 AND p.status=$2 ORDER BY")).
		WithArgs(companyA, StatusPending).WillReturnRows(listRows(t))

	out, err := svc.ListAll(context.Background(), companyA, Filter{Status: "pending"})
	require.NoError(t, err)
	require.Len(t, out, 3)
	require.True(t, out[0].Masked)
	require.Equal(t, map[string]string{"bank_code": MaskedValue, "bank_account_number": MaskedValue, "bank_account_name": MaskedValue}, out[0].Payload)
	require.Equal(t, MaskedValue, out[1].Payload["address_line1"])
	require.Equal(t, MaskedValue, out[1].Payload["city"])
	require.Equal(t, "0812", out[1].Payload["phone"])
	require.False(t, out[2].Masked)
	require.Equal(t, "Sari", out[2].Payload["name"])
	require.Equal(t, "Budi", out[0].EmployeeName)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestListAllUnmaskedWithSensitivePermission(t *testing.T) {
	db, svc, _ := newService(t, nil)
	db.ExpectQuery(q("WHERE p.company_id=$1 AND p.change_type=$2")).
		WithArgs(companyA, TypeBank).WillReturnRows(listRows(t))

	out, err := svc.ListAll(context.Background(), companyA, Filter{ChangeType: "bank", ShowSensitive: true})
	require.NoError(t, err)
	require.False(t, out[0].Masked)
	require.Equal(t, "1234567890", out[0].Payload["bank_account_number"])
	require.Equal(t, "Jl. Braga 5", out[1].Payload["address_line1"])
	require.NoError(t, db.ExpectationsWereMet())
}

func TestListCompanyRequired(t *testing.T) {
	_, err := (&Service{}).ListAll(context.Background(), 0, Filter{})
	require.ErrorIs(t, err, ErrCompanyRequired)
	_, err = (&Service{}).ListOwn(context.Background(), 0, userID)
	require.ErrorIs(t, err, ErrCompanyRequired)
}

func TestListOwnScopedToUserAndCompany(t *testing.T) {
	db, svc, _ := newService(t, nil)
	db.ExpectQuery(q("WHERE p.company_id=$1 AND e.user_id=$2")).WithArgs(companyA, userID).WillReturnRows(listRows(t))
	out, err := svc.ListOwn(context.Background(), companyA, userID)
	require.NoError(t, err)
	require.Equal(t, "1234567890", out[0].Payload["bank_account_number"])
	require.NoError(t, db.ExpectationsWereMet())
}
