package performance

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	pgxmock "github.com/pashagolub/pgxmock/v4"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
)

var (
	periodStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEnd   = time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	selfDue     = time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	managerDue  = time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	cycleCls    = []string{"id", "company_id", "name", "period_start", "period_end", "self_review_due", "manager_review_due", "status", "created_by", "created_at", "updated_at", "count", "count"}
)

const lockCycleSQL = `SELECT status, period_end FROM hr_performance_cycles WHERE id=$1 AND company_id=$2 FOR UPDATE`

func validCycleInput() CycleInput {
	return CycleInput{Name: " 2026 H1 ", PeriodStart: periodStart, PeriodEnd: periodEnd, SelfReviewDue: selfDue, ManagerReviewDue: managerDue, ActorID: adminUser}
}

func TestCreateCycleInsertsDraftAndAudits(t *testing.T) {
	db, service, audit, _ := newMock(t)
	actor := adminUser
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta(`INSERT INTO hr_performance_cycles(company_id,name,period_start,period_end,self_review_due,manager_review_due,status,created_by)`)).
		WithArgs(companyA, "2026 H1", periodStart, periodEnd, selfDue, managerDue, &actor).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(cycleID))
	db.ExpectQuery(regexp.QuoteMeta(`FROM hr_performance_cycles c WHERE c.company_id=$1 AND c.id=$2`)).
		WithArgs(companyA, cycleID).
		WillReturnRows(pgxmock.NewRows(cycleCls).AddRow(cycleID, companyA, "2026 H1", periodStart, periodEnd, selfDue, managerDue, CycleDraft, &actor, createdAt, createdAt, int64(0), int64(0)))
	db.ExpectCommit()

	c, err := service.CreateCycle(context.Background(), companyA, validCycleInput())
	if err != nil || c.ID != cycleID || c.Status != CycleDraft {
		t.Fatalf("CreateCycle() = %#v, %v", c, err)
	}
	if got := audit.actions(); len(got) != 1 || got[0] != ActionCycleCreated {
		t.Fatalf("audit = %v", got)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateCycleValidationAndDuplicate(t *testing.T) {
	_, service, _, _ := newMock(t)
	if _, err := service.CreateCycle(context.Background(), 0, validCycleInput()); !errors.Is(err, ErrCompanyRequired) {
		t.Fatalf("company 0 err = %v", err)
	}
	for name, mutate := range map[string]func(*CycleInput){
		"blank name":          func(in *CycleInput) { in.Name = "  " },
		"end before start":    func(in *CycleInput) { in.PeriodEnd = periodStart.AddDate(0, 0, -1) },
		"manager before self": func(in *CycleInput) { in.ManagerReviewDue = selfDue.AddDate(0, 0, -1) },
		"missing self due":    func(in *CycleInput) { in.SelfReviewDue = time.Time{} },
	} {
		in := validCycleInput()
		mutate(&in)
		if _, err := service.CreateCycle(context.Background(), companyA, in); !errors.Is(err, ErrInvalidCycle) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}

	db, service, audit, _ := newMock(t)
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta(`INSERT INTO hr_performance_cycles`)).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnError(&pgconn.PgError{Code: "23505"})
	db.ExpectRollback()
	if _, err := service.CreateCycle(context.Background(), companyA, validCycleInput()); !errors.Is(err, ErrDuplicateCycleName) {
		t.Fatalf("duplicate err = %v", err)
	}
	if len(audit.logs) != 0 {
		t.Fatalf("audit on failure: %v", audit.actions())
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCycleTransitions(t *testing.T) {
	cases := []struct {
		name         string
		activate     bool
		status       string
		lockErr      error
		participants int64
		wantErr      error
		wantAction   string
	}{
		{name: "activate draft with participants", activate: true, status: CycleDraft, participants: 2, wantAction: ActionCycleActivated},
		{name: "activate draft without participants", activate: true, status: CycleDraft, participants: 0, wantErr: ErrNoParticipants},
		{name: "activate active", activate: true, status: CycleActive, wantErr: ErrCycleNotDraft},
		{name: "activate cross-tenant cycle", activate: true, lockErr: pgx.ErrNoRows, wantErr: ErrCycleNotFound},
		{name: "close active", status: CycleActive, wantAction: ActionCycleClosed},
		{name: "close draft", status: CycleDraft, wantErr: ErrCycleNotActive},
		{name: "close closed", status: CycleClosed, wantErr: ErrCycleNotActive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, service, audit, _ := newMock(t)
			db.ExpectBegin()
			lock := db.ExpectQuery(regexp.QuoteMeta(lockCycleSQL)).WithArgs(cycleID, companyA)
			if tc.lockErr != nil {
				lock.WillReturnError(tc.lockErr)
			} else {
				lock.WillReturnRows(pgxmock.NewRows([]string{"status", "period_end"}).AddRow(tc.status, periodEnd))
			}
			want := CycleClosed
			if tc.activate {
				want = CycleActive
			}
			wantsUpdate := tc.wantErr == nil
			if tc.activate && tc.lockErr == nil && tc.status == CycleDraft {
				db.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM hr_performance_participants WHERE cycle_id=$1 AND company_id=$2`)).
					WithArgs(cycleID, companyA).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(tc.participants))
			}
			if wantsUpdate {
				db.ExpectExec(regexp.QuoteMeta(`UPDATE hr_performance_cycles SET status=$3, updated_at=NOW() WHERE id=$1 AND company_id=$2`)).
					WithArgs(cycleID, companyA, want).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				db.ExpectCommit()
			} else {
				db.ExpectRollback()
			}
			var err error
			if tc.activate {
				err = service.ActivateCycle(context.Background(), companyA, cycleID, adminUser)
			} else {
				err = service.CloseCycle(context.Background(), companyA, cycleID, adminUser)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantAction != "" {
				if got := audit.actions(); len(got) != 1 || got[0] != tc.wantAction {
					t.Fatalf("audit = %v", got)
				}
			} else if len(audit.logs) != 0 {
				t.Fatalf("unexpected audit %v", audit.actions())
			}
			if err := db.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAddParticipantsResolvesReviewerAsOfPeriodEnd(t *testing.T) {
	db, service, audit, resolver := newMock(t)
	mgr := managerEmpID
	resolver.byEmployee[employeeID] = employees.JobHistory{EmployeeID: employeeID, ManagerID: &mgr}
	// employee 12 has no effective job record at period_end: no reviewer.
	var noReviewer *int64
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta(lockCycleSQL)).WithArgs(cycleID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"status", "period_end"}).AddRow(CycleDraft, periodEnd))
	db.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*), COUNT(*) FILTER (WHERE status<>'ACTIVE') FROM hr_employees WHERE company_id=$1 AND id = ANY($2)`)).
		WithArgs(companyA, []int64{employeeID, 12}).WillReturnRows(pgxmock.NewRows([]string{"count", "inactive"}).AddRow(int64(2), int64(0)))
	insertSQL := regexp.QuoteMeta(`INSERT INTO hr_performance_participants(company_id,cycle_id,employee_id,reviewer_employee_id,status)`)
	db.ExpectExec(insertSQL).WithArgs(companyA, cycleID, employeeID, &mgr).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectExec(insertSQL).WithArgs(companyA, cycleID, int64(12), noReviewer).WillReturnResult(pgxmock.NewResult("INSERT", 0))
	db.ExpectCommit()

	added, err := service.AddParticipants(context.Background(), companyA, cycleID, []int64{employeeID, 12, employeeID}, adminUser)
	if err != nil || added != 1 {
		t.Fatalf("AddParticipants() = %d, %v", added, err)
	}
	if len(resolver.calls) != 2 || !resolver.calls[0].Equal(periodEnd) || resolver.companies[0] != companyA {
		t.Fatalf("resolver calls = %v companies = %v", resolver.calls, resolver.companies)
	}
	if got := audit.actions(); len(got) != 1 || got[0] != ActionParticipantsAdded {
		t.Fatalf("audit = %v", got)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAddParticipantsRejectsCrossTenantEmployee(t *testing.T) {
	db, service, audit, resolver := newMock(t)
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta(lockCycleSQL)).WithArgs(cycleID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"status", "period_end"}).AddRow(CycleActive, periodEnd))
	// Employee 900 belongs to company B, so only one of the two ids is found
	// in company A.
	db.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*), COUNT(*) FILTER (WHERE status<>'ACTIVE') FROM hr_employees WHERE company_id=$1 AND id = ANY($2)`)).
		WithArgs(companyA, []int64{employeeID, 900}).WillReturnRows(pgxmock.NewRows([]string{"count", "inactive"}).AddRow(int64(1), int64(0)))
	db.ExpectRollback()

	_, err := service.AddParticipants(context.Background(), companyA, cycleID, []int64{employeeID, 900}, adminUser)
	if !errors.Is(err, ErrEmployeeNotFound) || err.Error() != "hr: not found: employee" {
		t.Fatalf("err = %v", err)
	}
	if len(resolver.calls) != 0 || len(audit.logs) != 0 {
		t.Fatalf("resolver calls %v audit %v", resolver.calls, audit.actions())
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAddParticipantsRejectsInactiveEmployee(t *testing.T) {
	db, service, audit, resolver := newMock(t)
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta(lockCycleSQL)).WithArgs(cycleID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"status", "period_end"}).AddRow(CycleActive, periodEnd))
	// Both ids exist in the company, one of them is INACTIVE (offboarded).
	db.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*), COUNT(*) FILTER (WHERE status<>'ACTIVE') FROM hr_employees WHERE company_id=$1 AND id = ANY($2)`)).
		WithArgs(companyA, []int64{employeeID, 12}).WillReturnRows(pgxmock.NewRows([]string{"count", "inactive"}).AddRow(int64(2), int64(1)))
	db.ExpectRollback() // no insert, no audit

	_, err := service.AddParticipants(context.Background(), companyA, cycleID, []int64{employeeID, 12}, adminUser)
	if !errors.Is(err, ErrEmployeeInactive) || err.Error() != "hr: only active employees can be added to a performance cycle" {
		t.Fatalf("err = %v", err)
	}
	if len(resolver.calls) != 0 || len(audit.logs) != 0 {
		t.Fatalf("resolver calls %v audit %v", resolver.calls, audit.actions())
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAddParticipantsGuards(t *testing.T) {
	_, service, _, _ := newMock(t)
	if _, err := service.AddParticipants(context.Background(), 0, cycleID, []int64{1}, adminUser); !errors.Is(err, ErrCompanyRequired) {
		t.Fatalf("company 0 err = %v", err)
	}
	if _, err := service.AddParticipants(context.Background(), companyA, cycleID, nil, adminUser); !errors.Is(err, ErrNoEmployees) {
		t.Fatalf("empty err = %v", err)
	}
	if _, err := service.AddParticipants(context.Background(), companyA, cycleID, []int64{-1}, adminUser); !errors.Is(err, ErrEmployeeNotFound) {
		t.Fatalf("negative id err = %v", err)
	}

	db, service, _, _ := newMock(t)
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta(lockCycleSQL)).WithArgs(cycleID, companyA).
		WillReturnRows(pgxmock.NewRows([]string{"status", "period_end"}).AddRow(CycleClosed, periodEnd))
	db.ExpectRollback()
	if _, err := service.AddParticipants(context.Background(), companyA, cycleID, []int64{employeeID}, adminUser); !errors.Is(err, ErrCycleClosed) {
		t.Fatalf("closed err = %v", err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetCycleCrossTenantNotFound(t *testing.T) {
	db, service, _, _ := newMock(t)
	db.ExpectQuery(regexp.QuoteMeta(`FROM hr_performance_cycles c WHERE c.company_id=$1 AND c.id=$2`)).
		WithArgs(companyB, cycleID).WillReturnError(pgx.ErrNoRows)
	if _, err := service.GetCycle(context.Background(), companyB, cycleID); !errors.Is(err, ErrCycleNotFound) {
		t.Fatalf("err = %v", err)
	}
	db.ExpectQuery(regexp.QuoteMeta(`FROM hr_performance_cycles c WHERE c.company_id=$1 ORDER BY`)).
		WithArgs(companyA).
		WillReturnRows(pgxmock.NewRows(cycleCls).AddRow(cycleID, companyA, "2026 H1", periodStart, periodEnd, selfDue, managerDue, CycleActive, nil, createdAt, createdAt, int64(3), int64(1)))
	cycles, err := service.ListCycles(context.Background(), companyA)
	if err != nil || len(cycles) != 1 || cycles[0].ParticipantCount != 3 || cycles[0].CompletedCount != 1 {
		t.Fatalf("ListCycles() = %#v, %v", cycles, err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
