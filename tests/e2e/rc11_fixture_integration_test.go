//go:build integration

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// rc11Fixture holds the ids of the rows newRC11Fixture creates. Per-run rows
// are keyed by a nanosecond suffix so the helper can run repeatedly against
// the same database; the ledger prerequisites (accounts, account mappings, an
// OPEN period) are global, so they are looked up first and only inserted when
// absent.
//
// The TestRC11* suites share those ledger rows with each other and with
// fx_ar_ap_integration_test.go: they must not call t.Parallel() and the CI
// step runs the package with -p 1.
//
// A company-wide (company_id IS NULL AND supplier_id IS NULL) matching policy
// is deliberately never inserted: it would change the matching outcome of
// every other AP suite and the ISO-004 seeder aborts when one exists.
type rc11Fixture struct {
	Suffix      string
	CompanyID   int64
	BranchID    int64
	AdminID     int64
	OtherUserID int64
	AdminRoleID int64
	CustomerID  int64
	SupplierID  int64
	ProductID   int64
	WarehouseID int64
	POID        int64
	POLineID    int64
	GRNID       int64

	MatchingPolicyID int64
	CategoryID       int64 // document category
	ClassificationID int64 // document classification
	RetentionID      int64
	AssetID          int64 // cmms asset
	PeriodID         int64
}

// rc11Accounts is the minimal chart of accounts the AR/AP/inventory posting
// hooks need on a fresh database.
var rc11Accounts = []struct {
	code, name, typ string
}{
	{"1100", "Cash", "ASSET"},
	{"1200", "Accounts Receivable", "ASSET"},
	{"1300", "Inventory", "ASSET"},
	{"2100", "Accounts Payable", "LIABILITY"},
	{"4000", "Revenue", "REVENUE"},
	{"4900", "Inventory Gain", "REVENUE"},
	{"5000", "Expense", "EXPENSE"},
	{"5900", "Inventory Loss", "EXPENSE"},
}

// rc11Mappings are the (module, key) -> account code rows the AR, AP and
// inventory-adjustment posting paths resolve. Modules are stored upper case.
var rc11Mappings = []struct {
	module, key, code string
}{
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

// newRC11Fixture opens PG_DSN (skipping when unset), applies migrations and
// seeds one self-contained company plus the shared ledger prerequisites.
func newRC11Fixture(t *testing.T) (*pgxpool.Pool, rc11Fixture) {
	t.Helper()
	if os.Getenv("PG_DSN") == "" {
		t.Skip("PG_DSN is required for the RC11 integration suites")
	}
	p := openAPPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	f := rc11Fixture{Suffix: suffix}

	rc11SeedLedger(t, p, &f)

	scan := func(dst *int64, sql string, args ...any) {
		t.Helper()
		require.NoError(t, p.QueryRow(ctx, sql, args...).Scan(dst), sql)
	}

	scan(&f.CompanyID, `INSERT INTO companies(code,name,base_currency) VALUES($1,'RC11 integration','IDR') RETURNING id`, "RC11-"+suffix)
	scan(&f.BranchID, `INSERT INTO branches(company_id,code,name) VALUES($1,$2,'RC11 branch') RETURNING id`, f.CompanyID, "BR-"+suffix)
	scan(&f.AdminID, `INSERT INTO users(email,password_hash) VALUES($1,'integration') RETURNING id`, "rc11-admin-"+suffix+"@example.test")
	scan(&f.OtherUserID, `INSERT INTO users(email,password_hash) VALUES($1,'integration') RETURNING id`, "rc11-other-"+suffix+"@example.test")

	// roles.name is not unique in the schema, so look up before inserting.
	err := p.QueryRow(ctx, `SELECT id FROM roles WHERE name='admin' ORDER BY id LIMIT 1`).Scan(&f.AdminRoleID)
	if err != nil {
		scan(&f.AdminRoleID, `INSERT INTO roles(name,description) VALUES('admin','Administrator') RETURNING id`)
	}
	_, err = p.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, f.AdminID, f.AdminRoleID)
	require.NoError(t, err)
	// Company-wide assignment: branch_id IS NULL. The second user gets none.
	_, err = p.Exec(ctx, `INSERT INTO rbac_user_role_assignments(company_id,user_id,role_id,branch_id) VALUES($1,$2,$3,NULL)`, f.CompanyID, f.AdminID, f.AdminRoleID)
	require.NoError(t, err)

	scan(&f.CustomerID, `INSERT INTO customers(code,name,company_id,created_by) VALUES($1,'RC11 customer',$2,$3) RETURNING id`, "C-"+suffix, f.CompanyID, f.AdminID)
	scan(&f.SupplierID, `INSERT INTO suppliers(code,name,company_id) VALUES($1,'RC11 supplier',$2) RETURNING id`, "S-"+suffix, f.CompanyID)

	var categoryID, unitID int64
	scan(&categoryID, `INSERT INTO categories(code,name,company_id) VALUES($1,'RC11 cat',$2) RETURNING id`, "CAT-"+suffix, f.CompanyID)
	scan(&unitID, `INSERT INTO units(code,name) VALUES($1,'Unit') RETURNING id`, "U-"+suffix)
	scan(&f.ProductID, `INSERT INTO products(sku,name,category_id,unit_id,price,company_id) VALUES($1,'Widget',$2,$3,100,$4) RETURNING id`, "SKU-"+suffix, categoryID, unitID, f.CompanyID)
	scan(&f.WarehouseID, `INSERT INTO warehouses(branch_id,code,name) VALUES($1,$2,'RC11 wh') RETURNING id`, f.BranchID, "WH-"+suffix)

	scan(&f.POID, `INSERT INTO pos(number,supplier_id,status,company_id) VALUES($1,$2,'APPROVED',$3) RETURNING id`, "PO-"+suffix, f.SupplierID, f.CompanyID)
	scan(&f.POLineID, `INSERT INTO po_lines(po_id,product_id,qty,price) VALUES($1,$2,10,100) RETURNING id`, f.POID, f.ProductID)
	scan(&f.GRNID, `INSERT INTO grns(number,po_id,supplier_id,warehouse_id,status,company_id) VALUES($1,$2,$3,$4,'POSTED',$5) RETURNING id`, "GRN-"+suffix, f.POID, f.SupplierID, f.WarehouseID, f.CompanyID)
	_, err = p.Exec(ctx, `INSERT INTO grn_lines(grn_id,product_id,qty,unit_cost) VALUES($1,$2,10,100)`, f.GRNID, f.ProductID)
	require.NoError(t, err)

	// Supplier-scoped, zero-tolerance policy (all tolerances default to 0).
	scan(&f.MatchingPolicyID, `INSERT INTO ap_matching_policies(name,supplier_id,effective_from) VALUES($1,$2,CURRENT_DATE - 1) RETURNING id`, "RC11 exact "+suffix, f.SupplierID)

	scan(&f.ClassificationID, `INSERT INTO document_classifications(company_id,code,name,created_by) VALUES($1,$2,'RC11 internal',$3) RETURNING id`, f.CompanyID, "CLS-"+suffix, f.AdminID)
	scan(&f.CategoryID, `INSERT INTO document_categories(company_id,code,name,default_classification_id,created_by) VALUES($1,$2,'RC11 category',$3,$4) RETURNING id`, f.CompanyID, "DOC-"+suffix, f.ClassificationID, f.AdminID)
	scan(&f.RetentionID, `INSERT INTO retention_policies(company_id,code,name,trigger_event,retention_period_days,created_by) VALUES($1,$2,'RC11 7 years','CREATION',2555,$3) RETURNING id`, f.CompanyID, "RET-"+suffix, f.AdminID)

	scan(&f.AssetID, `INSERT INTO assets(company_id,code,name) VALUES($1,$2,'RC11 pump') RETURNING id`, f.CompanyID, "AST-"+suffix)

	return p, f
}

// rc11SeedLedger creates the shared chart of accounts, account mappings and
// OPEN period, each only when absent, and records the period id.
func rc11SeedLedger(t *testing.T, p *pgxpool.Pool, f *rc11Fixture) {
	t.Helper()
	ctx := context.Background()

	accountIDs := map[string]int64{}
	for _, a := range rc11Accounts {
		var id int64
		err := p.QueryRow(ctx, `SELECT id FROM accounts WHERE code=$1`, a.code).Scan(&id)
		if err != nil {
			require.NoError(t, p.QueryRow(ctx,
				`INSERT INTO accounts(code,name,type) VALUES($1,$2,$3::account_type) ON CONFLICT (code) DO UPDATE SET code=EXCLUDED.code RETURNING id`,
				a.code, a.name, a.typ).Scan(&id), "account %s", a.code)
		}
		accountIDs[a.code] = id
	}

	for _, m := range rc11Mappings {
		// (module, key) is globally unique: an existing row is reused as is.
		_, err := p.Exec(ctx,
			`INSERT INTO account_mappings(module,key,account_id) VALUES($1,$2,$3) ON CONFLICT (module,key) DO NOTHING`,
			m.module, m.key, accountIDs[m.code])
		require.NoError(t, err, "mapping %s/%s", m.module, m.key)
	}

	// No constraint prevents overlapping periods, so check for one covering
	// today before inserting.
	var status string
	err := p.QueryRow(ctx, `SELECT id, status::text FROM periods WHERE CURRENT_DATE BETWEEN start_date AND end_date ORDER BY (status='OPEN') DESC, id LIMIT 1`).Scan(&f.PeriodID, &status)
	if err != nil {
		require.NoError(t, p.QueryRow(ctx,
			`INSERT INTO periods(code,start_date,end_date,status)
			 VALUES('RC11-'||to_char(CURRENT_DATE,'YYYYMM'), date_trunc('month',CURRENT_DATE)::date, (date_trunc('month',CURRENT_DATE)+interval '1 month - 1 day')::date, 'OPEN')
			 RETURNING id`).Scan(&f.PeriodID))
		return
	}
	require.Equal(t, "OPEN", status, "the period covering today must be OPEN for ledger posting")
}

func TestRC11FixtureSmoke(t *testing.T) {
	p, f := newRC11Fixture(t)
	ctx := context.Background()

	count := func(sql string, args ...any) int {
		t.Helper()
		var n int
		require.NoError(t, p.QueryRow(ctx, sql, args...).Scan(&n), sql)
		return n
	}

	// Ledger prerequisites.
	for _, a := range rc11Accounts {
		require.Equal(t, 1, count(`SELECT COUNT(*) FROM accounts WHERE code=$1`, a.code), "account %s", a.code)
	}
	for _, m := range rc11Mappings {
		require.Equal(t, 1, count(`SELECT COUNT(*) FROM account_mappings WHERE module=$1 AND key=$2`, m.module, m.key), "mapping %s/%s", m.module, m.key)
	}
	require.Equal(t, 1, count(`SELECT COUNT(*) FROM periods WHERE id=$1 AND status='OPEN' AND CURRENT_DATE BETWEEN start_date AND end_date`, f.PeriodID))

	// PO + posted GRN linked to it.
	require.Equal(t, 1, count(`SELECT COUNT(*) FROM po_lines WHERE id=$1 AND po_id=$2`, f.POLineID, f.POID))
	require.Equal(t, 1, count(`SELECT COUNT(*) FROM pos WHERE id=$1 AND status='APPROVED' AND company_id=$2`, f.POID, f.CompanyID))
	require.Equal(t, 1, count(`SELECT COUNT(*) FROM grns WHERE id=$1 AND po_id=$2 AND status='POSTED'`, f.GRNID, f.POID))

	// Supplier-scoped policy only: no company-wide global policy was added.
	require.Equal(t, 1, count(`SELECT COUNT(*) FROM ap_matching_policies WHERE id=$1 AND supplier_id=$2 AND qty_tolerance_pct=0 AND price_tolerance_pct=0 AND total_tolerance_amt=0`, f.MatchingPolicyID, f.SupplierID))
	require.Equal(t, 0, count(`SELECT COUNT(*) FROM ap_matching_policies WHERE id=$1 AND supplier_id IS NULL AND company_id IS NULL`, f.MatchingPolicyID))

	// Documents, retention and CMMS.
	require.Equal(t, 1, count(`SELECT COUNT(*) FROM document_categories WHERE id=$1 AND default_classification_id=$2`, f.CategoryID, f.ClassificationID))
	require.Equal(t, 1, count(`SELECT COUNT(*) FROM document_classifications WHERE id=$1 AND company_id=$2`, f.ClassificationID, f.CompanyID))
	require.Equal(t, 1, count(`SELECT COUNT(*) FROM retention_policies WHERE id=$1 AND retention_period_days >= 2555`, f.RetentionID))
	require.Equal(t, 1, count(`SELECT COUNT(*) FROM assets WHERE id=$1 AND company_id=$2`, f.AssetID, f.CompanyID))

	// RBAC: admin has a company-wide assignment, the second user none.
	require.Equal(t, 1, count(`SELECT COUNT(*) FROM user_roles WHERE user_id=$1 AND role_id=$2`, f.AdminID, f.AdminRoleID))
	require.Equal(t, 1, count(`SELECT COUNT(*) FROM rbac_user_role_assignments WHERE company_id=$1 AND user_id=$2 AND role_id=$3 AND branch_id IS NULL AND valid_to IS NULL`, f.CompanyID, f.AdminID, f.AdminRoleID))
	require.Zero(t, count(`SELECT COUNT(*) FROM rbac_user_role_assignments WHERE user_id=$1`, f.OtherUserID))
	require.Zero(t, count(`SELECT COUNT(*) FROM user_roles WHERE user_id=$1`, f.OtherUserID))

	require.NotZero(t, f.CustomerID)
}
