package employees

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

const (
	lockEmployeeSQL = "SELECT termination_date FROM hr_employees WHERE id=$1 AND company_id=$2 FOR UPDATE"
	correctSQL      = "UPDATE hr_employee_job_history SET department_id=$2, position_id=$3, manager_id=$4, status=$5, employment_type=$6, grade=$7, reason=$8 WHERE id=$1"
	mirrorSQL       = "UPDATE hr_employees SET department_id=$1, position_id=$2, manager_id=$3, status=$4, employment_type=$5"
	clearTermSQL    = "UPDATE hr_employees SET termination_date=NULL, termination_reason='', updated_at=NOW() WHERE id=$1 AND company_id=$2"
)

func expectLock(db pgxmock.PgxPoolIface, employee, company int64, terminated *time.Time) {
	db.ExpectQuery(q(lockEmployeeSQL)).WithArgs(employee, company).
		WillReturnRows(pgxmock.NewRows([]string{"termination_date"}).AddRow(terminated))
}

func expectLatestRow(db pgxmock.PgxPoolIface, company, employee int64, prev JobHistory) {
	db.ExpectQuery(q("WHERE jh.employee_id=$2 ORDER BY jh.valid_from DESC LIMIT 1 FOR UPDATE OF jh")).
		WithArgs(company, employee).WillReturnRows(jobRows(prev))
}

func today() time.Time { return businessToday() }

func TestRecordJobChangeSameDayCorrectsOpenRecordInPlace(t *testing.T) {
	const company, employee = int64(2), int64(8)
	dept, newDept := int64(3), int64(30)
	hired := today().AddDate(0, 0, -10)
	prev := JobHistory{ID: 40, CompanyID: company, EmployeeID: employee, DepartmentID: &dept, Status: "ACTIVE", EmploymentType: "PERMANENT", ValidFrom: hired, Reason: "Hired", CreatedBy: i64(5)}

	db, service, audit := newMock(t)
	db.ExpectBegin()
	expectLock(db, employee, company, nil)
	db.ExpectQuery(q("SELECT 1 FROM hr_departments WHERE id=$1 AND company_id=$2")).WithArgs(newDept, company).WillReturnRows(pgxmock.NewRows([]string{"x"}).AddRow(1))
	expectLatestRow(db, company, employee, prev)
	db.ExpectExec(q(correctSQL)).WithArgs(int64(40), &newDept, (*int64)(nil), (*int64)(nil), "ACTIVE", "PERMANENT", "G2", "Fix dept").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(q(mirrorSQL)).WithArgs(&newDept, (*int64)(nil), (*int64)(nil), "ACTIVE", "PERMANENT", employee, company).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()
	db.ExpectRollback()

	got, err := service.RecordJobChange(context.Background(), company, employee, JobChangeInput{DepartmentID: &newDept, Grade: "G2", ValidFrom: hired, Reason: "Fix dept", ActorID: 99})
	require.NoError(t, err)
	require.Equal(t, int64(40), got.ID, "the open record is rewritten, not duplicated")
	require.Len(t, audit.logs, 1)
	require.Equal(t, ActionEmployeeJobChanged, audit.logs[0].Action)
	require.Equal(t, true, audit.logs[0].Meta["corrected"])
	require.Equal(t, []string{"department_id", "grade"}, audit.logs[0].Meta["fields"])
	require.NoError(t, db.ExpectationsWereMet())
}

func TestRecordJobChangeCorrectsFutureHireDateRecordWithoutMirroring(t *testing.T) {
	const company, employee = int64(2), int64(8)
	newDept := int64(30)
	hire := today().AddDate(0, 0, 14) // pre-boarded: the "Hired" record starts in the future
	prev := JobHistory{ID: 40, CompanyID: company, EmployeeID: employee, Status: "ACTIVE", EmploymentType: "PERMANENT", ValidFrom: hire, Reason: "Hired"}

	db, service, audit := newMock(t)
	db.ExpectBegin()
	expectLock(db, employee, company, nil)
	db.ExpectQuery(q("SELECT 1 FROM hr_departments WHERE id=$1 AND company_id=$2")).WithArgs(newDept, company).WillReturnRows(pgxmock.NewRows([]string{"x"}).AddRow(1))
	expectLatestRow(db, company, employee, prev)
	db.ExpectExec(q(correctSQL)).WithArgs(int64(40), &newDept, (*int64)(nil), (*int64)(nil), "ACTIVE", "PERMANENT", "", "").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit() // no hr_employees mirror: ApplyDueJobChanges does it on the start date
	db.ExpectRollback()

	got, err := service.RecordJobChange(context.Background(), company, employee, JobChangeInput{DepartmentID: &newDept, ValidFrom: hire})
	require.NoError(t, err)
	require.Equal(t, int64(40), got.ID)
	require.Equal(t, true, audit.logs[0].Meta["corrected"])
	require.NoError(t, db.ExpectationsWereMet())
}

func TestRecordJobChangeSecondSameDayEditCorrectsTheFirst(t *testing.T) {
	const company, employee = int64(2), int64(8)
	dept1, dept2 := int64(30), int64(31)
	old := JobHistory{ID: 40, CompanyID: company, EmployeeID: employee, Status: "ACTIVE", EmploymentType: "PERMANENT", ValidFrom: day(2025, 1, 1)}
	db, service, audit := newMock(t)

	// First edit today: closes the old record and inserts record 41.
	db.ExpectBegin()
	expectLock(db, employee, company, nil)
	db.ExpectQuery(q("SELECT 1 FROM hr_departments")).WithArgs(dept1, company).WillReturnRows(pgxmock.NewRows([]string{"x"}).AddRow(1))
	expectLatestRow(db, company, employee, old)
	db.ExpectExec(q("UPDATE hr_employee_job_history SET valid_to=$2 WHERE id=$1")).WithArgs(int64(40), today().AddDate(0, 0, -1)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectQuery(q("INSERT INTO hr_employee_job_history(")).WithArgs(anyArgs(11)...).WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow(int64(41), time.Now()))
	db.ExpectExec(q(mirrorSQL)).WithArgs(anyArgs(7)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()
	db.ExpectRollback()
	first, err := service.RecordJobChange(context.Background(), company, employee, JobChangeInput{DepartmentID: &dept1, ValidFrom: today()})
	require.NoError(t, err)
	require.Equal(t, int64(41), first.ID)

	// Second edit the same day: record 41 is open and starts today, so it is rewritten.
	db.ExpectBegin()
	expectLock(db, employee, company, nil)
	db.ExpectQuery(q("SELECT 1 FROM hr_departments")).WithArgs(dept2, company).WillReturnRows(pgxmock.NewRows([]string{"x"}).AddRow(1))
	expectLatestRow(db, company, employee, JobHistory{ID: 41, CompanyID: company, EmployeeID: employee, DepartmentID: &dept1, Status: "ACTIVE", EmploymentType: "PERMANENT", ValidFrom: today()})
	db.ExpectExec(q(correctSQL)).WithArgs(int64(41), &dept2, (*int64)(nil), (*int64)(nil), "ACTIVE", "PERMANENT", "", "").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(q(mirrorSQL)).WithArgs(anyArgs(7)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()
	db.ExpectRollback()
	second, err := service.RecordJobChange(context.Background(), company, employee, JobChangeInput{DepartmentID: &dept2, ValidFrom: today()})
	require.NoError(t, err)
	require.Equal(t, int64(41), second.ID)
	require.Len(t, audit.logs, 2)
	require.Nil(t, audit.logs[0].Meta["corrected"])
	require.Equal(t, true, audit.logs[1].Meta["corrected"])
	require.NoError(t, db.ExpectationsWereMet())
}

func anyArgs(n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = pgxmock.AnyArg()
	}
	return out
}

func updateInputFor(stored Employee) UpdateInput {
	return UpdateInput{EmployeeNumber: stored.EmployeeNumber, Name: "Ada Updated", Email: stored.Email, HireDate: stored.HireDate, ActorID: 99}
}

func TestUpdateWithJobChangeIsAtomic(t *testing.T) {
	const company, employee = int64(2), int64(8)
	dept, newDept := int64(3), int64(30)
	stored := Employee{ID: employee, CompanyID: company, EmployeeNumber: "E-007", Name: "Ada", Email: "ada@example.com", DepartmentID: &dept, HireDate: day(2026, 7, 1), Status: "ACTIVE", EmploymentType: "PERMANENT"}
	prev := JobHistory{ID: 40, CompanyID: company, EmployeeID: employee, DepartmentID: &dept, Status: "ACTIVE", EmploymentType: "PERMANENT", ValidFrom: day(2026, 7, 1)}

	t.Run("corrects the hire record and updates the profile in one commit", func(t *testing.T) {
		db, service, audit := newMock(t)
		db.ExpectBegin()
		db.ExpectQuery(q("FOR UPDATE OF e")).WithArgs(employee, company).WillReturnRows(employeeRows(stored))
		db.ExpectExec(q("UPDATE hr_employees SET user_id=$1")).WithArgs(anyArgs(23)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		expectLock(db, employee, company, nil)
		db.ExpectQuery(q("SELECT 1 FROM hr_departments")).WithArgs(newDept, company).WillReturnRows(pgxmock.NewRows([]string{"x"}).AddRow(1))
		expectLatestRow(db, company, employee, prev)
		db.ExpectExec(q(correctSQL)).WithArgs(anyArgs(8)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectExec(q(mirrorSQL)).WithArgs(anyArgs(7)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()
		db.ExpectRollback()
		emp, err := service.UpdateWithJobChange(context.Background(), company, employee, updateInputFor(stored),
			&JobChangeInput{DepartmentID: &newDept, ValidFrom: day(2026, 7, 1), ActorID: 99})
		require.NoError(t, err)
		require.Equal(t, "Ada Updated", emp.Name)
		require.Len(t, audit.logs, 2)
		require.NoError(t, db.ExpectationsWereMet())
	})

	t.Run("a rejected job change rolls back the profile edit", func(t *testing.T) {
		db, service, _ := newMock(t)
		db.ExpectBegin()
		db.ExpectQuery(q("FOR UPDATE OF e")).WithArgs(employee, company).WillReturnRows(employeeRows(stored))
		db.ExpectExec(q("UPDATE hr_employees SET user_id=$1")).WithArgs(anyArgs(23)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		expectLock(db, employee, company, nil)
		db.ExpectQuery(q("SELECT 1 FROM hr_departments")).WithArgs(newDept, company).WillReturnError(pgx.ErrNoRows)
		db.ExpectRollback() // no ExpectCommit: a commit would fail the test
		_, err := service.UpdateWithJobChange(context.Background(), company, employee, updateInputFor(stored),
			&JobChangeInput{DepartmentID: &newDept, ValidFrom: day(2026, 7, 1)})
		require.EqualError(t, err, "hr: not found: department")
		require.NoError(t, db.ExpectationsWereMet())
	})

	t.Run("an invalid job change is rejected before any database work", func(t *testing.T) {
		db, service, _ := newMock(t)
		_, err := service.UpdateWithJobChange(context.Background(), company, employee, updateInputFor(stored), &JobChangeInput{Status: "GONE", ValidFrom: day(2026, 7, 1)})
		require.ErrorIs(t, err, ErrInvalidStatus)
		require.NoError(t, db.ExpectationsWereMet())
	})
}

func TestRecordJobChangeRehireClearsTermination(t *testing.T) {
	const company, employee = int64(2), int64(8)
	terminated := day(2026, 3, 31)
	rehire := day(2026, 4, 1)
	// Offboarding closed the active record at the termination date and added an
	// INACTIVE record starting the day after.
	inactive := JobHistory{ID: 42, CompanyID: company, EmployeeID: employee, Status: "INACTIVE", EmploymentType: "PERMANENT", ValidFrom: rehire, Reason: "Offboarded"}

	t.Run("reactivation on the INACTIVE record's start date clears termination and is audited", func(t *testing.T) {
		db, service, audit := newMock(t)
		db.ExpectBegin()
		expectLock(db, employee, company, &terminated)
		expectLatestRow(db, company, employee, inactive)
		db.ExpectExec(q(correctSQL)).WithArgs(anyArgs(8)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectExec(q(mirrorSQL)).WithArgs(anyArgs(7)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectExec(q(clearTermSQL)).WithArgs(employee, company).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()
		db.ExpectRollback()
		_, err := service.RecordJobChange(context.Background(), company, employee, JobChangeInput{Status: "ACTIVE", ValidFrom: rehire, Reason: "Rehired", ActorID: 99})
		require.NoError(t, err)
		require.Len(t, audit.logs, 2)
		require.Equal(t, ActionEmployeeRehired, audit.logs[0].Action)
		require.Equal(t, "2026-03-31", audit.logs[0].Meta["previous_termination_date"])
		require.Equal(t, ActionEmployeeJobChanged, audit.logs[1].Action)
		require.NoError(t, db.ExpectationsWereMet())
	})

	t.Run("a later reactivation after the INACTIVE record also clears termination", func(t *testing.T) {
		db, service, audit := newMock(t)
		later := day(2026, 5, 1)
		db.ExpectBegin()
		expectLock(db, employee, company, &terminated)
		expectLatestRow(db, company, employee, inactive)
		db.ExpectExec(q("UPDATE hr_employee_job_history SET valid_to=$2 WHERE id=$1")).WithArgs(int64(42), day(2026, 4, 30)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectQuery(q("INSERT INTO hr_employee_job_history(")).WithArgs(anyArgs(11)...).WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow(int64(43), time.Now()))
		db.ExpectExec(q(mirrorSQL)).WithArgs(anyArgs(7)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectExec(q(clearTermSQL)).WithArgs(employee, company).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()
		db.ExpectRollback()
		_, err := service.RecordJobChange(context.Background(), company, employee, JobChangeInput{Status: "ACTIVE", ValidFrom: later})
		require.NoError(t, err)
		require.Equal(t, ActionEmployeeRehired, audit.logs[0].Action)
		require.NoError(t, db.ExpectationsWereMet())
	})

	t.Run("a non-rehire change keeps the termination date", func(t *testing.T) {
		db, service, audit := newMock(t)
		// Active employee with a future termination date gets a department change
		// effective on/before it: not a rehire.
		future := today().AddDate(0, 1, 0)
		active := JobHistory{ID: 40, CompanyID: company, EmployeeID: employee, Status: "ACTIVE", EmploymentType: "PERMANENT", ValidFrom: day(2025, 1, 1)}
		db.ExpectBegin()
		expectLock(db, employee, company, &future)
		expectLatestRow(db, company, employee, active)
		db.ExpectExec(q("UPDATE hr_employee_job_history SET valid_to=$2 WHERE id=$1")).WithArgs(anyArgs(2)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectQuery(q("INSERT INTO hr_employee_job_history(")).WithArgs(anyArgs(11)...).WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow(int64(43), time.Now()))
		db.ExpectExec(q(mirrorSQL)).WithArgs(anyArgs(7)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit() // no clear-termination Exec expected
		db.ExpectRollback()
		_, err := service.RecordJobChange(context.Background(), company, employee, JobChangeInput{Status: "ACTIVE", Grade: "G9", ValidFrom: today()})
		require.NoError(t, err)
		require.Len(t, audit.logs, 1)
		require.NoError(t, db.ExpectationsWereMet())
	})
}

// A rehired employee has termination_date NULL and status ACTIVE, which both
// the scheduled catch-up (e.termination_date IS NULL) and the payroll employee
// filter (termination_date IS NULL OR >= period start, AND status ACTIVE OR
// terminated) include; a terminated one is excluded from both once the period
// starts after the termination date.
func TestRehiredEmployeeIsPickedUpByScheduledCatchUp(t *testing.T) {
	db, service, audit := newMock(t)
	asOf := day(2026, 5, 4)
	dept := int64(3)
	db.ExpectQuery(q("e.termination_date IS NULL AND jh.valid_from<=$1")).WithArgs(asOf, int64(0)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id"}).AddRow(int64(8), int64(2)))
	db.ExpectBegin()
	db.ExpectQuery(q("SELECT id FROM hr_employees WHERE id=$1 AND company_id=$2 AND termination_date IS NULL FOR UPDATE")).WithArgs(int64(8), int64(2)).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(8)))
	db.ExpectQuery(q("ORDER BY jh.valid_from DESC LIMIT 1")).WithArgs(int64(8), int64(2), asOf).WillReturnRows(
		pgxmock.NewRows([]string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m"}).
			AddRow(int64(50), day(2026, 5, 1), (*int64)(nil), (*int64)(nil), (*int64)(nil), "INACTIVE", "PERMANENT", &dept, (*int64)(nil), (*int64)(nil), "ACTIVE", "PERMANENT", (*int64)(nil)))
	db.ExpectExec(q("UPDATE hr_employees SET")).WithArgs(anyArgs(7)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()
	db.ExpectRollback()
	n, err := service.ApplyDueJobChanges(context.Background(), asOf)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Len(t, audit.logs, 1)
	require.Equal(t, SystemActorID, audit.logs[0].ActorID, "a record without an author falls back to the system user")
	require.NoError(t, db.ExpectationsWereMet())
}

func TestApplyDueJobChangesLockedEmployeeGoneOrTerminatedIsSkipped(t *testing.T) {
	db, service, audit := newMock(t)
	asOf := day(2026, 5, 4)
	db.ExpectQuery(q("SELECT DISTINCT e.id, e.company_id")).WithArgs(asOf, int64(2)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id"}).AddRow(int64(8), int64(2)))
	db.ExpectBegin()
	// Offboarded between the candidate query and the lock: the guarded lock
	// finds nothing, so no job record is read and nothing is written.
	db.ExpectQuery(q("SELECT id FROM hr_employees WHERE id=$1 AND company_id=$2 AND termination_date IS NULL FOR UPDATE")).WithArgs(int64(8), int64(2)).WillReturnError(pgx.ErrNoRows)
	db.ExpectCommit()
	db.ExpectRollback()
	n, err := service.ApplyDueJobChangesForCompany(context.Background(), 2, asOf)
	require.NoError(t, err)
	require.Zero(t, n)
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())

	_, err = service.ApplyDueJobChangesForCompany(context.Background(), 0, asOf)
	require.ErrorIs(t, err, errCompanyRequired)
}
