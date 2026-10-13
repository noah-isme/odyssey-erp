package employees

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/compensation"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type auditFake struct{ logs []shared.AuditLog }

func (a *auditFake) RecordTx(_ context.Context, _ pgx.Tx, log shared.AuditLog) error {
	a.logs = append(a.logs, log)
	return nil
}

type compFake struct {
	calls []compensation.ChangeInput
	tx    compensation.DBTX
	err   error
}

func (c *compFake) CreateInitialTx(_ context.Context, tx compensation.DBTX, companyID, employeeID int64, in compensation.ChangeInput) (compensation.Assignment, error) {
	c.tx = tx
	c.calls = append(c.calls, in)
	if c.err != nil {
		return compensation.Assignment{}, c.err
	}
	// Prove the creator runs inside the employee transaction.
	if _, err := tx.Exec(context.Background(), "INSERT INTO payroll_compensation_assignments", employeeID); err != nil {
		return compensation.Assignment{}, err
	}
	return compensation.Assignment{ID: 77, EmployeeID: employeeID}, nil
}

func q(s string) string { return regexp.QuoteMeta(s) }

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func i64(v int64) *int64 { return &v }

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *Service, *auditFake) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(db.Close)
	audit := &auditFake{}
	return db, &Service{pool: db, audit: audit}, audit
}

var employeeColNames = []string{"id", "company_id", "user_id", "employee_number", "name", "email", "department_id", "position_id", "manager_id", "hire_date", "status", "department", "position", "manager",
	"date_of_birth", "gender", "national_id", "tax_id", "marital_status", "dependents_count", "phone", "address_line1", "address_line2", "city", "province", "postal_code", "country",
	"employment_type", "contract_start_date", "contract_end_date", "probation_end_date", "termination_date", "termination_reason"}

func employeeRows(es ...Employee) *pgxmock.Rows {
	rows := pgxmock.NewRows(employeeColNames)
	for _, e := range es {
		p := e.Profile
		rows.AddRow(e.ID, e.CompanyID, e.UserID, e.EmployeeNumber, e.Name, e.Email, e.DepartmentID, e.PositionID, e.ManagerID, e.HireDate, e.Status, e.Department, e.Position, e.Manager,
			p.DateOfBirth, p.Gender, p.NationalID, p.TaxID, p.MaritalStatus, p.DependentsCount, p.Phone, p.AddressLine1, p.AddressLine2, p.City, p.Province, p.PostalCode, p.Country,
			e.EmploymentType, p.ContractStartDate, p.ContractEndDate, p.ProbationEndDate, e.TerminationDate, e.TerminationReason)
	}
	return rows
}

// requireNoPII asserts audit meta never carries profile, salary or bank values.
func requireNoPII(t *testing.T, logs []shared.AuditLog, values ...string) {
	t.Helper()
	for _, l := range logs {
		for k, v := range l.Meta {
			for _, pii := range values {
				require.NotContains(t, k, pii)
				require.NotContains(t, toString(v), pii, "audit meta %q leaks a value", k)
			}
		}
	}
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []string:
		out := ""
		for _, s := range x {
			out += s + ","
		}
		return out
	}
	return ""
}

func TestCreateRejectsIncompleteEmployeeBeforeDatabaseAccess(t *testing.T) {
	service := NewService(nil, &auditFake{})
	base := CreateInput{CompanyID: 1, EmployeeNumber: "E-1", Name: "Ada", Email: "ada@example.com", HireDate: day(2026, 1, 1)}

	for name, tc := range map[string]struct {
		in   CreateInput
		want error
	}{
		"company":         {func() CreateInput { x := base; x.CompanyID = 0; return x }(), ErrInvalidEmployee},
		"number":          {func() CreateInput { x := base; x.EmployeeNumber = " "; return x }(), ErrInvalidEmployee},
		"name":            {func() CreateInput { x := base; x.Name = " "; return x }(), ErrInvalidEmployee},
		"email":           {func() CreateInput { x := base; x.Email = " "; return x }(), ErrInvalidEmployee},
		"hire date":       {func() CreateInput { x := base; x.HireDate = time.Time{}; return x }(), ErrInvalidEmployee},
		"employment type": {func() CreateInput { x := base; x.EmploymentType = "TEMP"; return x }(), ErrInvalidEmploymentType},
		"gender":          {func() CreateInput { x := base; x.Gender = "x"; return x }(), ErrInvalidGender},
		"marital":         {func() CreateInput { x := base; x.MaritalStatus = "x"; return x }(), ErrInvalidMaritalStatus},
		"dependents":      {func() CreateInput { x := base; x.DependentsCount = -1; return x }(), ErrInvalidDependents},
		"contract dates": {func() CreateInput {
			x := base
			s, e := day(2026, 2, 1), day(2026, 1, 1)
			x.ContractStartDate, x.ContractEndDate = &s, &e
			return x
		}(), ErrInvalidContractDates},
		"compensation without creator": {func() CreateInput { x := base; x.Compensation = &compensation.ChangeInput{}; return x }(), ErrCompensationDisabled},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := service.Create(context.Background(), tc.in)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestCreateWritesEmployeeHistoryAndCompensationInOneTx(t *testing.T) {
	department, position, manager, user := int64(3), int64(4), int64(5), int64(6)
	hireDate := day(2026, 7, 1)
	dob := day(1990, 5, 6)
	base := CreateInput{
		CompanyID: 2, UserID: &user, DepartmentID: &department, PositionID: &position, ManagerID: &manager,
		EmployeeNumber: " E-007 ", Name: " Ada Lovelace ", Email: " ada@example.com ", HireDate: hireDate,
		Profile:        Profile{DateOfBirth: &dob, Gender: "female", NationalID: "3171999", TaxID: "NPWP-1", Phone: "0812", City: "Jakarta"},
		EmploymentType: "contract", Grade: "G5", ActorID: 99,
	}
	expectRefs := func(db pgxmock.PgxPoolIface) {
		db.ExpectQuery(q("SELECT 1 FROM hr_departments WHERE id=$1 AND company_id=$2")).WithArgs(department, int64(2)).WillReturnRows(pgxmock.NewRows([]string{"x"}).AddRow(1))
		db.ExpectQuery(q("SELECT 1 FROM hr_positions WHERE id=$1 AND company_id=$2")).WithArgs(position, int64(2)).WillReturnRows(pgxmock.NewRows([]string{"x"}).AddRow(1))
		db.ExpectQuery(q("SELECT 1 FROM hr_employees WHERE id=$1 AND company_id=$2")).WithArgs(manager, int64(2)).WillReturnRows(pgxmock.NewRows([]string{"x"}).AddRow(1))
	}
	expectInserts := func(db pgxmock.PgxPoolIface) {
		db.ExpectQuery(q("INSERT INTO hr_employees(")).
			WithArgs(&user, int64(2), "E-007", "Ada Lovelace", "ada@example.com", &department, &position, &manager, hireDate,
				&dob, "FEMALE", "3171999", "NPWP-1", "", 0, "0812", "", "", "Jakarta", "", "", "",
				"CONTRACT", (*time.Time)(nil), (*time.Time)(nil), (*time.Time)(nil)).
			WillReturnRows(pgxmock.NewRows([]string{"id", "status"}).AddRow(int64(8), "ACTIVE"))
		db.ExpectExec(q("INSERT INTO hr_employee_job_history(company_id,employee_id,department_id,position_id,manager_id,status,employment_type,grade,valid_from,reason,created_by)")+`\s*`+q("VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'Hired',$10)")).
			WithArgs(int64(2), int64(8), &department, &position, &manager, "ACTIVE", "CONTRACT", "G5", hireDate, i64(99)).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
	}

	tests := []struct {
		name      string
		withComp  bool
		compErr   error
		refsFail  bool
		wantErr   error
		wantAudit bool
	}{
		{name: "employee and history only", wantAudit: true},
		{name: "with initial compensation", withComp: true, wantAudit: true},
		{name: "compensation failure rolls back", withComp: true, compErr: compensation.ErrInvalidSalary, wantErr: compensation.ErrInvalidSalary},
		{name: "cross-tenant department", refsFail: true, wantErr: ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, service, audit := newMock(t)
			comp := &compFake{err: tc.compErr}
			service.SetCompensationCreator(comp)
			in := base
			if tc.withComp {
				in.Compensation = &compensation.ChangeInput{BaseSalary: "9000000.00", PTKPCode: "TK/0", BankCode: "BCA", BankAccountNumber: "123456"}
			}
			db.ExpectBegin()
			if tc.refsFail {
				db.ExpectQuery(q("SELECT 1 FROM hr_departments WHERE id=$1 AND company_id=$2")).WithArgs(department, int64(2)).WillReturnError(pgx.ErrNoRows)
				db.ExpectRollback()
			} else {
				expectRefs(db)
				expectInserts(db)
				if tc.withComp && tc.compErr == nil {
					db.ExpectExec("INSERT INTO payroll_compensation_assignments").WithArgs(int64(8)).WillReturnResult(pgxmock.NewResult("INSERT", 1))
				}
				if tc.wantErr != nil {
					db.ExpectRollback()
				} else {
					db.ExpectCommit()
					db.ExpectRollback() // deferred rollback after commit is a no-op
				}
			}

			emp, err := service.Create(context.Background(), in)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Empty(t, audit.logs)
			} else {
				require.NoError(t, err)
				require.Equal(t, int64(8), emp.ID)
				require.Equal(t, "E-007", emp.EmployeeNumber)
				require.Equal(t, "CONTRACT", emp.EmploymentType)
				require.Len(t, audit.logs, 1)
				require.Equal(t, ActionEmployeeCreated, audit.logs[0].Action)
				require.Equal(t, "8", audit.logs[0].EntityID)
				require.Equal(t, tc.withComp, audit.logs[0].Meta["compensation"])
				require.Contains(t, audit.logs[0].Meta["fields"], "national_id")
				requireNoPII(t, audit.logs, "3171999", "NPWP-1", "Jakarta", "9000000", "123456", "1990")
			}
			if tc.withComp {
				require.Len(t, comp.calls, 1)
				require.NotNil(t, comp.tx, "compensation must receive the employee transaction")
				require.Equal(t, hireDate, comp.calls[0].EffectiveFrom)
				require.Equal(t, "Hired", comp.calls[0].Reason)
				require.Equal(t, int64(99), comp.calls[0].ActorID)
			} else {
				require.Empty(t, comp.calls)
			}
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}

func TestUpdateChangesProfileOnlyAndAuditsFieldNames(t *testing.T) {
	user := int64(6)
	dept := int64(3)
	hireDate := day(2026, 7, 1)
	dob := day(1990, 5, 6)
	stored := Employee{ID: 8, CompanyID: 2, UserID: &user, EmployeeNumber: "E-007", Name: "Ada", Email: "ada@example.com", DepartmentID: &dept, HireDate: hireDate, Status: "ACTIVE",
		Profile: Profile{DateOfBirth: &dob, NationalID: "3171999", TaxID: "NPWP-1", City: "Jakarta", Phone: "0812"}, EmploymentType: "PERMANENT"}

	tests := []struct {
		name             string
		includeSensitive bool
		wantNationalID   string
		wantCity         string
		wantFields       []string
	}{
		{name: "sensitive kept without permission", wantNationalID: "3171999", wantCity: "Jakarta", wantFields: []string{"name", "phone"}},
		{name: "sensitive written with permission", includeSensitive: true, wantNationalID: "999", wantCity: "Bandung", wantFields: []string{"name", "date_of_birth", "national_id", "tax_id", "phone", "city"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, service, audit := newMock(t)
			db.ExpectBegin()
			db.ExpectQuery(q("WHERE e.id=$1 AND e.company_id=$2 FOR UPDATE OF e")).WithArgs(int64(8), int64(2)).WillReturnRows(employeeRows(stored))
			var wantDOB *time.Time
			wantTax := "NPWP-1"
			if tc.includeSensitive {
				wantTax = ""
			} else {
				wantDOB = &dob
			}
			db.ExpectExec(q("UPDATE hr_employees SET user_id=$1, employee_number=$2, name=$3, email=$4, hire_date=$5,")).
				WithArgs(&user, "E-007", "Ada Updated", "ada@example.com", hireDate,
					wantDOB, "", tc.wantNationalID, wantTax, "", 0, "0813", "", "", tc.wantCity, "", "", "",
					(*time.Time)(nil), (*time.Time)(nil), (*time.Time)(nil), int64(8), int64(2)).
				WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			db.ExpectCommit()
			db.ExpectRollback()

			emp, err := service.Update(context.Background(), 2, 8, UpdateInput{
				UserID: &user, EmployeeNumber: "E-007", Name: " Ada Updated ", Email: "ada@example.com", HireDate: hireDate,
				Profile:          Profile{NationalID: "999", City: "Bandung", Phone: "0813"},
				IncludeSensitive: tc.includeSensitive, ActorID: 99,
			})
			require.NoError(t, err)
			require.Equal(t, "Ada Updated", emp.Name)
			require.Equal(t, tc.wantNationalID, emp.NationalID)
			require.Equal(t, &dept, emp.DepartmentID, "Update must not change job fields")
			require.Len(t, audit.logs, 1)
			require.Equal(t, ActionEmployeeProfileUpdated, audit.logs[0].Action)
			require.Equal(t, tc.wantFields, audit.logs[0].Meta["fields"])
			requireNoPII(t, audit.logs, "3171999", "999", "Jakarta", "Bandung", "0813", "NPWP")
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}

func TestUpdateOtherCompanyNotFound(t *testing.T) {
	db, service, audit := newMock(t)
	db.ExpectBegin()
	db.ExpectQuery(q("WHERE e.id=$1 AND e.company_id=$2 FOR UPDATE OF e")).WithArgs(int64(5), int64(1)).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()
	_, err := service.Update(context.Background(), 1, 5, UpdateInput{EmployeeNumber: "E", Name: "N", Email: "e@x", HireDate: day(2026, 1, 1)})
	require.EqualError(t, err, "hr: not found: employee")
	require.ErrorIs(t, err, ErrNotFound)
	require.Empty(t, audit.logs)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestMaskSensitive(t *testing.T) {
	dob := day(1990, 5, 6)
	start := day(2026, 1, 1)
	e := Employee{Name: "Ada", Email: "ada@example.com", Profile: Profile{
		DateOfBirth: &dob, Gender: "FEMALE", NationalID: "3171", TaxID: "NPWP", MaritalStatus: "SINGLE", DependentsCount: 2, Phone: "0812",
		AddressLine1: "Jl. A", AddressLine2: "No 1", City: "Jakarta", Province: "DKI", PostalCode: "10110", Country: "ID", ContractStartDate: &start,
	}}
	MaskSensitive(&e)
	require.Nil(t, e.DateOfBirth)
	for name, v := range map[string]string{"national_id": e.NationalID, "tax_id": e.TaxID, "address_line1": e.AddressLine1, "address_line2": e.AddressLine2,
		"city": e.City, "province": e.Province, "postal_code": e.PostalCode, "country": e.Country} {
		require.Empty(t, v, name)
	}
	// Non-sensitive fields stay.
	require.Equal(t, "Ada", e.Name)
	require.Equal(t, "FEMALE", e.Gender)
	require.Equal(t, "0812", e.Phone)
	require.Equal(t, 2, e.DependentsCount)
	require.Equal(t, &start, e.ContractStartDate)
	MaskSensitive(nil) // must not panic
}

func TestDepartmentsAndPositionsCRUD(t *testing.T) {
	db, service, _ := newMock(t)

	now := time.Now()
	db.ExpectQuery("INSERT INTO hr_departments").
		WithArgs(int64(1), "HR", "Human Resources").
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "code", "name", "created_at"}).
			AddRow(int64(10), int64(1), "HR", "Human Resources", now))
	dept, err := service.CreateDepartment(context.Background(), 1, "HR", "Human Resources")
	require.NoError(t, err)
	require.Equal(t, int64(10), dept.ID)

	db.ExpectQuery("SELECT d.id, d.company_id, d.code, d.name, COUNT").
		WithArgs(int64(1)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "code", "name", "count", "created_at"}).
			AddRow(int64(10), int64(1), "HR", "Human Resources", 5, now))
	depts, err := service.ListDepartments(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, depts, 1)
	require.Equal(t, 5, depts[0].EmployeeCount)

	db.ExpectQuery("INSERT INTO hr_positions").
		WithArgs(int64(1), "DEV", "Developer").
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "code", "name", "created_at"}).
			AddRow(int64(20), int64(1), "DEV", "Developer", now))
	pos, err := service.CreatePosition(context.Background(), 1, "DEV", "Developer")
	require.NoError(t, err)
	require.Equal(t, int64(20), pos.ID)

	db.ExpectQuery("SELECT p.id, p.company_id, p.code, p.name, COUNT").
		WithArgs(int64(1)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "code", "name", "count", "created_at"}).
			AddRow(int64(20), int64(1), "DEV", "Developer", 3, now))
	positions, err := service.ListPositions(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	require.Equal(t, 3, positions[0].EmployeeCount)

	require.NoError(t, db.ExpectationsWereMet())
}
