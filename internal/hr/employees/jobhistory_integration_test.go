package employees

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// TestJobHistoryAgainstLocalDatabase exercises the job-history SQL against a
// real schema, which pgxmock cannot validate (column constraints, audit_logs
// actor NOT NULL + FK, locking clauses). The catch-up is scoped to the test
// company, so seeded data in the database is never touched. It runs only with a localhost PG_DSN
// and removes everything it creates.
func TestJobHistoryAgainstLocalDatabase(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN is required")
	}
	if u, err := url.Parse(dsn); err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		t.Skip("PG_DSN is not a localhost URL; refusing to run")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(ctx))

	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	var userID, companyID int64
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users(email, password_hash) VALUES($1,'x') RETURNING id`, "jh-it-"+suffix+"@example.test").Scan(&userID))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO companies(code, name) VALUES($1,'JH IT') RETURNING id`, "JHIT"+suffix).Scan(&companyID))
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM audit_logs WHERE actor_id=$1`, userID)
		_, _ = pool.Exec(ctx, `DELETE FROM companies WHERE id=$1`, companyID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
	})
	var deptA, deptB int64
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO hr_departments(company_id,code,name) VALUES($1,'A','A') RETURNING id`, companyID).Scan(&deptA))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO hr_departments(company_id,code,name) VALUES($1,'B','B') RETURNING id`, companyID).Scan(&deptB))

	svc := NewService(pool, shared.NewAuditLogger(pool))
	today := businessToday()
	create := func(num string, hire time.Time) Employee {
		e, err := svc.Create(ctx, CreateInput{CompanyID: companyID, DepartmentID: &deptA, EmployeeNumber: num, Name: "Emp " + num, Email: num + "@example.test", HireDate: hire, ActorID: userID})
		require.NoError(t, err)
		return e
	}
	deptOf := func(id int64) (dept *int64, status string, term *time.Time) {
		require.NoError(t, pool.QueryRow(ctx, `SELECT department_id,status,termination_date FROM hr_employees WHERE id=$1`, id).Scan(&dept, &status, &term))
		return
	}
	auditCount := func(id int64, action string) (n int) {
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE entity='hr_employee' AND entity_id=$1 AND action=$2`, strconv.FormatInt(id, 10), action).Scan(&n))
		return
	}

	t.Run("future-dated change is applied once on its start date", func(t *testing.T) {
		e := create("E1", today.AddDate(0, 0, -30))
		start := today.AddDate(0, 0, 3)
		_, err := svc.RecordJobChange(ctx, companyID, e.ID, JobChangeInput{DepartmentID: &deptB, ValidFrom: start, Reason: "move", ActorID: userID})
		require.NoError(t, err)
		d, _, _ := deptOf(e.ID)
		require.Equal(t, deptA, *d, "future change must not be mirrored yet")

		n, err := svc.ApplyDueJobChangesForCompany(ctx, companyID, today)
		require.NoError(t, err)
		require.Zero(t, n)
		n, err = svc.ApplyDueJobChangesForCompany(ctx, companyID, start)
		require.NoError(t, err)
		require.Equal(t, 1, n)
		d, _, _ = deptOf(e.ID)
		require.Equal(t, deptB, *d)
		n, err = svc.ApplyDueJobChangesForCompany(ctx, companyID, start)
		require.NoError(t, err)
		require.Zero(t, n, "second run is a no-op")
		require.Equal(t, 2, auditCount(e.ID, ActionEmployeeJobChanged), "recorded + applied")
	})

	t.Run("same-day corrections rewrite the open record", func(t *testing.T) {
		e := create("E2", today) // hired today: the Hired record starts today
		_, err := svc.RecordJobChange(ctx, companyID, e.ID, JobChangeInput{DepartmentID: &deptB, ValidFrom: today, ActorID: userID})
		require.NoError(t, err)
		_, err = svc.RecordJobChange(ctx, companyID, e.ID, JobChangeInput{DepartmentID: &deptA, ValidFrom: today, ActorID: userID})
		require.NoError(t, err)
		hist, err := svc.ListJobHistory(ctx, companyID, e.ID)
		require.NoError(t, err)
		require.Len(t, hist, 1)
		require.Equal(t, deptA, *hist[0].DepartmentID)
		d, _, _ := deptOf(e.ID)
		require.Equal(t, deptA, *d)
	})

	t.Run("pre-boarded hire can be corrected and a failed combined edit leaves no partial profile change", func(t *testing.T) {
		hire := today.AddDate(0, 0, 10)
		e := create("E3", hire)
		_, err := svc.RecordJobChange(ctx, companyID, e.ID, JobChangeInput{DepartmentID: &deptB, ValidFrom: hire, ActorID: userID})
		require.NoError(t, err)
		hist, err := svc.ListJobHistory(ctx, companyID, e.ID)
		require.NoError(t, err)
		require.Len(t, hist, 1)

		in := UpdateInput{EmployeeNumber: "E3", Name: "Renamed", Email: "E3@example.test", HireDate: hire, ActorID: userID}
		_, err = svc.UpdateWithJobChange(ctx, companyID, e.ID, in, &JobChangeInput{DepartmentID: &deptA, ValidFrom: hire.AddDate(0, 0, -1), ActorID: userID})
		require.Error(t, err)
		var name string
		require.NoError(t, pool.QueryRow(ctx, `SELECT name FROM hr_employees WHERE id=$1`, e.ID).Scan(&name))
		require.Equal(t, "Emp E3", name, "profile edit must roll back with the rejected job change")
	})

	t.Run("rehire clears the termination so catch-up and payroll include the employee again", func(t *testing.T) {
		e := create("E4", today.AddDate(0, -2, 0))
		_, err := pool.Exec(ctx, `UPDATE hr_employees SET status='INACTIVE', termination_date=$2, termination_reason='left' WHERE id=$1`, e.ID, today.AddDate(0, 0, -5))
		require.NoError(t, err)
		_, err = svc.RecordJobChange(ctx, companyID, e.ID, JobChangeInput{DepartmentID: &deptA, Status: "ACTIVE", ValidFrom: today, Reason: "Rehired", ActorID: userID})
		require.NoError(t, err)
		_, status, term := deptOf(e.ID)
		require.Equal(t, "ACTIVE", status)
		require.Nil(t, term)
		require.Equal(t, 1, auditCount(e.ID, ActionEmployeeRehired))
	})
}
