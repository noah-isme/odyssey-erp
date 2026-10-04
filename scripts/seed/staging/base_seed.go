package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// Base staging seeder.
//
// The base seeder writes into a shared staging database, so it follows the
// same rules as the --iso004 extension:
//
//   - it runs only behind the shared staging guard (staging_guard.go);
//   - every INSERT is `ON CONFLICT DO NOTHING RETURNING ...`; when nothing is
//     returned the existing row is read back by its natural key and must
//     belong to the expected company/parent, otherwise the whole run aborts
//     and rolls back (fail closed);
//   - it never UPDATEs or DELETEs a row, so a row it did not create in this
//     run is never changed (base_seed_sql_test.go enforces this by scanning
//     the SQL literals in this file).
//
// The schema notes below are for migration ceiling 000124.

// Fixture identity. Codes that are globally unique in the schema (branches,
// warehouses, units, categories, products, suppliers, companies) are the
// natural key; customers and the document tables are unique per company.
const (
	baseCompanyCode      = "ODY-01"
	baseCompanyName      = "PT Odyssey Utama"
	baseOtherCompanyCode = "ODY-02"
	baseOtherCompanyName = "PT Odyssey Cabang"
	baseBranchCode       = "HQ-JKT"
	baseOtherBranchCode  = "BR-BDG"
	baseWarehouseCode    = "WH-JKT-01"
	baseUnitCode         = "EA"
	baseCategoryCode     = "CAT-GEN"
	baseProductSKU       = "PROD-STG-01"
	baseCustomerCode     = "CUST-STG-01"
	baseSupplierCode     = "SUPP-STG-01"
	baseOtherCustomer    = "CUST-OTHER-01"
	baseOtherInvoiceNo   = "AR-STG-OTHER-01"
	baseGRNNote          = "v0.10-core staging certification GRN"
	baseClassCode        = "CONFIDENTIAL"
	baseDocCategoryCode  = "CERT-DOCS"
	baseNumberingCode    = "DOC-RULE-STG"
	baseRetentionCode    = "RET-7Y-COMPLIANCE"
	baseReviewKey        = "STAGING-REVOKE-ISO003"
	baseAmount           = "100000"

	// baseRetentionDays is the statutory 7-year (COMPLIANCE) retention period.
	baseRetentionDays = 2557
	// baseMinAvgCost is the lowest average cost the seeded stock may carry.
	baseMinAvgCost = 1000
)

// baseCredentials are the three test identities.
type baseCredentials struct {
	AdminEmail       string
	AdminPassword    string
	BranchEmail      string
	BranchPassword   string
	NoAccessEmail    string
	NoAccessPassword string
}

func baseCredentialsFromEnv() baseCredentials {
	return baseCredentials{
		AdminEmail:       getenv("STAGING_CERT_ADMIN_EMAIL", "admin@staging.odyssey.local"),
		AdminPassword:    getenv("STAGING_CERT_ADMIN_PASSWORD", "admin123"),
		BranchEmail:      getenv("STAGING_CERT_BRANCH_EMAIL", "branch@staging.odyssey.local"),
		BranchPassword:   getenv("STAGING_CERT_BRANCH_PASSWORD", "branch123"),
		NoAccessEmail:    getenv("STAGING_CERT_NO_ACCESS_EMAIL", "noaccess@staging.odyssey.local"),
		NoAccessPassword: getenv("STAGING_CERT_NO_ACCESS_PASSWORD", "noaccess123"),
	}
}

// baseOptions are the inputs of the base seeder.
type baseOptions struct {
	Guard stagingGuardInput
	Creds baseCredentials
}

// baseStep records what happened to one fixture row.
type baseStep struct {
	Name   string
	Table  string
	Status string // created | reused
	ID     int64
}

// baseResult is everything the summary printers need.
type baseResult struct {
	Target   stagingTarget
	Fixtures *Fixtures
	Steps    []baseStep
}

func (r *baseResult) count(status string) int {
	n := 0
	for _, s := range r.Steps {
		if s.Status == status {
			n++
		}
	}
	return n
}

// runBaseSeed is the base seeder entry point: guard, connect, confirm
// current_database(), then seed in one transaction.
func runBaseSeed(ctx context.Context, opts baseOptions) (*baseResult, error) {
	target, err := checkStagingStatic(opts.Guard)
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
	if err := checkStagingCurrentDatabase(target, currentDB); err != nil {
		return nil, fmt.Errorf("staging guard: %w", err)
	}

	fixtures, steps, err := seedStagingFixtures(ctx, pool, opts.Creds)
	if err != nil {
		return nil, err
	}
	return &baseResult{Target: target, Fixtures: fixtures, Steps: steps}, nil
}

// printBaseSteps prints the per-row created/reused report.
func printBaseSteps(w io.Writer, res *baseResult) {
	_, _ = fmt.Fprintf(w, "Base seeder on %s@%s: %d created, %d reused\n",
		res.Target.Database, res.Target.Host, res.count("created"), res.count("reused"))
	for _, s := range res.Steps {
		_, _ = fmt.Fprintf(w, "  %-8s %-28s %-30s id=%d\n", s.Status, s.Name, s.Table, s.ID)
	}
}

// baseFixture describes one fixture row. Insert must be
// `INSERT ... ON CONFLICT DO NOTHING RETURNING <id column>`. Lookup reads the
// row by its natural key and returns (id, state); state must equal Want for
// an existing row to be reused.
type baseFixture struct {
	Name       string
	Table      string
	Insert     string
	InsertArgs []any
	Lookup     string
	LookupArgs []any
	Want       string
	// LookupFirst is for tables without a usable unique key: the lookup runs
	// before the insert so a rerun cannot create a duplicate.
	LookupFirst bool
}

type baseSeeder struct {
	tx    pgx.Tx
	steps []baseStep
}

func (s *baseSeeder) record(name, table, status string, id int64) {
	s.steps = append(s.steps, baseStep{Name: name, Table: table, Status: status, ID: id})
}

func (s *baseSeeder) lookup(ctx context.Context, f baseFixture) (id int64, state string, found bool, err error) {
	err = s.tx.QueryRow(ctx, f.Lookup, f.LookupArgs...).Scan(&id, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("look up %s (%s): %w", f.Name, f.Table, err)
	}
	return id, state, true, nil
}

func (s *baseSeeder) refuse(f baseFixture, state string) error {
	return fmt.Errorf("%s (%s) already exists as %q but the seeder expects %q; the seeder never changes existing rows, so refusing to reuse it", f.Name, f.Table, state, f.Want)
}

// reuse validates an existing row and records it as reused.
func (s *baseSeeder) reuse(f baseFixture, id int64, state string) (int64, bool, error) {
	if state != f.Want {
		return 0, false, s.refuse(f, state)
	}
	s.record(f.Name, f.Table, "reused", id)
	return id, false, nil
}

// ensure returns the row's id and whether this run created it.
func (s *baseSeeder) ensure(ctx context.Context, f baseFixture) (int64, bool, error) {
	if f.LookupFirst {
		id, state, found, err := s.lookup(ctx, f)
		if err != nil {
			return 0, false, err
		}
		if found {
			return s.reuse(f, id, state)
		}
		err = s.tx.QueryRow(ctx, f.Insert, f.InsertArgs...).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, fmt.Errorf("insert %s (%s) was suppressed by a conflict the lookup cannot see; refusing", f.Name, f.Table)
		}
		if err != nil {
			return 0, false, fmt.Errorf("insert %s (%s): %w", f.Name, f.Table, err)
		}
		s.record(f.Name, f.Table, "created", id)
		return id, true, nil
	}

	var id int64
	err := s.tx.QueryRow(ctx, f.Insert, f.InsertArgs...).Scan(&id)
	switch {
	case err == nil:
		s.record(f.Name, f.Table, "created", id)
		return id, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		// ON CONFLICT DO NOTHING suppressed the insert: accept only an
		// existing row the natural-key lookup returns in the expected state.
		id, state, found, err := s.lookup(ctx, f)
		if err != nil {
			return 0, false, err
		}
		if !found {
			return 0, false, fmt.Errorf("insert %s (%s) conflicted with an existing row that the natural-key lookup does not return; refusing", f.Name, f.Table)
		}
		return s.reuse(f, id, state)
	default:
		return 0, false, fmt.Errorf("insert %s (%s): %w", f.Name, f.Table, err)
	}
}

func (s *baseSeeder) mustEnsure(ctx context.Context, f baseFixture) (int64, error) {
	id, _, err := s.ensure(ctx, f)
	return id, err
}

// insertChild inserts a row that has no natural key and exists only together
// with a parent created in this run. It must return a row.
func (s *baseSeeder) insertChild(ctx context.Context, name, table, sql string, args ...any) error {
	var id int64
	err := s.tx.QueryRow(ctx, sql, args...).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("insert %s (%s) was suppressed by a conflict; refusing", name, table)
	}
	if err != nil {
		return fmt.Errorf("insert %s (%s): %w", name, table, err)
	}
	s.record(name, table, "created", id)
	return nil
}

// ensureUser creates a test identity or accepts an existing one whose active
// flag and password already match the configured credentials. An existing
// user is never modified.
func (s *baseSeeder) ensureUser(ctx context.Context, name, email, password string) (int64, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("hash %s password: %w", name, err)
	}
	var id int64
	err = s.tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, is_active, created_at, updated_at)
		VALUES ($1, $2, TRUE, NOW(), NOW())
		ON CONFLICT DO NOTHING RETURNING id`, email, string(hash)).Scan(&id)
	switch {
	case err == nil:
		s.record(name, "users", "created", id)
		return id, nil
	case errors.Is(err, pgx.ErrNoRows):
		var existingHash string
		var active bool
		err = s.tx.QueryRow(ctx, `SELECT id, password_hash, is_active FROM users WHERE email = $1`, email).Scan(&id, &existingHash, &active)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("insert %s (users) conflicted with an existing row that the email lookup does not return; refusing", name)
		}
		if err != nil {
			return 0, fmt.Errorf("look up %s (users): %w", name, err)
		}
		if !active {
			return 0, fmt.Errorf("%s (users) %q already exists but is inactive; the seeder never changes existing users, so refusing", name, email)
		}
		if bcrypt.CompareHashAndPassword([]byte(existingHash), []byte(password)) != nil {
			return 0, fmt.Errorf("%s (users) %q already exists with a different password; the seeder never changes existing users, so refusing (set the matching STAGING_CERT_*_PASSWORD or use another email)", name, email)
		}
		s.record(name, "users", "reused", id)
		return id, nil
	default:
		return 0, fmt.Errorf("insert %s (users): %w", name, err)
	}
}

// requireNoGlobalRoles aborts when a user that must stay role-less has a
// global user_roles row. The seeder never deletes it.
func (s *baseSeeder) requireNoGlobalRoles(ctx context.Context, name string, userID int64) error {
	var n int64
	if err := s.tx.QueryRow(ctx, `SELECT COUNT(*) FROM user_roles WHERE user_id = $1`, userID).Scan(&n); err != nil {
		return fmt.Errorf("count global roles of %s: %w", name, err)
	}
	if n != 0 {
		return fmt.Errorf("%s (id=%d) has %d global user_roles row(s) but must have none for the ISO-003 fixtures; the seeder never deletes role rows, so refusing", name, userID, n)
	}
	return nil
}

func companyState(id int64) string { return fmt.Sprintf("company:%d", id) }

// seedStagingFixtures seeds the base certification fixtures in one
// transaction. Callers must have passed the staging guard (runBaseSeed does).
// The returned steps say which rows were created in this run and which
// pre-existing rows were verified and reused.
func seedStagingFixtures(ctx context.Context, pool *pgxpool.Pool, creds baseCredentials) (*Fixtures, []baseStep, error) {
	seen := map[string]string{}
	for _, id := range []struct{ label, email string }{
		{"admin", creds.AdminEmail}, {"branch", creds.BranchEmail}, {"no-access", creds.NoAccessEmail},
	} {
		key := strings.ToLower(strings.TrimSpace(id.email))
		if key == "" {
			return nil, nil, fmt.Errorf("%s test identity email is empty", id.label)
		}
		if other, dup := seen[key]; dup {
			return nil, nil, fmt.Errorf("%s and %s test identities share the email %q; they must be distinct", other, id.label, id.email)
		}
		seen[key] = id.label
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize concurrent seeds so lookup-then-insert on tables without a
	// usable unique key cannot race.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('odyssey:seed:staging'))`); err != nil {
		return nil, nil, fmt.Errorf("acquire seed advisory lock: %w", err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL statement_timeout = '60s'`); err != nil {
		return nil, nil, fmt.Errorf("set statement_timeout: %w", err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '15s'`); err != nil {
		return nil, nil, fmt.Errorf("set lock_timeout: %w", err)
	}

	s := &baseSeeder{tx: tx}

	// 1. Test identities. Created first because customers and the document
	// tables require created_by NOT NULL.
	adminUserID, err := s.ensureUser(ctx, "user_admin", creds.AdminEmail, creds.AdminPassword)
	if err != nil {
		return nil, nil, err
	}
	branchUserID, err := s.ensureUser(ctx, "user_branch", creds.BranchEmail, creds.BranchPassword)
	if err != nil {
		return nil, nil, err
	}
	noAccessUserID, err := s.ensureUser(ctx, "user_no_access", creds.NoAccessEmail, creds.NoAccessPassword)
	if err != nil {
		return nil, nil, err
	}

	// Admin role: reuse any existing 'admin' role, create it only if missing.
	var adminRoleID int64
	err = tx.QueryRow(ctx, `SELECT id FROM roles WHERE LOWER(TRIM(name)) = 'admin' ORDER BY id LIMIT 1`).Scan(&adminRoleID)
	switch {
	case err == nil:
		s.record("role_admin", "roles", "reused", adminRoleID)
	case errors.Is(err, pgx.ErrNoRows):
		adminRoleID, _, err = s.ensure(ctx, baseFixture{
			Name: "role_admin", Table: "roles",
			Insert:     `INSERT INTO roles (name, description, created_at, updated_at) VALUES ($1, $2, NOW(), NOW()) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: []any{"admin", "System Administrator"},
			Lookup:     `SELECT id, 'ok' FROM roles WHERE LOWER(TRIM(name)) = 'admin' ORDER BY id LIMIT 1`,
			Want:       "ok",
		})
		if err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, fmt.Errorf("look up admin role: %w", err)
	}

	if _, _, err := s.ensure(ctx, baseFixture{
		Name: "user_admin_role", Table: "user_roles",
		Insert:     `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING RETURNING user_id`,
		InsertArgs: []any{adminUserID, adminRoleID},
		Lookup:     `SELECT user_id, 'ok' FROM user_roles WHERE user_id = $1 AND role_id = $2`,
		LookupArgs: []any{adminUserID, adminRoleID},
		Want:       "ok",
	}); err != nil {
		return nil, nil, err
	}

	// The branch and no-access users must have no global roles (the ISO-003
	// fixtures prove scoped, not global, access). Verified, never deleted.
	if err := s.requireNoGlobalRoles(ctx, "user_branch", branchUserID); err != nil {
		return nil, nil, err
	}
	if err := s.requireNoGlobalRoles(ctx, "user_no_access", noAccessUserID); err != nil {
		return nil, nil, err
	}

	// 2. Companies. companies.code is globally unique.
	primaryCompanyID, err := s.mustEnsure(ctx, baseFixture{
		Name: "company_primary", Table: "companies",
		Insert:     `INSERT INTO companies (code, name, address, tax_id) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{baseCompanyCode, baseCompanyName, "Jl. Sudirman No. 100, Jakarta", "01.234.567.8-901.000"},
		Lookup:     `SELECT id, name FROM companies WHERE code = $1`,
		LookupArgs: []any{baseCompanyCode},
		Want:       baseCompanyName,
	})
	if err != nil {
		return nil, nil, err
	}
	otherCompanyID, err := s.mustEnsure(ctx, baseFixture{
		Name: "company_other", Table: "companies",
		Insert:     `INSERT INTO companies (code, name, address, tax_id) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{baseOtherCompanyCode, baseOtherCompanyName, "Jl. Asia Afrika No. 50, Bandung", "02.345.678.9-012.000"},
		Lookup:     `SELECT id, name FROM companies WHERE code = $1`,
		LookupArgs: []any{baseOtherCompanyCode},
		Want:       baseOtherCompanyName,
	})
	if err != nil {
		return nil, nil, err
	}

	// 3. Branches. branches.code is globally unique, so an existing code must
	// already belong to the expected company.
	primaryBranchID, err := s.mustEnsure(ctx, baseFixture{
		Name: "branch_primary", Table: "branches",
		Insert:     `INSERT INTO branches (company_id, code, name, address) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{primaryCompanyID, baseBranchCode, "Kantor Pusat Jakarta", "Jl. Sudirman No. 100, Jakarta"},
		Lookup:     `SELECT id, 'company:' || company_id FROM branches WHERE code = $1`,
		LookupArgs: []any{baseBranchCode},
		Want:       companyState(primaryCompanyID),
	})
	if err != nil {
		return nil, nil, err
	}
	otherBranchID, err := s.mustEnsure(ctx, baseFixture{
		Name: "branch_other", Table: "branches",
		Insert:     `INSERT INTO branches (company_id, code, name, address) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{otherCompanyID, baseOtherBranchCode, "Kantor Bandung", "Jl. Asia Afrika No. 50, Bandung"},
		Lookup:     `SELECT id, 'company:' || company_id FROM branches WHERE code = $1`,
		LookupArgs: []any{baseOtherBranchCode},
		Want:       companyState(otherCompanyID),
	})
	if err != nil {
		return nil, nil, err
	}

	// 4. Warehouse. warehouses.code is globally unique; its company is the
	// company of its branch.
	warehouseID, err := s.mustEnsure(ctx, baseFixture{
		Name: "warehouse_primary", Table: "warehouses",
		Insert:     `INSERT INTO warehouses (branch_id, code, name, address) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{primaryBranchID, baseWarehouseCode, "Gudang Jakarta Pusat", "Jl. Industri No. 1, Jakarta"},
		Lookup:     `SELECT id, 'branch:' || branch_id FROM warehouses WHERE code = $1`,
		LookupArgs: []any{baseWarehouseCode},
		Want:       fmt.Sprintf("branch:%d", primaryBranchID),
	})
	if err != nil {
		return nil, nil, err
	}

	// 5. Master data. Units are global reference data; categories and
	// products are company scoped but keyed by a globally unique code/sku.
	unitID, err := s.mustEnsure(ctx, baseFixture{
		Name: "unit", Table: "units",
		Insert:     `INSERT INTO units (code, name) VALUES ($1, $2) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{baseUnitCode, "Each"},
		Lookup:     `SELECT id, 'ok' FROM units WHERE code = $1`,
		LookupArgs: []any{baseUnitCode},
		Want:       "ok",
	})
	if err != nil {
		return nil, nil, err
	}
	categoryID, err := s.mustEnsure(ctx, baseFixture{
		Name: "category", Table: "categories",
		Insert:     `INSERT INTO categories (code, name, company_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{baseCategoryCode, "General Category", primaryCompanyID},
		Lookup:     `SELECT id, 'company:' || COALESCE(company_id::text, 'NULL') FROM categories WHERE code = $1`,
		LookupArgs: []any{baseCategoryCode},
		Want:       companyState(primaryCompanyID),
	})
	if err != nil {
		return nil, nil, err
	}
	productID, err := s.mustEnsure(ctx, baseFixture{
		Name: "product", Table: "products",
		Insert:     `INSERT INTO products (sku, name, category_id, unit_id, price, is_active, company_id) VALUES ($1, $2, $3, $4, 100000.00, TRUE, $5) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{baseProductSKU, "v0.10 Certification Product", categoryID, unitID, primaryCompanyID},
		Lookup: `SELECT id, 'company:' || COALESCE(company_id::text, 'NULL') ||
			CASE WHEN is_active AND deleted_at IS NULL THEN '' ELSE ' inactive' END
			FROM products WHERE sku = $1`,
		LookupArgs: []any{baseProductSKU},
		Want:       companyState(primaryCompanyID),
	})
	if err != nil {
		return nil, nil, err
	}

	// Stock for the product in the warehouse (stock take and valuation). The
	// primary key (warehouse_id, product_id) is made of two verified parents;
	// existing stock keeps its quantity, but its average cost must still be
	// usable for valuation.
	if _, _, err := s.ensure(ctx, baseFixture{
		Name: "inventory_balance", Table: "inventory_balances",
		Insert:     `INSERT INTO inventory_balances (warehouse_id, product_id, qty, avg_cost, updated_at) VALUES ($1, $2, 100.0000, 50000.0000, NOW()) ON CONFLICT DO NOTHING RETURNING product_id`,
		InsertArgs: []any{warehouseID, productID},
		Lookup: `SELECT product_id, CASE WHEN avg_cost >= $3 THEN 'ok' ELSE 'avg_cost below minimum' END
			FROM inventory_balances WHERE warehouse_id = $1 AND product_id = $2`,
		LookupArgs: []any{warehouseID, productID, baseMinAvgCost},
		Want:       "ok",
	}); err != nil {
		return nil, nil, err
	}

	// 6. Customer and supplier in the primary company; customers are unique
	// per (company_id, code) and require created_by.
	customerID, err := s.mustEnsure(ctx, baseFixture{
		Name: "customer_primary", Table: "customers",
		Insert: `INSERT INTO customers (code, name, phone, email, address_line1, is_active, company_id, created_by)
			VALUES ($1, $2, $3, $4, $5, TRUE, $6, $7) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{baseCustomerCode, "PT Pelanggan Sertifikasi Staging", "081234567890", "customer.staging@odyssey.local", "Jl. Pelanggan No. 1", primaryCompanyID, adminUserID},
		Lookup: `SELECT id, 'company:' || company_id || CASE WHEN is_active THEN '' ELSE ' inactive' END
			FROM customers WHERE company_id = $1 AND code = $2`,
		LookupArgs: []any{primaryCompanyID, baseCustomerCode},
		Want:       companyState(primaryCompanyID),
	})
	if err != nil {
		return nil, nil, err
	}
	supplierID, err := s.mustEnsure(ctx, baseFixture{
		Name: "supplier_primary", Table: "suppliers",
		Insert: `INSERT INTO suppliers (code, name, phone, email, address, is_active, company_id)
			VALUES ($1, $2, $3, $4, $5, TRUE, $6) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{baseSupplierCode, "PT Pemasok Sertifikasi Staging", "081234567891", "supplier.staging@odyssey.local", "Jl. Pemasok No. 1", primaryCompanyID},
		Lookup: `SELECT id, 'company:' || COALESCE(company_id::text, 'NULL') || CASE WHEN is_active THEN '' ELSE ' inactive' END
			FROM suppliers WHERE code = $1`,
		LookupArgs: []any{baseSupplierCode},
		Want:       companyState(primaryCompanyID),
	})
	if err != nil {
		return nil, nil, err
	}

	// 7. Customer and AR invoice in the other company (tenant isolation).
	otherCustomerID, err := s.mustEnsure(ctx, baseFixture{
		Name: "customer_other", Table: "customers",
		Insert: `INSERT INTO customers (code, name, phone, email, address_line1, is_active, company_id, created_by)
			VALUES ($1, $2, $3, $4, $5, TRUE, $6, $7) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{baseOtherCustomer, "PT Pelanggan Cabang Bandung", "081234567892", "customer.other@odyssey.local", "Jl. Asia Afrika No. 51", otherCompanyID, adminUserID},
		Lookup: `SELECT id, 'company:' || company_id || CASE WHEN is_active THEN '' ELSE ' inactive' END
			FROM customers WHERE company_id = $1 AND code = $2`,
		LookupArgs: []any{otherCompanyID, baseOtherCustomer},
		Want:       companyState(otherCompanyID),
	})
	if err != nil {
		return nil, nil, err
	}
	if _, _, err := s.ensure(ctx, baseFixture{
		Name: "ar_invoice_other", Table: "ar_invoices",
		Insert: `INSERT INTO ar_invoices (number, customer_id, currency, total, status, due_at, created_at, updated_at)
			VALUES ($1, $2, 'IDR', 100000.00, 'POSTED', NOW() + INTERVAL '30 days', NOW(), NOW()) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{baseOtherInvoiceNo, otherCustomerID},
		Lookup: `SELECT i.id, 'company:' || c.company_id FROM ar_invoices i JOIN customers c ON c.id = i.customer_id
			WHERE i.number = $1`,
		LookupArgs: []any{baseOtherInvoiceNo},
		Want:       companyState(otherCompanyID),
	}); err != nil {
		return nil, nil, err
	}

	// 8. Un-invoiced POSTED GRN in the primary company. Only a GRN this
	// seeder created (same note, supplier and warehouse) is reused, so a real
	// GRN of the company is never claimed for the certification.
	grnID, grnCreated, err := s.ensure(ctx, baseFixture{
		Name: "grn_primary", Table: "grns",
		Insert: `INSERT INTO grns (number, supplier_id, warehouse_id, status, received_at, note, created_at, company_id)
			VALUES ($1, $2, $3, 'POSTED', NOW(), $4, NOW(), $5) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{fmt.Sprintf("GRN-STG-%d", time.Now().UnixNano()), supplierID, warehouseID, baseGRNNote, primaryCompanyID},
		Lookup: `SELECT g.id, 'company:' || g.company_id FROM grns g
			WHERE g.company_id = $1 AND g.status = 'POSTED' AND g.supplier_id = $2 AND g.warehouse_id = $3 AND g.note = $4
			  AND NOT EXISTS (SELECT 1 FROM ap_invoices ai WHERE ai.grn_id = g.id)
			ORDER BY g.id DESC LIMIT 1`,
		LookupArgs:  []any{primaryCompanyID, supplierID, warehouseID, baseGRNNote},
		Want:        companyState(primaryCompanyID),
		LookupFirst: true,
	})
	if err != nil {
		return nil, nil, err
	}
	if grnCreated {
		if err := s.insertChild(ctx, "grn_line_primary", "grn_lines",
			`INSERT INTO grn_lines (grn_id, product_id, qty, unit_cost) VALUES ($1, $2, 10.0000, 50000.0000) ON CONFLICT DO NOTHING RETURNING id`,
			grnID, productID); err != nil {
			return nil, nil, err
		}
	}

	// 9. Expired branch role assignment and completed REVOKE access review
	// (ISO-003). Assignments have no natural key (valid_from is part of it),
	// so the lookup runs first.
	if _, _, err := s.ensure(ctx, baseFixture{
		Name: "rbac_assignment_expired", Table: "rbac_user_role_assignments",
		Insert: `INSERT INTO rbac_user_role_assignments (company_id, user_id, role_id, branch_id, valid_from, valid_to, created_at)
			VALUES ($1, $2, $3, $4, NOW() - INTERVAL '30 days', NOW() - INTERVAL '1 day', NOW() - INTERVAL '30 days')
			ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{primaryCompanyID, branchUserID, adminRoleID, primaryBranchID},
		Lookup: `SELECT id, 'ok' FROM rbac_user_role_assignments
			WHERE company_id = $1 AND user_id = $2 AND branch_id = $3 AND valid_to IS NOT NULL AND valid_to <= NOW()
			ORDER BY id LIMIT 1`,
		LookupArgs:  []any{primaryCompanyID, branchUserID, primaryBranchID},
		Want:        "ok",
		LookupFirst: true,
	}); err != nil {
		return nil, nil, err
	}
	if _, _, err := s.ensure(ctx, baseFixture{
		Name: "rbac_access_review_revoke", Table: "rbac_access_reviews",
		Insert: `INSERT INTO rbac_access_reviews (
				company_id, subject_user_id, review_key, status, decision,
				opened_by_user_id, decided_by_user_id, decided_at, created_at, updated_at
			)
			VALUES ($1, $2, $3, 'COMPLETED', 'REVOKE', $4, $4,
				NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day')
			ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{primaryCompanyID, branchUserID, baseReviewKey, adminUserID},
		Lookup: `SELECT id, status || '/' || COALESCE(decision, 'NULL') ||
				CASE WHEN decided_by_user_id IS NOT NULL AND decided_at IS NOT NULL THEN '' ELSE ' undecided' END
			FROM rbac_access_reviews WHERE company_id = $1 AND subject_user_id = $2 AND review_key = $3`,
		LookupArgs: []any{primaryCompanyID, branchUserID, baseReviewKey},
		Want:       "COMPLETED/REVOKE",
	}); err != nil {
		return nil, nil, err
	}

	// 10. Document management and the 7-year retention policy (J-DOC-001).
	// Numbering rules, classifications and policies are unique per
	// (company_id, code).
	if _, _, err := s.ensure(ctx, baseFixture{
		Name: "document_numbering_rule", Table: "document_numbering_rules",
		Insert: `INSERT INTO document_numbering_rules (company_id, code, name, prefix, pattern, scope, active, created_by)
			VALUES ($1, $2, 'Staging Document Rule', 'DOC', '{PREFIX}-{YYYY}-{SEQ:04d}', 'COMPANY', TRUE, $3)
			ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{primaryCompanyID, baseNumberingCode, adminUserID},
		Lookup: `SELECT id, 'company:' || company_id || CASE WHEN active THEN '' ELSE ' inactive' END
			FROM document_numbering_rules WHERE company_id = $1 AND code = $2`,
		LookupArgs: []any{primaryCompanyID, baseNumberingCode},
		Want:       companyState(primaryCompanyID),
	}); err != nil {
		return nil, nil, err
	}
	docClassID, err := s.mustEnsure(ctx, baseFixture{
		Name: "document_classification", Table: "document_classifications",
		Insert: `INSERT INTO document_classifications (company_id, code, name, active, created_by)
			VALUES ($1, $2, 'Confidential Document', TRUE, $3) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{primaryCompanyID, baseClassCode, adminUserID},
		Lookup: `SELECT id, 'company:' || company_id || CASE WHEN active THEN '' ELSE ' inactive' END
			FROM document_classifications WHERE company_id = $1 AND code = $2`,
		LookupArgs: []any{primaryCompanyID, baseClassCode},
		Want:       companyState(primaryCompanyID),
	})
	if err != nil {
		return nil, nil, err
	}
	// UNIQUE (company_id, parent_id, code) treats NULL parents as distinct,
	// so a root category is never caught by ON CONFLICT: look it up first.
	docCategoryID, err := s.mustEnsure(ctx, baseFixture{
		Name: "document_category", Table: "document_categories",
		Insert: `INSERT INTO document_categories (company_id, code, name, active, created_by)
			VALUES ($1, $2, 'Certification Documents', TRUE, $3) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{primaryCompanyID, baseDocCategoryCode, adminUserID},
		Lookup: `SELECT id, 'company:' || company_id || CASE WHEN active THEN '' ELSE ' inactive' END
			FROM document_categories WHERE company_id = $1 AND parent_id IS NULL AND code = $2
			ORDER BY id LIMIT 1`,
		LookupArgs:  []any{primaryCompanyID, baseDocCategoryCode},
		Want:        companyState(primaryCompanyID),
		LookupFirst: true,
	})
	if err != nil {
		return nil, nil, err
	}
	if _, _, err := s.ensure(ctx, baseFixture{
		Name: "retention_policy", Table: "retention_policies",
		Insert: `INSERT INTO retention_policies (
				company_id, code, name, description, trigger_event,
				retention_period_days, classification_ids, category_ids, active, created_by
			)
			VALUES ($1, $2, '7-Year Compliance Retention',
				'Statutory 7-year immutable retention policy for staging certification',
				'APPROVAL', $3, ARRAY[$4::bigint], ARRAY[$5::bigint], TRUE, $6)
			ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{primaryCompanyID, baseRetentionCode, baseRetentionDays, docClassID, docCategoryID, adminUserID},
		Lookup: `SELECT id, 'company:' || company_id ||
				CASE WHEN active THEN '' ELSE ' inactive' END ||
				CASE WHEN retention_period_days = $3 THEN '' ELSE ' period:' || retention_period_days END ||
				CASE WHEN COALESCE(classification_ids, '{}') @> ARRAY[$4::bigint] THEN '' ELSE ' classification-unlinked' END ||
				CASE WHEN COALESCE(category_ids, '{}') @> ARRAY[$5::bigint] THEN '' ELSE ' category-unlinked' END
			FROM retention_policies WHERE company_id = $1 AND code = $2`,
		LookupArgs: []any{primaryCompanyID, baseRetentionCode, baseRetentionDays, docClassID, docCategoryID},
		Want:       companyState(primaryCompanyID),
	}); err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("commit tx: %w", err)
	}

	return &Fixtures{
		CompanyID:          primaryCompanyID,
		BranchID:           primaryBranchID,
		OtherCompanyID:     otherCompanyID,
		OtherBranchID:      otherBranchID,
		CustomerID:         customerID,
		SupplierID:         supplierID,
		ProductID:          productID,
		WarehouseID:        warehouseID,
		GRNID:              grnID,
		DocumentCategoryID: docCategoryID,
		DocumentClassID:    docClassID,
		Amount:             baseAmount,
		AdminEmail:         creds.AdminEmail,
		AdminPassword:      creds.AdminPassword,
		BranchEmail:        creds.BranchEmail,
		BranchPassword:     creds.BranchPassword,
		NoAccessEmail:      creds.NoAccessEmail,
		NoAccessPassword:   creds.NoAccessPassword,
	}, s.steps, nil
}
