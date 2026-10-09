package main

import (
	"context"
	"fmt"
	"time"
)

// v0.10.0-rc.11 additions to the base seeder (plan Step 13): admin
// permissions, scoped role assignments, the supplier-scoped AP matching
// policy, a PO-backed certification GRN and the ledger prerequisites. Every
// statement follows the base_seed.go rules (lookup or ON CONFLICT DO NOTHING
// RETURNING, existing rows are verified and reused, never changed), with one
// reviewed exception: baseRevokeRoleOneSQL.

const (
	baseBranchRoleName   = "staging-branch-user"
	baseMatchPolicyName  = "STG-CERT-MATCH-SUPP-STG-01"
	basePONote           = "v0.10-core staging certification PO"
	basePermissionSource = "v0.10-core staging seed"
)

// baseAdminPermissions is every permission named by a RequireAny/RequireAll
// call in the AR, AP, sales (customers, orders, quotations), delivery,
// inventory, documents, CMMS and approvals route handlers that the strict
// certification test drives. base_seed_sql_test.go re-derives this set from
// the handler sources, so a new route permission cannot be missed silently.
// On a migrations-only database finance.ar.view, finance.ar.edit,
// inventory.view and inventory.edit do not exist yet; the seeder creates
// every missing name before granting it.
var baseAdminPermissions = []string{
	"approvals.delegate",
	"approvals.inbox",
	"approvals.policy.admin",
	"cmms.asset.manage",
	"cmms.asset.view",
	"cmms.plan.manage",
	"cmms.plan.view",
	"cmms.request.create",
	"cmms.work_order.close",
	"cmms.work_order.execute",
	"cmms.work_order.view",
	"delivery.order.cancel",
	"delivery.order.complete",
	"delivery.order.confirm",
	"delivery.order.create",
	"delivery.order.edit",
	"delivery.order.ship",
	"delivery.order.view",
	"delivery.return.create",
	"delivery.return.post",
	"delivery.return.view",
	"delivery.return.void",
	"documents.admin",
	"documents.approve",
	"documents.review",
	"documents.sign",
	"documents.upload",
	"documents.version",
	"documents.view",
	"finance.ap.create",
	"finance.ap.debit_note.create",
	"finance.ap.debit_note.post",
	"finance.ap.debit_note.void",
	"finance.ap.payment",
	"finance.ap.post",
	"finance.ap.view",
	"finance.ap.void",
	"finance.ar.credit_note.create",
	"finance.ar.credit_note.post",
	"finance.ar.credit_note.view",
	"finance.ar.credit_note.void",
	"finance.ar.edit",
	"finance.ar.view",
	"inventory.edit",
	"inventory.view",
	"sales.customer.create",
	"sales.customer.edit",
	"sales.customer.view",
	"sales.order.create",
	"sales.order.edit",
	"sales.order.view",
	"sales.quotation.approve",
	"sales.quotation.create",
	"sales.quotation.edit",
	"sales.quotation.view",
}

// baseRevokeRoleOneSQL is the only statement of the base seeder that removes
// rows. Migration 000083 grants cmms.%, qms.% and documents.% to role_id = 1,
// which is 'Sales Manager' on a migrations-only database (000013), not an
// administrator. The grant is revoked only while role 1 is not named
// admin/administrator; the permanent fix (a migration keyed on the role name)
// is deferred to v0.11. base_seed_sql_test.go allows exactly this literal.
const baseRevokeRoleOneSQL = `DELETE FROM role_permissions rp
	USING roles r, permissions p
	WHERE rp.role_id = 1 AND r.id = rp.role_id AND p.id = rp.permission_id
	  AND LOWER(TRIM(r.name)) NOT IN ('admin', 'administrator')
	  AND (p.name LIKE 'cmms.%' OR p.name LIKE 'qms.%' OR p.name LIKE 'documents.%')
	RETURNING r.name, p.name`

// baseLedgerAccounts is the minimal chart of accounts the AR, AP and
// inventory-adjustment posting hooks need. An existing code is reused as is.
var baseLedgerAccounts = []struct{ Code, Name, Type string }{
	{"1100", "Cash", "ASSET"},
	{"1200", "Accounts Receivable", "ASSET"},
	{"1300", "Inventory", "ASSET"},
	{"2100", "Accounts Payable", "LIABILITY"},
	{"4000", "Revenue", "REVENUE"},
	{"4900", "Inventory Gain", "REVENUE"},
	{"5000", "Expense", "EXPENSE"},
	{"5900", "Inventory Loss", "EXPENSE"},
}

// baseLedgerMappings are the (module, key) rows the posting hooks resolve.
// account_mappings is globally unique on (module, key): an existing mapping
// is reused whatever account it points at and is never overwritten. Modules
// are stored upper case (mappings/repository.go upper-cases on lookup).
var baseLedgerMappings = []struct{ Module, Key, Code string }{
	{"AR", "ar.invoice.ar", "1200"},
	{"AR", "ar.invoice.revenue", "4000"},
	{"AP", "ap.invoice.inventory", "1300"},
	{"AP", "ap.invoice.expense", "5000"},
	{"AP", "ap.invoice.ap", "2100"},
	{"AP", "ap.payment.ap", "2100"},
	{"AP", "ap.payment.cash", "1100"},
	{"INVENTORY", "inventory.adjustment.inventory", "1300"},
	{"INVENTORY", "inventory.adjustment.gain", "4900"},
	{"INVENTORY", "inventory.adjustment.loss", "5900"},
}

// ensureAdminPermissions creates every missing baseAdminPermissions row and
// grants all of them to the admin role. It returns the granted names.
func (s *baseSeeder) ensureAdminPermissions(ctx context.Context, adminRoleID int64) ([]string, error) {
	for _, name := range baseAdminPermissions {
		permID, err := s.mustEnsure(ctx, baseFixture{
			Name: "permission " + name, Table: "permissions",
			Insert:      `INSERT INTO permissions (name, description) VALUES ($1, $2) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs:  []any{name, basePermissionSource},
			Lookup:      `SELECT id, 'ok' FROM permissions WHERE name = $1`,
			LookupArgs:  []any{name},
			Want:        "ok",
			LookupFirst: true,
		})
		if err != nil {
			return nil, err
		}
		if _, err := s.mustEnsure(ctx, baseFixture{
			Name: "admin permission " + name, Table: "role_permissions",
			Insert:      `INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2) ON CONFLICT DO NOTHING RETURNING permission_id`,
			InsertArgs:  []any{adminRoleID, permID},
			Lookup:      `SELECT permission_id, 'ok' FROM role_permissions WHERE role_id = $1 AND permission_id = $2`,
			LookupArgs:  []any{adminRoleID, permID},
			Want:        "ok",
			LookupFirst: true,
		}); err != nil {
			return nil, err
		}
	}
	return append([]string(nil), baseAdminPermissions...), nil
}

// revokeRoleOneModulePermissions runs baseRevokeRoleOneSQL and returns the
// role name and the revoked permission names (none on a rerun).
func (s *baseSeeder) revokeRoleOneModulePermissions(ctx context.Context) (string, []string, error) {
	rows, err := s.tx.Query(ctx, baseRevokeRoleOneSQL)
	if err != nil {
		return "", nil, fmt.Errorf("revoke cmms/qms/documents grants from role 1: %w", err)
	}
	defer rows.Close()
	var role string
	var revoked []string
	for rows.Next() {
		var perm string
		if err := rows.Scan(&role, &perm); err != nil {
			return "", nil, fmt.Errorf("read revoked role 1 grant: %w", err)
		}
		revoked = append(revoked, perm)
	}
	if err := rows.Err(); err != nil {
		return "", nil, fmt.Errorf("revoke cmms/qms/documents grants from role 1: %w", err)
	}
	return role, revoked, nil
}

// ensureAssignments seeds the admin's company-wide assignment in the primary
// company and the branch user's active branch-scoped assignment under the
// permission-less staging-branch-user role. Both are looked up first: an
// existing active assignment of the same scope is reused.
func (s *baseSeeder) ensureAssignments(ctx context.Context, primaryCompanyID, primaryBranchID, adminUserID, adminRoleID, branchUserID int64) error {
	if _, err := s.mustEnsure(ctx, baseFixture{
		Name: "rbac_assignment_admin", Table: "rbac_user_role_assignments",
		Insert: `INSERT INTO rbac_user_role_assignments (company_id, user_id, role_id, branch_id, valid_from, valid_to)
			VALUES ($1, $2, $3, NULL, TIMESTAMPTZ '1970-01-01 00:00:00+00', NULL) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{primaryCompanyID, adminUserID, adminRoleID},
		Lookup: `SELECT id, 'ok' FROM rbac_user_role_assignments
			WHERE company_id = $1 AND user_id = $2 AND role_id = $3 AND branch_id IS NULL
			  AND valid_from <= NOW() AND (valid_to IS NULL OR valid_to > NOW())
			ORDER BY id LIMIT 1`,
		LookupArgs:  []any{primaryCompanyID, adminUserID, adminRoleID},
		Want:        "ok",
		LookupFirst: true,
	}); err != nil {
		return err
	}

	branchRoleID, err := s.mustEnsure(ctx, baseFixture{
		Name: "role_branch_user", Table: "roles",
		Insert:     `INSERT INTO roles (name, description, created_at, updated_at) VALUES ($1, $2, NOW(), NOW()) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{baseBranchRoleName, "Permission-less role for the branch-scoped certification identity"},
		Lookup: `SELECT r.id, CASE WHEN EXISTS (SELECT 1 FROM role_permissions rp WHERE rp.role_id = r.id) THEN 'has permissions' ELSE 'ok' END
			FROM roles r WHERE r.name = $1`,
		LookupArgs: []any{baseBranchRoleName},
		Want:       "ok",
	})
	if err != nil {
		return err
	}
	if _, err := s.mustEnsure(ctx, baseFixture{
		Name: "rbac_assignment_branch", Table: "rbac_user_role_assignments",
		Insert: `INSERT INTO rbac_user_role_assignments (company_id, user_id, role_id, branch_id, valid_from, valid_to)
			VALUES ($1, $2, $3, $4, TIMESTAMPTZ '1970-01-01 00:00:00+00', NULL) ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{primaryCompanyID, branchUserID, branchRoleID, primaryBranchID},
		Lookup: `SELECT id, 'ok' FROM rbac_user_role_assignments
			WHERE company_id = $1 AND user_id = $2 AND role_id = $3 AND branch_id = $4
			  AND valid_from <= NOW() AND (valid_to IS NULL OR valid_to > NOW())
			ORDER BY id LIMIT 1`,
		LookupArgs:  []any{primaryCompanyID, branchUserID, branchRoleID, primaryBranchID},
		Want:        "ok",
		LookupFirst: true,
	}); err != nil {
		return err
	}
	return nil
}

// requireActiveAssignmentsOnlyIn aborts when a test identity has an active
// assignment outside the allowed company (allowedCompanyID 0 allows none).
// The workspace lists every company of an active assignment, so a stray row
// would change /api/me. The seeder never expires or deletes it.
func (s *baseSeeder) requireActiveAssignmentsOnlyIn(ctx context.Context, name string, userID, allowedCompanyID int64) error {
	var n int64
	if err := s.tx.QueryRow(ctx, `SELECT COUNT(*) FROM rbac_user_role_assignments
		WHERE user_id = $1 AND company_id <> $2
		  AND valid_from <= NOW() AND (valid_to IS NULL OR valid_to > NOW())`, userID, allowedCompanyID).Scan(&n); err != nil {
		return fmt.Errorf("count active assignments of %s: %w", name, err)
	}
	if n != 0 {
		where := "in any company"
		if allowedCompanyID != 0 {
			where = fmt.Sprintf("outside company %d", allowedCompanyID)
		}
		return fmt.Errorf("%s (id=%d) has %d active rbac_user_role_assignments row(s) %s; the certification needs exactly the seeded scope and the seeder never expires assignments, so refusing", name, userID, n, where)
	}
	return nil
}

// ensureMatchingPolicy seeds one zero-tolerance AP matching policy scoped to
// the base supplier (company_id NULL, the iso004 ap_policy_b shape). A global
// policy (company_id and supplier_id NULL) would trip the ISO-004 guard.
func (s *baseSeeder) ensureMatchingPolicy(ctx context.Context, supplierID int64) (int64, error) {
	return s.mustEnsure(ctx, baseFixture{
		Name: "ap_matching_policy", Table: "ap_matching_policies",
		Insert: `INSERT INTO ap_matching_policies (name, company_id, supplier_id, effective_from)
			VALUES ($1, NULL, $2, DATE '2000-01-01') ON CONFLICT DO NOTHING RETURNING id`,
		InsertArgs: []any{baseMatchPolicyName, supplierID},
		Lookup: `SELECT id,
				CASE WHEN company_id IS NULL AND category_id IS NULL
				      AND qty_tolerance_pct = 0 AND price_tolerance_pct = 0 AND tax_tolerance_pct = 0
				      AND freight_tolerance_pct = 0 AND total_tolerance_amt = 0
				      AND effective_from <= CURRENT_DATE AND (effective_to IS NULL OR effective_to >= CURRENT_DATE)
				THEN 'ok' ELSE 'not an active zero-tolerance supplier policy' END
			FROM ap_matching_policies WHERE name = $1 AND supplier_id = $2 ORDER BY id LIMIT 1`,
		LookupArgs:  []any{baseMatchPolicyName, supplierID},
		Want:        "ok",
		LookupFirst: true,
	})
}

// baseGRNRefs are the parents of the certification PO and GRN.
type baseGRNRefs struct {
	CompanyID, SupplierID, WarehouseID, ProductID, AdminUserID int64
}

// ensureCertificationGRN reuses the newest un-invoiced POSTED certification
// GRN that references an APPROVED certification PO with a line for the base
// product. Otherwise (first run, the recorded GRN was invoiced by a dispatch,
// or only a PO-less GRN from an older seeder exists) it creates a fresh PO
// with one line and a fresh GRN referencing it. Existing GRNs are never
// rebound to a PO. It returns the GRN id, the PO id and whether both were
// created in this run.
func (s *baseSeeder) ensureCertificationGRN(ctx context.Context, r baseGRNRefs) (grnID, poID int64, created bool, err error) {
	grn := baseFixture{
		Name: "grn_primary", Table: "grns",
		Lookup: `SELECT g.id, 'company:' || g.company_id FROM grns g
			JOIN pos po ON po.id = g.po_id
			WHERE g.company_id = $1 AND g.status = 'POSTED' AND g.supplier_id = $2 AND g.warehouse_id = $3 AND g.note = $4
			  AND po.status = 'APPROVED' AND po.company_id = g.company_id AND po.supplier_id = g.supplier_id AND po.note = $5
			  AND EXISTS (SELECT 1 FROM po_lines pl WHERE pl.po_id = po.id AND pl.product_id = $6)
			  AND NOT EXISTS (SELECT 1 FROM ap_invoices ai WHERE ai.grn_id = g.id)
			ORDER BY g.id DESC LIMIT 1`,
		LookupArgs: []any{r.CompanyID, r.SupplierID, r.WarehouseID, baseGRNNote, basePONote, r.ProductID},
		Want:       companyState(r.CompanyID),
	}
	id, state, found, err := s.lookup(ctx, grn)
	if err != nil {
		return 0, 0, false, err
	}
	if found {
		if state != grn.Want {
			return 0, 0, false, s.refuse(grn, state)
		}
		if err := s.tx.QueryRow(ctx, `SELECT po_id FROM grns WHERE id = $1`, id).Scan(&poID); err != nil {
			return 0, 0, false, fmt.Errorf("read po of grn %d: %w", id, err)
		}
		s.record("po_primary", "pos", "reused", poID)
		s.record(grn.Name, grn.Table, "reused", id)
		return id, poID, false, nil
	}

	suffix := time.Now().UnixNano()
	err = s.tx.QueryRow(ctx, `INSERT INTO pos (number, supplier_id, status, currency, note, company_id, expected_warehouse_id, approved_by, approved_at)
		VALUES ($1, $2, 'APPROVED', 'IDR', $3, $4, $5, $6, NOW()) ON CONFLICT DO NOTHING RETURNING id`,
		fmt.Sprintf("PO-STG-%d", suffix), r.SupplierID, basePONote, r.CompanyID, r.WarehouseID, r.AdminUserID).Scan(&poID)
	if err != nil {
		return 0, 0, false, fmt.Errorf("insert po_primary (pos): %w", err)
	}
	s.record("po_primary", "pos", "created", poID)
	if err := s.insertChild(ctx, "po_line_primary", "po_lines",
		`INSERT INTO po_lines (po_id, product_id, qty, price, note) VALUES ($1, $2, 10.0000, 50000.0000, 'staging certification') ON CONFLICT DO NOTHING RETURNING id`,
		poID, r.ProductID); err != nil {
		return 0, 0, false, err
	}
	err = s.tx.QueryRow(ctx, `INSERT INTO grns (number, po_id, supplier_id, warehouse_id, status, received_at, note, created_at, company_id)
		VALUES ($1, $2, $3, $4, 'POSTED', NOW(), $5, NOW(), $6) ON CONFLICT DO NOTHING RETURNING id`,
		fmt.Sprintf("GRN-STG-%d", suffix), poID, r.SupplierID, r.WarehouseID, baseGRNNote, r.CompanyID).Scan(&grnID)
	if err != nil {
		return 0, 0, false, fmt.Errorf("insert grn_primary (grns): %w", err)
	}
	s.record("grn_primary", "grns", "created", grnID)
	if err := s.insertChild(ctx, "grn_line_primary", "grn_lines",
		`INSERT INTO grn_lines (grn_id, product_id, qty, unit_cost) VALUES ($1, $2, 10.0000, 50000.0000) ON CONFLICT DO NOTHING RETURNING id`,
		grnID, r.ProductID); err != nil {
		return 0, 0, false, err
	}
	return grnID, poID, true, nil
}

// ensureLedgerPrerequisites seeds the chart of accounts, the account
// mappings and an OPEN period covering today, each only when absent.
func (s *baseSeeder) ensureLedgerPrerequisites(ctx context.Context) error {
	accountIDs := map[string]int64{}
	for _, a := range baseLedgerAccounts {
		id, err := s.mustEnsure(ctx, baseFixture{
			Name: "account " + a.Code, Table: "accounts",
			Insert:      `INSERT INTO accounts (code, name, type, is_active) VALUES ($1, $2, $3::account_type, TRUE) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs:  []any{a.Code, a.Name, a.Type},
			Lookup:      `SELECT id, 'ok' FROM accounts WHERE code = $1`,
			LookupArgs:  []any{a.Code},
			Want:        "ok",
			LookupFirst: true,
		})
		if err != nil {
			return err
		}
		accountIDs[a.Code] = id
	}
	for _, m := range baseLedgerMappings {
		if _, err := s.mustEnsure(ctx, baseFixture{
			Name: "mapping " + m.Module + "/" + m.Key, Table: "account_mappings",
			Insert:      `INSERT INTO account_mappings (module, key, account_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs:  []any{m.Module, m.Key, accountIDs[m.Code]},
			Lookup:      `SELECT id, 'ok' FROM account_mappings WHERE module = $1 AND key = $2`,
			LookupArgs:  []any{m.Module, m.Key},
			Want:        "ok",
			LookupFirst: true,
		}); err != nil {
			return err
		}
	}

	// An OPEN period covering today is reused. Otherwise the current month
	// is inserted, but only when no period (of any status) overlaps it: no
	// constraint prevents overlapping periods, so the seeder checks itself.
	period := baseFixture{
		Name: "period_open_today", Table: "periods",
		Insert: `INSERT INTO periods (code, start_date, end_date, status)
			VALUES ('STG-' || to_char(CURRENT_DATE, 'YYYYMM'), date_trunc('month', CURRENT_DATE)::date,
				(date_trunc('month', CURRENT_DATE) + INTERVAL '1 month - 1 day')::date, 'OPEN')
			ON CONFLICT DO NOTHING RETURNING id`,
		Lookup: `SELECT id, 'ok' FROM periods WHERE status = 'OPEN' AND CURRENT_DATE BETWEEN start_date AND end_date
			ORDER BY start_date, id LIMIT 1`,
		Want:        "ok",
		LookupFirst: true,
	}
	if _, _, found, err := s.lookup(ctx, period); err != nil {
		return err
	} else if !found {
		var overlap string
		if err := s.tx.QueryRow(ctx, `SELECT COALESCE(string_agg(code || ' (' || status::text || ')', ', ' ORDER BY start_date), '')
			FROM periods
			WHERE start_date <= (date_trunc('month', CURRENT_DATE) + INTERVAL '1 month - 1 day')::date
			  AND end_date >= date_trunc('month', CURRENT_DATE)::date`).Scan(&overlap); err != nil {
			return fmt.Errorf("check periods overlapping the current month: %w", err)
		}
		if overlap != "" {
			return fmt.Errorf("no OPEN period covers today and the current month overlaps existing period(s) %s; the seeder never inserts an overlapping period or reopens one, so refusing", overlap)
		}
	}
	_, err := s.mustEnsure(ctx, period)
	return err
}
