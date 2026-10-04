package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// DB-backed tests for the base seeder. They run only when STAGING_SEED_PG_DSN
// is a postgres:// URL for a loopback Postgres that holds a database which is
// fully migrated (migration 000124) but NOT seeded. Every test gets its own
// throwaway copy of that database (CREATE DATABASE ... TEMPLATE), so the
// template is never written and tests cannot affect each other. They are
// never meant for staging.

const baseTestDSNEnv = "STAGING_SEED_PG_DSN"

var baseTestCreds = baseCredentials{
	AdminEmail: "admin@staging.odyssey.local", AdminPassword: "admin-test-pw",
	BranchEmail: "branch@staging.odyssey.local", BranchPassword: "branch-test-pw",
	NoAccessEmail: "noaccess@staging.odyssey.local", NoAccessPassword: "noaccess-test-pw",
}

// baseWrittenTables lists every table the base seeder writes to.
var baseWrittenTables = []string{
	"companies", "branches", "warehouses", "units", "categories", "products", "inventory_balances",
	"customers", "suppliers", "ar_invoices", "grns", "grn_lines", "users", "roles", "user_roles",
	"rbac_user_role_assignments", "rbac_access_reviews", "document_numbering_rules",
	"document_classifications", "document_categories", "retention_policies",
}

type baseTestDB struct {
	DSN  string
	Name string
	Host string
	Pool *pgxpool.Pool
}

func newBaseTestDB(t *testing.T) *baseTestDB {
	t.Helper()
	tplDSN := os.Getenv(baseTestDSNEnv)
	if tplDSN == "" {
		t.Skipf("%s not set; skipping DB-backed base seeder tests", baseTestDSNEnv)
	}
	cfg, err := pgx.ParseConfig(tplDSN)
	if err != nil {
		t.Fatalf("parse %s: %v", baseTestDSNEnv, err)
	}
	if ip := net.ParseIP(cfg.Host); cfg.Host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Skipf("%s host %q is not loopback; DB tests only run against a local throwaway database", baseTestDSNEnv, cfg.Host)
	}
	u, err := url.Parse(tplDSN)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatalf("%s must be a postgres:// URL", baseTestDSNEnv)
	}

	ctx := context.Background()
	admin := cfg.Copy()
	admin.Database = "postgres"
	conn, err := pgx.ConnectConfig(ctx, admin)
	if err != nil {
		t.Fatalf("connect maintenance database: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	name := fmt.Sprintf("seedtest_%d", time.Now().UnixNano())
	// Identifiers come from the template DSN and a generated name, quoted by pgx.
	create := "CREATE DATABASE " + pgx.Identifier{name}.Sanitize() + " TEMPLATE " + pgx.Identifier{cfg.Database}.Sanitize()
	if _, err := conn.Exec(ctx, create); err != nil {
		t.Fatalf("copy template database %q (it must have no open connections): %v", cfg.Database, err)
	}
	t.Cleanup(func() {
		c, err := pgx.ConnectConfig(context.Background(), admin)
		if err != nil {
			t.Logf("cleanup connect: %v", err)
			return
		}
		defer func() { _ = c.Close(context.Background()) }()
		if _, err := c.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Logf("drop %s: %v", name, err)
		}
	})

	u.Path = "/" + name
	d := &baseTestDB{DSN: u.String(), Name: name, Host: cfg.Host}
	d.Pool, err = pgxpool.New(ctx, d.DSN)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(d.Pool.Close)

	var seeded bool
	if err := d.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM companies) OR EXISTS (SELECT 1 FROM users)`).Scan(&seeded); err != nil {
		t.Fatalf("inspect template: %v", err)
	}
	if seeded {
		t.Fatalf("template database %q already contains companies or users; %s must point at a migrated but unseeded database", cfg.Database, baseTestDSNEnv)
	}
	return d
}

func (d *baseTestDB) guard() stagingGuardInput {
	return stagingGuardInput{
		Label:          "the base seeder",
		DSN:            d.DSN,
		Confirm:        d.Name + "@" + d.Host,
		AppEnv:         "staging",
		DenyHostRegex:  stagingDefaultDenyHosts,
		ExplicitDSNSet: true,
	}
}

func (d *baseTestDB) opts() baseOptions { return baseOptions{Guard: d.guard(), Creds: baseTestCreds} }

func (d *baseTestDB) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := d.Pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func (d *baseTestDB) scalar(t *testing.T, sql string, args ...any) string {
	t.Helper()
	var out string
	if err := d.Pool.QueryRow(context.Background(), sql, args...).Scan(&out); err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	return out
}

func (d *baseTestDB) counts(t *testing.T) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	for _, table := range baseWrittenTables {
		var n int64
		// Table names come from the fixed list above, not from input.
		if err := d.Pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		out[table] = n
	}
	return out
}

// snapshot fingerprints the full content of every table the seeder writes to,
// so a rerun that modified any column of any row is detected.
func (d *baseTestDB) snapshot(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, table := range baseWrittenTables {
		q := "SELECT COALESCE(md5(string_agg(t::text, '|' ORDER BY t::text)), '') FROM " + pgx.Identifier{table}.Sanitize() + " t"
		out[table] = d.scalar(t, q)
	}
	return out
}

func stepIDs(res *baseResult) map[string]int64 {
	out := map[string]int64{}
	for _, s := range res.Steps {
		out[s.Name] = s.ID
	}
	return out
}

func TestBaseSeedDBCleanApply(t *testing.T) {
	d := newBaseTestDB(t)
	ctx := context.Background()

	res, err := runBaseSeed(ctx, d.opts())
	if err != nil {
		t.Fatalf("clean apply: %v", err)
	}
	if res.count("reused") != 0 || res.count("created") != len(res.Steps) || len(res.Steps) < 20 {
		t.Fatalf("clean apply: %d created, %d reused of %d steps; want all created", res.count("created"), res.count("reused"), len(res.Steps))
	}
	f := res.Fixtures
	for name, id := range map[string]int64{
		"company": f.CompanyID, "branch": f.BranchID, "other company": f.OtherCompanyID, "other branch": f.OtherBranchID,
		"customer": f.CustomerID, "supplier": f.SupplierID, "product": f.ProductID, "warehouse": f.WarehouseID,
		"grn": f.GRNID, "document category": f.DocumentCategoryID, "document classification": f.DocumentClassID,
	} {
		if id <= 0 {
			t.Errorf("%s id = %d", name, id)
		}
	}
	if f.Amount != "100000" {
		t.Errorf("amount = %q", f.Amount)
	}

	// Ownership, derived from the database rather than from the seeder.
	checks := []struct {
		name, sql string
		args      []any
		want      string
	}{
		{"branch belongs to company", `SELECT company_id::text FROM branches WHERE id = $1`, []any{f.BranchID}, fmt.Sprint(f.CompanyID)},
		{"other branch belongs to other company", `SELECT company_id::text FROM branches WHERE id = $1`, []any{f.OtherBranchID}, fmt.Sprint(f.OtherCompanyID)},
		{"warehouse belongs to branch", `SELECT branch_id::text FROM warehouses WHERE id = $1`, []any{f.WarehouseID}, fmt.Sprint(f.BranchID)},
		{"customer belongs to company", `SELECT company_id::text FROM customers WHERE id = $1`, []any{f.CustomerID}, fmt.Sprint(f.CompanyID)},
		{"customer created_by is the admin", `SELECT (created_by = (SELECT id FROM users WHERE email = $2))::text FROM customers WHERE id = $1`, []any{f.CustomerID, baseTestCreds.AdminEmail}, "true"},
		{"supplier belongs to company", `SELECT company_id::text FROM suppliers WHERE id = $1`, []any{f.SupplierID}, fmt.Sprint(f.CompanyID)},
		{"product belongs to company", `SELECT company_id::text FROM products WHERE id = $1`, []any{f.ProductID}, fmt.Sprint(f.CompanyID)},
		{"stock exists for product in warehouse", `SELECT qty::text FROM inventory_balances WHERE warehouse_id = $1 AND product_id = $2`, []any{f.WarehouseID, f.ProductID}, "100.0000"},
		{"grn is posted, in company, un-invoiced", `SELECT (g.status = 'POSTED' AND g.company_id = $2 AND NOT EXISTS (SELECT 1 FROM ap_invoices a WHERE a.grn_id = g.id))::text FROM grns g WHERE g.id = $1`, []any{f.GRNID, f.CompanyID}, "true"},
		{"grn has one line", `SELECT COUNT(*)::text FROM grn_lines WHERE grn_id = $1`, []any{f.GRNID}, "1"},
		{"other company has an AR invoice", `SELECT COUNT(*)::text FROM ar_invoices i JOIN customers c ON c.id = i.customer_id WHERE c.company_id = $1`, []any{f.OtherCompanyID}, "1"},
		{"document category belongs to company", `SELECT company_id::text FROM document_categories WHERE id = $1`, []any{f.DocumentCategoryID}, fmt.Sprint(f.CompanyID)},
		{"classification belongs to company", `SELECT company_id::text FROM document_classifications WHERE id = $1`, []any{f.DocumentClassID}, fmt.Sprint(f.CompanyID)},
		{"retention policy is 7 years and linked", `SELECT (retention_period_days = 2557 AND active AND classification_ids = ARRAY[$2::bigint] AND category_ids = ARRAY[$3::bigint])::text FROM retention_policies WHERE company_id = $1 AND code = 'RET-7Y-COMPLIANCE'`, []any{f.CompanyID, f.DocumentClassID, f.DocumentCategoryID}, "true"},
		{"admin has the admin role", `SELECT COUNT(*)::text FROM user_roles ur JOIN users u ON u.id = ur.user_id JOIN roles r ON r.id = ur.role_id WHERE u.email = $1 AND LOWER(r.name) = 'admin'`, []any{baseTestCreds.AdminEmail}, "1"},
		{"branch and no-access users have no global roles", `SELECT COUNT(*)::text FROM user_roles ur JOIN users u ON u.id = ur.user_id WHERE u.email IN ($1, $2)`, []any{baseTestCreds.BranchEmail, baseTestCreds.NoAccessEmail}, "0"},
		{"branch user has an expired scoped assignment", `SELECT COUNT(*)::text FROM rbac_user_role_assignments a JOIN users u ON u.id = a.user_id WHERE u.email = $1 AND a.company_id = $2 AND a.branch_id = $3 AND a.valid_to <= NOW()`, []any{baseTestCreds.BranchEmail, f.CompanyID, f.BranchID}, "1"},
		{"branch user has a completed REVOKE review", `SELECT COUNT(*)::text FROM rbac_access_reviews r JOIN users u ON u.id = r.subject_user_id WHERE u.email = $1 AND r.company_id = $2 AND r.status = 'COMPLETED' AND r.decision = 'REVOKE' AND r.decided_by_user_id IS NOT NULL AND r.decided_at IS NOT NULL`, []any{baseTestCreds.BranchEmail, f.CompanyID}, "1"},
	}
	for _, c := range checks {
		if got := d.scalar(t, c.sql, c.args...); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}

	// The configured passwords authenticate against the stored hashes.
	for email, pw := range map[string]string{
		baseTestCreds.AdminEmail: baseTestCreds.AdminPassword, baseTestCreds.BranchEmail: baseTestCreds.BranchPassword,
		baseTestCreds.NoAccessEmail: baseTestCreds.NoAccessPassword,
	} {
		hash := d.scalar(t, `SELECT password_hash FROM users WHERE email = $1 AND is_active`, email)
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)); err != nil {
			t.Errorf("password for %s does not match stored hash: %v", email, err)
		}
	}
}

func TestBaseSeedDBIdempotentRerun(t *testing.T) {
	d := newBaseTestDB(t)
	ctx := context.Background()

	first, err := runBaseSeed(ctx, d.opts())
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	countsAfterFirst := d.counts(t)
	snapAfterFirst := d.snapshot(t)

	second, err := runBaseSeed(ctx, d.opts())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	// The GRN line is only inserted together with a new GRN, so a rerun has
	// one step fewer than the first run and every step is a reuse.
	if second.count("created") != 0 || second.count("reused") != len(second.Steps) || len(second.Steps) != len(first.Steps)-1 {
		t.Fatalf("second run: %d created, %d reused of %d steps; want a no-op", second.count("created"), second.count("reused"), len(second.Steps))
	}
	if *first.Fixtures != *second.Fixtures {
		t.Fatalf("second run returned different fixtures:\nfirst  %+v\nsecond %+v", *first.Fixtures, *second.Fixtures)
	}
	for name, id := range stepIDs(second) {
		if want, ok := stepIDs(first)[name]; !ok || want != id {
			t.Errorf("step %s id = %d, first run %d (present=%v)", name, id, want, ok)
		}
	}
	if got := d.counts(t); fmt.Sprint(got) != fmt.Sprint(countsAfterFirst) {
		t.Fatalf("second run changed row counts:\nbefore %v\nafter  %v", countsAfterFirst, got)
	}
	if got := d.snapshot(t); fmt.Sprint(got) != fmt.Sprint(snapAfterFirst) {
		t.Fatalf("second run modified existing rows:\nbefore %v\nafter  %v", snapAfterFirst, got)
	}
}

// TestBaseSeedDBRefusesRowsOutsideExpectedScope plants a row that conflicts
// with a fixture but sits outside the expected company/parent/state. The run
// must abort, roll everything back and leave the planted row untouched.
func TestBaseSeedDBRefusesRowsOutsideExpectedScope(t *testing.T) {
	otherHash, err := bcrypt.GenerateFromPassword([]byte("someone-elses-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	correctBranchHash, err := bcrypt.GenerateFromPassword([]byte(baseTestCreds.BranchPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}

	const plantCompanyX = `INSERT INTO companies (code, name) VALUES ('X-OTHER', 'Another Tenant')`
	const plantUserP = `INSERT INTO users (email, password_hash) VALUES ('planter@staging.invalid', 'x')`
	const plantODY01 = `INSERT INTO companies (code, name) VALUES ('ODY-01', 'PT Odyssey Utama')`

	cases := []struct {
		name    string
		plant   func(t *testing.T, d *baseTestDB)
		wantErr string
		// unchanged re-reads the planted row after the refused run.
		unchangedSQL, unchangedWant string
	}{
		{
			name: "branch code owned by another company",
			plant: func(t *testing.T, d *baseTestDB) {
				d.exec(t, plantCompanyX)
				d.exec(t, `INSERT INTO branches (company_id, code, name) SELECT id, 'HQ-JKT', 'Not ours' FROM companies WHERE code = 'X-OTHER'`)
			},
			wantErr:      "branch_primary (branches) already exists",
			unchangedSQL: `SELECT c.code FROM branches b JOIN companies c ON c.id = b.company_id WHERE b.code = 'HQ-JKT'`, unchangedWant: "X-OTHER",
		},
		{
			name: "warehouse code owned by another branch",
			plant: func(t *testing.T, d *baseTestDB) {
				d.exec(t, plantCompanyX)
				d.exec(t, `INSERT INTO branches (company_id, code, name) SELECT id, 'X-BR', 'X branch' FROM companies WHERE code = 'X-OTHER'`)
				d.exec(t, `INSERT INTO warehouses (branch_id, code, name) SELECT id, 'WH-JKT-01', 'Not ours' FROM branches WHERE code = 'X-BR'`)
			},
			wantErr:      "warehouse_primary (warehouses) already exists",
			unchangedSQL: `SELECT b.code FROM warehouses w JOIN branches b ON b.id = w.branch_id WHERE w.code = 'WH-JKT-01'`, unchangedWant: "X-BR",
		},
		{
			name: "supplier code owned by another company",
			plant: func(t *testing.T, d *baseTestDB) {
				d.exec(t, plantCompanyX)
				d.exec(t, `INSERT INTO suppliers (code, name, company_id) SELECT 'SUPP-STG-01', 'Not ours', id FROM companies WHERE code = 'X-OTHER'`)
			},
			wantErr:      "supplier_primary (suppliers) already exists",
			unchangedSQL: `SELECT c.code FROM suppliers s JOIN companies c ON c.id = s.company_id WHERE s.code = 'SUPP-STG-01'`, unchangedWant: "X-OTHER",
		},
		{
			name: "product sku owned by another company",
			plant: func(t *testing.T, d *baseTestDB) {
				d.exec(t, plantCompanyX)
				d.exec(t, `INSERT INTO categories (code, name, company_id) SELECT 'X-CAT', 'X cat', id FROM companies WHERE code = 'X-OTHER'`)
				d.exec(t, `INSERT INTO units (code, name) VALUES ('XU', 'X unit')`)
				d.exec(t, `INSERT INTO products (sku, name, category_id, unit_id, price, company_id)
					SELECT 'PROD-STG-01', 'Not ours', (SELECT id FROM categories WHERE code = 'X-CAT'), (SELECT id FROM units WHERE code = 'XU'), 1, id
					FROM companies WHERE code = 'X-OTHER'`)
			},
			wantErr:      "product (products) already exists",
			unchangedSQL: `SELECT c.code FROM products p JOIN companies c ON c.id = p.company_id WHERE p.sku = 'PROD-STG-01'`, unchangedWant: "X-OTHER",
		},
		{
			name: "company code with a different identity",
			plant: func(t *testing.T, d *baseTestDB) {
				d.exec(t, `INSERT INTO companies (code, name) VALUES ('ODY-01', 'A Different Tenant')`)
			},
			wantErr:      "company_primary (companies) already exists",
			unchangedSQL: `SELECT name FROM companies WHERE code = 'ODY-01'`, unchangedWant: "A Different Tenant",
		},
		{
			name: "inactive customer",
			plant: func(t *testing.T, d *baseTestDB) {
				d.exec(t, plantODY01)
				d.exec(t, plantUserP)
				d.exec(t, `INSERT INTO customers (code, name, company_id, is_active, created_by)
					SELECT 'CUST-STG-01', 'Inactive', c.id, FALSE, u.id FROM companies c, users u WHERE c.code = 'ODY-01' AND u.email = 'planter@staging.invalid'`)
			},
			wantErr:      "customer_primary (customers) already exists",
			unchangedSQL: `SELECT is_active::text FROM customers WHERE code = 'CUST-STG-01'`, unchangedWant: "false",
		},
		{
			name: "existing user with a different password",
			plant: func(t *testing.T, d *baseTestDB) {
				d.exec(t, `INSERT INTO users (email, password_hash) VALUES ($1, $2)`, baseTestCreds.AdminEmail, string(otherHash))
			},
			wantErr:      "already exists with a different password",
			unchangedSQL: `SELECT password_hash FROM users WHERE email = 'admin@staging.odyssey.local'`, unchangedWant: string(otherHash),
		},
		{
			name: "branch user holding a global role",
			plant: func(t *testing.T, d *baseTestDB) {
				d.exec(t, `INSERT INTO users (email, password_hash) VALUES ($1, $2)`, baseTestCreds.BranchEmail, string(correctBranchHash))
				d.exec(t, `INSERT INTO user_roles (user_id, role_id) SELECT u.id, r.id FROM users u, roles r WHERE u.email = $1 AND r.name = 'Sales Manager'`, baseTestCreds.BranchEmail)
			},
			wantErr:      "has 1 global user_roles row",
			unchangedSQL: `SELECT COUNT(*)::text FROM user_roles ur JOIN users u ON u.id = ur.user_id WHERE u.email = 'branch@staging.odyssey.local'`, unchangedWant: "1",
		},
		{
			name: "retention policy with a different period",
			plant: func(t *testing.T, d *baseTestDB) {
				d.exec(t, plantODY01)
				d.exec(t, plantUserP)
				d.exec(t, `INSERT INTO retention_policies (company_id, code, name, trigger_event, retention_period_days, created_by)
					SELECT c.id, 'RET-7Y-COMPLIANCE', 'Short', 'APPROVAL', 30, u.id FROM companies c, users u WHERE c.code = 'ODY-01' AND u.email = 'planter@staging.invalid'`)
			},
			wantErr:      "retention_policy (retention_policies) already exists",
			unchangedSQL: `SELECT retention_period_days::text FROM retention_policies WHERE code = 'RET-7Y-COMPLIANCE'`, unchangedWant: "30",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newBaseTestDB(t)
			tc.plant(t, d)
			before := d.counts(t)
			snap := d.snapshot(t)

			res, err := runBaseSeed(context.Background(), d.opts())
			if err == nil {
				t.Fatalf("expected a refusal, got success: %+v", res.Steps)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
			if got := d.counts(t); fmt.Sprint(got) != fmt.Sprint(before) {
				t.Fatalf("refused run left rows behind:\nbefore %v\nafter  %v", before, got)
			}
			if got := d.snapshot(t); fmt.Sprint(got) != fmt.Sprint(snap) {
				t.Fatalf("refused run modified existing rows")
			}
			if got := d.scalar(t, tc.unchangedSQL); got != tc.unchangedWant {
				t.Fatalf("planted row changed: got %q, want %q", got, tc.unchangedWant)
			}
		})
	}
}

func TestBaseSeedDBRefusedWithoutStagingConfirmation(t *testing.T) {
	d := newBaseTestDB(t)
	before := d.counts(t)

	cases := map[string]func(g *stagingGuardInput){
		"missing APP_ENV":        func(g *stagingGuardInput) { g.AppEnv = "" },
		"APP_ENV production":     func(g *stagingGuardInput) { g.AppEnv = "production" },
		"missing confirmation":   func(g *stagingGuardInput) { g.Confirm = "" },
		"database mismatch":      func(g *stagingGuardInput) { g.Confirm = "not_this_db@" + strings.SplitN(g.Confirm, "@", 2)[1] },
		"host mismatch":          func(g *stagingGuardInput) { g.Confirm = strings.SplitN(g.Confirm, "@", 2)[0] + "@elsewhere" },
		"denied host":            func(g *stagingGuardInput) { g.DenyHostRegex = "." },
		"DSN not given explicit": func(g *stagingGuardInput) { g.ExplicitDSNSet = false },
	}
	for name, mutate := range cases {
		o := d.opts()
		mutate(&o.Guard)
		if _, err := runBaseSeed(context.Background(), o); err == nil || !strings.Contains(err.Error(), "staging guard") {
			t.Errorf("%s: expected staging guard refusal, got %v", name, err)
		}
	}
	if got := d.counts(t); fmt.Sprint(got) != fmt.Sprint(before) {
		t.Fatalf("guard refusals changed row counts:\nbefore %v\nafter  %v", before, got)
	}
}
