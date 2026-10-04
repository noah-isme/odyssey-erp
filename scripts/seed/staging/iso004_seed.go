package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// iso004Options are the parsed flags for the --iso004 extension.
type iso004Options struct {
	Guard      iso004GuardInput
	AdminEmail string
	DryRun     bool
}

// iso004StepResult records what happened to one fixture row.
type iso004StepResult struct {
	Step   iso004Step
	Status string // created | reused | would-insert | would-insert (parent pending)
	ID     int64
}

// iso004Verification is one row of the post-commit ownership verification.
type iso004Verification struct {
	Name, Table, Owner, Expected string
	ID                           int64
}

func (v iso004Verification) OK() bool { return v.Owner == v.Expected }

// iso004Result is everything the summary printers need.
type iso004Result struct {
	Target       iso004Target
	DryRun       bool
	Refs         *iso004Refs
	Steps        []iso004StepResult
	Warnings     []string
	Verification []iso004Verification
	VerifySQL    string
	PayslipAt    string
	APCreatedBy  int64
}

type iso004Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// runISO004 is the --iso004 entry point. It never runs the base seeder.
func runISO004(ctx context.Context, opts iso004Options) (*iso004Result, error) {
	target, err := checkISO004Static(opts.Guard)
	if err != nil {
		return nil, fmt.Errorf("staging guard: %w", err)
	}

	pool, err := pgxpool.New(ctx, opts.Guard.DSN)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	var currentDB string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&currentDB); err != nil {
		return nil, fmt.Errorf("read current_database(): %w", err)
	}
	if err := checkISO004CurrentDatabase(target, currentDB); err != nil {
		return nil, fmt.Errorf("staging guard: %w", err)
	}

	res := &iso004Result{Target: target, DryRun: opts.DryRun, Refs: newISO004Refs(opts.Guard.Key)}

	if opts.DryRun {
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
		if err != nil {
			return nil, fmt.Errorf("begin read-only tx: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if err := iso004ResolveBase(ctx, tx, res, opts.AdminEmail); err != nil {
			return nil, err
		}
		if err := iso004RunSteps(ctx, tx, res, true); err != nil {
			return nil, err
		}
		return res, nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize concurrent ISO-004 seeds so SELECT-then-INSERT on tables
	// without a natural unique key cannot race.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('odyssey:seed:iso004'))`); err != nil {
		return nil, fmt.Errorf("acquire iso004 advisory lock: %w", err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL statement_timeout = '30s'`); err != nil {
		return nil, fmt.Errorf("set statement_timeout: %w", err)
	}
	if err := iso004ResolveBase(ctx, tx, res, opts.AdminEmail); err != nil {
		return nil, err
	}
	if err := iso004RunSteps(ctx, tx, res, false); err != nil {
		return nil, err
	}
	// Ownership of every fixture (including rows created above) is checked
	// before commit, so a mismatch rolls the whole extension back.
	for _, s := range res.Steps {
		if err := iso004CheckOwner(ctx, tx, res.Refs, s.Step, s.ID); err != nil {
			return nil, fmt.Errorf("pre-commit verification: %w", err)
		}
	}
	if err := iso004ReadBack(ctx, tx, res); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit iso004 fixtures: %w", err)
	}

	if err := iso004Verify(ctx, pool, res); err != nil {
		return res, err
	}
	return res, nil
}

// iso004ResolveBase resolves prerequisites with SELECTs only and aborts on
// anything missing or inconsistent.
func iso004ResolveBase(ctx context.Context, q iso004Querier, res *iso004Result, adminEmail string) error {
	r := res.Refs
	for _, b := range iso004BaseQueries {
		var id int64
		if err := q.QueryRow(ctx, b.SQL, b.Args(adminEmail)...).Scan(&id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("missing prerequisite: %s (create it with the base staging seeder before --iso004)", b.What)
			}
			return fmt.Errorf("resolve %s: %w", b.What, err)
		}
		r.ids[b.Name] = id
	}
	for _, c := range iso004BaseChecks {
		var ok bool
		if err := q.QueryRow(ctx, c.SQL, c.Args(r)...).Scan(&ok); err != nil {
			return fmt.Errorf("check %q: %w", c.What, err)
		}
		if !ok {
			return fmt.Errorf("prerequisite check failed: %s", c.What)
		}
	}
	for _, b := range iso004LateBase {
		var id int64
		if err := q.QueryRow(ctx, b.SQL, b.Args(r)...).Scan(&id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("missing prerequisite: %s", b.What)
			}
			return fmt.Errorf("resolve %s: %w", b.What, err)
		}
		r.ids[b.Name] = id
	}

	var currency string
	if err := q.QueryRow(ctx, `SELECT base_currency FROM companies WHERE id = $1`, r.ids["company_b"]).Scan(&currency); err != nil {
		return fmt.Errorf("read company B base_currency: %w", err)
	}
	r.strs["currency_b"] = strings.TrimSpace(currency)
	r.strs["payslip_email"] = "iso004-payslip-" + strings.ToLower(r.key) + "@staging.invalid"
	r.strs["ap_invoice_b_missing.number"] = r.code("ISO004-AP-B-MISSING")
	r.strs["ap_invoice_b_exception.number"] = r.code("ISO004-AP-B-EXCEPTION")

	matched, err := iso004ResolveMatchedNumber(ctx, q, r.code("ISO004-AP-B-MATCHED"))
	if err != nil {
		return err
	}
	r.strs["ap_invoice_b_matched.number"] = matched

	for _, a := range iso004Advisories {
		var ok bool
		if err := q.QueryRow(ctx, a.SQL).Scan(&ok); err != nil {
			return fmt.Errorf("advisory %q: %w", a.What, err)
		}
		if !ok {
			res.Warnings = append(res.Warnings, "not satisfied: "+a.What)
		}
	}
	return nil
}

// iso004ResolveMatchedNumber returns the number to use for the MATCHED
// invoice: the latest variant while it is still DRAFT, otherwise a fresh
// "-rN" number because a previous run consumed (posted) it.
func iso004ResolveMatchedNumber(ctx context.Context, q iso004Querier, base string) (string, error) {
	var number, status string
	var variants int64
	err := q.QueryRow(ctx, iso004MatchedVariantSQL, base).Scan(&number, &status, &variants)
	if errors.Is(err, pgx.ErrNoRows) {
		return base, nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve MATCHED invoice number: %w", err)
	}
	if status == "DRAFT" {
		return number, nil
	}
	return fmt.Sprintf("%s-r%d", base, variants), nil
}

func iso004HasPlaceholder(args []any) bool {
	for _, a := range args {
		if s, ok := a.(string); ok && strings.HasPrefix(s, "<") && strings.HasSuffix(s, ".id>") {
			return true
		}
	}
	return false
}

func iso004RunSteps(ctx context.Context, q iso004Querier, res *iso004Result, dry bool) error {
	for _, step := range iso004Steps() {
		out, err := iso004RunStep(ctx, q, res.Refs, step, dry)
		if err != nil {
			return err
		}
		res.Steps = append(res.Steps, out)
	}
	return nil
}

func iso004Lookup(ctx context.Context, q iso004Querier, r *iso004Refs, step iso004Step) (int64, bool, error) {
	var id int64
	err := q.QueryRow(ctx, step.Lookup, step.LookupArgs(r)...).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("look up %s (%s): %w", step.Name, step.Table, err)
	}
	return id, true, nil
}

func iso004CheckOwner(ctx context.Context, q iso004Querier, r *iso004Refs, step iso004Step, id int64) error {
	var owner string
	if err := q.QueryRow(ctx, step.OwnerByID, id).Scan(&owner); err != nil {
		return fmt.Errorf("read ownership of %s id=%d (%s): %w", step.Name, id, step.Table, err)
	}
	if want := step.Expect(r); owner != want {
		return fmt.Errorf("ownership check failed for %s id=%d (%s): found %q, expected %q; refusing to reuse a row outside the expected scope", step.Name, id, step.Table, owner, want)
	}
	return nil
}

func iso004RunStep(ctx context.Context, q iso004Querier, r *iso004Refs, step iso004Step, dry bool) (iso004StepResult, error) {
	out := iso004StepResult{Step: step}
	if dry && iso004HasPlaceholder(step.LookupArgs(r)) {
		out.Status = "would-insert (parent pending)"
		return out, nil
	}
	id, found, err := iso004Lookup(ctx, q, r, step)
	if err != nil {
		return out, err
	}
	if found {
		if err := iso004CheckOwner(ctx, q, r, step, id); err != nil {
			return out, err
		}
		r.ids[step.Name] = id
		out.Status, out.ID = "reused", id
		return out, nil
	}
	if dry {
		out.Status = "would-insert"
		return out, nil
	}

	err = q.QueryRow(ctx, step.Insert, step.InsertArgs(r)...).Scan(&id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// ON CONFLICT DO NOTHING suppressed the insert. Accept only a row the
		// natural-key lookup now finds and that passes the ownership check.
		id, found, err = iso004Lookup(ctx, q, r, step)
		if err != nil {
			return out, err
		}
		if !found {
			return out, fmt.Errorf("inserting the %s row for %s conflicted with an existing row that the natural-key lookup does not return; refusing", step.Table, step.Name)
		}
		if err := iso004CheckOwner(ctx, q, r, step, id); err != nil {
			return out, err
		}
		r.ids[step.Name] = id
		out.Status, out.ID = "reused", id
		return out, nil
	case err != nil:
		return out, fmt.Errorf("insert %s (%s): %w", step.Name, step.Table, err)
	}
	// A row created here carries the expected scope by construction; some
	// scopes (periods -> accounting_periods) only exist after a later step,
	// so freshly created rows are checked by iso004Verify after commit.
	r.ids[step.Name] = id
	out.Status, out.ID = "created", id
	return out, nil
}

// iso004ReadBack reads values the operator needs that the database assigns.
func iso004ReadBack(ctx context.Context, q iso004Querier, res *iso004Result) error {
	r := res.Refs
	if err := q.QueryRow(ctx,
		`SELECT to_char(generated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM payroll_payslips WHERE id = $1`,
		r.ids["payslip_b"]).Scan(&res.PayslipAt); err != nil {
		return fmt.Errorf("read payslip generated_at: %w", err)
	}
	var createdBy *int64
	if err := q.QueryRow(ctx, `SELECT created_by FROM ap_invoices WHERE id = $1`, r.ids["ap_invoice_b_matched"]).Scan(&createdBy); err != nil {
		return fmt.Errorf("read AP invoice created_by: %w", err)
	}
	if createdBy == nil {
		return errors.New("AP invoice fixture has NULL created_by")
	}
	res.APCreatedBy = *createdBy
	return nil
}

// iso004Verify re-reads ownership of every fixture after commit in a
// read-only transaction and builds the printable verification SQL.
func iso004Verify(ctx context.Context, pool *pgxpool.Pool, res *iso004Result) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin verification tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var failed []string
	for _, s := range res.Steps {
		var owner string
		if err := tx.QueryRow(ctx, s.Step.OwnerByID, s.ID).Scan(&owner); err != nil {
			return fmt.Errorf("verify %s id=%d: %w", s.Step.Name, s.ID, err)
		}
		v := iso004Verification{Name: s.Step.Name, Table: s.Step.Table, ID: s.ID, Owner: owner, Expected: s.Step.Expect(res.Refs)}
		res.Verification = append(res.Verification, v)
		if !v.OK() {
			failed = append(failed, fmt.Sprintf("%s id=%d owner=%q expected=%q", v.Name, v.ID, v.Owner, v.Expected))
		}
	}
	res.VerifySQL = iso004VerificationSQL(res)
	if len(failed) > 0 {
		return fmt.Errorf("post-commit ownership verification failed: %s", strings.Join(failed, "; "))
	}
	return nil
}

// iso004VerificationSQL renders a self-contained read-only query an operator
// can paste into psql to re-check fixture ownership. Only integer IDs and
// scope strings built from integer IDs are interpolated.
func iso004VerificationSQL(res *iso004Result) string {
	var parts []string
	for _, s := range res.Steps {
		owner := strings.ReplaceAll(s.Step.OwnerByID, "$1", strconv.FormatInt(s.ID, 10))
		parts = append(parts, fmt.Sprintf("  SELECT '%s' AS fixture, '%s' AS tbl, %d::bigint AS id, (%s) AS owner, '%s' AS expected",
			s.Step.Name, s.Step.Table, s.ID, owner, s.Step.Expect(res.Refs)))
	}
	return "SELECT fixture, tbl, id, owner, expected, owner = expected AS ok FROM (\n" +
		strings.Join(parts, "\n  UNION ALL\n") + "\n) v ORDER BY fixture;"
}
