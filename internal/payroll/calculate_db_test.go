package payroll

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestCalculateRunsAgainstLocalDatabase runs Repository.Calculate on a real
// connection. pgxmock cannot detect a statement issued while a result set is
// still open ("conn busy"), which is what Calculate used to do. It creates and
// removes its own company, employee, compensation and period, needs the
// reviewed rule versions from migration 000046, and runs only with a localhost
// PG_DSN.
func TestCalculateRunsAgainstLocalDatabase(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN is required")
	}
	if u, err := url.Parse(dsn); err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		t.Skip("PG_DSN is not a localhost URL; refusing to run")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close) // registered first, so it runs after the data cleanup below
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	var bpjsVersion int64
	if err := pool.QueryRow(ctx, `SELECT id FROM payroll_rule_versions WHERE rule_type='BPJS' AND reviewed_at IS NOT NULL AND effective_from <= DATE '2026-09-25' AND (effective_to IS NULL OR effective_to >= DATE '2026-09-25') ORDER BY effective_from DESC LIMIT 1`).Scan(&bpjsVersion); err != nil {
		t.Skip("no reviewed BPJS rule version covering the test pay date")
	}
	var taxVersion int64
	if err := pool.QueryRow(ctx, `SELECT id FROM payroll_rule_versions WHERE rule_type='TAX' AND reviewed_at IS NOT NULL AND effective_from <= DATE '2026-09-25' AND (effective_to IS NULL OR effective_to >= DATE '2026-09-25') ORDER BY effective_from DESC LIMIT 1`).Scan(&taxVersion); err != nil {
		t.Skip("no reviewed TAX rule version covering the test pay date")
	}

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	var userID, companyID, employeeID, periodID, runID int64
	must(pool.QueryRow(ctx, `INSERT INTO users(email, password_hash) VALUES($1,'x') RETURNING id`, "pay-it-"+suffix+"@example.test").Scan(&userID))
	must(pool.QueryRow(ctx, `INSERT INTO companies(code, name) VALUES($1,'PAY IT') RETURNING id`, "PAYIT"+suffix).Scan(&companyID))
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM payroll_run_lines WHERE run_id IN (SELECT id FROM payroll_runs WHERE company_id=$1)`, companyID)
		_, _ = pool.Exec(ctx, `DELETE FROM payroll_runs WHERE company_id=$1`, companyID)
		_, _ = pool.Exec(ctx, `DELETE FROM companies WHERE id=$1`, companyID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
	})
	must(pool.QueryRow(ctx, `INSERT INTO hr_employees(company_id, employee_number, name, email, hire_date) VALUES($1,'P1','Pay One','p1@example.test', DATE '2026-01-01') RETURNING id`, companyID).Scan(&employeeID))
	_, err = pool.Exec(ctx, `INSERT INTO hr_employees(company_id, employee_number, name, email, hire_date) VALUES($1,'P2','Pay Two','p2@example.test', DATE '2026-01-01')`, companyID)
	must(err)
	_, err = pool.Exec(ctx, `INSERT INTO payroll_compensation_assignments(employee_id, base_salary, ptkp_code, effective_from) SELECT id, 8500000, 'TK/0', DATE '2026-01-01' FROM hr_employees WHERE company_id=$1`, companyID)
	must(err)
	_, err = pool.Exec(ctx, `INSERT INTO payroll_company_policies(rule_version_id, company_id, effective_from) VALUES($1,$2,DATE '2026-01-01')`, bpjsVersion, companyID)
	must(err)
	must(pool.QueryRow(ctx, `INSERT INTO payroll_periods(company_id, code, starts_on, ends_on, pay_date) VALUES($1,'2026-09',DATE '2026-09-01',DATE '2026-09-30',DATE '2026-09-25') RETURNING id`, companyID).Scan(&periodID))

	repo := NewRepository(pool)
	run, err := repo.CreateDraft(ctx, companyID, periodID, userID)
	must(err)
	runID = run.ID
	if run.TaxRuleVersionID != taxVersion {
		t.Logf("tax rule version %d (expected the latest reviewed %d)", run.TaxRuleVersionID, taxVersion)
	}
	// Calculate twice: the second call replaces the draft snapshot.
	for i := 0; i < 2; i++ {
		if _, err := repo.Calculate(ctx, runID); err != nil {
			t.Fatalf("Calculate #%d: %v", i+1, err)
		}
		var lines int
		must(pool.QueryRow(ctx, `SELECT count(*) FROM payroll_run_lines WHERE run_id=$1`, runID).Scan(&lines))
		if lines != 2 {
			t.Fatalf("run lines after Calculate #%d = %d, want 2", i+1, lines)
		}
	}
}
