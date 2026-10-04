package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB-backed tests for the --iso004 extension. They run only when
// ISO004_PG_DSN points at a disposable, migrated, loopback Postgres that
// already holds the base staging prerequisites (companies ODY-01/ODY-02,
// branches HQ-JKT/BR-BDG, the admin user, unit EA, classification
// CONFIDENTIAL). They write fixture rows and are never meant for staging.

func iso004TestDB(t *testing.T) (string, iso004GuardInput) {
	t.Helper()
	dsn := os.Getenv("ISO004_PG_DSN")
	if dsn == "" {
		t.Skip("ISO004_PG_DSN not set; skipping DB-backed iso004 tests")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse ISO004_PG_DSN: %v", err)
	}
	if ip := net.ParseIP(cfg.Host); cfg.Host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Skipf("ISO004_PG_DSN host %q is not loopback; DB tests only run against a local throwaway database", cfg.Host)
	}
	return dsn, iso004GuardInput{
		DSN:            dsn,
		Confirm:        cfg.Database + "@" + cfg.Host,
		AppEnv:         "staging",
		DenyHostRegex:  iso004DefaultDenyHosts,
		ExplicitDSNSet: true,
	}
}

func iso004RequirePrereqs(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var ok bool
	err := pool.QueryRow(ctx, `SELECT
		EXISTS (SELECT 1 FROM companies WHERE code = 'ODY-01') AND
		EXISTS (SELECT 1 FROM companies WHERE code = 'ODY-02') AND
		EXISTS (SELECT 1 FROM branches WHERE code = 'BR-BDG') AND
		EXISTS (SELECT 1 FROM units WHERE code = 'EA')`).Scan(&ok)
	if err != nil || !ok {
		t.Skipf("base prerequisites missing (err=%v); seed them before running DB tests", err)
	}
}

func iso004FixtureTables() []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range iso004Steps() {
		if !seen[s.Table] {
			seen[s.Table] = true
			out = append(out, s.Table)
		}
	}
	return out
}

func iso004RowCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool) map[string]int64 {
	t.Helper()
	counts := map[string]int64{}
	for _, table := range iso004FixtureTables() {
		var n int64
		// Table names come from the fixed step list, not from input.
		if err := pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", pgx.Identifier{table}.Sanitize())).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = n
	}
	return counts
}

func iso004StepIDs(res *iso004Result) map[string]int64 {
	out := map[string]int64{}
	for _, s := range res.Steps {
		out[s.Step.Name] = s.ID
	}
	return out
}

func TestISO004DBIdempotentAndOwned(t *testing.T) {
	dsn, guard := iso004TestDB(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	iso004RequirePrereqs(t, ctx, pool)

	guard.Key = fmt.Sprintf("t%d", time.Now().UnixNano())
	opts := iso004Options{Guard: guard, AdminEmail: getenv("STAGING_CERT_ADMIN_EMAIL", "admin@staging.odyssey.local")}

	// Dry run writes nothing.
	before := iso004RowCounts(t, ctx, pool)
	dry := opts
	dry.DryRun = true
	plan, err := runISO004(ctx, dry)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	for _, s := range plan.Steps {
		if s.Step.Name == "variance_rule_b" && s.Status != "would-insert" {
			t.Errorf("dry run status for fresh key = %q", s.Status)
		}
	}
	if after := iso004RowCounts(t, ctx, pool); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("dry run changed row counts:\nbefore %v\nafter  %v", before, after)
	}

	first, err := runISO004(ctx, opts)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	afterFirst := iso004RowCounts(t, ctx, pool)
	second, err := runISO004(ctx, opts)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if fmt.Sprint(iso004StepIDs(first)) != fmt.Sprint(iso004StepIDs(second)) {
		t.Fatalf("second run returned different IDs:\nfirst  %v\nsecond %v", iso004StepIDs(first), iso004StepIDs(second))
	}
	if afterSecond := iso004RowCounts(t, ctx, pool); fmt.Sprint(afterSecond) != fmt.Sprint(afterFirst) {
		t.Fatalf("second run inserted rows:\nafter first  %v\nafter second %v", afterFirst, afterSecond)
	}
	for _, s := range second.Steps {
		if s.Status != "reused" {
			t.Errorf("%s: status %q on second run", s.Step.Name, s.Status)
		}
	}
	if len(second.Verification) != len(second.Steps) {
		t.Fatalf("verification rows = %d, steps = %d", len(second.Verification), len(second.Steps))
	}
	for _, v := range second.Verification {
		if !v.OK() {
			t.Errorf("%s id=%d owner %q expected %q", v.Name, v.ID, v.Owner, v.Expected)
		}
	}

	// The printed verification SQL runs and reports ok for every row.
	rows, err := pool.Query(ctx, second.VerifySQL)
	if err != nil {
		t.Fatalf("run printed verification SQL: %v", err)
	}
	n := 0
	for rows.Next() {
		var fixture, tbl, owner, expected string
		var id int64
		var ok bool
		if err := rows.Scan(&fixture, &tbl, &id, &owner, &expected, &ok); err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Errorf("verification SQL: %s id=%d owner=%q expected=%q", fixture, id, owner, expected)
		}
		n++
	}
	rows.Close()
	if n != len(second.Steps) {
		t.Fatalf("verification SQL returned %d rows, want %d", n, len(second.Steps))
	}
	if second.PayslipAt == "" || second.APCreatedBy != second.Refs.ids["actor_b"] {
		t.Fatalf("read-back values: payslip_at=%q created_by=%d actor_b=%d", second.PayslipAt, second.APCreatedBy, second.Refs.ids["actor_b"])
	}
	if second.Refs.ids["actor_b"] == second.Refs.ids["admin_a"] {
		t.Fatal("company B attribution user must differ from the company A admin")
	}
}

func TestISO004DBOwnershipMismatchAborts(t *testing.T) {
	dsn, guard := iso004TestDB(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	iso004RequirePrereqs(t, ctx, pool)

	guard.Key = fmt.Sprintf("o%d", time.Now().UnixNano())
	// Test setup: plant the key's company-B supplier code in company A.
	var companyA int64
	if err := pool.QueryRow(ctx, `SELECT id FROM companies WHERE code = 'ODY-01'`).Scan(&companyA); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO suppliers (code, name, company_id) VALUES ($1, 'planted by iso004 test', $2)`, "ISO004-SUP-B-NOPOL-"+guard.Key, companyA); err != nil {
		t.Fatal(err)
	}
	before := iso004RowCounts(t, ctx, pool)

	_, err = runISO004(ctx, iso004Options{Guard: guard, AdminEmail: getenv("STAGING_CERT_ADMIN_EMAIL", "admin@staging.odyssey.local")})
	if err == nil || !strings.Contains(err.Error(), "ownership check failed for supplier_b_nopolicy") {
		t.Fatalf("expected ownership abort, got %v", err)
	}
	if after := iso004RowCounts(t, ctx, pool); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("aborted run left rows behind:\nbefore %v\nafter  %v", before, after)
	}
}

func TestISO004DBGuardRefusesBeforeWriting(t *testing.T) {
	dsn, guard := iso004TestDB(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	guard.Key = fmt.Sprintf("g%d", time.Now().UnixNano())
	before := iso004RowCounts(t, ctx, pool)

	cases := map[string]func(g *iso004GuardInput){
		"missing APP_ENV": func(g *iso004GuardInput) { g.AppEnv = "" },
		"db mismatch":     func(g *iso004GuardInput) { g.Confirm = "not_this_db@" + strings.SplitN(g.Confirm, "@", 2)[1] },
		"host mismatch":   func(g *iso004GuardInput) { g.Confirm = strings.SplitN(g.Confirm, "@", 2)[0] + "@elsewhere" },
		"denied host":     func(g *iso004GuardInput) { g.DenyHostRegex = "." },
	}
	for name, mutate := range cases {
		g := guard
		mutate(&g)
		if _, err := runISO004(ctx, iso004Options{Guard: g}); err == nil || !strings.Contains(err.Error(), "staging guard") {
			t.Errorf("%s: expected staging guard refusal, got %v", name, err)
		}
	}
	if after := iso004RowCounts(t, ctx, pool); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("guard refusals changed row counts")
	}
}
