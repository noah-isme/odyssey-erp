package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/google/uuid"
)

// ISO-004 fixture definitions. Every statement in this file is either a
// SELECT or an INSERT ... ON CONFLICT DO NOTHING. The extension never
// modifies rows it finds; iso004_sql_test.go scans these files and fails if a
// row-modifying statement appears.

const (
	iso004CompanyACode       = "ODY-01"
	iso004CompanyBCode       = "ODY-02"
	iso004BranchACode        = "HQ-JKT"
	iso004BranchBCode        = "BR-BDG"
	iso004UnitCode           = "EA"
	iso004ClassificationCode = "CONFIDENTIAL"
	iso004ForecastName       = "ISO004-CERT"
	iso004PeriodCode         = "ISO004-B"
	iso004ActorBEmail        = "iso004-actor-b@staging.invalid"
	// iso004NoLoginHash is not a bcrypt hash, so the attribution user can
	// never authenticate.
	iso004NoLoginHash = "!iso004-no-login"
)

// iso004Refs carries resolved base IDs, created fixture IDs and derived
// strings. In dry-run mode an unresolved reference renders as a placeholder.
type iso004Refs struct {
	key  string
	ids  map[string]int64
	strs map[string]string
}

func newISO004Refs(key string) *iso004Refs {
	return &iso004Refs{key: key, ids: map[string]int64{}, strs: map[string]string{}}
}

// id returns the resolved ID or a "<name>" placeholder for dry-run output.
func (r *iso004Refs) id(name string) any {
	if v, ok := r.ids[name]; ok {
		return v
	}
	return "<" + name + ".id>"
}

func (r *iso004Refs) str(name string) string { return r.strs[name] }

func (r *iso004Refs) company(name string) string {
	if v, ok := r.ids[name]; ok {
		return "company:" + strconv.FormatInt(v, 10)
	}
	return "company:<" + name + ".id>"
}

func (r *iso004Refs) code(prefix string) string { return prefix + "-" + r.key }

// iso004Step is one fixture row. Lookup finds an existing row by its natural
// key (never filtered by company, so a row owned by the wrong company is
// found and rejected); Insert creates it; OwnerByID returns the row's scope
// string, which must equal Expect.
type iso004Step struct {
	Name       string
	Table      string
	Purpose    string
	Lookup     string
	LookupArgs func(r *iso004Refs) []any
	Insert     string
	InsertArgs func(r *iso004Refs) []any
	OwnerByID  string
	Expect     func(r *iso004Refs) string
}

// iso004BaseQueries resolve prerequisites created by the base seeder. They
// are SELECT-only; a missing row aborts the extension.
var iso004BaseQueries = []struct {
	Name  string
	What  string
	SQL   string
	Args  func(adminEmail string) []any
	Check string // owner query returning scope text, optional
}{
	{Name: "company_a", What: "company " + iso004CompanyACode, SQL: `SELECT id FROM companies WHERE code = $1`, Args: func(string) []any { return []any{iso004CompanyACode} }},
	{Name: "company_b", What: "company " + iso004CompanyBCode, SQL: `SELECT id FROM companies WHERE code = $1`, Args: func(string) []any { return []any{iso004CompanyBCode} }},
	{Name: "branch_a", What: "branch " + iso004BranchACode, SQL: `SELECT id FROM branches WHERE code = $1`, Args: func(string) []any { return []any{iso004BranchACode} }},
	{Name: "branch_b", What: "branch " + iso004BranchBCode, SQL: `SELECT id FROM branches WHERE code = $1`, Args: func(string) []any { return []any{iso004BranchBCode} }},
	{Name: "admin_a", What: "admin user (STAGING_CERT_ADMIN_EMAIL)", SQL: `SELECT id FROM users WHERE email = $1`, Args: func(e string) []any { return []any{e} }},
	{Name: "unit", What: "unit " + iso004UnitCode, SQL: `SELECT id FROM units WHERE code = $1`, Args: func(string) []any { return []any{iso004UnitCode} }},
}

// iso004BaseChecks are SELECT-only consistency checks over the resolved base
// rows; each must return exactly true.
var iso004BaseChecks = []struct {
	What string
	SQL  string
	Args func(r *iso004Refs) []any
}{
	{What: "company A and B are distinct", SQL: `SELECT $1::bigint <> $2::bigint`, Args: func(r *iso004Refs) []any { return []any{r.ids["company_a"], r.ids["company_b"]} }},
	{What: "branch " + iso004BranchACode + " belongs to company A", SQL: `SELECT EXISTS (SELECT 1 FROM branches WHERE id = $1 AND company_id = $2)`, Args: func(r *iso004Refs) []any { return []any{r.ids["branch_a"], r.ids["company_a"]} }},
	{What: "branch " + iso004BranchBCode + " belongs to company B", SQL: `SELECT EXISTS (SELECT 1 FROM branches WHERE id = $1 AND company_id = $2)`, Args: func(r *iso004Refs) []any { return []any{r.ids["branch_b"], r.ids["company_b"]} }},
	{What: "company A and B have base_currency set", SQL: `SELECT COUNT(*) = 2 FROM companies WHERE id IN ($1, $2) AND base_currency ~ '^[A-Z]{3}$'`, Args: func(r *iso004Refs) []any { return []any{r.ids["company_a"], r.ids["company_b"]} }},
	{What: "no active global AP matching policy (company_id IS NULL AND supplier_id IS NULL); it would turn the MISSING_MAPPING path into a match", SQL: `SELECT NOT EXISTS (SELECT 1 FROM ap_matching_policies WHERE company_id IS NULL AND supplier_id IS NULL AND effective_from <= CURRENT_DATE AND (effective_to IS NULL OR effective_to >= CURRENT_DATE))`, Args: func(*iso004Refs) []any { return nil }},
}

// iso004LateBase are resolved after the base checks (they need company IDs).
var iso004LateBase = []struct {
	Name string
	What string
	SQL  string
	Args func(r *iso004Refs) []any
}{
	{Name: "classification_a", What: "document classification " + iso004ClassificationCode + " in company A", SQL: `SELECT id FROM document_classifications WHERE company_id = $1 AND code = $2`, Args: func(r *iso004Refs) []any { return []any{r.ids["company_a"], iso004ClassificationCode} }},
	{Name: "tax_rule", What: "reviewed payroll TAX rule version effective today", SQL: `SELECT id FROM payroll_rule_versions WHERE rule_type = 'TAX' AND reviewed_at IS NOT NULL AND effective_from <= CURRENT_DATE AND (effective_to IS NULL OR effective_to >= CURRENT_DATE) ORDER BY effective_from DESC, id DESC LIMIT 1`, Args: func(*iso004Refs) []any { return nil }},
	{Name: "bpjs_rule", What: "reviewed payroll BPJS rule version effective today", SQL: `SELECT id FROM payroll_rule_versions WHERE rule_type = 'BPJS' AND reviewed_at IS NOT NULL AND effective_from <= CURRENT_DATE AND (effective_to IS NULL OR effective_to >= CURRENT_DATE) ORDER BY effective_from DESC, id DESC LIMIT 1`, Args: func(*iso004Refs) []any { return nil }},
}

// iso004Advisories are SELECT-only observations about global configuration
// the extension deliberately does not create. A false result is printed as a
// warning, not an abort.
var iso004Advisories = []struct {
	What string
	SQL  string
}{
	{What: "an OPEN periods row covers today (needed for ISO004-AP-B-MATCHED auto-post; otherwise the run records CLOSED_PERIOD)", SQL: `SELECT EXISTS (SELECT 1 FROM periods WHERE status = 'OPEN' AND CURRENT_DATE BETWEEN start_date AND end_date)`},
	{What: "account_mappings AP/ap.invoice.inventory exists (needed for auto-post journal)", SQL: `SELECT EXISTS (SELECT 1 FROM account_mappings WHERE module = 'AP' AND key = 'ap.invoice.inventory')`},
	{What: "account_mappings AP/ap.invoice.ap exists (needed for auto-post journal)", SQL: `SELECT EXISTS (SELECT 1 FROM account_mappings WHERE module = 'AP' AND key = 'ap.invoice.ap')`},
}

// iso004MatchedVariantSQL finds the latest ISO004-AP-B-MATCHED invoice for
// the key (base number or a "-rN" recreation) and whether it is still DRAFT.
const iso004MatchedVariantSQL = `SELECT number, status, (SELECT COUNT(*) FROM ap_invoices WHERE number = $1 OR left(number, length($1) + 2) = $1 || '-r')
FROM ap_invoices
WHERE number = $1 OR left(number, length($1) + 2) = $1 || '-r'
ORDER BY id DESC
LIMIT 1`

func iso004Checksum(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func iso004RunUUID(key string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("odyssey:iso004:payroll_run:"+key))
}

func iso004DocContent(key string) string {
	return "ISO-004 certification fixture document " + key + "\n"
}

// iso004Steps returns the ordered fixture plan.
func iso004Steps() []iso004Step {
	companyB := func(r *iso004Refs) string { return r.company("company_b") }
	companyA := func(r *iso004Refs) string { return r.company("company_a") }

	return []iso004Step{
		// --- attribution user for company B rows (cannot log in, no roles) ---
		{
			Name: "actor_b", Table: "users", Purpose: "company B attribution user (created_by); inactive, no password, no roles",
			Lookup:     `SELECT id FROM users WHERE email = $1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{iso004ActorBEmail} },
			Insert:     `INSERT INTO users (email, password_hash, is_active, name) VALUES ($1, $2, FALSE, 'ISO-004 company B attribution') ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{iso004ActorBEmail, iso004NoLoginHash} },
			OwnerByID:  `SELECT CASE WHEN is_active = FALSE AND password_hash = '` + iso004NoLoginHash + `' AND NOT EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = u.id) THEN 'no-login' ELSE 'login-capable' END FROM users u WHERE u.id = $1`,
			Expect:     func(*iso004Refs) string { return "no-login" },
		},

		// --- forecast scenarios (A and B), keyed by company + fixed name ---
		{
			Name: "forecast_a", Table: "forecast_scenarios", Purpose: "forecast scenario for company A",
			Lookup:     `SELECT id FROM forecast_scenarios WHERE company_id = $1 AND name = $2 ORDER BY id LIMIT 1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.id("company_a"), iso004ForecastName} },
			Insert:     `INSERT INTO forecast_scenarios (company_id, name, policy_version) VALUES ($1, $2, 'iso004-cert') ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{r.id("company_a"), iso004ForecastName} },
			OwnerByID:  `SELECT 'company:' || company_id FROM forecast_scenarios WHERE id = $1`,
			Expect:     companyA,
		},
		{
			Name: "forecast_b", Table: "forecast_scenarios", Purpose: "forecast scenario for company B",
			Lookup:     `SELECT id FROM forecast_scenarios WHERE company_id = $1 AND name = $2 ORDER BY id LIMIT 1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.id("company_b"), iso004ForecastName} },
			Insert:     `INSERT INTO forecast_scenarios (company_id, name, policy_version) VALUES ($1, $2, 'iso004-cert') ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{r.id("company_b"), iso004ForecastName} },
			OwnerByID:  `SELECT 'company:' || company_id FROM forecast_scenarios WHERE id = $1`,
			Expect:     companyB,
		},

		// --- company B variance fixture ---
		{
			Name: "period_b", Table: "periods", Purpose: "dedicated historical period row backing company B's accounting period (status CLOSED so no date is made postable)",
			Lookup:     `SELECT id FROM periods WHERE code = $1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{iso004PeriodCode} },
			Insert:     `INSERT INTO periods (code, start_date, end_date, status) VALUES ($1, DATE '1999-12-01', DATE '1999-12-31', 'CLOSED') ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{iso004PeriodCode} },
			OwnerByID:  `SELECT COALESCE('company:' || ap.company_id, 'unbound') FROM periods p LEFT JOIN accounting_periods ap ON ap.period_id = p.id WHERE p.id = $1`,
			Expect:     companyB,
		},
		{
			Name: "accounting_period_b", Table: "accounting_periods", Purpose: "OPEN accounting period for company B",
			Lookup:     `SELECT id FROM accounting_periods WHERE period_id = $1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.id("period_b")} },
			Insert:     `INSERT INTO accounting_periods (period_id, company_id, name, start_date, end_date, status) VALUES ($1, $2, $3, DATE '1999-12-01', DATE '1999-12-31', 'OPEN') ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{r.id("period_b"), r.id("company_b"), iso004PeriodCode} },
			OwnerByID:  `SELECT 'company:' || company_id FROM accounting_periods WHERE id = $1`,
			Expect:     companyB,
		},
		{
			Name: "variance_rule_b", Table: "variance_rules", Purpose: "variance rule for company B",
			Lookup:     `SELECT id FROM variance_rules WHERE name = $1 ORDER BY id LIMIT 1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.code("ISO004-VAR-B")} },
			Insert:     `INSERT INTO variance_rules (company_id, name, comparison_type, base_period_id, compare_period_id, created_by) VALUES ($1, $2, 'ACTUAL_VS_ACTUAL', $3, $3, $4) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any {
				return []any{r.id("company_b"), r.code("ISO004-VAR-B"), r.id("accounting_period_b"), r.id("actor_b")}
			},
			OwnerByID: `SELECT 'company:' || vr.company_id || CASE WHEN ap.company_id = vr.company_id THEN '' ELSE '/period-mismatch' END FROM variance_rules vr JOIN accounting_periods ap ON ap.id = vr.base_period_id WHERE vr.id = $1`,
			Expect:    companyB,
		},
		{
			Name: "variance_snapshot_b", Table: "variance_snapshots", Purpose: "PENDING variance snapshot for company B",
			Lookup:     `SELECT id FROM variance_snapshots WHERE rule_id = $1 ORDER BY id LIMIT 1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.id("variance_rule_b")} },
			Insert:     `INSERT INTO variance_snapshots (rule_id, period_id, status) VALUES ($1, $2, 'PENDING') ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{r.id("variance_rule_b"), r.id("accounting_period_b")} },
			OwnerByID:  `SELECT 'company:' || vr.company_id FROM variance_snapshots vs JOIN variance_rules vr ON vr.id = vs.rule_id WHERE vs.id = $1`,
			Expect:     companyB,
		},

		// --- company B AP fixtures ---
		{
			Name: "category_b", Table: "categories", Purpose: "product category for company B",
			Lookup:     `SELECT id FROM categories WHERE code = $1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.code("ISO004-CAT-B")} },
			Insert:     `INSERT INTO categories (code, name, company_id) VALUES ($1, 'ISO-004 certification category', $2) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{r.code("ISO004-CAT-B"), r.id("company_b")} },
			OwnerByID:  `SELECT 'company:' || company_id FROM categories WHERE id = $1`,
			Expect:     companyB,
		},
		{
			Name: "product_b", Table: "products", Purpose: "product for company B",
			Lookup:     `SELECT id FROM products WHERE sku = $1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.code("ISO004-PROD-B")} },
			Insert:     `INSERT INTO products (sku, name, category_id, unit_id, price, is_active, company_id) VALUES ($1, 'ISO-004 certification product', $2, $3, 50000.00, TRUE, $4) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any {
				return []any{r.code("ISO004-PROD-B"), r.id("category_b"), r.id("unit"), r.id("company_b")}
			},
			OwnerByID: `SELECT 'company:' || p.company_id || CASE WHEN c.company_id = p.company_id THEN '' ELSE '/category-mismatch' END FROM products p JOIN categories c ON c.id = p.category_id WHERE p.id = $1`,
			Expect:    companyB,
		},
		{
			Name: "warehouse_b", Table: "warehouses", Purpose: "warehouse on branch " + iso004BranchBCode,
			Lookup:     `SELECT id FROM warehouses WHERE code = $1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.code("ISO004-WH-B")} },
			Insert:     `INSERT INTO warehouses (branch_id, code, name) VALUES ($1, $2, 'ISO-004 certification warehouse') ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{r.id("branch_b"), r.code("ISO004-WH-B")} },
			OwnerByID:  `SELECT 'company:' || b.company_id FROM warehouses w JOIN branches b ON b.id = w.branch_id WHERE w.id = $1`,
			Expect:     companyB,
		},
		{
			Name: "supplier_b_nopolicy", Table: "suppliers", Purpose: "company B supplier without a matching policy (MISSING_MAPPING path)",
			Lookup:     `SELECT id FROM suppliers WHERE code = $1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.code("ISO004-SUP-B-NOPOL")} },
			Insert:     `INSERT INTO suppliers (code, name, is_active, company_id) VALUES ($1, 'ISO-004 supplier without policy', TRUE, $2) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{r.code("ISO004-SUP-B-NOPOL"), r.id("company_b")} },
			OwnerByID:  `SELECT 'company:' || company_id FROM suppliers WHERE id = $1`,
			Expect:     companyB,
		},
		{
			Name: "supplier_b_policy", Table: "suppliers", Purpose: "company B supplier with a supplier-scoped matching policy",
			Lookup:     `SELECT id FROM suppliers WHERE code = $1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.code("ISO004-SUP-B-POL")} },
			Insert:     `INSERT INTO suppliers (code, name, is_active, company_id) VALUES ($1, 'ISO-004 supplier with policy', TRUE, $2) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{r.code("ISO004-SUP-B-POL"), r.id("company_b")} },
			OwnerByID:  `SELECT 'company:' || company_id FROM suppliers WHERE id = $1`,
			Expect:     companyB,
		},
		{
			Name: "ap_policy_b", Table: "ap_matching_policies", Purpose: "matching policy with company_id NULL and supplier_id = policy supplier (the matcher passes company_id = nil)",
			Lookup:     `SELECT id FROM ap_matching_policies WHERE name = $1 ORDER BY id LIMIT 1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.code("ISO004-POL-B")} },
			Insert:     `INSERT INTO ap_matching_policies (name, company_id, supplier_id, effective_from) VALUES ($1, NULL, $2, DATE '2000-01-01') ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{r.code("ISO004-POL-B"), r.id("supplier_b_policy")} },
			OwnerByID:  `SELECT CASE WHEN p.company_id IS NULL THEN 'company:' || s.company_id ELSE 'policy-company:' || p.company_id END FROM ap_matching_policies p JOIN suppliers s ON s.id = p.supplier_id WHERE p.id = $1`,
			Expect:     companyB,
		},
		{
			Name: "po_b", Table: "pos", Purpose: "approved purchase order for the policy supplier",
			Lookup:     `SELECT id FROM pos WHERE number = $1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.code("ISO004-PO-B")} },
			Insert:     `INSERT INTO pos (number, supplier_id, status, currency, note, company_id, expected_warehouse_id, approved_by, approved_at) VALUES ($1, $2, 'APPROVED', $3, 'ISO-004 certification fixture', $4, $5, $6, NOW()) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any {
				return []any{r.code("ISO004-PO-B"), r.id("supplier_b_policy"), r.str("currency_b"), r.id("company_b"), r.id("warehouse_b"), r.id("actor_b")}
			},
			OwnerByID: `SELECT 'company:' || po.company_id || CASE WHEN s.company_id = po.company_id THEN '' ELSE '/supplier-mismatch' END FROM pos po JOIN suppliers s ON s.id = po.supplier_id WHERE po.id = $1`,
			Expect:    companyB,
		},
		{
			Name: "po_line_b", Table: "po_lines", Purpose: "PO line: 10 x 50000",
			Lookup:     `SELECT id FROM po_lines WHERE po_id = $1 ORDER BY id LIMIT 1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.id("po_b")} },
			Insert:     `INSERT INTO po_lines (po_id, product_id, qty, price, note) VALUES ($1, $2, 10.0000, 50000.0000, 'ISO-004') ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{r.id("po_b"), r.id("product_b")} },
			OwnerByID:  `SELECT 'company:' || po.company_id FROM po_lines l JOIN pos po ON po.id = l.po_id WHERE l.id = $1`,
			Expect:     companyB,
		},
		{
			Name: "grn_b", Table: "grns", Purpose: "POSTED GRN receiving the full PO quantity",
			Lookup:     `SELECT id FROM grns WHERE number = $1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.code("ISO004-GRN-B")} },
			Insert:     `INSERT INTO grns (number, po_id, supplier_id, warehouse_id, status, received_at, note, company_id) VALUES ($1, $2, $3, $4, 'POSTED', NOW(), 'ISO-004 certification fixture', $5) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any {
				return []any{r.code("ISO004-GRN-B"), r.id("po_b"), r.id("supplier_b_policy"), r.id("warehouse_b"), r.id("company_b")}
			},
			OwnerByID: `SELECT 'company:' || g.company_id || CASE WHEN po.company_id = g.company_id THEN '' ELSE '/po-mismatch' END FROM grns g JOIN pos po ON po.id = g.po_id WHERE g.id = $1`,
			Expect:    companyB,
		},
		{
			Name: "grn_line_b", Table: "grn_lines", Purpose: "GRN line: 10 x 50000",
			Lookup:     `SELECT id FROM grn_lines WHERE grn_id = $1 ORDER BY id LIMIT 1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.id("grn_b")} },
			Insert:     `INSERT INTO grn_lines (grn_id, product_id, qty, unit_cost) VALUES ($1, $2, 10.0000, 50000.0000) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{r.id("grn_b"), r.id("product_b")} },
			OwnerByID:  `SELECT 'company:' || g.company_id FROM grn_lines l JOIN grns g ON g.id = l.grn_id WHERE l.id = $1`,
			Expect:     companyB,
		},
		iso004InvoiceStep("ap_invoice_b_missing", "DRAFT invoice from the supplier without a policy (expects MISSING_MAPPING)", "supplier_b_nopolicy", false, 100000),
		iso004InvoiceLineStep("ap_invoice_b_missing", false, 1, 100000),
		iso004InvoiceStep("ap_invoice_b_exception", "DRAFT invoice with a quantity mismatch against the PO (expects MISMATCH exception)", "supplier_b_policy", true, 600000),
		iso004InvoiceLineStep("ap_invoice_b_exception", true, 12, 50000),
		iso004InvoiceStep("ap_invoice_b_matched", "DRAFT invoice with exact PO/GRN totals (expects MATCHED and auto-post)", "supplier_b_policy", true, 500000),
		iso004InvoiceLineStep("ap_invoice_b_matched", true, 10, 50000),

		// --- forged OCR job: company B job pointing at a company A version ---
		{
			Name: "blob_a", Table: "storage_blobs", Purpose: "minimal company A blob record (no file is written)",
			Lookup:     `SELECT id FROM storage_blobs WHERE company_id = $1 AND storage_key = $2`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.id("company_a"), "iso004/" + r.key + "/document-a.txt"} },
			Insert:     `INSERT INTO storage_blobs (company_id, storage_key, storage_driver, size_bytes, checksum_sha256, declared_content_type, detected_content_type, created_by) VALUES ($1, $2, 'local', $3, $4, 'text/plain', 'text/plain', $5) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any {
				content := iso004DocContent(r.key)
				sum := sha256.Sum256([]byte(content))
				return []any{r.id("company_a"), "iso004/" + r.key + "/document-a.txt", int64(len(content)), hex.EncodeToString(sum[:]), r.id("admin_a")}
			},
			OwnerByID: `SELECT 'company:' || company_id FROM storage_blobs WHERE id = $1`,
			Expect:    companyA,
		},
		{
			Name: "document_a", Table: "documents", Purpose: "minimal company A document",
			Lookup:     `SELECT id FROM documents WHERE company_id = $1 AND document_number = $2`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.id("company_a"), r.code("ISO004-DOC-A")} },
			Insert:     `INSERT INTO documents (company_id, classification_id, document_number, title, owner_id, status, created_by, updated_by) VALUES ($1, $2, $3, 'ISO-004 certification document', $4, 'DRAFT', $4, $4) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any {
				return []any{r.id("company_a"), r.id("classification_a"), r.code("ISO004-DOC-A"), r.id("admin_a")}
			},
			OwnerByID: `SELECT 'company:' || company_id FROM documents WHERE id = $1`,
			Expect:    companyA,
		},
		{
			Name: "document_version_a", Table: "document_versions", Purpose: "company A document version 1",
			Lookup:     `SELECT id FROM document_versions WHERE company_id = $1 AND document_id = $2 AND version_number = 1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.id("company_a"), r.id("document_a")} },
			Insert:     `INSERT INTO document_versions (company_id, document_id, version_number, blob_id, status, classification_id, created_by) VALUES ($1, $2, 1, $3, 'DRAFT', $4, $5) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any {
				return []any{r.id("company_a"), r.id("document_a"), r.id("blob_a"), r.id("classification_a"), r.id("admin_a")}
			},
			OwnerByID: `SELECT 'company:' || v.company_id || CASE WHEN d.company_id = v.company_id AND b.company_id = v.company_id THEN '' ELSE '/chain-mismatch' END FROM document_versions v JOIN documents d ON d.id = v.document_id JOIN storage_blobs b ON b.id = v.blob_id WHERE v.id = $1`,
			Expect:    companyA,
		},
		{
			Name: "ocr_job_forged", Table: "doc_ocr_jobs", Purpose: "forged OCR job: company_id = B, document version and blob owned by company A",
			Lookup:     `SELECT id FROM doc_ocr_jobs WHERE document_version_id = $1 ORDER BY id LIMIT 1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.id("document_version_a")} },
			Insert:     `INSERT INTO doc_ocr_jobs (company_id, document_version_id, blob_id, status) VALUES ($1, $2, $3, 'PENDING') ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{r.id("company_b"), r.id("document_version_a"), r.id("blob_a")} },
			OwnerByID:  `SELECT 'company:' || j.company_id || '/version-company:' || v.company_id FROM doc_ocr_jobs j JOIN document_versions v ON v.id = j.document_version_id WHERE j.id = $1`,
			Expect: func(r *iso004Refs) string {
				return r.company("company_b") + "/version-" + r.company("company_a")
			},
		},

		// --- company B payslip chain (fresh per key) ---
		{
			Name: "payroll_policy_b", Table: "payroll_company_policies", Purpose: "company B payroll policy (existing one reused; otherwise a single-day policy on 2000-01-01)",
			Lookup:     `SELECT id FROM payroll_company_policies WHERE company_id = $1 ORDER BY effective_from DESC, id DESC LIMIT 1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.id("company_b")} },
			Insert:     `INSERT INTO payroll_company_policies (rule_version_id, company_id, currency, effective_from, effective_to) VALUES ($1, $2, $3, DATE '2000-01-01', DATE '2000-01-01') ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{r.id("bpjs_rule"), r.id("company_b"), r.str("currency_b")} },
			OwnerByID:  `SELECT 'company:' || company_id FROM payroll_company_policies WHERE id = $1`,
			Expect:     companyB,
		},
		{
			Name: "employee_b", Table: "hr_employees", Purpose: "company B employee with a unique non-deliverable address",
			Lookup:     `SELECT id FROM hr_employees WHERE company_id = $1 AND employee_number = $2`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.id("company_b"), r.code("ISO004-EMP-B")} },
			Insert:     `INSERT INTO hr_employees (company_id, employee_number, name, email, hire_date, status) VALUES ($1, $2, 'ISO-004 payslip recipient', $3, DATE '2000-01-01', 'ACTIVE') ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any {
				return []any{r.id("company_b"), r.code("ISO004-EMP-B"), r.str("payslip_email")}
			},
			OwnerByID: `SELECT 'company:' || company_id FROM hr_employees WHERE id = $1`,
			Expect:    companyB,
		},
		{
			Name: "payroll_period_b", Table: "payroll_periods", Purpose: "company B payroll period (historical, CLOSED)",
			Lookup:     `SELECT id FROM payroll_periods WHERE company_id = $1 AND code = $2`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.id("company_b"), r.code("ISO004")} },
			Insert:     `INSERT INTO payroll_periods (company_id, code, starts_on, ends_on, pay_date, status) VALUES ($1, $2, DATE '2000-01-01', DATE '2000-01-31', DATE '2000-01-31', 'CLOSED') ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any { return []any{r.id("company_b"), r.code("ISO004")} },
			OwnerByID:  `SELECT 'company:' || company_id FROM payroll_periods WHERE id = $1`,
			Expect:     companyB,
		},
		{
			Name: "payroll_run_b", Table: "payroll_runs", Purpose: "company B DRAFT payroll run (run_uuid derived from the key)",
			Lookup:     `SELECT id FROM payroll_runs WHERE run_uuid = $1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{iso004RunUUID(r.key)} },
			Insert:     `INSERT INTO payroll_runs (run_uuid, company_id, period_id, run_type, tax_rule_version_id, bpjs_rule_version_id, company_policy_id, status, created_by) VALUES ($1, $2, $3, 'REGULAR', $4, $5, $6, 'DRAFT', $7) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any {
				return []any{iso004RunUUID(r.key), r.id("company_b"), r.id("payroll_period_b"), r.id("tax_rule"), r.id("bpjs_rule"), r.id("payroll_policy_b"), r.id("actor_b")}
			},
			OwnerByID: `SELECT 'company:' || r.company_id || CASE WHEN p.company_id = r.company_id AND cp.company_id = r.company_id THEN '' ELSE '/chain-mismatch' END FROM payroll_runs r JOIN payroll_periods p ON p.id = r.period_id JOIN payroll_company_policies cp ON cp.id = r.company_policy_id WHERE r.id = $1`,
			Expect:    companyB,
		},
		{
			Name: "payroll_run_line_b", Table: "payroll_run_lines", Purpose: "run line for the company B employee",
			Lookup:     `SELECT id FROM payroll_run_lines WHERE run_id = $1 AND employee_id = $2`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.id("payroll_run_b"), r.id("employee_b")} },
			Insert:     `INSERT INTO payroll_run_lines (run_id, employee_id, ptkp_code, ter_category, base_salary, gross, net_pay, breakdown) VALUES ($1, $2, 'TK/0', 'A', 1000000.00, 1000000.00, 1000000.00, $3::jsonb) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any {
				return []any{r.id("payroll_run_b"), r.id("employee_b"), fmt.Sprintf(`{"EmployeeID":%v,"PTKPCode":"TK/0","TERCategory":"A","BaseSalary":1000000,"Gross":1000000,"NetPay":1000000}`, r.id("employee_b"))}
			},
			OwnerByID: `SELECT 'company:' || r.company_id || CASE WHEN e.company_id = r.company_id THEN '' ELSE '/employee-mismatch' END FROM payroll_run_lines l JOIN payroll_runs r ON r.id = l.run_id JOIN hr_employees e ON e.id = l.employee_id WHERE l.id = $1`,
			Expect:    companyB,
		},
		{
			Name: "payslip_b", Table: "payroll_payslips", Purpose: "undelivered company B payslip",
			Lookup:     `SELECT id FROM payroll_payslips WHERE run_line_id = $1`,
			LookupArgs: func(r *iso004Refs) []any { return []any{r.id("payroll_run_line_b")} },
			Insert:     `INSERT INTO payroll_payslips (run_line_id, document_key, checksum) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING RETURNING id`,
			InsertArgs: func(r *iso004Refs) []any {
				return []any{r.id("payroll_run_line_b"), "iso004/" + r.key + "/payslip-b.pdf", iso004Checksum("payslip", r.key)}
			},
			OwnerByID: `SELECT 'company:' || r.company_id || '/employee-company:' || e.company_id FROM payroll_payslips ps JOIN payroll_run_lines l ON l.id = ps.run_line_id JOIN payroll_runs r ON r.id = l.run_id JOIN hr_employees e ON e.id = l.employee_id WHERE ps.id = $1`,
			Expect: func(r *iso004Refs) string {
				return r.company("company_b") + "/employee-" + r.company("company_b")
			},
		},
	}
}

// iso004InvoiceStep builds an AP invoice fixture. The invoice number comes
// from refs (resolved before the steps run) so a consumed MATCHED invoice can
// be recreated under a "-rN" suffix.
func iso004InvoiceStep(name, purpose, supplierRef string, withPO bool, total int64) iso004Step {
	numberRef := name + ".number"
	insertSQL := `INSERT INTO ap_invoices (number, supplier_id, currency, total, subtotal, status, issued_at, due_at, company_id, created_by, base_currency, duplicate_status) VALUES ($1, $2, $3::text, $4, $4, 'DRAFT', CURRENT_DATE, CURRENT_DATE + 30, $5, $6, $3::text, 'OK') ON CONFLICT DO NOTHING RETURNING id`
	insertArgs := func(r *iso004Refs) []any {
		return []any{r.str(numberRef), r.id(supplierRef), r.str("currency_b"), total, r.id("company_b"), r.id("actor_b")}
	}
	if withPO {
		insertSQL = `INSERT INTO ap_invoices (number, supplier_id, grn_id, po_id, currency, total, subtotal, status, issued_at, due_at, company_id, created_by, base_currency, duplicate_status) VALUES ($1, $2, $7, $8, $3::text, $4, $4, 'DRAFT', CURRENT_DATE, CURRENT_DATE + 30, $5, $6, $3::text, 'OK') ON CONFLICT DO NOTHING RETURNING id`
		insertArgs = func(r *iso004Refs) []any {
			return []any{r.str(numberRef), r.id(supplierRef), r.str("currency_b"), total, r.id("company_b"), r.id("actor_b"), r.id("grn_b"), r.id("po_b")}
		}
	}
	return iso004Step{
		Name: name, Table: "ap_invoices", Purpose: purpose,
		Lookup:     `SELECT id FROM ap_invoices WHERE number = $1`,
		LookupArgs: func(r *iso004Refs) []any { return []any{r.str(numberRef)} },
		Insert:     insertSQL,
		InsertArgs: insertArgs,
		OwnerByID:  `SELECT 'company:' || i.company_id || CASE WHEN s.company_id = i.company_id THEN '' ELSE '/supplier-mismatch' END FROM ap_invoices i JOIN suppliers s ON s.id = i.supplier_id WHERE i.id = $1`,
		Expect:     func(r *iso004Refs) string { return r.company("company_b") },
	}
}

func iso004InvoiceLineStep(invoiceRef string, withPO bool, qty, unitPrice int64) iso004Step {
	subtotal := qty * unitPrice
	insertSQL := `INSERT INTO ap_invoice_lines (ap_invoice_id, product_id, description, quantity, unit_price, subtotal, total) VALUES ($1, $2, 'ISO-004 certification line', $3, $4, $5, $5) ON CONFLICT DO NOTHING RETURNING id`
	insertArgs := func(r *iso004Refs) []any {
		return []any{r.id(invoiceRef), r.id("product_b"), qty, unitPrice, subtotal}
	}
	if withPO {
		insertSQL = `INSERT INTO ap_invoice_lines (ap_invoice_id, product_id, description, quantity, unit_price, subtotal, total, po_line_id, grn_line_id) VALUES ($1, $2, 'ISO-004 certification line', $3, $4, $5, $5, $6, $7) ON CONFLICT DO NOTHING RETURNING id`
		insertArgs = func(r *iso004Refs) []any {
			return []any{r.id(invoiceRef), r.id("product_b"), qty, unitPrice, subtotal, r.id("po_line_b"), r.id("grn_line_b")}
		}
	}
	return iso004Step{
		Name: invoiceRef + "_line", Table: "ap_invoice_lines", Purpose: fmt.Sprintf("invoice line %d x %d", qty, unitPrice),
		Lookup:     `SELECT id FROM ap_invoice_lines WHERE ap_invoice_id = $1 ORDER BY id LIMIT 1`,
		LookupArgs: func(r *iso004Refs) []any { return []any{r.id(invoiceRef)} },
		Insert:     insertSQL,
		InsertArgs: insertArgs,
		OwnerByID:  `SELECT 'company:' || i.company_id || CASE WHEN p.company_id = i.company_id THEN '' ELSE '/product-mismatch' END FROM ap_invoice_lines l JOIN ap_invoices i ON i.id = l.ap_invoice_id JOIN products p ON p.id = l.product_id WHERE l.id = $1`,
		Expect:     func(r *iso004Refs) string { return r.company("company_b") },
	}
}
