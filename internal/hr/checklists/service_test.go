package checklists

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

	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

const (
	companyA      = int64(7)
	companyB      = int64(8)
	employeeID    = int64(11)
	templateID    = int64(21)
	checklistID   = int64(31)
	actorID       = int64(99)
	employeeUser  = int64(42)
	managerEmpID  = int64(5)
	managerUserID = int64(77)
	hrUserID      = int64(300)
	assetID       = int64(61)
)

var (
	today       = time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	hireDate    = time.Date(2022, 1, 3, 0, 0, 0, 0, time.UTC)
	createdAt   = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	templateCls = []string{"id", "company_id", "code", "name", "kind", "is_active", "created_at"}
	itemCls     = []string{"id", "company_id", "template_id", "seq", "title", "description", "assignee_role", "default_assignee_user_id", "name", "due_offset_days", "action_key"}
	lockCls     = []string{"employee_id", "kind", "status", "termination_date", "user_id"}
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

type employeesFake struct {
	assignment    employees.JobHistory
	assignmentErr error
	reports       map[time.Time][]employees.JobHistory
	asOfCalls     []time.Time
	reportCalls   []time.Time
	companies     []int64
}

func (e *employeesFake) AssignmentAsOf(_ context.Context, companyID, _ int64, asOf time.Time) (employees.JobHistory, error) {
	e.companies = append(e.companies, companyID)
	e.asOfCalls = append(e.asOfCalls, asOf)
	return e.assignment, e.assignmentErr
}

func (e *employeesFake) DirectReportsAsOf(_ context.Context, companyID, _ int64, asOf time.Time) ([]employees.JobHistory, error) {
	e.companies = append(e.companies, companyID)
	e.reportCalls = append(e.reportCalls, asOf)
	return e.reports[asOf], nil
}

func ptr(v int64) *int64 { return &v }

func datePtr(t time.Time) *time.Time { return &t }

func newService(t *testing.T) (pgxmock.PgxPoolIface, *Service, *auditFake, *notifierFake, *employeesFake) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(db.Close)
	audit := &auditFake{}
	notifier := &notifierFake{}
	emp := &employeesFake{}
	svc := &Service{pool: db, audit: audit, employees: emp, notifier: notifier, now: func() time.Time { return today.Add(10 * time.Hour) }}
	return db, svc, audit, notifier, emp
}

func q(s string) string { return regexp.QuoteMeta(s) }

// ---------------------------------------------------------------- Start

func expectStartPrelude(db pgxmock.PgxPoolIface, kind string) {
	db.ExpectBegin()
	db.ExpectQuery(q(`FROM hr_checklist_templates t WHERE t.id=$1 AND t.company_id=$2`)).WithArgs(templateID, companyA).
		WillReturnRows(pgxmock.NewRows(templateCls).AddRow(templateID, companyA, "OFF-STD", "Standard offboarding", kind, true, createdAt))
	db.ExpectQuery(q(`SELECT name, status, hire_date, manager_id FROM hr_employees WHERE id=$1 AND company_id=$2 FOR UPDATE`)).WithArgs(employeeID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"name", "status", "hire_date", "manager_id"}).AddRow("Rina", "ACTIVE", hireDate, ptr(4)))
}

func TestStartResolvesManagerSetsDueDatesAndKeepsStatus(t *testing.T) {
	db, svc, audit, notifier, emp := newService(t)
	// The as-of job record names manager 5, while hr_employees.manager_id
	// still says 4: the as-of record wins.
	emp.assignment = employees.JobHistory{EmployeeID: employeeID, ManagerID: ptr(managerEmpID)}
	term := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	termDay := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)

	expectStartPrelude(db, KindOffboarding)
	db.ExpectQuery(q(`FROM hr_checklist_template_items i`)).WithArgs(companyA, templateID).
		WillReturnRows(pgxmock.NewRows(itemCls).
			AddRow(int64(1), companyA, templateID, 1, "Exit interview", "", RoleManager, nil, "", 0, "").
			AddRow(int64(2), companyA, templateID, 2, "Collect laptop", "", RoleIT, ptr(hrUserID), "Ops", 3, ActionKeyReturnAssets).
			AddRow(int64(3), companyA, templateID, 3, "Final payslip", "", RoleHR, ptr(hrUserID), "Ops", 5, ""))
	db.ExpectQuery(q(`SELECT m.user_id, u.is_active FROM hr_employees m LEFT JOIN users u ON u.id=m.user_id WHERE m.id=$1 AND m.company_id=$2`)).WithArgs(managerEmpID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "is_active"}).AddRow(ptr(managerUserID), func() *bool { b := true; return &b }()))
	for i := 0; i < 2; i++ {
		db.ExpectQuery(q(`SELECT is_active FROM users WHERE id=$1`)).WithArgs(hrUserID).
			WillReturnRows(pgxmock.NewRows([]string{"is_active"}).AddRow(true))
	}
	db.ExpectQuery(q(`INSERT INTO hr_employee_checklists`)).
		WithArgs(companyA, employeeID, templateID, KindOffboarding, today, datePtr(termDay), "Resigned", ptr(actorID)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow(checklistID, createdAt))
	for i, tc := range []struct {
		seq      int
		role     string
		assignee *int64
		due      time.Time
		action   any
	}{
		{1, RoleManager, ptr(managerUserID), today, (*string)(nil)},
		{2, RoleIT, ptr(hrUserID), today.AddDate(0, 0, 3), func() *string { s := ActionKeyReturnAssets; return &s }()},
		{3, RoleHR, ptr(hrUserID), today.AddDate(0, 0, 5), (*string)(nil)},
	} {
		db.ExpectQuery(q(`INSERT INTO hr_employee_checklist_tasks`)).
			WithArgs(companyA, checklistID, tc.seq, pgxmock.AnyArg(), "", tc.role, tc.assignee, tc.due, tc.action).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(100 + i)))
	}
	// termination date and reason only: the status column is not touched.
	db.ExpectExec(`^`+q(`UPDATE hr_employees SET termination_date=$1, termination_reason=$2, updated_at=NOW() WHERE id=$3 AND company_id=$4`)+`$`).
		WithArgs(termDay, "Resigned", employeeID, companyA).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	c, err := svc.Start(context.Background(), StartInput{CompanyID: companyA, EmployeeID: employeeID, TemplateID: templateID, ActorID: actorID, TerminationDate: &term, TerminationReason: " Resigned "})
	require.NoError(t, err)
	require.NoError(t, db.ExpectationsWereMet())

	require.Equal(t, checklistID, c.ID)
	require.Equal(t, StatusOpen, c.Status)
	require.Len(t, c.Tasks, 3)
	require.Equal(t, managerUserID, *c.Tasks[0].AssigneeUserID)
	require.Equal(t, today.AddDate(0, 0, 3), *c.Tasks[1].DueDate)
	require.Equal(t, []time.Time{today}, emp.asOfCalls, "manager is resolved as of today")
	require.Equal(t, []string{ActionChecklistStarted}, audit.actions())
	require.Equal(t, "2026-10-31", audit.logs[0].Meta["termination_date"])

	// One message per distinct assignee, keyed by that assignee's first task.
	require.Len(t, notifier.msgs, 2)
	require.Equal(t, managerUserID, notifier.msgs[0].RecipientID)
	require.Equal(t, "hr-checklist-task-100", notifier.msgs[0].DedupeKey)
	require.Equal(t, hrUserID, notifier.msgs[1].RecipientID)
	require.Equal(t, "hr-checklist-task-101", notifier.msgs[1].DedupeKey)
	for _, m := range notifier.msgs {
		require.Equal(t, NotificationTypeTaskAssigned, m.Type)
		require.Equal(t, "/hr/checklists/31", m.URL)
		require.NotEmpty(t, m.Title)
	}
	require.Contains(t, notifier.msgs[1].Body, "2 tasks")
}

func TestStartOnboardingFallsBackToCurrentManagerAndLeavesUnassigned(t *testing.T) {
	db, svc, audit, notifier, emp := newService(t)
	emp.assignmentErr = employees.ErrNoJobAssignment

	expectStartPrelude(db, KindOnboarding)
	db.ExpectQuery(q(`FROM hr_checklist_template_items i`)).WithArgs(companyA, templateID).
		WillReturnRows(pgxmock.NewRows(itemCls).
			AddRow(int64(1), companyA, templateID, 1, "Welcome lunch", "", RoleManager, nil, "", 1, "").
			AddRow(int64(2), companyA, templateID, 2, "Badge", "", RoleHR, nil, "", 0, ""))
	// fallback to hr_employees.manager_id (4); that manager's user is inactive.
	db.ExpectQuery(q(`SELECT m.user_id, u.is_active FROM hr_employees m`)).WithArgs(int64(4), companyA).
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "is_active"}).AddRow(ptr(55), func() *bool { b := false; return &b }()))
	db.ExpectQuery(q(`INSERT INTO hr_employee_checklists`)).
		WithArgs(companyA, employeeID, templateID, KindOnboarding, today, (*time.Time)(nil), "", ptr(actorID)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow(checklistID, createdAt))
	db.ExpectQuery(q(`INSERT INTO hr_employee_checklist_tasks`)).
		WithArgs(companyA, checklistID, 1, "Welcome lunch", "", RoleManager, (*int64)(nil), today.AddDate(0, 0, 1), (*string)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(100)))
	db.ExpectQuery(q(`INSERT INTO hr_employee_checklist_tasks`)).
		WithArgs(companyA, checklistID, 2, "Badge", "", RoleHR, (*int64)(nil), today, (*string)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(101)))
	db.ExpectCommit()

	tp := today.AddDate(0, 1, 0)
	_, err := svc.Start(context.Background(), StartInput{CompanyID: companyA, EmployeeID: employeeID, TemplateID: templateID, ActorID: actorID, TerminationDate: &tp})
	require.NoError(t, err)
	require.NoError(t, db.ExpectationsWereMet())
	require.Equal(t, []string{ActionChecklistStarted}, audit.actions())
	require.Empty(t, notifier.msgs, "unassigned tasks send no notifications")
}

func TestStartRefusals(t *testing.T) {
	t.Run("offboarding without termination date", func(t *testing.T) {
		db, svc, audit, notifier, _ := newService(t)
		expectStartPrelude(db, KindOffboarding)
		db.ExpectRollback()
		_, err := svc.Start(context.Background(), StartInput{CompanyID: companyA, EmployeeID: employeeID, TemplateID: templateID, ActorID: actorID})
		require.ErrorIs(t, err, ErrTerminationRequired)
		require.NoError(t, db.ExpectationsWereMet())
		require.Empty(t, audit.logs)
		require.Empty(t, notifier.msgs)
	})
	t.Run("inactive default assignee", func(t *testing.T) {
		db, svc, _, _, _ := newService(t)
		expectStartPrelude(db, KindOnboarding)
		db.ExpectQuery(q(`FROM hr_checklist_template_items i`)).WithArgs(companyA, templateID).
			WillReturnRows(pgxmock.NewRows(itemCls).AddRow(int64(2), companyA, templateID, 1, "Laptop", "", RoleIT, ptr(hrUserID), "Ops", 0, ""))
		db.ExpectQuery(q(`SELECT is_active FROM users WHERE id=$1`)).WithArgs(hrUserID).
			WillReturnRows(pgxmock.NewRows([]string{"is_active"}).AddRow(false))
		db.ExpectRollback()
		_, err := svc.Start(context.Background(), StartInput{CompanyID: companyA, EmployeeID: employeeID, TemplateID: templateID, ActorID: actorID})
		require.ErrorIs(t, err, ErrAssigneeInactive)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("already open", func(t *testing.T) {
		db, svc, _, _, _ := newService(t)
		expectStartPrelude(db, KindOnboarding)
		db.ExpectQuery(q(`FROM hr_checklist_template_items i`)).WithArgs(companyA, templateID).
			WillReturnRows(pgxmock.NewRows(itemCls).AddRow(int64(2), companyA, templateID, 1, "Badge", "", RoleHR, nil, "", 0, ""))
		db.ExpectQuery(q(`INSERT INTO hr_employee_checklists`)).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnError(&pgconn.PgError{Code: "23505"})
		db.ExpectRollback()
		_, err := svc.Start(context.Background(), StartInput{CompanyID: companyA, EmployeeID: employeeID, TemplateID: templateID, ActorID: actorID})
		require.ErrorIs(t, err, ErrAlreadyOpen)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("cross-tenant template", func(t *testing.T) {
		db, svc, _, _, _ := newService(t)
		db.ExpectBegin()
		db.ExpectQuery(q(`FROM hr_checklist_templates t WHERE t.id=$1 AND t.company_id=$2`)).WithArgs(templateID, companyB).WillReturnError(pgx.ErrNoRows)
		db.ExpectRollback()
		_, err := svc.Start(context.Background(), StartInput{CompanyID: companyB, EmployeeID: employeeID, TemplateID: templateID, ActorID: actorID})
		require.ErrorIs(t, err, ErrNotFound)
		require.EqualError(t, err, "hr: not found: checklist template")
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("cross-tenant employee", func(t *testing.T) {
		db, svc, _, _, _ := newService(t)
		db.ExpectBegin()
		db.ExpectQuery(q(`FROM hr_checklist_templates t WHERE t.id=$1 AND t.company_id=$2`)).WithArgs(templateID, companyA).
			WillReturnRows(pgxmock.NewRows(templateCls).AddRow(templateID, companyA, "ON", "On", KindOnboarding, true, createdAt))
		db.ExpectQuery(q(`FROM hr_employees WHERE id=$1 AND company_id=$2 FOR UPDATE`)).WithArgs(employeeID, companyA).WillReturnError(pgx.ErrNoRows)
		db.ExpectRollback()
		_, err := svc.Start(context.Background(), StartInput{CompanyID: companyA, EmployeeID: employeeID, TemplateID: templateID, ActorID: actorID})
		require.ErrorIs(t, err, ErrEmployeeNotFound)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("company required", func(t *testing.T) {
		_, svc, _, _, _ := newService(t)
		_, err := svc.Start(context.Background(), StartInput{EmployeeID: employeeID, TemplateID: templateID})
		require.ErrorIs(t, err, ErrCompanyRequired)
	})
}

// ---------------------------------------------------- CompleteOffboarding

func expectLockChecklist(db pgxmock.PgxPoolIface, companyID int64, kind, status string, term *time.Time) {
	db.ExpectQuery(q(`FROM hr_employee_checklists c JOIN hr_employees e ON e.id=c.employee_id AND e.company_id=c.company_id WHERE c.id=$1 AND c.company_id=$2 FOR UPDATE`)).
		WithArgs(checklistID, companyID).
		WillReturnRows(pgxmock.NewRows(lockCls).AddRow(employeeID, kind, status, term, ptr(employeeUser)))
}

func expectOpenTasks(db pgxmock.PgxPoolIface, n int) {
	db.ExpectQuery(q(`SELECT COUNT(*) FROM hr_employee_checklist_tasks WHERE checklist_id=$1 AND company_id=$2 AND status='OPEN'`)).
		WithArgs(checklistID, companyA).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(n))
}

func expectHeldAssets(db pgxmock.PgxPoolIface, n int) {
	db.ExpectQuery(q(`SELECT COUNT(*) FROM fixed_assets WHERE company_id=$1 AND custodian_employee_id=$2`)).
		WithArgs(companyA, employeeID).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(n))
}

func TestCompleteOffboardingRefusals(t *testing.T) {
	past := today.AddDate(0, 0, -2)
	cases := []struct {
		name    string
		setup   func(db pgxmock.PgxPoolIface, emp *employeesFake)
		wantErr string
		is      error
	}{
		{
			name: "future termination date",
			setup: func(db pgxmock.PgxPoolIface, _ *employeesFake) {
				expectLockChecklist(db, companyA, KindOffboarding, StatusOpen, datePtr(today.AddDate(0, 0, 1)))
			},
			is: ErrFutureTermination,
		},
		{
			name: "open task",
			setup: func(db pgxmock.PgxPoolIface, _ *employeesFake) {
				expectLockChecklist(db, companyA, KindOffboarding, StatusOpen, &past)
				expectOpenTasks(db, 2)
			},
			wantErr: "hr: complete or skip 2 open checklist tasks first",
		},
		{
			name: "held asset",
			setup: func(db pgxmock.PgxPoolIface, _ *employeesFake) {
				expectLockChecklist(db, companyA, KindOffboarding, StatusOpen, &past)
				expectOpenTasks(db, 0)
				expectHeldAssets(db, 1)
			},
			wantErr: "hr: return 1 assets held by the employee first",
		},
		{
			name: "direct reports today and as of termination date",
			setup: func(db pgxmock.PgxPoolIface, emp *employeesFake) {
				expectLockChecklist(db, companyA, KindOffboarding, StatusOpen, &past)
				expectOpenTasks(db, 0)
				expectHeldAssets(db, 0)
				emp.reports = map[time.Time][]employees.JobHistory{
					today: {{EmployeeID: 12}},
					past:  {{EmployeeID: 12}, {EmployeeID: 13}},
				}
			},
			wantErr: "hr: reassign 2 direct reports first",
		},
		{
			name: "not open",
			setup: func(db pgxmock.PgxPoolIface, _ *employeesFake) {
				expectLockChecklist(db, companyA, KindOffboarding, StatusCompleted, &past)
			},
			is: ErrNotOpen,
		},
		{
			name: "onboarding checklist",
			setup: func(db pgxmock.PgxPoolIface, _ *employeesFake) {
				expectLockChecklist(db, companyA, KindOnboarding, StatusOpen, nil)
			},
			is: ErrWrongKind,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, svc, audit, _, emp := newService(t)
			db.ExpectBegin()
			tc.setup(db, emp)
			db.ExpectRollback()
			_, err := svc.CompleteOffboarding(context.Background(), companyA, checklistID, actorID)
			require.Error(t, err)
			if tc.is != nil {
				require.ErrorIs(t, err, tc.is)
			} else {
				require.EqualError(t, err, tc.wantErr)
			}
			require.NoError(t, db.ExpectationsWereMet(), "no update runs after a refusal")
			require.Empty(t, audit.logs)
		})
	}
}

func TestCompleteOffboardingCrossTenant(t *testing.T) {
	db, svc, _, _, _ := newService(t)
	db.ExpectBegin()
	db.ExpectQuery(q(`FROM hr_employee_checklists c JOIN hr_employees e`)).WithArgs(checklistID, companyB).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()
	_, err := svc.CompleteOffboarding(context.Background(), companyB, checklistID, actorID)
	require.EqualError(t, err, "hr: not found: checklist")
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCompleteOffboardingRunsEveryUpdateInOneTransaction(t *testing.T) {
	db, svc, audit, _, emp := newService(t)
	term := today.AddDate(0, 0, -1)
	jhFrom := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	db.ExpectBegin()
	expectLockChecklist(db, companyA, KindOffboarding, StatusOpen, &term)
	expectOpenTasks(db, 0)
	expectHeldAssets(db, 0)
	for _, tc := range []struct {
		table string
		ids   []any
	}{
		{"hr_leave_requests", []any{ptr(901), nil}},
		{"hr_overtime_requests", []any{ptr(902)}},
		{"hr_expense_claims", nil},
		{"hr_profile_change_requests", []any{ptr(903)}},
	} {
		rows := pgxmock.NewRows([]string{"approval_request_id"})
		for _, id := range tc.ids {
			rows.AddRow(id)
		}
		db.ExpectQuery(q(`UPDATE ` + tc.table + ` SET status='CANCELLED', updated_at=NOW() WHERE employee_id=$1 AND status='PENDING' RETURNING approval_request_id`)).
			WithArgs(employeeID).WillReturnRows(rows)
	}
	db.ExpectExec(q(`UPDATE approval_requests SET status='CANCELLED', updated_at=NOW() WHERE id = ANY($1) AND status='PENDING'`)).
		WithArgs([]int64{901, 902, 903}).WillReturnResult(pgxmock.NewResult("UPDATE", 3))
	db.ExpectExec(q(`UPDATE approval_assignments SET status='CANCELLED', updated_at=NOW() WHERE request_id = ANY($1) AND status='PENDING'`)).
		WithArgs([]int64{901, 902, 903}).WillReturnResult(pgxmock.NewResult("UPDATE", 3))
	db.ExpectExec(q(`UPDATE approval_assignments aa SET status='CANCELLED', updated_at=NOW() FROM approval_requests ar WHERE ar.id=aa.request_id AND ar.company_id=$2 AND aa.approver_id=$1 AND aa.status='PENDING'`)).
		WithArgs(employeeUser, companyA).WillReturnResult(pgxmock.NewResult("UPDATE", 2))
	db.ExpectExec(q(`UPDATE users SET is_active=false, updated_at=NOW() WHERE id=$1`)).
		WithArgs(employeeUser).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(q(`UPDATE hr_leave_balances SET entitled=used, pending=0, updated_at=NOW() WHERE employee_id=$1`)).
		WithArgs(employeeID).WillReturnResult(pgxmock.NewResult("UPDATE", 2))
	db.ExpectExec(q(`UPDATE hr_employee_benefits SET status='ENDED' WHERE employee_id=$1`)).
		WithArgs(employeeID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectQuery(q(`FROM hr_employee_job_history jh JOIN hr_employees e ON e.id=jh.employee_id AND e.company_id=$2 WHERE jh.employee_id=$1 ORDER BY jh.valid_from DESC LIMIT 1 FOR UPDATE OF jh`)).
		WithArgs(employeeID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"id", "valid_from", "valid_to", "department_id", "position_id", "manager_id", "employment_type", "grade"}).
			AddRow(int64(700), jhFrom, nil, ptr(3), ptr(4), ptr(managerEmpID), "PERMANENT", "G5"))
	db.ExpectExec(q(`UPDATE hr_employee_job_history SET valid_to=$2 WHERE id=$1`)).
		WithArgs(int64(700), term).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(q(`INSERT INTO hr_employee_job_history`)+`.*'INACTIVE'`).
		WithArgs(companyA, employeeID, ptr(3), ptr(4), ptr(managerEmpID), "PERMANENT", "G5", today, ptr(actorID)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectExec(q(`UPDATE hr_employees SET status='INACTIVE', updated_at=NOW() WHERE id=$1 AND company_id=$2`)).
		WithArgs(employeeID, companyA).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(q(`UPDATE hr_employee_checklists SET status='COMPLETED', completed_by=$2, completed_at=NOW() WHERE id=$1`)).
		WithArgs(checklistID, ptr(actorID)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	res, err := svc.CompleteOffboarding(context.Background(), companyA, checklistID, actorID)
	require.NoError(t, err)
	require.NoError(t, db.ExpectationsWereMet())

	require.Equal(t, []time.Time{today, term}, emp.reportCalls, "direct reports checked as of today and the termination date")
	require.Equal(t, 2, res.CancelledLeave)
	require.Equal(t, 1, res.CancelledOvertime)
	require.Equal(t, 0, res.CancelledExpenses)
	require.Equal(t, 1, res.CancelledProfileChanges)
	require.Equal(t, 3, res.CancelledApprovalRequests)
	require.Equal(t, 5, res.CancelledAssignments)
	require.True(t, res.UserDeactivated)
	require.Equal(t, []string{ActionEmployeeOffboarded}, audit.actions())
	require.Equal(t, "hr_employee", audit.logs[0].Entity)
	require.Equal(t, "11", audit.logs[0].EntityID)
	require.Equal(t, checklistID, audit.logs[0].Meta["checklist_id"])
}

func TestCompleteOffboardingRejectsJobChangeAfterTermination(t *testing.T) {
	db, svc, audit, _, _ := newService(t)
	term := today.AddDate(0, 0, -10)
	db.ExpectBegin()
	expectLockChecklist(db, companyA, KindOffboarding, StatusOpen, &term)
	expectOpenTasks(db, 0)
	expectHeldAssets(db, 0)
	for _, table := range []string{"hr_leave_requests", "hr_overtime_requests", "hr_expense_claims", "hr_profile_change_requests"} {
		db.ExpectQuery(q(`UPDATE ` + table)).WithArgs(employeeID).WillReturnRows(pgxmock.NewRows([]string{"approval_request_id"}))
	}
	db.ExpectExec(q(`UPDATE approval_assignments aa`)).WithArgs(employeeUser, companyA).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	db.ExpectExec(q(`UPDATE users SET is_active=false`)).WithArgs(employeeUser).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(q(`UPDATE hr_leave_balances`)).WithArgs(employeeID).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	db.ExpectExec(q(`UPDATE hr_employee_benefits`)).WithArgs(employeeID).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	db.ExpectQuery(q(`FROM hr_employee_job_history jh`)).WithArgs(employeeID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"id", "valid_from", "valid_to", "department_id", "position_id", "manager_id", "employment_type", "grade"}).
			AddRow(int64(701), today.AddDate(0, 0, -5), nil, nil, nil, nil, "PERMANENT", ""))
	db.ExpectRollback()
	_, err := svc.CompleteOffboarding(context.Background(), companyA, checklistID, actorID)
	require.ErrorIs(t, err, ErrJobHistoryAfterLeaving)
	require.NoError(t, db.ExpectationsWereMet())
	require.Empty(t, audit.logs)
}

// ------------------------------------------- CancelOffboarding / Onboarding

func TestCancelOffboarding(t *testing.T) {
	term := today.AddDate(0, 0, 14)
	t.Run("clears termination fields", func(t *testing.T) {
		db, svc, audit, _, _ := newService(t)
		db.ExpectBegin()
		expectLockChecklist(db, companyA, KindOffboarding, StatusOpen, &term)
		db.ExpectExec(q(`UPDATE hr_employee_checklists SET status='CANCELLED'`)).WithArgs(checklistID, ptr(actorID)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectExec(q(`UPDATE hr_employees SET termination_date=NULL, termination_reason='', updated_at=NOW() WHERE id=$1 AND company_id=$2`)).
			WithArgs(employeeID, companyA).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()
		require.NoError(t, svc.CancelOffboarding(context.Background(), companyA, checklistID, actorID, " counter-offer accepted "))
		require.NoError(t, db.ExpectationsWereMet())
		require.Equal(t, []string{ActionOffboardingCanceled}, audit.actions())
		require.Equal(t, map[string]any{"checklist_id": checklistID, "reason": "counter-offer accepted"}, audit.logs[0].Meta)
	})
	t.Run("refuses when not open", func(t *testing.T) {
		db, svc, audit, _, _ := newService(t)
		db.ExpectBegin()
		expectLockChecklist(db, companyA, KindOffboarding, StatusCompleted, &term)
		db.ExpectRollback()
		require.ErrorIs(t, svc.CancelOffboarding(context.Background(), companyA, checklistID, actorID, "x"), ErrNotOpen)
		require.NoError(t, db.ExpectationsWereMet())
		require.Empty(t, audit.logs)
	})
	t.Run("cross-tenant", func(t *testing.T) {
		db, svc, _, _, _ := newService(t)
		db.ExpectBegin()
		db.ExpectQuery(q(`FROM hr_employee_checklists c`)).WithArgs(checklistID, companyB).WillReturnError(pgx.ErrNoRows)
		db.ExpectRollback()
		require.ErrorIs(t, svc.CancelOffboarding(context.Background(), companyB, checklistID, actorID, "x"), ErrNotFound)
		require.NoError(t, db.ExpectationsWereMet())
	})
}

func TestCompleteOnboarding(t *testing.T) {
	t.Run("completes when no task is open", func(t *testing.T) {
		db, svc, audit, _, _ := newService(t)
		db.ExpectBegin()
		expectLockChecklist(db, companyA, KindOnboarding, StatusOpen, nil)
		expectOpenTasks(db, 0)
		db.ExpectExec(q(`UPDATE hr_employee_checklists SET status='COMPLETED'`)).WithArgs(checklistID, ptr(actorID)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()
		require.NoError(t, svc.CompleteOnboarding(context.Background(), companyA, checklistID, actorID))
		require.NoError(t, db.ExpectationsWereMet())
		require.Equal(t, []string{ActionChecklistCompleted}, audit.actions())
	})
	t.Run("refuses offboarding checklist", func(t *testing.T) {
		db, svc, _, _, _ := newService(t)
		db.ExpectBegin()
		expectLockChecklist(db, companyA, KindOffboarding, StatusOpen, datePtr(today))
		db.ExpectRollback()
		require.ErrorIs(t, svc.CompleteOnboarding(context.Background(), companyA, checklistID, actorID), ErrWrongKind)
		require.NoError(t, db.ExpectationsWereMet())
	})
}

// ------------------------------------------------------------------ tasks

func TestCompleteTaskAuthorization(t *testing.T) {
	expectTask := func(db pgxmock.PgxPoolIface, companyID int64, assignee *int64) {
		db.ExpectQuery(q(`FROM hr_employee_checklist_tasks k JOIN hr_employee_checklists c ON c.id=k.checklist_id AND c.company_id=$2 WHERE k.id=$1 FOR UPDATE OF k`)).
			WithArgs(int64(100), companyID).
			WillReturnRows(pgxmock.NewRows([]string{"checklist_id", "status", "assignee_user_id", "status"}).AddRow(checklistID, TaskOpen, assignee, StatusOpen))
	}
	t.Run("assignee completes", func(t *testing.T) {
		db, svc, audit, _, _ := newService(t)
		db.ExpectBegin()
		expectTask(db, companyA, ptr(hrUserID))
		db.ExpectExec(q(`UPDATE hr_employee_checklist_tasks SET status=$2`)).WithArgs(int64(100), TaskDone, "done", hrUserID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()
		require.NoError(t, svc.CompleteTask(context.Background(), companyA, 100, hrUserID, false, " done "))
		require.NoError(t, db.ExpectationsWereMet())
		require.Equal(t, []string{ActionTaskCompleted}, audit.actions())
	})
	t.Run("other user is forbidden", func(t *testing.T) {
		db, svc, audit, _, _ := newService(t)
		db.ExpectBegin()
		expectTask(db, companyA, ptr(hrUserID))
		db.ExpectRollback()
		require.ErrorIs(t, svc.CompleteTask(context.Background(), companyA, 100, actorID, false, ""), ErrForbidden)
		require.NoError(t, db.ExpectationsWereMet())
		require.Empty(t, audit.logs)
	})
	t.Run("admin skips unassigned task", func(t *testing.T) {
		db, svc, audit, _, _ := newService(t)
		db.ExpectBegin()
		expectTask(db, companyA, nil)
		db.ExpectExec(q(`UPDATE hr_employee_checklist_tasks SET status=$2`)).WithArgs(int64(100), TaskSkipped, "", actorID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()
		require.NoError(t, svc.SkipTask(context.Background(), companyA, 100, actorID, true, ""))
		require.NoError(t, db.ExpectationsWereMet())
		require.Equal(t, []string{ActionTaskSkipped}, audit.actions())
	})
	t.Run("cross-tenant task", func(t *testing.T) {
		db, svc, _, _, _ := newService(t)
		db.ExpectBegin()
		db.ExpectQuery(q(`FROM hr_employee_checklist_tasks k`)).WithArgs(int64(100), companyB).WillReturnError(pgx.ErrNoRows)
		db.ExpectRollback()
		require.ErrorIs(t, svc.CompleteTask(context.Background(), companyB, 100, actorID, true, ""), ErrNotFound)
		require.NoError(t, db.ExpectationsWereMet())
	})
}

func TestGetAndMyTasksAreCompanyScoped(t *testing.T) {
	db, svc, _, _, _ := newService(t)
	db.ExpectQuery(q(`WHERE c.company_id=$1 AND c.id=$2`)).WithArgs(companyB, checklistID).WillReturnError(pgx.ErrNoRows)
	_, err := svc.Get(context.Background(), companyB, checklistID)
	require.EqualError(t, err, "hr: not found: checklist")

	db.ExpectQuery(q(`JOIN hr_employee_checklists c ON c.id=k.checklist_id AND c.company_id=$1`)+`.*`+q(`WHERE k.assignee_user_id=$2 AND k.status='OPEN' AND c.status='OPEN'`)).
		WithArgs(companyA, hrUserID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "checklist_id", "seq", "title", "description", "assignee_role", "assignee_user_id", "name", "due_date", "status", "action_key", "notes", "completed_by", "completed_at", "employee_id", "ename", "kind"}).
			AddRow(int64(100), companyA, checklistID, 1, "Laptop", "", RoleIT, ptr(hrUserID), "Ops", datePtr(today.AddDate(0, 0, -1)), TaskOpen, "", "", nil, nil, employeeID, "Rina", KindOffboarding))
	tasks, err := svc.MyTasks(context.Background(), companyA, hrUserID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.True(t, tasks[0].Overdue(today))
	require.NoError(t, db.ExpectationsWereMet())

	_, err = svc.List(context.Background(), 0, Filter{})
	require.ErrorIs(t, err, ErrCompanyRequired)
}

// -------------------------------------------------------------- templates

func TestTemplateCRUD(t *testing.T) {
	t.Run("create duplicate code", func(t *testing.T) {
		db, svc, audit, _, _ := newService(t)
		db.ExpectBegin()
		db.ExpectQuery(q(`INSERT INTO hr_checklist_templates`)).WithArgs(companyA, "OFF-STD", "Standard", KindOffboarding, true).WillReturnError(&pgconn.PgError{Code: "23505"})
		db.ExpectRollback()
		_, err := svc.CreateTemplate(context.Background(), companyA, TemplateInput{Code: " off-std ", Name: "Standard", Kind: "offboarding", IsActive: true}, actorID)
		require.ErrorIs(t, err, ErrDuplicateCode)
		require.NoError(t, db.ExpectationsWereMet())
		require.Empty(t, audit.logs)
	})
	t.Run("create audited", func(t *testing.T) {
		db, svc, audit, _, _ := newService(t)
		db.ExpectBegin()
		db.ExpectQuery(q(`INSERT INTO hr_checklist_templates`)).WithArgs(companyA, "ON", "Onboarding", KindOnboarding, true).
			WillReturnRows(pgxmock.NewRows(templateCls).AddRow(templateID, companyA, "ON", "Onboarding", KindOnboarding, true, createdAt))
		db.ExpectCommit()
		tpl, err := svc.CreateTemplate(context.Background(), companyA, TemplateInput{Code: "on", Name: "Onboarding", Kind: KindOnboarding, IsActive: true}, actorID)
		require.NoError(t, err)
		require.Equal(t, templateID, tpl.ID)
		require.Equal(t, []string{ActionTemplateCreated}, audit.actions())
	})
	t.Run("invalid kind", func(t *testing.T) {
		_, svc, _, _, _ := newService(t)
		_, err := svc.CreateTemplate(context.Background(), companyA, TemplateInput{Code: "X", Name: "X", Kind: "TRANSFER"}, actorID)
		require.ErrorIs(t, err, ErrInvalidKind)
	})
	t.Run("update cross-tenant", func(t *testing.T) {
		db, svc, _, _, _ := newService(t)
		db.ExpectBegin()
		db.ExpectQuery(q(`UPDATE hr_checklist_templates t SET code=$3, name=$4, is_active=$5 WHERE t.id=$1 AND t.company_id=$2`)).
			WithArgs(templateID, companyB, "ON", "On", false).WillReturnError(pgx.ErrNoRows)
		db.ExpectRollback()
		_, err := svc.UpdateTemplate(context.Background(), companyB, templateID, TemplateUpdate{Code: "on", Name: "On"}, actorID)
		require.ErrorIs(t, err, ErrNotFound)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("add item to cross-tenant template", func(t *testing.T) {
		db, svc, _, _, _ := newService(t)
		db.ExpectBegin()
		db.ExpectQuery(q(`SELECT id FROM hr_checklist_templates WHERE id=$1 AND company_id=$2 FOR UPDATE`)).WithArgs(templateID, companyB).WillReturnError(pgx.ErrNoRows)
		db.ExpectRollback()
		_, err := svc.AddItem(context.Background(), companyB, templateID, ItemInput{Title: "Laptop", AssigneeRole: "it"}, actorID)
		require.ErrorIs(t, err, ErrTemplateNotFound)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("add item appends and checks assignee", func(t *testing.T) {
		db, svc, audit, _, _ := newService(t)
		db.ExpectBegin()
		db.ExpectQuery(q(`SELECT id FROM hr_checklist_templates WHERE id=$1 AND company_id=$2 FOR UPDATE`)).WithArgs(templateID, companyA).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(templateID))
		db.ExpectQuery(q(`SELECT is_active FROM users WHERE id=$1`)).WithArgs(hrUserID).WillReturnRows(pgxmock.NewRows([]string{"is_active"}).AddRow(true))
		db.ExpectQuery(q(`SELECT COALESCE(MAX(seq),0)+1 FROM hr_checklist_template_items WHERE template_id=$1`)).WithArgs(templateID).
			WillReturnRows(pgxmock.NewRows([]string{"seq"}).AddRow(4))
		db.ExpectQuery(q(`INSERT INTO hr_checklist_template_items`)).
			WithArgs(companyA, templateID, 4, "Laptop", "", RoleIT, ptr(hrUserID), 2, func() *string { s := ActionKeyReturnAssets; return &s }()).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(9)))
		db.ExpectCommit()
		it, err := svc.AddItem(context.Background(), companyA, templateID, ItemInput{Title: "Laptop", AssigneeRole: "it", DefaultAssigneeUserID: ptr(hrUserID), DueOffsetDays: 2, ActionKey: "return_assets"}, actorID)
		require.NoError(t, err)
		require.Equal(t, 4, it.Seq)
		require.NoError(t, db.ExpectationsWereMet())
		require.Equal(t, []string{ActionTemplateItemAdded}, audit.actions())
	})
	t.Run("add item invalid role or action", func(t *testing.T) {
		_, svc, _, _, _ := newService(t)
		_, err := svc.AddItem(context.Background(), companyA, templateID, ItemInput{Title: "x", AssigneeRole: "FINANCE"}, actorID)
		require.ErrorIs(t, err, ErrInvalidRole)
		_, err = svc.AddItem(context.Background(), companyA, templateID, ItemInput{Title: "x", AssigneeRole: "HR", ActionKey: "DROP_TABLES"}, actorID)
		require.ErrorIs(t, err, ErrInvalidActionKey)
	})
	t.Run("delete item cross-tenant", func(t *testing.T) {
		db, svc, _, _, _ := newService(t)
		db.ExpectBegin()
		db.ExpectQuery(q(`DELETE FROM hr_checklist_template_items i USING hr_checklist_templates t WHERE i.id=$1 AND t.id=i.template_id AND t.company_id=$2`)).
			WithArgs(int64(9), companyB).WillReturnError(pgx.ErrNoRows)
		db.ExpectRollback()
		require.ErrorIs(t, svc.DeleteItem(context.Background(), companyB, 9, actorID), ErrItemNotFound)
		require.NoError(t, db.ExpectationsWereMet())
	})
}

// ----------------------------------------------------------------- assets

func TestAssetCustody(t *testing.T) {
	assetCols := []string{"number", "name", "status", "custodian_employee_id"}
	t.Run("assign audited", func(t *testing.T) {
		db, svc, audit, _, _ := newService(t)
		db.ExpectBegin()
		db.ExpectQuery(q(`FROM fixed_assets WHERE id=$1 AND company_id=$2 FOR UPDATE`)).WithArgs(assetID, companyA).
			WillReturnRows(pgxmock.NewRows(assetCols).AddRow("FA-1", "Laptop", "ACTIVE", nil))
		db.ExpectQuery(q(`SELECT status FROM hr_employees WHERE id=$1 AND company_id=$2`)).WithArgs(employeeID, companyA).
			WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
		db.ExpectExec(q(`UPDATE fixed_assets SET custodian_employee_id=$1, updated_at=NOW() WHERE id=$2 AND company_id=$3`)).WithArgs(employeeID, assetID, companyA).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()
		require.NoError(t, svc.AssignAsset(context.Background(), companyA, assetID, employeeID, actorID))
		require.NoError(t, db.ExpectationsWereMet())
		require.Equal(t, []string{ActionAssetAssigned}, audit.actions())
		require.Equal(t, "fixed_asset", audit.logs[0].Entity)
	})
	t.Run("assign cross-tenant asset", func(t *testing.T) {
		db, svc, _, _, _ := newService(t)
		db.ExpectBegin()
		db.ExpectQuery(q(`FROM fixed_assets WHERE id=$1 AND company_id=$2 FOR UPDATE`)).WithArgs(assetID, companyB).WillReturnError(pgx.ErrNoRows)
		db.ExpectRollback()
		require.EqualError(t, svc.AssignAsset(context.Background(), companyB, assetID, employeeID, actorID), "hr: not found: asset")
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("assign cross-tenant employee", func(t *testing.T) {
		db, svc, _, _, _ := newService(t)
		db.ExpectBegin()
		db.ExpectQuery(q(`FROM fixed_assets WHERE id=$1 AND company_id=$2 FOR UPDATE`)).WithArgs(assetID, companyA).
			WillReturnRows(pgxmock.NewRows(assetCols).AddRow("FA-1", "Laptop", "ACTIVE", nil))
		db.ExpectQuery(q(`SELECT status FROM hr_employees WHERE id=$1 AND company_id=$2`)).WithArgs(int64(999), companyA).WillReturnError(pgx.ErrNoRows)
		db.ExpectRollback()
		require.ErrorIs(t, svc.AssignAsset(context.Background(), companyA, assetID, 999, actorID), ErrEmployeeNotFound)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("assign held by other", func(t *testing.T) {
		db, svc, _, _, _ := newService(t)
		db.ExpectBegin()
		db.ExpectQuery(q(`FROM fixed_assets WHERE id=$1`)).WithArgs(assetID, companyA).
			WillReturnRows(pgxmock.NewRows(assetCols).AddRow("FA-1", "Laptop", "ACTIVE", ptr(12)))
		db.ExpectRollback()
		require.ErrorIs(t, svc.AssignAsset(context.Background(), companyA, assetID, employeeID, actorID), ErrAssetHeldByOther)
		require.NoError(t, db.ExpectationsWereMet())
	})
	t.Run("return audited", func(t *testing.T) {
		db, svc, audit, _, _ := newService(t)
		db.ExpectBegin()
		db.ExpectQuery(q(`FROM fixed_assets WHERE id=$1`)).WithArgs(assetID, companyA).
			WillReturnRows(pgxmock.NewRows(assetCols).AddRow("FA-1", "Laptop", "ACTIVE", ptr(employeeID)))
		db.ExpectExec(q(`UPDATE fixed_assets SET custodian_employee_id=NULL, updated_at=NOW() WHERE id=$1 AND company_id=$2`)).WithArgs(assetID, companyA).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()
		require.NoError(t, svc.ReturnAsset(context.Background(), companyA, assetID, actorID))
		require.NoError(t, db.ExpectationsWereMet())
		require.Equal(t, []string{ActionAssetReturned}, audit.actions())
		require.Equal(t, employeeID, audit.logs[0].Meta["employee_id"])
	})
	t.Run("held by scoped to company", func(t *testing.T) {
		db, svc, _, _, _ := newService(t)
		db.ExpectQuery(q(`FROM fixed_assets fa WHERE fa.company_id=$1 AND fa.custodian_employee_id=$2`)).WithArgs(companyB, employeeID).
			WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "number", "name", "status", "custodian_employee_id"}))
		assets, err := svc.AssetsHeldBy(context.Background(), companyB, employeeID)
		require.NoError(t, err)
		require.Empty(t, assets)
		require.NoError(t, db.ExpectationsWereMet())
	})
}

func TestNotifierFailureDoesNotFailStart(t *testing.T) {
	// notifyAssignees is best effort: a failing dispatcher is ignored.
	svc := &Service{notifier: failingNotifier{}}
	svc.notifyAssignees(context.Background(), Checklist{ID: 1, Kind: KindOnboarding}, []pendingTask{{id: 1, assignee: ptr(3)}})
}

type failingNotifier struct{}

func (failingNotifier) Dispatch(context.Context, notifications.Message) error {
	return errors.New("boom")
}

func TestIsTaskAssigneeIsCompanyScoped(t *testing.T) {
	db, svc, _, _, _ := newService(t)
	db.ExpectQuery(regexp.QuoteMeta("JOIN hr_employee_checklists c ON c.id=k.checklist_id AND c.company_id=$2 WHERE k.id=$1 AND k.assignee_user_id=$3")).
		WithArgs(int64(401), int64(1), int64(3)).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	ok, err := svc.IsTaskAssignee(context.Background(), 1, 401, 3)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	// Invalid ids never reach the database.
	for _, args := range [][3]int64{{0, 401, 3}, {1, 0, 3}, {1, 401, 0}} {
		if ok, err := svc.IsTaskAssignee(context.Background(), args[0], args[1], args[2]); ok || err != nil {
			t.Fatalf("args %v: ok=%v err=%v", args, ok, err)
		}
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
