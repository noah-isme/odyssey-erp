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
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
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
	"permissions", "role_permissions", "pos", "po_lines", "ap_matching_policies",
	"accounts", "account_mappings", "periods",
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

	// On a migrations-only database some admin permissions already exist
	// (migrations insert them) and role 1 holds the 000083 module grants.
	migrationPerms := d.scalar(t, `SELECT COUNT(*)::text FROM permissions WHERE name = ANY($1)`, baseAdminPermissions)
	roleOneGrants := d.scalar(t, `SELECT COUNT(*)::text FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
		WHERE rp.role_id = 1 AND (p.name LIKE 'cmms.%' OR p.name LIKE 'qms.%' OR p.name LIKE 'documents.%')`)
	roleOneName := d.scalar(t, `SELECT name FROM roles WHERE id = 1`)

	res, err := runBaseSeed(ctx, d.opts())
	if err != nil {
		t.Fatalf("clean apply: %v", err)
	}
	// Only pre-existing permission rows (from migrations) are reused.
	for _, s := range res.Steps {
		if s.Status == "reused" && s.Table != "permissions" {
			t.Errorf("clean apply reused %s (%s)", s.Name, s.Table)
		}
	}
	if fmt.Sprint(res.count("reused")) != migrationPerms || res.count("created")+res.count("reused") != len(res.Steps) || len(res.Steps) < 20 {
		t.Fatalf("clean apply: %d created, %d reused of %d steps; want all created except the %s migration permissions", res.count("created"), res.count("reused"), len(res.Steps), migrationPerms)
	}
	if fmt.Sprint(len(res.Revoked)) != roleOneGrants || roleOneGrants == "0" || res.RevokedRole != roleOneName {
		t.Errorf("revoked %d grant(s) from %q; want the %s cmms/qms/documents grants of role 1 %q", len(res.Revoked), res.RevokedRole, roleOneGrants, roleOneName)
	}
	if !res.GRNCreated || res.POID <= 0 {
		t.Errorf("clean apply: GRNCreated=%v POID=%d; want a fresh PO and GRN", res.GRNCreated, res.POID)
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
		// Admin permissions: the granted list equals the enumerated set.
		{"admin role holds exactly the enumerated permissions", `SELECT string_agg(p.name, ',' ORDER BY p.name) FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id JOIN roles r ON r.id = rp.role_id WHERE LOWER(r.name) = 'admin'`, nil, strings.Join(baseAdminPermissions, ",")},
		{"every enumerated permission exists once", `SELECT COUNT(*)::text FROM permissions WHERE name = ANY($1)`, []any{baseAdminPermissions}, fmt.Sprint(len(baseAdminPermissions))},
		{"role 1 keeps no cmms/qms/documents grants", `SELECT COUNT(*)::text FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id WHERE rp.role_id = 1 AND (p.name LIKE 'cmms.%' OR p.name LIKE 'qms.%' OR p.name LIKE 'documents.%')`, nil, "0"},
		// Assignments.
		{"admin has exactly one active assignment, company-wide in the primary company", `SELECT string_agg(a.company_id || '/' || COALESCE(a.branch_id::text, 'company-wide'), ',') FROM rbac_user_role_assignments a JOIN users u ON u.id = a.user_id WHERE u.email = $1 AND a.valid_from <= NOW() AND (a.valid_to IS NULL OR a.valid_to > NOW())`, []any{baseTestCreds.AdminEmail}, fmt.Sprintf("%d/company-wide", f.CompanyID)},
		{"branch user has exactly one active assignment, in the primary branch", `SELECT string_agg(a.company_id || '/' || COALESCE(a.branch_id::text, 'company-wide'), ',') FROM rbac_user_role_assignments a JOIN users u ON u.id = a.user_id WHERE u.email = $1 AND a.valid_from <= NOW() AND (a.valid_to IS NULL OR a.valid_to > NOW())`, []any{baseTestCreds.BranchEmail}, fmt.Sprintf("%d/%d", f.CompanyID, f.BranchID)},
		{"branch user's active role has no permissions", `SELECT r.name || ':' || (SELECT COUNT(*) FROM role_permissions rp WHERE rp.role_id = r.id) FROM rbac_user_role_assignments a JOIN users u ON u.id = a.user_id JOIN roles r ON r.id = a.role_id WHERE u.email = $1 AND a.valid_to IS NULL`, []any{baseTestCreds.BranchEmail}, "staging-branch-user:0"},
		{"no-access user has no assignment", `SELECT COUNT(*)::text FROM rbac_user_role_assignments a JOIN users u ON u.id = a.user_id WHERE u.email = $1`, []any{baseTestCreds.NoAccessEmail}, "0"},
		// AP matching policy, PO and GRN.
		{"one supplier-scoped zero-tolerance matching policy", `SELECT COUNT(*)::text FROM ap_matching_policies WHERE supplier_id = $1 AND company_id IS NULL AND qty_tolerance_pct = 0 AND price_tolerance_pct = 0 AND total_tolerance_amt = 0 AND effective_from <= CURRENT_DATE`, []any{f.SupplierID}, "1"},
		{"no global matching policy (ISO-004 guard)", `SELECT COUNT(*)::text FROM ap_matching_policies WHERE company_id IS NULL AND supplier_id IS NULL`, nil, "0"},
		{"grn references an approved PO of the same company and supplier", `SELECT (po.status = 'APPROVED' AND po.company_id = g.company_id AND po.supplier_id = g.supplier_id)::text FROM grns g JOIN pos po ON po.id = g.po_id WHERE g.id = $1`, []any{f.GRNID}, "true"},
		{"po has one line matching the grn line", `SELECT COUNT(*)::text FROM grns g JOIN po_lines pl ON pl.po_id = g.po_id JOIN grn_lines gl ON gl.grn_id = g.id AND gl.product_id = pl.product_id AND gl.qty = pl.qty AND gl.unit_cost = pl.price WHERE g.id = $1 AND pl.product_id = $2`, []any{f.GRNID, f.ProductID}, "1"},
		// Ledger prerequisites.
		{"chart of accounts", `SELECT string_agg(code, ',' ORDER BY code) FROM accounts`, nil, "1100,1200,1300,2100,4000,4900,5000,5900"},
		{"account mappings", `SELECT string_agg(m.module || '/' || m.key || '=' || a.code, ',' ORDER BY m.module, m.key) FROM account_mappings m JOIN accounts a ON a.id = m.account_id`, nil,
			"AP/ap.invoice.ap=2100,AP/ap.invoice.expense=5000,AP/ap.invoice.inventory=1300,AP/ap.payment.ap=2100,AP/ap.payment.cash=1100,AR/ar.invoice.ar=1200,AR/ar.invoice.revenue=4000,INVENTORY/inventory.adjustment.gain=4900,INVENTORY/inventory.adjustment.inventory=1300,INVENTORY/inventory.adjustment.loss=5900"},
		{"one OPEN period covers today", `SELECT COUNT(*)::text FROM periods WHERE status = 'OPEN' AND CURRENT_DATE BETWEEN start_date AND end_date`, nil, "1"},
		{"branch user has a completed REVOKE review", `SELECT COUNT(*)::text FROM rbac_access_reviews r JOIN users u ON u.id = r.subject_user_id WHERE u.email = $1 AND r.company_id = $2 AND r.status = 'COMPLETED' AND r.decision = 'REVOKE' AND r.decided_by_user_id IS NOT NULL AND r.decided_at IS NOT NULL`, []any{baseTestCreds.BranchEmail, f.CompanyID}, "1"},
	}
	for _, c := range checks {
		if got := d.scalar(t, c.sql, c.args...); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}

	// /api/me lists the companies of ListUserCompanies when
	// RBAC_GLOBAL_ROLES is empty (internal/app/workspace.go): the admin sees
	// exactly the primary company, the no-access user none.
	q := sqlc.New(d.Pool)
	for email, want := range map[string]string{
		baseTestCreds.AdminEmail:    fmt.Sprintf("[%d]", f.CompanyID),
		baseTestCreds.BranchEmail:   fmt.Sprintf("[%d]", f.CompanyID),
		baseTestCreds.NoAccessEmail: "[]",
	} {
		var uid int64
		if err := d.Pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&uid); err != nil {
			t.Fatal(err)
		}
		rows, err := q.ListUserCompanies(ctx, uid)
		if err != nil {
			t.Fatalf("ListUserCompanies(%s): %v", email, err)
		}
		ids := []int64{}
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		if got := fmt.Sprint(ids); got != want {
			t.Errorf("workspace companies of %s = %s, want %s", email, got, want)
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
	// The PO and GRN lines are only inserted together with a new PO and GRN,
	// and accounts are only looked at for absent mappings, so a rerun has
	// two plus len(baseLedgerAccounts) steps fewer than the first run and
	// every step is a reuse; nothing is left to revoke.
	if second.count("created") != 0 || second.count("reused") != len(second.Steps) || len(second.Steps) != len(first.Steps)-2-len(baseLedgerAccounts) || len(second.Revoked) != 0 || second.GRNCreated || second.POID != first.POID {
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

// TestBaseSeedDBFreshGRNAfterInvoicing: a dispatch consumes the GRN
// (ErrAlreadyInvoiced), so the next run provisions a fresh PO and GRN and
// leaves the invoiced pair untouched. A PO-less GRN left by the rc.10 seeder
// is never reused or rebound.
func TestBaseSeedDBFreshGRNAfterInvoicing(t *testing.T) {
	d := newBaseTestDB(t)
	ctx := context.Background()

	first, err := runBaseSeed(ctx, d.opts())
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	f := first.Fixtures
	d.exec(t, `INSERT INTO ap_invoices (number, supplier_id, grn_id, total, status) VALUES ('AP-CONSUMED-1', $1, $2, 500000, 'DRAFT')`, f.SupplierID, f.GRNID)
	// An rc.10-shaped certification GRN: same note, no PO.
	d.exec(t, `INSERT INTO grns (number, supplier_id, warehouse_id, status, note, company_id) VALUES ('GRN-LEGACY-1', $1, $2, 'POSTED', $3, $4)`, f.SupplierID, f.WarehouseID, baseGRNNote, f.CompanyID)
	invoicedSnap := d.scalar(t, `SELECT g::text || '|' || po::text FROM grns g JOIN pos po ON po.id = g.po_id WHERE g.id = $1`, f.GRNID)

	second, err := runBaseSeed(ctx, d.opts())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !second.GRNCreated || second.Fixtures.GRNID == f.GRNID || second.POID == first.POID {
		t.Fatalf("second run: GRNCreated=%v grn %d->%d po %d->%d; want a fresh PO and GRN", second.GRNCreated, f.GRNID, second.Fixtures.GRNID, first.POID, second.POID)
	}
	created := map[string]bool{}
	for _, s := range second.Steps {
		if s.Status == "created" {
			created[s.Name] = true
		}
	}
	if fmt.Sprint(created) != fmt.Sprint(map[string]bool{"po_primary": true, "po_line_primary": true, "grn_primary": true, "grn_line_primary": true}) {
		t.Errorf("second run created %v; want only the fresh PO, PO line, GRN and GRN line", created)
	}
	if got := d.scalar(t, `SELECT g::text || '|' || po::text FROM grns g JOIN pos po ON po.id = g.po_id WHERE g.id = $1`, f.GRNID); got != invoicedSnap {
		t.Errorf("invoiced GRN or its PO changed:\nbefore %s\nafter  %s", invoicedSnap, got)
	}
	if got := d.scalar(t, `SELECT COALESCE(po_id::text, 'NULL') FROM grns WHERE number = 'GRN-LEGACY-1'`); got != "NULL" {
		t.Errorf("legacy PO-less GRN was rebound to po %s", got)
	}
	if got := d.scalar(t, `SELECT (g.po_id <> $2 AND NOT EXISTS (SELECT 1 FROM ap_invoices a WHERE a.grn_id = g.id))::text FROM grns g WHERE g.id = $1`, second.Fixtures.GRNID, first.POID); got != "true" {
		t.Errorf("fresh GRN must be un-invoiced with its own PO")
	}

	// A third run reuses the fresh pair.
	third, err := runBaseSeed(ctx, d.opts())
	if err != nil {
		t.Fatalf("third run: %v", err)
	}
	if third.GRNCreated || third.Fixtures.GRNID != second.Fixtures.GRNID || third.count("created") != 0 {
		t.Errorf("third run: GRNCreated=%v grn=%d created=%d; want a reuse of grn %d", third.GRNCreated, third.Fixtures.GRNID, third.count("created"), second.Fixtures.GRNID)
	}
}

// TestBaseSeedDBCheckMode: --check rolls back, so it writes nothing and
// reports missing rows on an unseeded database and none after a seed.
func TestBaseSeedDBCheckMode(t *testing.T) {
	d := newBaseTestDB(t)
	ctx := context.Background()
	check := d.opts()
	check.Check = true

	before := d.snapshot(t)
	res, err := runBaseSeed(ctx, check)
	if err != nil {
		t.Fatalf("check on unseeded db: %v", err)
	}
	if !res.Check || res.count("created") == 0 || len(res.Revoked) == 0 {
		t.Fatalf("check on unseeded db: check=%v missing=%d revoke=%d; want missing rows and pending revocations", res.Check, res.count("created"), len(res.Revoked))
	}
	if got := d.snapshot(t); fmt.Sprint(got) != fmt.Sprint(before) {
		t.Fatalf("--check wrote to the database")
	}

	if _, err := runBaseSeed(ctx, d.opts()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	afterSeed := d.snapshot(t)
	res, err = runBaseSeed(ctx, check)
	if err != nil {
		t.Fatalf("check after seed: %v", err)
	}
	if res.count("created") != 0 || len(res.Revoked) != 0 {
		t.Fatalf("check after seed: %d missing, %d to revoke; want none", res.count("created"), len(res.Revoked))
	}
	if got := d.snapshot(t); fmt.Sprint(got) != fmt.Sprint(afterSeed) {
		t.Fatalf("--check after seed wrote to the database")
	}
}

// TestBaseSeedDBNeverOverwritesOperatorRows: existing accounts, mappings and
// an OPEN period are reused as they are, and role 1 named Administrator keeps
// its grants.
func TestBaseSeedDBNeverOverwritesOperatorRows(t *testing.T) {
	d := newBaseTestDB(t)
	ctx := context.Background()
	d.exec(t, `INSERT INTO accounts (code, name, type) VALUES ('1200', 'Operator AR', 'ASSET'), ('1299', 'Operator Other AR', 'ASSET')`)
	d.exec(t, `INSERT INTO account_mappings (module, key, account_id) SELECT 'AR', 'ar.invoice.ar', id FROM accounts WHERE code = '1299'`)
	d.exec(t, `INSERT INTO periods (code, start_date, end_date, status) VALUES ('OPS-WIDE', CURRENT_DATE - 400, CURRENT_DATE + 400, 'OPEN')`)
	d.exec(t, `UPDATE roles SET name = 'Administrator' WHERE id = 1`)
	roleOne := d.scalar(t, `SELECT COUNT(*)::text FROM role_permissions WHERE role_id = 1`)

	res, err := runBaseSeed(ctx, d.opts())
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if len(res.Revoked) != 0 {
		t.Errorf("revoked %v from a role 1 named Administrator", res.Revoked)
	}
	for name, c := range map[string]struct{ sql, want string }{
		"role 1 grants kept":       {`SELECT COUNT(*)::text FROM role_permissions WHERE role_id = 1`, roleOne},
		"operator account 1200":    {`SELECT name FROM accounts WHERE code = '1200'`, "Operator AR"},
		"operator mapping kept":    {`SELECT a.code FROM account_mappings m JOIN accounts a ON a.id = m.account_id WHERE m.module = 'AR' AND m.key = 'ar.invoice.ar'`, "1299"},
		"no extra period":          {`SELECT string_agg(code, ',') FROM periods`, "OPS-WIDE"},
		"other mappings completed": {`SELECT COUNT(*)::text FROM account_mappings`, fmt.Sprint(len(baseLedgerMappings))},
	} {
		if got := d.scalar(t, c.sql); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

// TestBaseSeedDBCreatesOnlyAccountsForMissingMappings: accounts are created
// (or verified) only for the codes that an absent mapping needs. Codes whose
// mappings already exist are neither created nor inspected, even when an
// operator account with that code has a type the seeder would refuse.
func TestBaseSeedDBCreatesOnlyAccountsForMissingMappings(t *testing.T) {
	d := newBaseTestDB(t)
	ctx := context.Background()
	// Operator chart: every mapping except the inventory gain points at an
	// operator account, and code 1100 is an inactive expense account that
	// the seeder must leave alone because ap.payment.cash is already mapped.
	d.exec(t, `INSERT INTO accounts (code, name, type) VALUES ('OPS-ANY', 'Operator account', 'ASSET')`)
	d.exec(t, `INSERT INTO accounts (code, name, type, is_active) VALUES ('1100', 'Operator retired expense', 'EXPENSE', FALSE)`)
	for _, m := range baseLedgerMappings {
		if m.Key == "inventory.adjustment.gain" {
			continue
		}
		d.exec(t, `INSERT INTO account_mappings (module, key, account_id) SELECT $1, $2, id FROM accounts WHERE code = 'OPS-ANY'`, m.Module, m.Key)
	}

	res, err := runBaseSeed(ctx, d.opts())
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	var accountSteps []string
	for _, s := range res.Steps {
		if s.Table == "accounts" {
			accountSteps = append(accountSteps, s.Name+"="+s.Status)
		}
	}
	if got, want := strings.Join(accountSteps, ","), "account 4900=created"; got != want {
		t.Errorf("account steps = %q, want %q", got, want)
	}
	for name, c := range map[string]struct{ sql, want string }{
		"chart":         {`SELECT string_agg(code || ':' || type::text || ':' || is_active::text, ',' ORDER BY code) FROM accounts`, "1100:EXPENSE:false,4900:REVENUE:true,OPS-ANY:ASSET:true"},
		"gain mapping":  {`SELECT a.code FROM account_mappings m JOIN accounts a ON a.id = m.account_id WHERE m.module = 'INVENTORY' AND m.key = 'inventory.adjustment.gain'`, "4900"},
		"mapping count": {`SELECT COUNT(*)::text FROM account_mappings`, fmt.Sprint(len(baseLedgerMappings))},
	} {
		if got := d.scalar(t, c.sql); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

// TestBaseSeedDBRefusesScopeConflicts: rows that would break the tenancy or
// ledger fixtures abort the run, roll back and stay untouched.
func TestBaseSeedDBRefusesScopeConflicts(t *testing.T) {
	cases := []struct {
		name    string
		plant   func(t *testing.T, d *baseTestDB)
		wantErr string
	}{
		{
			name: "admin with an active assignment in the other company",
			plant: func(t *testing.T, d *baseTestDB) {
				if _, err := runBaseSeed(context.Background(), d.opts()); err != nil {
					t.Fatalf("seed: %v", err)
				}
				d.exec(t, `INSERT INTO rbac_user_role_assignments (company_id, user_id, role_id, valid_from)
					SELECT c.id, u.id, r.id, NOW() - INTERVAL '1 day' FROM companies c, users u, roles r
					WHERE c.code = 'ODY-02' AND u.email = $1 AND LOWER(r.name) = 'admin'`, baseTestCreds.AdminEmail)
			},
			wantErr: "user_admin (id=",
		},
		{
			name: "no-access user with an active assignment",
			plant: func(t *testing.T, d *baseTestDB) {
				if _, err := runBaseSeed(context.Background(), d.opts()); err != nil {
					t.Fatalf("seed: %v", err)
				}
				d.exec(t, `INSERT INTO rbac_user_role_assignments (company_id, user_id, role_id, valid_from)
					SELECT c.id, u.id, r.id, NOW() - INTERVAL '1 day' FROM companies c, users u, roles r
					WHERE c.code = 'ODY-01' AND u.email = $1 AND r.name = 'staging-branch-user'`, baseTestCreds.NoAccessEmail)
			},
			wantErr: "user_no_access (id=",
		},
		{
			name: "closed period overlapping the current month",
			plant: func(t *testing.T, d *baseTestDB) {
				d.exec(t, `INSERT INTO periods (code, start_date, end_date, status) VALUES ('OPS-CLOSED', date_trunc('month', CURRENT_DATE)::date, (date_trunc('month', CURRENT_DATE) + INTERVAL '1 month - 1 day')::date, 'CLOSED')`)
			},
			wantErr: "overlaps existing period(s) OPS-CLOSED (CLOSED)",
		},
		{
			name: "gain account 4900 of the wrong type",
			plant: func(t *testing.T, d *baseTestDB) {
				d.exec(t, `INSERT INTO accounts (code, name, type) VALUES ('4900', 'Operator Expense', 'EXPENSE')`)
			},
			wantErr: "account 4900 (accounts) already exists as \"EXPENSE\" but the seeder expects \"REVENUE\"",
		},
		{
			name: "loss account 5900 of the wrong type",
			plant: func(t *testing.T, d *baseTestDB) {
				d.exec(t, `INSERT INTO accounts (code, name, type) VALUES ('5900', 'Operator Revenue', 'REVENUE')`)
			},
			wantErr: "account 5900 (accounts) already exists as \"REVENUE\" but the seeder expects \"EXPENSE\"",
		},
		{
			name: "inactive AP account 2100",
			plant: func(t *testing.T, d *baseTestDB) {
				d.exec(t, `INSERT INTO accounts (code, name, type, is_active) VALUES ('2100', 'Operator AP', 'LIABILITY', FALSE)`)
			},
			wantErr: "account 2100 (accounts) already exists as \"LIABILITY inactive\" but the seeder expects \"LIABILITY\"",
		},
		{
			name: "cash account 1100 that is a liability",
			plant: func(t *testing.T, d *baseTestDB) {
				d.exec(t, `INSERT INTO accounts (code, name, type) VALUES ('1100', 'Operator Loan', 'LIABILITY')`)
			},
			wantErr: "account 1100 (accounts) already exists as \"LIABILITY\" but the seeder expects \"ASSET\"",
		},
		{
			name: "branch role that holds permissions",
			plant: func(t *testing.T, d *baseTestDB) {
				d.exec(t, `INSERT INTO roles (name) VALUES ('staging-branch-user')`)
				d.exec(t, `INSERT INTO role_permissions (role_id, permission_id) SELECT r.id, p.id FROM roles r, permissions p WHERE r.name = 'staging-branch-user' AND p.name = 'finance.ap.view'`)
			},
			wantErr: "role_branch_user (roles) already exists as \"has permissions\"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newBaseTestDB(t)
			tc.plant(t, d)
			snap := d.snapshot(t)
			if _, err := runBaseSeed(context.Background(), d.opts()); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
			if got := d.snapshot(t); fmt.Sprint(got) != fmt.Sprint(snap) {
				t.Fatalf("refused run modified the database")
			}
		})
	}
}
