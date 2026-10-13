package compensation

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type auditFake struct{ logs []shared.AuditLog }

func (a *auditFake) RecordTx(_ context.Context, _ pgx.Tx, log shared.AuditLog) error {
	a.logs = append(a.logs, log)
	return nil
}

const (
	company  = int64(7)
	employee = int64(42)
	actor    = int64(99)
)

var assignmentColNames = []string{"id", "employee_id", "base_salary", "ptkp_code", "bpjs_health", "bpjs_employment",
	"bank_code", "bank_account_number", "bank_account_name", "effective_from", "effective_to",
	"reason", "created_by", "created_at"}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func ptrTime(t time.Time) *time.Time { return &t }

type row struct {
	id         int64
	salary     string
	ptkp       string
	bank       [3]string
	from       time.Time
	to         *time.Time
	reason     string
	health, em bool
}

func assignmentRows(rs ...row) *pgxmock.Rows {
	out := pgxmock.NewRows(assignmentColNames)
	for _, r := range rs {
		out.AddRow(r.id, employee, r.salary, r.ptkp, r.health, r.em, r.bank[0], r.bank[1], r.bank[2],
			r.from, r.to, r.reason, (*int64)(nil), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	}
	return out
}

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *Service, *auditFake) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(db.Close)
	audit := &auditFake{}
	return db, &Service{pool: db, audit: audit}, audit
}

func q(s string) string { return regexp.QuoteMeta(s) }

func expectLock(db pgxmock.PgxPoolIface, rs ...row) {
	db.ExpectQuery(q("SELECT e.id FROM hr_employees e WHERE e.id=$2 AND e.company_id=$1 FOR NO KEY UPDATE")).
		WithArgs(company, employee).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(employee))
	db.ExpectQuery(q("JOIN hr_employees e ON e.id=ca.employee_id AND e.company_id=$1")+`(.|\n)*`+q("FOR UPDATE OF ca")).
		WithArgs(company, employee).
		WillReturnRows(assignmentRows(rs...))
}

// expectMoveComponents asserts the exact three-statement component move.
func expectMoveComponents(db pgxmock.PgxPoolIface, oldID, newID int64, newFrom time.Time) {
	db.ExpectExec(q("INSERT INTO payroll_recurring_components(assignment_id, component_id, amount, effective_from, effective_to)")+`\s*`+
		q("SELECT $1, component_id, amount, GREATEST(effective_from, $3::date), effective_to")+`\s*`+
		q("FROM payroll_recurring_components")+`\s*`+
		q("WHERE assignment_id = $2 AND (effective_to IS NULL OR effective_to >= $3::date)")).
		WithArgs(newID, oldID, newFrom).
		WillReturnResult(pgxmock.NewResult("INSERT", 2))
	db.ExpectExec(q("DELETE FROM payroll_recurring_components")+`\s*`+
		q("WHERE assignment_id = $1 AND effective_from >= $2::date")+`$`).
		WithArgs(oldID, newFrom).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	db.ExpectExec(q("UPDATE payroll_recurring_components SET effective_to = $3::date")+`\s*`+
		q("WHERE assignment_id = $1 AND effective_from < $2::date AND (effective_to IS NULL OR effective_to >= $2::date)")+`$`).
		WithArgs(oldID, newFrom, newFrom.AddDate(0, 0, -1)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
}

func requireMetaHasNoMoney(t *testing.T, log shared.AuditLog) {
	t.Helper()
	require.Len(t, log.Meta, 3)
	require.Contains(t, log.Meta, "assignment_id")
	require.Contains(t, log.Meta, "effective_from")
	require.Contains(t, log.Meta, "reason")
	for k := range log.Meta {
		require.NotContains(t, []string{"base_salary", "bank_code", "bank_account_number", "bank_account_name", "salary"}, k)
	}
}

func TestChangeMovesRecurringComponents(t *testing.T) {
	db, svc, audit := newMock(t)
	cur := row{id: 10, salary: "8000000.00", ptkp: "TK/0", bank: [3]string{"BCA", "111", "Budi"}, from: day(2026, 1, 1), health: true, em: true}
	newFrom := day(2026, 7, 1)

	db.ExpectBegin()
	expectLock(db, cur)
	db.ExpectExec(q("UPDATE payroll_compensation_assignments SET effective_to=$2 WHERE id=$1")).
		WithArgs(int64(10), day(2026, 6, 30)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	// Bank fields come from cur, not from the (stale) input.
	db.ExpectQuery(q("INSERT INTO payroll_compensation_assignments AS ca")).
		WithArgs(employee, "9000000.50", "K/1", true, false, "BCA", "111", "Budi", newFrom, (*time.Time)(nil), "Promotion", pgxmock.AnyArg()).
		WillReturnRows(assignmentRows(row{id: 11, salary: "9000000.50", ptkp: "K/1", bank: [3]string{"BCA", "111", "Budi"}, from: newFrom, reason: "Promotion", health: true}))
	expectMoveComponents(db, 10, 11, newFrom)
	db.ExpectCommit()
	db.ExpectRollback()

	got, err := svc.Change(context.Background(), company, employee, ChangeInput{
		EffectiveFrom: newFrom, BaseSalary: "9000000.50", PTKPCode: "K/1", BPJSHealth: true, BPJSEmployment: false,
		BankCode: "STALE", BankAccountNumber: "999", BankAccountName: "Stale", Reason: "Promotion", ActorID: actor,
	})
	require.NoError(t, err)
	require.Equal(t, int64(11), got.ID)
	require.Equal(t, "9000000.50", got.BaseSalary)
	require.Equal(t, "BCA", got.BankCode)
	require.Len(t, audit.logs, 1)
	require.Equal(t, ActionCompensationChanged, audit.logs[0].Action)
	require.Equal(t, actor, audit.logs[0].ActorID)
	require.Equal(t, "42", audit.logs[0].EntityID)
	require.Equal(t, int64(11), audit.logs[0].Meta["assignment_id"])
	require.Equal(t, "2026-07-01", audit.logs[0].Meta["effective_from"])
	requireMetaHasNoMoney(t, audit.logs[0])
	require.NoError(t, db.ExpectationsWereMet())
}

func TestChangeSameDayUpdatesInPlace(t *testing.T) {
	db, svc, audit := newMock(t)
	from := day(2026, 7, 1)
	db.ExpectBegin()
	expectLock(db,
		row{id: 10, salary: "8000000.00", ptkp: "TK/0", from: day(2026, 1, 1), to: ptrTime(day(2026, 6, 30))},
		row{id: 11, salary: "9000000.00", ptkp: "TK/0", bank: [3]string{"BCA", "111", "Budi"}, from: from},
	)
	db.ExpectExec(q("UPDATE payroll_compensation_assignments")+`\s*`+
		q("SET base_salary=$2::text::numeric, ptkp_code=$3, bpjs_health=$4, bpjs_employment=$5, reason=$6")+`\s*`+q("WHERE id=$1")).
		WithArgs(int64(11), "9500000", "K/0", true, true, "Correction").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()
	db.ExpectRollback()

	got, err := svc.Change(context.Background(), company, employee, ChangeInput{
		EffectiveFrom: from, BaseSalary: " 9500000 ", PTKPCode: "K/0", BPJSHealth: true, BPJSEmployment: true,
		BankCode: "STALE", Reason: "Correction", ActorID: actor,
	})
	require.NoError(t, err)
	require.Equal(t, int64(11), got.ID)
	require.Equal(t, "9500000", got.BaseSalary)
	require.Equal(t, "BCA", got.BankCode, "salary change must not touch bank fields")
	require.Len(t, audit.logs, 1)
	requireMetaHasNoMoney(t, audit.logs[0])
	require.NoError(t, db.ExpectationsWereMet())
}

func TestChangeRejectsLaterAssignment(t *testing.T) {
	db, svc, audit := newMock(t)
	db.ExpectBegin()
	expectLock(db,
		row{id: 10, salary: "8000000.00", ptkp: "TK/0", from: day(2026, 1, 1), to: ptrTime(day(2026, 8, 31))},
		row{id: 11, salary: "9000000.00", ptkp: "TK/0", from: day(2026, 9, 1)},
	)
	db.ExpectRollback()

	_, err := svc.Change(context.Background(), company, employee, ChangeInput{
		EffectiveFrom: day(2026, 7, 1), BaseSalary: "8500000", PTKPCode: "TK/0", ActorID: actor,
	})
	require.ErrorIs(t, err, ErrLaterChangeExists)
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestChangeBankPropagatesToFutureAssignments(t *testing.T) {
	db, svc, audit := newMock(t)
	ctx := context.Background()
	bankFrom := day(2026, 5, 1)
	raiseFrom := day(2026, 9, 1)
	bank := BankDetails{BankCode: "MANDIRI", BankAccountNumber: "222", BankAccountName: "Budi S"}

	db.ExpectBegin()
	expectLock(db,
		row{id: 10, salary: "8000000.00", ptkp: "TK/0", bank: [3]string{"BCA", "111", "Budi"}, from: day(2026, 1, 1), to: ptrTime(day(2026, 8, 31)), health: true, em: true},
		row{id: 11, salary: "9000000.00", ptkp: "TK/0", bank: [3]string{"BCA", "111", "Budi"}, from: raiseFrom, health: true, em: true},
	)
	db.ExpectExec(q("UPDATE payroll_compensation_assignments SET effective_to=$2 WHERE id=$1")).
		WithArgs(int64(10), day(2026, 4, 30)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	// Salary/PTKP/BPJS copied from cur; effective_to inherited (raise - 1), not NULL.
	db.ExpectQuery(q("INSERT INTO payroll_compensation_assignments AS ca")).
		WithArgs(employee, "8000000.00", "TK/0", true, true, "MANDIRI", "222", "Budi S", bankFrom, ptrTime(day(2026, 8, 31)), "New account", pgxmock.AnyArg()).
		WillReturnRows(assignmentRows(row{id: 12, salary: "8000000.00", ptkp: "TK/0", bank: [3]string{"MANDIRI", "222", "Budi S"}, from: bankFrom, to: ptrTime(day(2026, 8, 31)), reason: "New account", health: true, em: true}))
	// Components move only off the split assignment (10); the raise (11) keeps its own.
	expectMoveComponents(db, 10, 12, bankFrom)
	db.ExpectExec(q("UPDATE payroll_compensation_assignments")+`\s*`+
		q("SET bank_code=$1, bank_account_number=$2, bank_account_name=$3")+`\s*`+
		q("WHERE employee_id=$4 AND effective_from > $5")).
		WithArgs("MANDIRI", "222", "Budi S", employee, bankFrom).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	tx, err := db.Begin(ctx)
	require.NoError(t, err)
	got, err := svc.ChangeBankTx(ctx, tx, company, employee, bank, bankFrom, "New account", actor)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	require.Equal(t, int64(12), got.ID)
	require.Equal(t, "8000000.00", got.BaseSalary)
	require.Equal(t, "MANDIRI", got.BankCode)
	require.NotNil(t, got.EffectiveTo)
	require.Equal(t, day(2026, 8, 31), *got.EffectiveTo)
	require.Len(t, audit.logs, 1)
	require.Equal(t, ActionBankChanged, audit.logs[0].Action)
	requireMetaHasNoMoney(t, audit.logs[0])
	require.NoError(t, db.ExpectationsWereMet())
}

func TestChangeBankSameDayAsSalaryChange(t *testing.T) {
	db, svc, audit := newMock(t)
	ctx := context.Background()
	from := day(2026, 7, 1)
	oldBank := [3]string{"BCA", "111", "Budi"}

	// 1. Salary change on D splits the assignment.
	db.ExpectBegin()
	expectLock(db, row{id: 10, salary: "8000000.00", ptkp: "TK/0", bank: oldBank, from: day(2026, 1, 1), health: true, em: true})
	db.ExpectExec(q("UPDATE payroll_compensation_assignments SET effective_to=$2 WHERE id=$1")).
		WithArgs(int64(10), day(2026, 6, 30)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectQuery(q("INSERT INTO payroll_compensation_assignments AS ca")).
		WithArgs(employee, "9000000.00", "TK/0", true, true, "BCA", "111", "Budi", from, (*time.Time)(nil), "Raise", pgxmock.AnyArg()).
		WillReturnRows(assignmentRows(row{id: 11, salary: "9000000.00", ptkp: "TK/0", bank: oldBank, from: from, reason: "Raise", health: true, em: true}))
	expectMoveComponents(db, 10, 11, from)
	db.ExpectCommit()
	db.ExpectRollback()

	// 2. Bank change on the same D updates that row in place.
	db.ExpectBegin()
	expectLock(db,
		row{id: 10, salary: "8000000.00", ptkp: "TK/0", bank: oldBank, from: day(2026, 1, 1), to: ptrTime(day(2026, 6, 30)), health: true, em: true},
		row{id: 11, salary: "9000000.00", ptkp: "TK/0", bank: oldBank, from: from, reason: "Raise", health: true, em: true},
	)
	db.ExpectExec(q("UPDATE payroll_compensation_assignments")+`\s*`+
		q("SET bank_code=$2, bank_account_number=$3, bank_account_name=$4, reason=$5")+`\s*`+q("WHERE id=$1")).
		WithArgs(int64(11), "BNI", "333", "Budi", "New bank").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(q("WHERE employee_id=$4 AND effective_from > $5")).
		WithArgs("BNI", "333", "Budi", employee, from).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	db.ExpectCommit()
	db.ExpectRollback()

	_, err := svc.Change(ctx, company, employee, ChangeInput{EffectiveFrom: from, BaseSalary: "9000000.00", PTKPCode: "TK/0", BPJSHealth: true, BPJSEmployment: true, Reason: "Raise", ActorID: actor})
	require.NoError(t, err)
	got, err := svc.ChangeBank(ctx, company, employee, BankDetails{BankCode: "BNI", BankAccountNumber: "333", BankAccountName: "Budi"}, from, "New bank", actor)
	require.NoError(t, err)
	require.Equal(t, int64(11), got.ID, "same-day bank change must not create another row")
	require.Equal(t, "9000000.00", got.BaseSalary)
	require.Equal(t, "BNI", got.BankCode)
	require.Equal(t, "333", got.BankAccountNumber)
	require.Len(t, audit.logs, 2)
	require.Equal(t, ActionCompensationChanged, audit.logs[0].Action)
	require.Equal(t, ActionBankChanged, audit.logs[1].Action)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestChangeRejectsInvalidSalary(t *testing.T) {
	for _, salary := range []string{"-1", "-0.01", "", "abc", "1.234", "1e6", "12345678901234567"} {
		t.Run(salary, func(t *testing.T) {
			db, svc, _ := newMock(t)
			_, err := svc.Change(context.Background(), company, employee, ChangeInput{EffectiveFrom: day(2026, 7, 1), BaseSalary: salary, PTKPCode: "TK/0"})
			require.ErrorIs(t, err, ErrInvalidSalary)
			tx, err := db.Begin(context.Background())
			require.Error(t, err, "no DB call expected")
			require.Nil(t, tx)
		})
	}
}

func TestValidationErrorsWithoutQuerying(t *testing.T) {
	ctx := context.Background()
	valid := ChangeInput{EffectiveFrom: day(2026, 7, 1), BaseSalary: "1000", PTKPCode: "TK/0"}
	cases := []struct {
		name string
		run  func(*Service) error
		want error
	}{
		{"change no company", func(s *Service) error { _, err := s.Change(ctx, 0, employee, valid); return err }, ErrCompanyRequired},
		{"change no employee", func(s *Service) error { _, err := s.Change(ctx, company, 0, valid); return err }, ErrEmployeeRequired},
		{"change no date", func(s *Service) error {
			in := valid
			in.EffectiveFrom = time.Time{}
			_, err := s.Change(ctx, company, employee, in)
			return err
		}, ErrEffectiveFromNeeded},
		{"change no ptkp", func(s *Service) error {
			in := valid
			in.PTKPCode = " "
			_, err := s.Change(ctx, company, employee, in)
			return err
		}, ErrPTKPRequired},
		{"list no company", func(s *Service) error { _, err := s.ListForEmployee(ctx, 0, employee); return err }, ErrCompanyRequired},
		{"asof no company", func(s *Service) error { _, err := s.AsOf(ctx, 0, employee, day(2026, 1, 1)); return err }, ErrCompanyRequired},
		{"bank no account", func(s *Service) error {
			_, err := s.ChangeBank(ctx, company, employee, BankDetails{BankCode: "BCA"}, day(2026, 1, 1), "", actor)
			return err
		}, ErrBankRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, svc, _ := newMock(t)
			require.ErrorIs(t, tc.run(svc), tc.want)
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}

func TestChangeRejectsCrossTenantEmployee(t *testing.T) {
	db, svc, audit := newMock(t)
	other := int64(8)
	db.ExpectBegin()
	db.ExpectQuery(q("SELECT e.id FROM hr_employees e WHERE e.id=$2 AND e.company_id=$1 FOR NO KEY UPDATE")).
		WithArgs(other, employee).
		WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()

	_, err := svc.Change(context.Background(), other, employee, ChangeInput{EffectiveFrom: day(2026, 7, 1), BaseSalary: "1000", PTKPCode: "TK/0", ActorID: actor})
	require.ErrorIs(t, err, ErrEmployeeNotFound)
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestReadsAreCompanyScoped(t *testing.T) {
	db, svc, _ := newMock(t)
	scope := q("JOIN hr_employees e ON e.id=ca.employee_id AND e.company_id=$1")
	db.ExpectQuery(scope+`(.|\n)*`+q("ORDER BY ca.effective_from DESC")).WithArgs(company, employee).
		WillReturnRows(assignmentRows(row{id: 10, salary: "8000000.00", ptkp: "TK/0", from: day(2026, 1, 1)}))
	db.ExpectQuery(scope+`(.|\n)*`+q("ca.effective_from<=$3 AND (ca.effective_to IS NULL OR ca.effective_to>=$3)")).
		WithArgs(company, employee, day(2026, 3, 1)).
		WillReturnRows(assignmentRows(row{id: 10, salary: "8000000.00", ptkp: "TK/0", from: day(2026, 1, 1)}))
	db.ExpectQuery(scope).WithArgs(int64(8), employee, day(2026, 3, 1)).
		WillReturnRows(pgxmock.NewRows(assignmentColNames))

	list, err := svc.ListForEmployee(context.Background(), company, employee)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "8000000.00", list[0].BaseSalary)
	a, err := svc.AsOf(context.Background(), company, employee, time.Date(2026, 3, 1, 15, 4, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, int64(10), a.ID)
	_, err = svc.AsOf(context.Background(), 8, employee, day(2026, 3, 1))
	require.ErrorIs(t, err, ErrNoAssignment)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestChangeWithoutAssignmentCreatesInitial(t *testing.T) {
	db, svc, audit := newMock(t)
	from := day(2026, 7, 1)
	db.ExpectBegin()
	expectLock(db)
	db.ExpectQuery(q("INSERT INTO payroll_compensation_assignments AS ca")).
		WithArgs(employee, "5000000", "TK/0", true, true, "BCA", "111", "Budi", from, (*time.Time)(nil), "Hired", pgxmock.AnyArg()).
		WillReturnRows(assignmentRows(row{id: 20, salary: "5000000.00", ptkp: "TK/0", bank: [3]string{"BCA", "111", "Budi"}, from: from, reason: "Hired", health: true, em: true}))
	db.ExpectCommit()
	db.ExpectRollback()

	got, err := svc.Change(context.Background(), company, employee, ChangeInput{
		EffectiveFrom: from, BaseSalary: "5000000", PTKPCode: "TK/0", BPJSHealth: true, BPJSEmployment: true,
		BankCode: "BCA", BankAccountNumber: "111", BankAccountName: "Budi", Reason: "Hired", ActorID: actor,
	})
	require.NoError(t, err)
	require.Equal(t, int64(20), got.ID)
	require.Len(t, audit.logs, 1)
	requireMetaHasNoMoney(t, audit.logs[0])
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCreateInitialTxRejectsExistingAssignment(t *testing.T) {
	db, svc, audit := newMock(t)
	ctx := context.Background()
	db.ExpectBegin()
	expectLock(db, row{id: 10, salary: "8000000.00", ptkp: "TK/0", from: day(2026, 1, 1)})
	db.ExpectRollback()

	tx, err := db.Begin(ctx)
	require.NoError(t, err)
	_, err = svc.CreateInitialTx(ctx, tx, company, employee, ChangeInput{EffectiveFrom: day(2026, 7, 1), BaseSalary: "1000", PTKPCode: "TK/0"})
	require.ErrorIs(t, err, ErrAssignmentExists)
	require.NoError(t, tx.Rollback(ctx))
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestChangeBankWithoutAssignmentErrors(t *testing.T) {
	db, svc, _ := newMock(t)
	db.ExpectBegin()
	expectLock(db, row{id: 10, salary: "8000000.00", ptkp: "TK/0", from: day(2026, 7, 1)})
	db.ExpectRollback()

	_, err := svc.ChangeBank(context.Background(), company, employee, BankDetails{BankCode: "BCA", BankAccountNumber: "1"}, day(2026, 6, 1), "", actor)
	require.ErrorIs(t, err, ErrNoAssignment)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestAuditAdapterForMinimalTx(t *testing.T) {
	// A DBTX that is not a pgx.Tx must still reach the audit writer via Exec.
	db, err := pgxmock.NewConn()
	require.NoError(t, err)
	defer func() { _ = db.Close(context.Background()) }()
	logger := shared.NewAuditLogger(nil)
	svc := &Service{audit: logger}
	db.ExpectExec(q("INSERT INTO audit_logs")).
		WithArgs(actor, ActionBankChanged, "hr_employee", "42", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	require.NoError(t, svc.record(context.Background(), minimalTx{db}, actor, ActionBankChanged, employee, 5, day(2026, 1, 1), "r"))
	require.NoError(t, db.ExpectationsWereMet())
}

type minimalTx struct{ DBTX }
