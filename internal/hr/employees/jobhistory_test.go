package employees

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

var jobHistoryColNames = []string{"id", "company_id", "employee_id", "department_id", "position_id", "manager_id", "employee_name", "department", "position", "manager",
	"status", "employment_type", "grade", "valid_from", "valid_to", "reason", "created_by", "created_at"}

func jobRows(js ...JobHistory) *pgxmock.Rows {
	rows := pgxmock.NewRows(jobHistoryColNames)
	for _, j := range js {
		rows.AddRow(j.ID, j.CompanyID, j.EmployeeID, j.DepartmentID, j.PositionID, j.ManagerID, j.EmployeeName, j.Department, j.Position, j.Manager,
			j.Status, j.EmploymentType, j.Grade, j.ValidFrom, j.ValidTo, j.Reason, j.CreatedBy, j.CreatedAt)
	}
	return rows
}

const asOfSQL = `jh.valid_from<=$3 AND (jh.valid_to IS NULL OR jh.valid_to>=$3)`

func TestRecordJobChange(t *testing.T) {
	const company, employee = int64(2), int64(8)
	oldDept, newDept, manager := int64(3), int64(30), int64(5)
	prevFrom := day(2025, 1, 1)
	prev := JobHistory{ID: 40, CompanyID: company, EmployeeID: employee, DepartmentID: &oldDept, ManagerID: &manager, Status: "ACTIVE", EmploymentType: "PERMANENT", Grade: "G1", ValidFrom: prevFrom}
	newFrom := day(2026, 3, 1)

	expectLockEmployee := func(db pgxmock.PgxPoolIface) {
		db.ExpectQuery(q("SELECT termination_date FROM hr_employees WHERE id=$1 AND company_id=$2 FOR UPDATE")).WithArgs(employee, company).
			WillReturnRows(pgxmock.NewRows([]string{"termination_date"}).AddRow((*time.Time)(nil)))
	}
	expectLatest := func(db pgxmock.PgxPoolIface) {
		db.ExpectQuery(q("JOIN hr_employees e ON e.id=jh.employee_id AND e.company_id=$1")+`(.|\n)*`+q("WHERE jh.employee_id=$2 ORDER BY jh.valid_from DESC LIMIT 1 FOR UPDATE OF jh")).
			WithArgs(company, employee).WillReturnRows(jobRows(prev))
	}

	tests := []struct {
		name      string
		in        JobChangeInput
		setup     func(db pgxmock.PgxPoolIface)
		wantErr   string
		wantAudit []string
	}{
		{
			name: "closes previous and inserts new",
			in:   JobChangeInput{DepartmentID: &newDept, ManagerID: &manager, Status: "active", EmploymentType: "permanent", Grade: "G2", ValidFrom: newFrom, Reason: "Promotion", ActorID: 99},
			setup: func(db pgxmock.PgxPoolIface) {
				db.ExpectBegin()
				expectLockEmployee(db)
				db.ExpectQuery(q("SELECT 1 FROM hr_departments WHERE id=$1 AND company_id=$2")).WithArgs(newDept, company).WillReturnRows(pgxmock.NewRows([]string{"x"}).AddRow(1))
				db.ExpectQuery(q("SELECT 1 FROM hr_employees WHERE id=$1 AND company_id=$2")).WithArgs(manager, company).WillReturnRows(pgxmock.NewRows([]string{"x"}).AddRow(1))
				expectLatest(db)
				db.ExpectExec(q("UPDATE hr_employee_job_history SET valid_to=$2 WHERE id=$1")).WithArgs(int64(40), day(2026, 2, 28)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				db.ExpectQuery(q("INSERT INTO hr_employee_job_history(")).
					WithArgs(company, employee, &newDept, (*int64)(nil), &manager, "ACTIVE", "PERMANENT", "G2", newFrom, "Promotion", i64(99)).
					WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow(int64(41), time.Now()))
				db.ExpectExec(q("UPDATE hr_employees SET department_id=$1, position_id=$2, manager_id=$3, status=$4, employment_type=$5")).
					WithArgs(&newDept, (*int64)(nil), &manager, "ACTIVE", "PERMANENT", employee, company).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				db.ExpectCommit()
				db.ExpectRollback()
			},
			wantAudit: []string{"department_id", "grade"},
		},
		{
			name: "rejects valid_from before previous",
			in:   JobChangeInput{Status: "ACTIVE", EmploymentType: "PERMANENT", ValidFrom: day(2024, 12, 31)},
			setup: func(db pgxmock.PgxPoolIface) {
				db.ExpectBegin()
				expectLockEmployee(db)
				expectLatest(db)
				db.ExpectRollback()
			},
			wantErr: "hr: job change must start after 2025-01-01 (the latest job record)",
		},
		{
			name: "rejects cross-tenant department",
			in:   JobChangeInput{DepartmentID: &newDept, ValidFrom: newFrom},
			setup: func(db pgxmock.PgxPoolIface) {
				db.ExpectBegin()
				expectLockEmployee(db)
				db.ExpectQuery(q("SELECT 1 FROM hr_departments WHERE id=$1 AND company_id=$2")).WithArgs(newDept, company).WillReturnError(pgx.ErrNoRows)
				db.ExpectRollback()
			},
			wantErr: "hr: not found: department",
		},
		{
			name: "rejects cross-tenant employee",
			in:   JobChangeInput{ValidFrom: newFrom},
			setup: func(db pgxmock.PgxPoolIface) {
				db.ExpectBegin()
				db.ExpectQuery(q("SELECT termination_date FROM hr_employees WHERE id=$1 AND company_id=$2 FOR UPDATE")).WithArgs(employee, company).WillReturnError(pgx.ErrNoRows)
				db.ExpectRollback()
			},
			wantErr: "hr: not found: employee",
		},
		{name: "rejects self manager without db", in: JobChangeInput{ManagerID: func() *int64 { v := employee; return &v }(), ValidFrom: newFrom}, setup: func(pgxmock.PgxPoolIface) {}, wantErr: "hr: employee cannot manage themselves"},
		{name: "rejects invalid status without db", in: JobChangeInput{Status: "GONE", ValidFrom: newFrom}, setup: func(pgxmock.PgxPoolIface) {}, wantErr: "hr: invalid employee status"},
		{name: "rejects missing date without db", in: JobChangeInput{}, setup: func(pgxmock.PgxPoolIface) {}, wantErr: "hr: invalid job change"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, service, audit := newMock(t)
			tc.setup(db)
			got, err := service.RecordJobChange(context.Background(), company, employee, tc.in)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				require.Empty(t, audit.logs)
			} else {
				require.NoError(t, err)
				require.Equal(t, int64(41), got.ID)
				require.Nil(t, got.ValidTo)
				require.Len(t, audit.logs, 1)
				require.Equal(t, ActionEmployeeJobChanged, audit.logs[0].Action)
				require.Equal(t, tc.wantAudit, audit.logs[0].Meta["fields"])
				require.Equal(t, "2026-03-01", audit.logs[0].Meta["valid_from"])
				require.NotContains(t, audit.logs[0].Meta, "reason")
			}
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}

func TestRecordJobChangeFutureDatedDoesNotMirror(t *testing.T) {
	db, service, audit := newMock(t)
	future := dateOnly(time.Now()).AddDate(0, 1, 0)
	db.ExpectBegin()
	db.ExpectQuery(q("SELECT termination_date FROM hr_employees")).WithArgs(int64(8), int64(2)).WillReturnRows(pgxmock.NewRows([]string{"termination_date"}).AddRow((*time.Time)(nil)))
	db.ExpectQuery(q("FOR UPDATE OF jh")).WithArgs(int64(2), int64(8)).WillReturnRows(jobRows(JobHistory{ID: 40, CompanyID: 2, EmployeeID: 8, Status: "ACTIVE", EmploymentType: "PERMANENT", ValidFrom: day(2025, 1, 1)}))
	db.ExpectExec(q("UPDATE hr_employee_job_history SET valid_to=$2")).WithArgs(int64(40), future.AddDate(0, 0, -1)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectQuery(q("INSERT INTO hr_employee_job_history(")).WithArgs(int64(2), int64(8), (*int64)(nil), (*int64)(nil), (*int64)(nil), "INACTIVE", "PERMANENT", "", future, "", (*int64)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow(int64(41), time.Now()))
	db.ExpectCommit()
	db.ExpectRollback()
	_, err := service.RecordJobChange(context.Background(), 2, 8, JobChangeInput{Status: "INACTIVE", ValidFrom: future})
	require.NoError(t, err)
	require.Equal(t, []string{"status"}, audit.logs[0].Meta["fields"])
	require.NoError(t, db.ExpectationsWereMet())
}

func TestAssignmentAsOf(t *testing.T) {
	asOf := time.Date(2026, 3, 15, 13, 45, 0, 0, time.UTC)
	dept := int64(3)
	tests := []struct {
		name    string
		rows    *pgxmock.Rows
		err     error
		wantErr error
	}{
		{name: "effective row", rows: jobRows(JobHistory{ID: 41, CompanyID: 2, EmployeeID: 8, DepartmentID: &dept, Department: "Finance", Status: "ACTIVE", EmploymentType: "PERMANENT", ValidFrom: day(2026, 3, 1)})},
		{name: "none effective or other company", err: pgx.ErrNoRows, wantErr: ErrNoJobAssignment},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, service, _ := newMock(t)
			exp := db.ExpectQuery(q("JOIN hr_employees e ON e.id=jh.employee_id AND e.company_id=$1")+`(.|\n)*`+q("WHERE jh.employee_id=$2 AND "+asOfSQL+" ORDER BY jh.valid_from DESC LIMIT 1")).
				WithArgs(int64(2), int64(8), day(2026, 3, 15))
			if tc.err != nil {
				exp.WillReturnError(tc.err)
			} else {
				exp.WillReturnRows(tc.rows)
			}
			got, err := service.AssignmentAsOf(context.Background(), 2, 8, asOf)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.ErrorIs(t, err, ErrNotFound)
			} else {
				require.NoError(t, err)
				require.Equal(t, int64(41), got.ID)
				require.Equal(t, "Finance", got.Department)
			}
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
	_, err := (&Service{}).AssignmentAsOf(context.Background(), 0, 8, asOf)
	require.EqualError(t, err, "hr: company required")
}

func TestDirectReportsAsOf(t *testing.T) {
	db, service, _ := newMock(t)
	mgr := int64(5)
	db.ExpectQuery(q("JOIN hr_employees e ON e.id=jh.employee_id AND e.company_id=$1")+`(.|\n)*`+q("WHERE jh.manager_id=$2 AND jh.status='ACTIVE' AND "+asOfSQL+" ORDER BY e.name")).
		WithArgs(int64(2), mgr, day(2026, 3, 15)).
		WillReturnRows(jobRows(
			JobHistory{ID: 1, CompanyID: 2, EmployeeID: 8, ManagerID: &mgr, EmployeeName: "Ada", Status: "ACTIVE", EmploymentType: "PERMANENT", ValidFrom: day(2026, 1, 1)},
			JobHistory{ID: 2, CompanyID: 2, EmployeeID: 9, ManagerID: &mgr, EmployeeName: "Bob", Status: "ACTIVE", EmploymentType: "CONTRACT", ValidFrom: day(2025, 6, 1)},
		))
	got, err := service.DirectReportsAsOf(context.Background(), 2, mgr, day(2026, 3, 15))
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, []int64{8, 9}, []int64{got[0].EmployeeID, got[1].EmployeeID})
	require.NoError(t, db.ExpectationsWereMet())

	_, err = service.DirectReportsAsOf(context.Background(), 0, mgr, day(2026, 3, 15))
	require.EqualError(t, err, "hr: company required")
}

func chainRows(pairs ...[2]int64) *pgxmock.Rows {
	rows := pgxmock.NewRows([]string{"depth", "manager_id"})
	for _, p := range pairs {
		rows.AddRow(int(p[0]), p[1])
	}
	return rows
}

func expectChain(db pgxmock.PgxPoolIface, employee int64, rows *pgxmock.Rows) {
	db.ExpectQuery(q("WITH RECURSIVE chain")+`(.|\n)*`+q("JOIN hr_employees e ON e.id=jh.employee_id AND e.company_id=$1")+`(.|\n)*`+q("WHERE c.depth < $4 AND NOT (jh.employee_id = ANY(c.path))")).
		WithArgs(int64(2), employee, day(2026, 3, 15), MaxManagerChainDepth).WillReturnRows(rows)
}

func TestManagerChainAsOf(t *testing.T) {
	tests := []struct {
		name string
		rows *pgxmock.Rows
		want []int64
	}{
		{name: "nearest first", rows: chainRows([2]int64{1, 5}, [2]int64{2, 3}, [2]int64{3, 1}), want: []int64{5, 3, 1}},
		{name: "no manager", rows: chainRows(), want: nil},
		{name: "cycle stops", rows: chainRows([2]int64{1, 5}, [2]int64{2, 8}, [2]int64{3, 5}), want: []int64{5}},
		{name: "depth capped at 10", rows: func() *pgxmock.Rows {
			r := chainRows()
			for d := int64(1); d <= 12; d++ {
				r.AddRow(int(d), 100+d)
			}
			return r
		}(), want: []int64{101, 102, 103, 104, 105, 106, 107, 108, 109, 110}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, service, _ := newMock(t)
			expectChain(db, 8, tc.rows)
			got, err := service.ManagerChainAsOf(context.Background(), 2, 8, day(2026, 3, 15))
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}

func TestIsManagerOfAsOf(t *testing.T) {
	const user = int64(70)
	tests := []struct {
		name     string
		resolved *int64 // manager user's employee id in the company; nil = none
		chain    *pgxmock.Rows
		want     bool
	}{
		{name: "direct manager", resolved: i64(5), chain: chainRows([2]int64{1, 5}, [2]int64{2, 3}), want: true},
		{name: "indirect manager", resolved: i64(3), chain: chainRows([2]int64{1, 5}, [2]int64{2, 3}), want: true},
		{name: "not in chain", resolved: i64(4), chain: chainRows([2]int64{1, 5}, [2]int64{2, 3}), want: false},
		{name: "user without employee in company", resolved: nil, want: false},
		{name: "self is not own manager", resolved: i64(8), want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, service, _ := newMock(t)
			exp := db.ExpectQuery(q("SELECT id FROM hr_employees WHERE user_id=$1 AND company_id=$2")).WithArgs(user, int64(2))
			if tc.resolved == nil {
				exp.WillReturnError(pgx.ErrNoRows)
			} else {
				exp.WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(*tc.resolved))
			}
			if tc.chain != nil {
				expectChain(db, 8, tc.chain)
			}
			got, err := service.IsManagerOfAsOf(context.Background(), 2, user, 8, day(2026, 3, 15))
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.NoError(t, db.ExpectationsWereMet())
		})
	}
}

func TestApplyDueJobChanges(t *testing.T) {
	asOf := time.Date(2026, 5, 4, 15, 30, 0, 0, time.UTC)
	asOfDay := day(2026, 5, 4)
	oldDept, newDept, newPos, oldMgr, newMgr := int64(3), int64(30), int64(7), int64(5), int64(6)

	candidates := func(db pgxmock.PgxPoolIface, ids ...int64) {
		rows := pgxmock.NewRows([]string{"id", "company_id"})
		for _, id := range ids {
			rows.AddRow(id, int64(2))
		}
		db.ExpectQuery(q("SELECT DISTINCT e.id, e.company_id")+`(.|\n)*`+q("e.termination_date IS NULL AND jh.valid_from<=$1 AND (jh.valid_to IS NULL OR jh.valid_to>=$1)")).
			WithArgs(asOfDay, int64(0)).WillReturnRows(rows)
	}
	lock := func(db pgxmock.PgxPoolIface, employee int64, cur, next JobHistory) {
		db.ExpectBegin()
		db.ExpectQuery(q("SELECT id FROM hr_employees WHERE id=$1 AND company_id=$2 AND termination_date IS NULL FOR UPDATE")).WithArgs(employee, int64(2)).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(employee))
		db.ExpectQuery(q("ORDER BY jh.valid_from DESC LIMIT 1")).WithArgs(employee, int64(2), asOfDay).WillReturnRows(
			pgxmock.NewRows([]string{"jh_id", "valid_from", "c_dept", "c_pos", "c_mgr", "c_status", "c_type", "n_dept", "n_pos", "n_mgr", "n_status", "n_type", "created_by"}).
				AddRow(int64(41), day(2026, 5, 1), cur.DepartmentID, cur.PositionID, cur.ManagerID, cur.Status, cur.EmploymentType,
					next.DepartmentID, next.PositionID, next.ManagerID, next.Status, next.EmploymentType, next.CreatedBy))
	}
	author := int64(77)
	cur := JobHistory{DepartmentID: &oldDept, ManagerID: &oldMgr, Status: "ACTIVE", EmploymentType: "PERMANENT"}
	due := JobHistory{DepartmentID: &newDept, PositionID: &newPos, ManagerID: &newMgr, Status: "ACTIVE", EmploymentType: "PERMANENT", CreatedBy: &author}

	t.Run("applies a due change and audits it", func(t *testing.T) {
		db, service, audit := newMock(t)
		candidates(db, 8)
		lock(db, 8, cur, due)
		db.ExpectExec(q("UPDATE hr_employees SET department_id=$1, position_id=$2, manager_id=$3, status=$4, employment_type=$5, updated_at=NOW() WHERE id=$6 AND company_id=$7")).
			WithArgs(&newDept, &newPos, &newMgr, "ACTIVE", "PERMANENT", int64(8), int64(2)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()
		db.ExpectRollback()

		n, err := service.ApplyDueJobChanges(context.Background(), asOf)
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.Len(t, audit.logs, 1)
		require.Equal(t, ActionEmployeeJobChanged, audit.logs[0].Action)
		require.Equal(t, author, audit.logs[0].ActorID, "attributed to the user who recorded the change; audit_logs.actor_id is NOT NULL")
		require.Equal(t, "8", audit.logs[0].EntityID)
		require.Equal(t, []string{"department_id", "position_id", "manager_id"}, audit.logs[0].Meta["fields"])
		require.Equal(t, "scheduled", audit.logs[0].Meta["source"])
		require.Equal(t, int64(41), audit.logs[0].Meta["job_history_id"])
		require.Equal(t, "2026-05-01", audit.logs[0].Meta["valid_from"])
		require.NoError(t, db.ExpectationsWereMet())
	})

	t.Run("future-dated change is not selected and nothing is written", func(t *testing.T) {
		db, service, audit := newMock(t)
		candidates(db) // the future record is not effective on asOf, so no employee differs
		n, err := service.ApplyDueJobChanges(context.Background(), asOf)
		require.NoError(t, err)
		require.Zero(t, n)
		require.Empty(t, audit.logs)
		require.NoError(t, db.ExpectationsWereMet())
	})

	t.Run("second run is a no-op", func(t *testing.T) {
		db, service, audit := newMock(t)
		// Run 1 applies; run 2 finds nothing that differs any more.
		candidates(db, 8)
		lock(db, 8, cur, due)
		db.ExpectExec(q("UPDATE hr_employees SET")).WithArgs(&newDept, &newPos, &newMgr, "ACTIVE", "PERMANENT", int64(8), int64(2)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()
		db.ExpectRollback()
		candidates(db)

		n, err := service.ApplyDueJobChanges(context.Background(), asOf)
		require.NoError(t, err)
		require.Equal(t, 1, n)
		n, err = service.ApplyDueJobChanges(context.Background(), asOf)
		require.NoError(t, err)
		require.Zero(t, n)
		require.Len(t, audit.logs, 1)
		require.NoError(t, db.ExpectationsWereMet())
	})

	t.Run("row already in sync under the lock is skipped without write or audit", func(t *testing.T) {
		db, service, audit := newMock(t)
		candidates(db, 8)
		lock(db, 8, due, due) // a concurrent worker applied it first
		db.ExpectCommit()
		db.ExpectRollback()
		n, err := service.ApplyDueJobChanges(context.Background(), asOf)
		require.NoError(t, err)
		require.Zero(t, n)
		require.Empty(t, audit.logs)
		require.NoError(t, db.ExpectationsWereMet())
	})

	t.Run("one failing employee does not stop the others", func(t *testing.T) {
		db, service, audit := newMock(t)
		candidates(db, 8, 9)
		lock(db, 8, cur, due)
		db.ExpectExec(q("UPDATE hr_employees SET")).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), int64(8), int64(2)).WillReturnError(errors.New("boom"))
		db.ExpectRollback()
		lock(db, 9, cur, due)
		db.ExpectExec(q("UPDATE hr_employees SET")).WithArgs(&newDept, &newPos, &newMgr, "ACTIVE", "PERMANENT", int64(9), int64(2)).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()
		db.ExpectRollback()
		n, err := service.ApplyDueJobChanges(context.Background(), asOf)
		require.Equal(t, 1, n)
		require.ErrorContains(t, err, "employee 8 (company 2)")
		require.Len(t, audit.logs, 1)
		require.Equal(t, "9", audit.logs[0].EntityID)
		require.NoError(t, db.ExpectationsWereMet())
	})

	t.Run("requires audit", func(t *testing.T) {
		db, service, _ := newMock(t)
		service.audit = nil
		_, err := service.ApplyDueJobChanges(context.Background(), asOf)
		require.ErrorIs(t, err, errAuditNotConfigured)
		require.NoError(t, db.ExpectationsWereMet())
	})
}
