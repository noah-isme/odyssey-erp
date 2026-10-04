package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Fixtures are the IDs exported by `go run ./scripts/seed/staging --iso004`
// (scripts/seed/staging/iso004_output.go). The JSON keys are the exported
// variable names.
type Fixtures struct {
	Key                 string `json:"STAGING_CERT_ISO004_KEY"`
	CompanyA            int64  `json:"STAGING_CERT_ISO004_COMPANY_A_ID"`
	CompanyB            int64  `json:"STAGING_CERT_ISO004_COMPANY_B_ID"`
	AdminUserA          int64  `json:"STAGING_CERT_ISO004_ADMIN_USER_ID"`
	ActorUserB          int64  `json:"STAGING_CERT_ISO004_ACTOR_B_USER_ID"`
	ForecastScenarioA   int64  `json:"STAGING_CERT_ISO004_FORECAST_SCENARIO_A_ID"`
	ForecastScenarioB   int64  `json:"STAGING_CERT_ISO004_FORECAST_SCENARIO_B_ID"`
	AccountingPeriodB   int64  `json:"STAGING_CERT_ISO004_ACCOUNTING_PERIOD_B_ID"`
	VarianceRuleB       int64  `json:"STAGING_CERT_ISO004_VARIANCE_RULE_B_ID"`
	VarianceSnapshotB   int64  `json:"STAGING_CERT_ISO004_VARIANCE_SNAPSHOT_B_ID"`
	SupplierBNoPolicy   int64  `json:"STAGING_CERT_ISO004_AP_SUPPLIER_B_NOPOLICY_ID"`
	SupplierBPolicy     int64  `json:"STAGING_CERT_ISO004_AP_SUPPLIER_B_POLICY_ID"`
	APPolicy            int64  `json:"STAGING_CERT_ISO004_AP_POLICY_ID"`
	POB                 int64  `json:"STAGING_CERT_ISO004_AP_PO_B_ID"`
	GRNB                int64  `json:"STAGING_CERT_ISO004_AP_GRN_B_ID"`
	APInvoiceBMissing   int64  `json:"STAGING_CERT_ISO004_AP_INVOICE_B_MISSING_ID"`
	APInvoiceBException int64  `json:"STAGING_CERT_ISO004_AP_INVOICE_B_EXCEPTION_ID"`
	APInvoiceBMatched   int64  `json:"STAGING_CERT_ISO004_AP_INVOICE_B_MATCHED_ID"`
	APInvoiceBCreatedBy int64  `json:"STAGING_CERT_ISO004_AP_INVOICE_B_CREATED_BY"`
	DocumentVersionA    int64  `json:"STAGING_CERT_ISO004_DOCUMENT_VERSION_A_ID"`
	OCRJobForged        int64  `json:"STAGING_CERT_ISO004_OCR_JOB_FORGED_ID"`
	PayslipB            int64  `json:"STAGING_CERT_ISO004_PAYSLIP_B_ID"`
	PayslipBEmail       string `json:"STAGING_CERT_ISO004_PAYSLIP_B_EMAIL"`
	PayslipBCreatedAt   string `json:"STAGING_CERT_ISO004_PAYSLIP_B_CREATED_AT"`
}

// fixtureField binds one exported variable to its struct field.
type fixtureField struct {
	Name    string
	Numeric bool
	// Fallbacks are older STAGING_CERT_* names consulted after the ISO-004
	// variable itself (company IDs are also exported by the base seeder).
	Fallbacks []string
	int       func(*Fixtures) *int64
	str       func(*Fixtures) *string
}

func fixtureFields() []fixtureField {
	n := func(name string, f func(*Fixtures) *int64, fallbacks ...string) fixtureField {
		return fixtureField{Name: name, Numeric: true, int: f, Fallbacks: fallbacks}
	}
	s := func(name string, f func(*Fixtures) *string) fixtureField {
		return fixtureField{Name: name, str: f}
	}
	return []fixtureField{
		s("STAGING_CERT_ISO004_KEY", func(x *Fixtures) *string { return &x.Key }),
		n("STAGING_CERT_ISO004_COMPANY_A_ID", func(x *Fixtures) *int64 { return &x.CompanyA }, "STAGING_CERT_COMPANY_ID"),
		n("STAGING_CERT_ISO004_COMPANY_B_ID", func(x *Fixtures) *int64 { return &x.CompanyB }, "STAGING_CERT_OTHER_COMPANY_ID"),
		n("STAGING_CERT_ISO004_ADMIN_USER_ID", func(x *Fixtures) *int64 { return &x.AdminUserA }),
		n("STAGING_CERT_ISO004_ACTOR_B_USER_ID", func(x *Fixtures) *int64 { return &x.ActorUserB }),
		n("STAGING_CERT_ISO004_FORECAST_SCENARIO_A_ID", func(x *Fixtures) *int64 { return &x.ForecastScenarioA }),
		n("STAGING_CERT_ISO004_FORECAST_SCENARIO_B_ID", func(x *Fixtures) *int64 { return &x.ForecastScenarioB }),
		n("STAGING_CERT_ISO004_ACCOUNTING_PERIOD_B_ID", func(x *Fixtures) *int64 { return &x.AccountingPeriodB }),
		n("STAGING_CERT_ISO004_VARIANCE_RULE_B_ID", func(x *Fixtures) *int64 { return &x.VarianceRuleB }),
		n("STAGING_CERT_ISO004_VARIANCE_SNAPSHOT_B_ID", func(x *Fixtures) *int64 { return &x.VarianceSnapshotB }),
		n("STAGING_CERT_ISO004_AP_SUPPLIER_B_NOPOLICY_ID", func(x *Fixtures) *int64 { return &x.SupplierBNoPolicy }),
		n("STAGING_CERT_ISO004_AP_SUPPLIER_B_POLICY_ID", func(x *Fixtures) *int64 { return &x.SupplierBPolicy }),
		n("STAGING_CERT_ISO004_AP_POLICY_ID", func(x *Fixtures) *int64 { return &x.APPolicy }),
		n("STAGING_CERT_ISO004_AP_PO_B_ID", func(x *Fixtures) *int64 { return &x.POB }),
		n("STAGING_CERT_ISO004_AP_GRN_B_ID", func(x *Fixtures) *int64 { return &x.GRNB }),
		n("STAGING_CERT_ISO004_AP_INVOICE_B_MISSING_ID", func(x *Fixtures) *int64 { return &x.APInvoiceBMissing }),
		n("STAGING_CERT_ISO004_AP_INVOICE_B_EXCEPTION_ID", func(x *Fixtures) *int64 { return &x.APInvoiceBException }),
		n("STAGING_CERT_ISO004_AP_INVOICE_B_MATCHED_ID", func(x *Fixtures) *int64 { return &x.APInvoiceBMatched }),
		n("STAGING_CERT_ISO004_AP_INVOICE_B_CREATED_BY", func(x *Fixtures) *int64 { return &x.APInvoiceBCreatedBy }),
		n("STAGING_CERT_ISO004_DOCUMENT_VERSION_A_ID", func(x *Fixtures) *int64 { return &x.DocumentVersionA }),
		n("STAGING_CERT_ISO004_OCR_JOB_FORGED_ID", func(x *Fixtures) *int64 { return &x.OCRJobForged }),
		n("STAGING_CERT_ISO004_PAYSLIP_B_ID", func(x *Fixtures) *int64 { return &x.PayslipB }),
		s("STAGING_CERT_ISO004_PAYSLIP_B_EMAIL", func(x *Fixtures) *string { return &x.PayslipBEmail }),
		s("STAGING_CERT_ISO004_PAYSLIP_B_CREATED_AT", func(x *Fixtures) *string { return &x.PayslipBCreatedAt }),
	}
}

// FixtureSource records where each value came from ("file" or "env:<NAME>").
type FixtureSource map[string]string

// loadFixtures reads the fixtures file (when path is non-empty) and fills
// every key the file does not define from the environment. The file wins
// over the environment. Unknown keys in the file are rejected so a typo is
// never silently replaced by an environment value. Missing keys are not an
// error here; validateFixtures reports them.
func loadFixtures(path string, getenv func(string) string) (*Fixtures, FixtureSource, error) {
	fx := &Fixtures{}
	src := FixtureSource{}
	fields := fixtureFields()
	known := map[string]fixtureField{}
	for _, f := range fields {
		known[f.Name] = f
	}

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("read fixtures file: %w", err)
		}
		raw := map[string]json.RawMessage{}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&raw); err != nil {
			return nil, nil, fmt.Errorf("parse fixtures file %s: %w", path, err)
		}
		var unknown []string
		for name, val := range raw {
			f, ok := known[name]
			if !ok {
				unknown = append(unknown, name)
				continue
			}
			if err := setFixture(fx, f, val); err != nil {
				return nil, nil, fmt.Errorf("fixtures file %s: %w", path, err)
			}
			src[name] = "file"
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			return nil, nil, fmt.Errorf("fixtures file %s: unknown keys %s", path, strings.Join(unknown, ", "))
		}
	}

	for _, f := range fields {
		if _, ok := src[f.Name]; ok {
			continue
		}
		for _, envName := range append([]string{f.Name}, f.Fallbacks...) {
			v := strings.TrimSpace(getenv(envName))
			if v == "" {
				continue
			}
			if err := setFixtureString(fx, f, v); err != nil {
				return nil, nil, fmt.Errorf("environment %s: %w", envName, err)
			}
			src[f.Name] = "env:" + envName
			break
		}
	}
	return fx, src, nil
}

func setFixture(fx *Fixtures, f fixtureField, val json.RawMessage) error {
	var v any
	dec := json.NewDecoder(bytes.NewReader(val))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return fmt.Errorf("%s: %w", f.Name, err)
	}
	switch t := v.(type) {
	case json.Number:
		return setFixtureString(fx, f, t.String())
	case string:
		return setFixtureString(fx, f, t)
	default:
		return fmt.Errorf("%s: expected a number or string, got %s", f.Name, string(val))
	}
}

func setFixtureString(fx *Fixtures, f fixtureField, v string) error {
	if f.Numeric {
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return fmt.Errorf("%s: %q is not an integer ID: %w", f.Name, v, err)
		}
		*f.int(fx) = n
		return nil
	}
	*f.str(fx) = v
	return nil
}

// validateFixtures performs the static checks that need neither Redis nor
// Postgres. runID must equal the seed key (the operator passes the run ID as
// --iso004-key), so fixtures and evidence share one key.
func validateFixtures(fx *Fixtures, runID string) []string {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	if fx == nil {
		return []string{"no fixtures loaded"}
	}
	for _, f := range fixtureFields() {
		if f.Numeric {
			if v := *f.int(fx); v <= 0 {
				add("%s is missing or not a positive ID", f.Name)
			}
		} else if strings.TrimSpace(*f.str(fx)) == "" {
			add("%s is missing", f.Name)
		}
	}
	if fx.Key != "" && fx.Key != runID {
		add("STAGING_CERT_ISO004_KEY %q does not equal --run-id %q; seed fixtures with --iso004-key <run-id>", fx.Key, runID)
	}
	if fx.CompanyA > 0 && fx.CompanyA == fx.CompanyB {
		add("company A and company B are the same ID %d", fx.CompanyA)
	}
	if fx.ForecastScenarioA > 0 && fx.ForecastScenarioA == fx.ForecastScenarioB {
		add("forecast scenarios A and B are the same ID %d", fx.ForecastScenarioA)
	}
	if fx.SupplierBNoPolicy > 0 && fx.SupplierBNoPolicy == fx.SupplierBPolicy {
		add("AP suppliers (no-policy and policy) are the same ID %d", fx.SupplierBNoPolicy)
	}
	inv := []int64{fx.APInvoiceBMissing, fx.APInvoiceBException, fx.APInvoiceBMatched}
	if inv[0] > 0 && inv[1] > 0 && inv[2] > 0 && (inv[0] == inv[1] || inv[0] == inv[2] || inv[1] == inv[2]) {
		add("the three AP invoice fixtures must be distinct, got %v", inv)
	}
	if fx.AdminUserA > 0 && fx.AdminUserA == fx.APInvoiceBCreatedBy {
		add("ADMIN_USER_ID equals AP_INVOICE_B_CREATED_BY (%d); the forged-actor scenario needs two different users", fx.AdminUserA)
	}
	if fx.ActorUserB > 0 && fx.APInvoiceBCreatedBy > 0 && fx.ActorUserB != fx.APInvoiceBCreatedBy {
		add("AP_INVOICE_B_CREATED_BY %d does not equal ACTOR_B_USER_ID %d", fx.APInvoiceBCreatedBy, fx.ActorUserB)
	}
	if fx.Key != "" && fx.PayslipBEmail != "" {
		if want := "iso004-payslip-" + fx.Key + "@staging.invalid"; fx.PayslipBEmail != want {
			add("STAGING_CERT_ISO004_PAYSLIP_B_EMAIL %q must be %q", fx.PayslipBEmail, want)
		}
	}
	if fx.PayslipBCreatedAt != "" {
		if _, err := time.Parse(time.RFC3339Nano, fx.PayslipBCreatedAt); err != nil {
			add("STAGING_CERT_ISO004_PAYSLIP_B_CREATED_AT %q is not RFC3339: %v", fx.PayslipBCreatedAt, err)
		}
	}
	return problems
}

// errFixtures wraps static fixture validation failures.
var errFixtures = errors.New("fixture validation failed")

// fixtureCheck is one SELECT-only ownership probe. The query returns a single
// text value that must equal Expect.
type fixtureCheck struct {
	Name   string
	SQL    string
	Args   []any
	Expect string
}

func company(id int64) string { return "company:" + strconv.FormatInt(id, 10) }

// fixtureChecks returns the ownership/state probes for the fixtures. Every
// statement is a parameterized SELECT; preflight_test.go asserts that.
func fixtureChecks(fx *Fixtures) []fixtureCheck {
	a, b := company(fx.CompanyA), company(fx.CompanyB)
	invoice := func(name string, id, supplier int64) fixtureCheck {
		return fixtureCheck{
			Name: name,
			SQL: `SELECT 'company:' || COALESCE(i.company_id::text, 'NULL') || '/supplier:' || i.supplier_id || '/supplier-company:' || s.company_id || '/created_by:' || COALESCE(i.created_by::text, 'NULL') || '/status:' || i.status
FROM ap_invoices i JOIN suppliers s ON s.id = i.supplier_id WHERE i.id = $1`,
			Args:   []any{id},
			Expect: fmt.Sprintf("%s/supplier:%d/supplier-%s/created_by:%d/status:DRAFT", b, supplier, b, fx.APInvoiceBCreatedBy),
		}
	}
	return []fixtureCheck{
		{Name: "company_a", SQL: `SELECT 'company:' || id FROM companies WHERE id = $1`, Args: []any{fx.CompanyA}, Expect: a},
		{Name: "company_b", SQL: `SELECT 'company:' || id FROM companies WHERE id = $1`, Args: []any{fx.CompanyB}, Expect: b},
		{Name: "admin_user_a", SQL: `SELECT 'user:' || id FROM users WHERE id = $1`, Args: []any{fx.AdminUserA}, Expect: fmt.Sprintf("user:%d", fx.AdminUserA)},
		{Name: "actor_user_b", SQL: `SELECT 'user:' || id FROM users WHERE id = $1`, Args: []any{fx.ActorUserB}, Expect: fmt.Sprintf("user:%d", fx.ActorUserB)},
		{Name: "forecast_scenario_a", SQL: `SELECT 'company:' || company_id FROM forecast_scenarios WHERE id = $1`, Args: []any{fx.ForecastScenarioA}, Expect: a},
		{Name: "forecast_scenario_b", SQL: `SELECT 'company:' || company_id FROM forecast_scenarios WHERE id = $1`, Args: []any{fx.ForecastScenarioB}, Expect: b},
		{Name: "accounting_period_b", SQL: `SELECT 'company:' || company_id FROM accounting_periods WHERE id = $1`, Args: []any{fx.AccountingPeriodB}, Expect: b},
		{Name: "variance_rule_b", SQL: `SELECT 'company:' || company_id FROM variance_rules WHERE id = $1`, Args: []any{fx.VarianceRuleB}, Expect: b},
		{
			Name:   "variance_snapshot_b",
			SQL:    `SELECT 'company:' || vr.company_id || '/rule:' || vs.rule_id FROM variance_snapshots vs JOIN variance_rules vr ON vr.id = vs.rule_id WHERE vs.id = $1`,
			Args:   []any{fx.VarianceSnapshotB},
			Expect: fmt.Sprintf("%s/rule:%d", b, fx.VarianceRuleB),
		},
		{Name: "ap_supplier_b_nopolicy", SQL: `SELECT 'company:' || company_id FROM suppliers WHERE id = $1`, Args: []any{fx.SupplierBNoPolicy}, Expect: b},
		{Name: "ap_supplier_b_policy", SQL: `SELECT 'company:' || company_id FROM suppliers WHERE id = $1`, Args: []any{fx.SupplierBPolicy}, Expect: b},
		{
			Name:   "ap_policy",
			SQL:    `SELECT CASE WHEN p.company_id IS NULL THEN 'company:' || s.company_id ELSE 'policy-company:' || p.company_id END || '/supplier:' || p.supplier_id FROM ap_matching_policies p JOIN suppliers s ON s.id = p.supplier_id WHERE p.id = $1`,
			Args:   []any{fx.APPolicy},
			Expect: fmt.Sprintf("%s/supplier:%d", b, fx.SupplierBPolicy),
		},
		{
			Name:   "ap_po_b",
			SQL:    `SELECT 'company:' || po.company_id || '/supplier:' || po.supplier_id FROM pos po WHERE po.id = $1`,
			Args:   []any{fx.POB},
			Expect: fmt.Sprintf("%s/supplier:%d", b, fx.SupplierBPolicy),
		},
		{
			Name:   "ap_grn_b",
			SQL:    `SELECT 'company:' || g.company_id || '/po:' || g.po_id FROM grns g WHERE g.id = $1`,
			Args:   []any{fx.GRNB},
			Expect: fmt.Sprintf("%s/po:%d", b, fx.POB),
		},
		invoice("ap_invoice_b_missing", fx.APInvoiceBMissing, fx.SupplierBNoPolicy),
		invoice("ap_invoice_b_exception", fx.APInvoiceBException, fx.SupplierBPolicy),
		invoice("ap_invoice_b_matched", fx.APInvoiceBMatched, fx.SupplierBPolicy),
		{
			Name:   "document_version_a",
			SQL:    `SELECT 'company:' || v.company_id || '/document-company:' || d.company_id FROM document_versions v JOIN documents d ON d.id = v.document_id WHERE v.id = $1`,
			Args:   []any{fx.DocumentVersionA},
			Expect: fmt.Sprintf("%s/document-%s", a, a),
		},
		{
			Name:   "ocr_job_forged",
			SQL:    `SELECT 'company:' || j.company_id || '/version:' || j.document_version_id || '/version-company:' || v.company_id || '/status:' || j.status FROM doc_ocr_jobs j JOIN document_versions v ON v.id = j.document_version_id WHERE j.id = $1`,
			Args:   []any{fx.OCRJobForged},
			Expect: fmt.Sprintf("%s/version:%d/version-%s/status:PENDING", b, fx.DocumentVersionA, a),
		},
		{
			Name: "payslip_b",
			SQL: `SELECT 'company:' || r.company_id || '/employee-company:' || e.company_id || '/email:' || e.email
FROM payroll_payslips ps JOIN payroll_run_lines l ON l.id = ps.run_line_id JOIN payroll_runs r ON r.id = l.run_id JOIN hr_employees e ON e.id = l.employee_id WHERE ps.id = $1`,
			Args:   []any{fx.PayslipB},
			Expect: fmt.Sprintf("%s/employee-%s/email:%s", b, b, fx.PayslipBEmail),
		},
	}
}
