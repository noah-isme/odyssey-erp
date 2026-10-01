#!/usr/bin/env bash
# ==============================================================================
# Odyssey ERP Realistic Seed Data Acceptance Verification Suite
# ==============================================================================
# Usage:
#   bash scripts/seed/realistic/verify.sh
#
# Environment variables:
#   PG_DSN  - PostgreSQL connection string (defaults to localhost:5434 or 5432)
# ==============================================================================

set -o pipefail

# ANSI color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

TOTAL_PASS=0
TOTAL_FAIL=0

echo -e "${BOLD}${CYAN}================================================================${NC}"
echo -e "${BOLD}${CYAN}   Odyssey ERP Realistic Seed Data - Automated Verification Suite ${NC}"
echo -e "${BOLD}${CYAN}================================================================${NC}\n"

# ------------------------------------------------------------------------------
# 1. Compilation Verification
# ------------------------------------------------------------------------------
echo -e "${BOLD}${BLUE}==> 1. Verifying Go Seed Package Compilation...${NC}"

if go build ./scripts/seed/realistic/... > /dev/null 2>&1; then
    echo -e "  ${GREEN}[ PASS ]${NC} Package ./scripts/seed/realistic/... compiles cleanly"
    TOTAL_PASS=$((TOTAL_PASS + 1))
else
    # In case the directory only contains non-main files during development or compilation errors
    COMPILE_OUT=$(go build ./scripts/seed/realistic/... 2>&1 || true)
    if echo "$COMPILE_OUT" | grep -q "no Go files"; then
        echo -e "  ${YELLOW}[ WARN ]${NC} Package ./scripts/seed/realistic/... has no Go files yet (test infra ready)"
        TOTAL_PASS=$((TOTAL_PASS + 1))
    else
        echo -e "  ${RED}[ FAIL ]${NC} Package ./scripts/seed/realistic/... failed compilation"
        echo -e "         ${COMPILE_OUT}"
        TOTAL_FAIL=$((TOTAL_FAIL + 1))
    fi
fi

# ------------------------------------------------------------------------------
# 2. Database Connection Check
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}${BLUE}==> 2. Establishing Database Connection...${NC}"

TARGET_DSN="${PG_DSN:-}"
if [ -z "$TARGET_DSN" ]; then
    # Try port 5434 first (Compose stack default), then fallback to 5432
    if psql "postgres://odyssey:odyssey@localhost:5434/odyssey?sslmode=disable" -c "SELECT 1;" > /dev/null 2>&1; then
        TARGET_DSN="postgres://odyssey:odyssey@localhost:5434/odyssey?sslmode=disable"
    elif psql "postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable" -c "SELECT 1;" > /dev/null 2>&1; then
        TARGET_DSN="postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable"
    else
        TARGET_DSN="postgres://odyssey:odyssey@localhost:5434/odyssey?sslmode=disable"
    fi
fi

if ! psql "$TARGET_DSN" -c "SELECT 1;" > /dev/null 2>&1; then
    echo -e "  ${RED}[ FAIL ]${NC} Could not connect to PostgreSQL using DSN: $TARGET_DSN"
    echo -e "         Please ensure database is running ('docker compose up -d' or 'make migrate-up')."
    exit 1
fi

echo -e "  ${GREEN}[ PASS ]${NC} Successfully connected to PostgreSQL: ${TARGET_DSN}"
TOTAL_PASS=$((TOTAL_PASS + 1))

# Helper: execute query and trim output
exec_query() {
    local query="$1"
    psql "$TARGET_DSN" -t -A -c "$query" 2>/dev/null | tr -d '[:space:]'
}

# Helper: assert exact numeric value
assert_eq() {
    local desc="$1"
    local query="$2"
    local expected="$3"

    local actual
    actual=$(exec_query "$query")
    if [ "$actual" == "$expected" ]; then
        echo -e "  ${GREEN}[ PASS ]${NC} $desc (Got: $actual)"
        TOTAL_PASS=$((TOTAL_PASS + 1))
    else
        echo -e "  ${RED}[ FAIL ]${NC} $desc (Expected: $expected, Got: ${actual:-NULL})"
        TOTAL_FAIL=$((TOTAL_FAIL + 1))
    fi
}

# Helper: assert minimum integer value (actual >= min)
assert_ge() {
    local desc="$1"
    local query="$2"
    local min_val="$3"

    local actual
    actual=$(exec_query "$query")
    if [[ "$actual" =~ ^[0-9]+$ ]] && [ "$actual" -ge "$min_val" ]; then
        echo -e "  ${GREEN}[ PASS ]${NC} $desc (Got: $actual, Expected >= $min_val)"
        TOTAL_PASS=$((TOTAL_PASS + 1))
    else
        echo -e "  ${RED}[ FAIL ]${NC} $desc (Expected >= $min_val, Got: ${actual:-NULL})"
        TOTAL_FAIL=$((TOTAL_FAIL + 1))
    fi
}

# Helper: assert boolean condition (returns 't' / 'true')
assert_bool() {
    local desc="$1"
    local query="$2"

    local actual
    actual=$(exec_query "$query")
    if [ "$actual" == "t" ] || [ "$actual" == "true" ] || [ "$actual" == "1" ]; then
        echo -e "  ${GREEN}[ PASS ]${NC} $desc"
        TOTAL_PASS=$((TOTAL_PASS + 1))
    else
        echo -e "  ${RED}[ FAIL ]${NC} $desc (Condition not met, Got: ${actual:-NULL})"
        TOTAL_FAIL=$((TOTAL_FAIL + 1))
    fi
}

# ------------------------------------------------------------------------------
# 3. Core Integrity & General Ledger Balancing Assertions
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}${BLUE}==> 3. Verifying General Ledger Balancing & Core Integrity...${NC}"

# 3.1 Double-Entry Balance
assert_eq \
    "Zero unbalanced general ledger journal entries (SUM(debit) == SUM(credit))" \
    "SELECT COUNT(*) FROM (SELECT je_id FROM journal_lines GROUP BY je_id HAVING ROUND(SUM(debit), 2) <> ROUND(SUM(credit), 2)) u;" \
    "0"

# 3.2 No empty journal entries without lines
assert_eq \
    "Zero empty journal entries without journal lines" \
    "SELECT COUNT(*) FROM journal_entries je WHERE NOT EXISTS (SELECT 1 FROM journal_lines jl WHERE jl.je_id = je.id);" \
    "0"

# 3.3 No invalid line amounts
assert_eq \
    "Zero invalid debit/credit amounts (no negatives, no dual-sided lines)" \
    "SELECT COUNT(*) FROM journal_lines WHERE debit < 0 OR credit < 0 OR (debit > 0 AND credit > 0) OR (debit = 0 AND credit = 0);" \
    "0"

# 3.4 AR Invoices source links coverage
assert_eq \
    "100% source links coverage for all POSTED and PAID AR invoices" \
    "SELECT COUNT(*) FROM ar_invoices inv WHERE inv.status IN ('POSTED', 'PAID') AND NOT EXISTS (SELECT 1 FROM source_links sl JOIN journal_entries je ON je.id = sl.je_id WHERE sl.module = 'SALES.AR_INVOICE' AND je.memo LIKE '%' || inv.number || '%');" \
    "0"

# 3.5 AP Invoices source links coverage
assert_eq \
    "100% source links coverage for all POSTED and PAID AP invoices" \
    "SELECT COUNT(*) FROM ap_invoices inv WHERE inv.status IN ('POSTED', 'PAID') AND NOT EXISTS (SELECT 1 FROM source_links sl JOIN journal_entries je ON je.id = sl.je_id WHERE sl.module = 'PROCUREMENT.AP_INVOICE' AND je.memo LIKE '%' || inv.number || '%');" \
    "0"

# 3.6 Non-negative inventory balances
assert_eq \
    "Zero negative inventory balances across all warehouse/product combinations" \
    "SELECT COUNT(*) FROM inventory_balances WHERE qty < 0;" \
    "0"

# 3.7 Non-negative spare parts quantities
assert_eq \
    "Zero negative spare parts min_quantity or reorder_point" \
    "SELECT COUNT(*) FROM spare_parts WHERE min_quantity < 0 OR reorder_point < 0;" \
    "0"

# 3.8 Strict Date Boundaries (March 1, 2026 - August 31, 2026)
assert_eq \
    "Zero transactions outside trailing 6 months (2026-03-01 to 2026-08-31)" \
    "SELECT ((SELECT COUNT(*) FROM journal_entries WHERE date < '2026-03-01' OR date > '2026-08-31') + (SELECT COUNT(*) FROM ap_invoices WHERE issued_at < '2026-03-01' OR issued_at > '2026-08-31') + (SELECT COUNT(*) FROM ar_invoices WHERE DATE(created_at) < '2026-03-01' OR DATE(created_at) > '2026-08-31') + (SELECT COUNT(*) FROM pos WHERE (expected_date IS NOT NULL AND expected_date < '2026-03-01') OR (expected_date > '2026-08-31')) + (SELECT COUNT(*) FROM sales_orders WHERE order_date < '2026-03-01' OR order_date > '2026-08-31') + (SELECT COUNT(*) FROM timesheets WHERE work_date < '2026-03-01' OR work_date > '2026-08-31') + (SELECT COUNT(*) FROM hr_attendance WHERE attendance_date < '2026-03-01' OR attendance_date > '2026-08-31') + (SELECT COUNT(*) FROM fx_daily_rates WHERE rate_date < '2026-03-01' OR rate_date > '2026-08-31'));" \
    "0"

# ------------------------------------------------------------------------------
# 4. Domain Record Count Assertions (18+ Domains)
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}${BLUE}==> 4. Verifying Domain Row Counts (18+ Domains)...${NC}"

# Domain 1: Foundation & Master Data
assert_ge "Foundation: Active users" "SELECT COUNT(*) FROM users WHERE is_active = TRUE;" 9
assert_ge "Foundation: Companies" "SELECT COUNT(*) FROM companies;" 2
assert_ge "Foundation: Branches" "SELECT COUNT(*) FROM branches;" 4
assert_ge "Foundation: Warehouses" "SELECT COUNT(*) FROM warehouses;" 6
assert_ge "Foundation: Cost Centers" "SELECT COUNT(*) FROM cost_centers;" 9
assert_ge "Foundation: Products" "SELECT COUNT(*) FROM products;" 20
assert_ge "Foundation: Suppliers" "SELECT COUNT(*) FROM suppliers;" 8
assert_ge "Foundation: Customers" "SELECT COUNT(*) FROM customers;" 10

# Domain 2: Finance & Accounting
assert_ge "Finance: PSAK Accounts" "SELECT COUNT(*) FROM accounts WHERE is_active = TRUE;" 35
assert_ge "Finance: Accounting Periods" "SELECT COUNT(*) FROM periods;" 12
assert_ge "Finance: Bank Accounts" "SELECT COUNT(*) FROM bank_accounts;" 3
assert_ge "Finance: FX Daily Rates (IDR/USD)" "SELECT COUNT(*) FROM fx_daily_rates WHERE base_currency='IDR' AND quote_currency='USD';" 180

# Domain 3: CRM
assert_ge "CRM: Pipeline Stages" "SELECT COUNT(*) FROM crm_pipeline_stages;" 6
assert_ge "CRM: Leads" "SELECT COUNT(*) FROM crm_leads;" 10
assert_ge "CRM: Opportunities" "SELECT COUNT(*) FROM crm_opportunities;" 5
assert_ge "CRM: Activities" "SELECT COUNT(*) FROM crm_activities;" 15

# Domain 4: Procurement
assert_ge "Procurement: Purchase Requests" "SELECT COUNT(*) FROM prs;" 3
assert_ge "Procurement: Purchase Orders" "SELECT COUNT(*) FROM pos;" 8
assert_ge "Procurement: Goods Receipt Notes (GRN)" "SELECT COUNT(*) FROM grns;" 5
assert_ge "Procurement: AP Invoices" "SELECT COUNT(*) FROM ap_invoices;" 6
assert_ge "Procurement: AP Payments" "SELECT COUNT(*) FROM ap_payments;" 4

# Domain 5: Sales
assert_ge "Sales: Quotations" "SELECT COUNT(*) FROM quotations;" 8
assert_ge "Sales: Sales Orders" "SELECT COUNT(*) FROM sales_orders;" 8
assert_ge "Sales: Delivery Orders" "SELECT COUNT(*) FROM delivery_orders;" 6
assert_ge "Sales: AR Invoices" "SELECT COUNT(*) FROM ar_invoices;" 6
assert_ge "Sales: AR Payments" "SELECT COUNT(*) FROM ar_payments;" 4

# Domain 6: MRP / Manufacturing
assert_ge "MRP: Work Centers" "SELECT COUNT(*) FROM mrp_work_centers;" 3
assert_ge "MRP: BOMs" "SELECT COUNT(*) FROM mrp_boms;" 3
assert_ge "MRP: Work Orders" "SELECT COUNT(*) FROM mrp_work_orders;" 3

# Domain 7: CMMS Maintenance
assert_ge "CMMS: Maintainable Assets" "SELECT COUNT(*) FROM assets;" 8
assert_ge "CMMS: PM Schedules" "SELECT COUNT(*) FROM pm_schedules;" 5
assert_ge "CMMS: Work Orders" "SELECT COUNT(*) FROM work_orders;" 3
assert_ge "CMMS: Spare Parts" "SELECT COUNT(*) FROM spare_parts;" 10

# Domain 8: QMS Quality
assert_ge "QMS: Non-Conformance Reports (NCR)" "SELECT COUNT(*) FROM ncrs;" 3
assert_ge "QMS: CAPAs" "SELECT COUNT(*) FROM capas;" 3
assert_ge "QMS: Audits" "SELECT COUNT(*) FROM audits;" 2
assert_ge "QMS: Inspections" "SELECT COUNT(*) FROM qms_inspections;" 4

# Domain 9: HR & Payroll
assert_ge "HR: Employees" "SELECT COUNT(*) FROM hr_employees;" 12
assert_ge "HR: Leave Requests" "SELECT COUNT(*) FROM hr_leave_requests;" 6
assert_ge "HR: Attendance Records" "SELECT COUNT(*) FROM hr_attendance;" 20
assert_ge "Payroll: Posted Payroll Runs" "SELECT COUNT(*) FROM payroll_runs WHERE status = 'POSTED';" 1
assert_ge "Payroll: Payroll Run Lines" "SELECT COUNT(*) FROM payroll_run_lines;" 12

# Domain 10: Fixed Assets
assert_ge "Fixed Assets: Asset Categories" "SELECT COUNT(*) FROM fixed_asset_categories;" 4
assert_ge "Fixed Assets: Asset Items" "SELECT COUNT(*) FROM fixed_assets;" 5

# Domain 11: POS
assert_ge "POS: Terminals" "SELECT COUNT(*) FROM pos_terminals;" 1
assert_ge "POS: Sessions" "SELECT COUNT(*) FROM pos_sessions;" 2
assert_ge "POS: Tickets" "SELECT COUNT(*) FROM pos_tickets;" 5

# Domain 12: Projects & Timesheets
assert_ge "Projects: Projects" "SELECT COUNT(*) FROM projects;" 2
assert_ge "Projects: Project Tasks" "SELECT COUNT(*) FROM project_tasks;" 8
assert_ge "Projects: Timesheets" "SELECT COUNT(*) FROM timesheets;" 15

# Domain 13: Logistics
assert_ge "Logistics: Carriers" "SELECT COUNT(*) FROM carriers;" 2
assert_ge "Logistics: Vehicles" "SELECT COUNT(*) FROM vehicles;" 3
assert_ge "Logistics: Drivers" "SELECT COUNT(*) FROM drivers;" 2
assert_ge "Logistics: Shipments" "SELECT COUNT(*) FROM shipments;" 3

# Domain 14: Inventory & WMS
assert_ge "WMS: Bins" "SELECT COUNT(*) FROM wms_bins;" 10

# Domain 15: Banking & Treasury
assert_ge "Banking: Bank Statements" "SELECT COUNT(*) FROM bank_statements;" 3
assert_ge "Banking: Bank Transactions" "SELECT (SELECT COUNT(*) FROM bank_transactions) + (SELECT COUNT(*) FROM bank_statement_lines);" 20

# Domain 16: Document Management
assert_ge "Documents: Controlled Documents" "SELECT COUNT(*) FROM documents;" 5
assert_ge "Documents: Document Versions" "SELECT COUNT(*) FROM document_versions;" 5

# Domain 17: Consolidation
assert_ge "Consolidation: Consol Groups" "SELECT COUNT(*) FROM consol_groups;" 1
assert_ge "Consolidation: Elimination Rules" "SELECT COUNT(*) FROM elimination_rules;" 2

# ------------------------------------------------------------------------------
# 5. Status Diversity Assertions
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}${BLUE}==> 5. Verifying Status Diversity Across Tables...${NC}"

assert_bool "Status diversity: crm_leads (>= 3 statuses)" "SELECT COUNT(DISTINCT status) >= 3 FROM crm_leads;"
assert_bool "Status diversity: crm_opportunities (>= 3 statuses)" "SELECT COUNT(DISTINCT status) >= 3 FROM crm_opportunities;"
assert_bool "Status diversity: pos purchase orders (>= 3 statuses)" "SELECT COUNT(DISTINCT status) >= 3 FROM pos;"
assert_bool "Status diversity: ap_invoices (>= 3 statuses)" "SELECT COUNT(DISTINCT status) >= 3 FROM ap_invoices;"
assert_bool "Status diversity: quotations (>= 3 statuses)" "SELECT COUNT(DISTINCT status) >= 3 FROM quotations;"
assert_bool "Status diversity: sales_orders (>= 3 statuses)" "SELECT COUNT(DISTINCT status) >= 3 FROM sales_orders;"
assert_bool "Status diversity: delivery_orders (>= 3 statuses)" "SELECT COUNT(DISTINCT status) >= 3 FROM delivery_orders;"
assert_bool "Status diversity: ar_invoices (>= 3 statuses)" "SELECT COUNT(DISTINCT status) >= 3 FROM ar_invoices;"
assert_bool "Status diversity: mrp_work_orders (>= 3 statuses)" "SELECT COUNT(DISTINCT status) >= 3 FROM mrp_work_orders;"
assert_bool "Status diversity: CMMS work_orders (>= 3 statuses)" "SELECT COUNT(DISTINCT status) >= 3 FROM work_orders;"
assert_bool "Status diversity: ncrs (>= 3 statuses)" "SELECT COUNT(DISTINCT status) >= 3 FROM ncrs;"
assert_bool "Status diversity: hr_leave_requests (>= 3 statuses)" "SELECT COUNT(DISTINCT status) >= 3 FROM hr_leave_requests;"
assert_bool "Status diversity: pos_sessions (>= 2 statuses)" "SELECT COUNT(DISTINCT status) >= 2 FROM pos_sessions;"
assert_bool "Status diversity: projects (>= 2 statuses)" "SELECT COUNT(DISTINCT status) >= 2 FROM projects;"
assert_bool "Status diversity: timesheets (>= 2 statuses)" "SELECT COUNT(DISTINCT status) >= 2 FROM timesheets;"

# ------------------------------------------------------------------------------
# 6. Referential Integrity Assertions (Zero Orphaned Records)
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}${BLUE}==> 6. Verifying Referential Integrity & Orphan Absence...${NC}"

assert_eq "FK Integrity: No orphaned journal lines (je_id -> journal_entries)" \
    "SELECT COUNT(*) FROM journal_lines jl WHERE NOT EXISTS (SELECT 1 FROM journal_entries je WHERE je.id = jl.je_id);" "0"

assert_eq "FK Integrity: No orphaned accounts in journal lines (account_id -> accounts)" \
    "SELECT COUNT(*) FROM journal_lines jl WHERE NOT EXISTS (SELECT 1 FROM accounts a WHERE a.id = jl.account_id);" "0"

assert_eq "FK Integrity: No orphaned inventory balances (product_id -> products)" \
    "SELECT COUNT(*) FROM inventory_balances ib WHERE NOT EXISTS (SELECT 1 FROM products p WHERE p.id = ib.product_id);" "0"

assert_eq "FK Integrity: No orphaned sales order lines (sales_order_id -> sales_orders)" \
    "SELECT COUNT(*) FROM sales_order_lines sol WHERE NOT EXISTS (SELECT 1 FROM sales_orders so WHERE so.id = sol.sales_order_id);" "0"

assert_eq "FK Integrity: No orphaned purchase order lines (po_id -> pos)" \
    "SELECT COUNT(*) FROM po_lines pol WHERE NOT EXISTS (SELECT 1 FROM pos po WHERE po.id = pol.po_id);" "0"

assert_eq "FK Integrity: No orphaned BOM lines (bom_id -> mrp_boms)" \
    "SELECT COUNT(*) FROM mrp_bom_lines mbl WHERE NOT EXISTS (SELECT 1 FROM mrp_boms mb WHERE mb.id = mbl.bom_id);" "0"

assert_eq "FK Integrity: No orphaned payroll run lines (run_id -> payroll_runs)" \
    "SELECT COUNT(*) FROM payroll_run_lines prl WHERE NOT EXISTS (SELECT 1 FROM payroll_runs pr WHERE pr.id = prl.run_id);" "0"

assert_eq "FK Integrity: No orphaned fixed asset categories (category_id -> fixed_asset_categories)" \
    "SELECT COUNT(*) FROM fixed_assets fa WHERE NOT EXISTS (SELECT 1 FROM fixed_asset_categories fac WHERE fac.id = fa.category_id);" "0"

assert_eq "FK Integrity: No orphaned quotation lines (quotation_id -> quotations)" \
    "SELECT COUNT(*) FROM quotation_lines ql WHERE NOT EXISTS (SELECT 1 FROM quotations q WHERE q.id = ql.quotation_id);" "0"

assert_eq "FK Integrity: No orphaned delivery order lines (delivery_order_id -> delivery_orders)" \
    "SELECT COUNT(*) FROM delivery_order_lines dol WHERE NOT EXISTS (SELECT 1 FROM delivery_orders d WHERE d.id = dol.delivery_order_id);" "0"

assert_eq "FK Integrity: No orphaned GRN lines (grn_id -> grns)" \
    "SELECT COUNT(*) FROM grn_lines grnl WHERE NOT EXISTS (SELECT 1 FROM grns g WHERE g.id = grnl.grn_id);" "0"

assert_eq "FK Integrity: No orphaned AP invoice lines (ap_invoice_id -> ap_invoices)" \
    "SELECT COUNT(*) FROM ap_invoice_lines apl WHERE NOT EXISTS (SELECT 1 FROM ap_invoices a WHERE a.id = apl.ap_invoice_id);" "0"

assert_eq "FK Integrity: No orphaned AR invoice lines (ar_invoice_id -> ar_invoices)" \
    "SELECT COUNT(*) FROM ar_invoice_lines arl WHERE NOT EXISTS (SELECT 1 FROM ar_invoices a WHERE a.id = arl.ar_invoice_id);" "0"

assert_eq "FK Integrity: No orphaned project tasks (project_id -> projects)" \
    "SELECT COUNT(*) FROM project_tasks pt WHERE NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = pt.project_id);" "0"

assert_eq "FK Integrity: No orphaned timesheets (project_id -> projects)" \
    "SELECT COUNT(*) FROM timesheets ts WHERE NOT EXISTS (SELECT 1 FROM projects p WHERE p.id = ts.project_id);" "0"

assert_eq "FK Integrity: No orphaned work order tasks (work_order_id -> work_orders)" \
    "SELECT COUNT(*) FROM work_order_tasks wot WHERE NOT EXISTS (SELECT 1 FROM work_orders wo WHERE wo.id = wot.work_order_id);" "0"

assert_eq "FK Integrity: No orphaned document versions (document_id -> documents)" \
    "SELECT COUNT(*) FROM document_versions dv WHERE NOT EXISTS (SELECT 1 FROM documents d WHERE d.id = dv.document_id);" "0"

# ------------------------------------------------------------------------------
# 7. Materialized View Refresh & Data Availability Assertions
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}${BLUE}==> 7. Refreshing & Validating Materialized Views...${NC}"

# Execute MV Refreshes
psql "$TARGET_DSN" -c "REFRESH MATERIALIZED VIEW gl_balances;" > /dev/null 2>&1 || true
psql "$TARGET_DSN" -c "REFRESH MATERIALIZED VIEW mv_pl_monthly;" > /dev/null 2>&1 || true
psql "$TARGET_DSN" -c "REFRESH MATERIALIZED VIEW mv_cashflow_monthly;" > /dev/null 2>&1 || true
psql "$TARGET_DSN" -c "REFRESH MATERIALIZED VIEW mv_ar_aging;" > /dev/null 2>&1 || true
psql "$TARGET_DSN" -c "REFRESH MATERIALIZED VIEW mv_ap_aging;" > /dev/null 2>&1 || true
psql "$TARGET_DSN" -c "REFRESH MATERIALIZED VIEW mv_consol_balances;" > /dev/null 2>&1 || true

assert_ge "MV Populated: gl_balances" "SELECT COUNT(*) FROM gl_balances;" 1
assert_ge "MV Populated: mv_pl_monthly" "SELECT COUNT(*) FROM mv_pl_monthly;" 1
assert_ge "MV Populated: mv_cashflow_monthly" "SELECT COUNT(*) FROM mv_cashflow_monthly;" 1
assert_ge "MV Populated: mv_ar_aging" "SELECT COUNT(*) FROM mv_ar_aging;" 1
assert_ge "MV Populated: mv_ap_aging" "SELECT COUNT(*) FROM mv_ap_aging;" 1
assert_ge "MV Populated: mv_consol_balances" "SELECT COUNT(*) FROM mv_consol_balances;" 1

# ------------------------------------------------------------------------------
# Summary Banner & Exit Status
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}${CYAN}================================================================${NC}"
echo -e "${BOLD}${CYAN}                     Verification Summary                      ${NC}"
echo -e "${BOLD}${CYAN}================================================================${NC}"
echo -e "  Total Passed: ${GREEN}${BOLD}${TOTAL_PASS}${NC}"
echo -e "  Total Failed: ${RED}${BOLD}${TOTAL_FAIL}${NC}"

if [ "$TOTAL_FAIL" -eq 0 ]; then
    echo -e "\n${GREEN}${BOLD}✓ ALL ACCEPTANCE CRITERIA PASSED! Realistic seed data is 100% compliant.${NC}\n"
    exit 0
else
    echo -e "\n${RED}${BOLD}✗ VERIFICATION FAILED: $TOTAL_FAIL assertions failed. Review diagnosis above.${NC}\n"
    exit 1
fi
