package main

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// iso004SourceFiles returns the non-test files that implement --iso004.
func iso004SourceFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("iso004*.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range files {
		if !strings.HasSuffix(f, "_test.go") {
			out = append(out, f)
		}
	}
	if len(out) < 4 {
		t.Fatalf("expected the iso004 source files, found %v", out)
	}
	return out
}

func iso004StringLiterals(t *testing.T, file string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	var lits []string
	ast.Inspect(f, func(n ast.Node) bool {
		if bl, ok := n.(*ast.BasicLit); ok && bl.Kind == token.STRING {
			v, err := strconv.Unquote(bl.Value)
			if err != nil {
				t.Fatalf("unquote %s: %v", bl.Value, err)
			}
			lits = append(lits, v)
		}
		return true
	})
	return lits
}

// TestISO004SQLHasNoUpdate is the grep check required by the plan: no
// string literal in the extension may contain UPDATE (which also covers
// "DO UPDATE" and "FOR UPDATE") or any other row-modifying/DDL keyword.
func TestISO004SQLHasNoUpdate(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)\b(update|delete|truncate|alter|drop|merge|grant)\b`)
	for _, file := range iso004SourceFiles(t) {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(bytes.ToUpper(raw), []byte("DO UPDATE")) {
			t.Errorf("%s contains DO UPDATE", file)
		}
		for _, lit := range iso004StringLiterals(t, file) {
			if m := forbidden.FindString(lit); m != "" {
				t.Errorf("%s: string literal contains %q: %s", file, m, lit)
			}
		}
	}
}

// TestISO004InsertsAreDoNothing checks every INSERT literal uses
// ON CONFLICT DO NOTHING.
func TestISO004InsertsAreDoNothing(t *testing.T) {
	insert := regexp.MustCompile(`(?i)\binsert\s+into\b`)
	found := 0
	for _, file := range iso004SourceFiles(t) {
		for _, lit := range iso004StringLiterals(t, file) {
			if !insert.MatchString(lit) {
				continue
			}
			found++
			if !strings.Contains(lit, "ON CONFLICT DO NOTHING RETURNING id") {
				t.Errorf("%s: INSERT without ON CONFLICT DO NOTHING RETURNING id: %s", file, lit)
			}
		}
	}
	if found == 0 {
		t.Fatal("no INSERT statements found; the scan is not looking at the right files")
	}
}

func TestISO004NeverCallsBaseSeeder(t *testing.T) {
	for _, file := range iso004SourceFiles(t) {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte("seedStagingFixtures")) {
			t.Errorf("%s references seedStagingFixtures", file)
		}
	}
	// runISO004Main lives in main.go; make sure its body does not call the
	// base seeder either.
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "runISO004Main" {
			continue
		}
		seen = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == "seedStagingFixtures" {
				t.Errorf("runISO004Main calls seedStagingFixtures")
			}
			return true
		})
	}
	if !seen {
		t.Fatal("runISO004Main not found in main.go")
	}
}

func TestISO004StepsShape(t *testing.T) {
	names := map[string]bool{}
	selectRe := regexp.MustCompile(`^\s*SELECT\b`)
	paramRe := regexp.MustCompile(`\$[0-9]+`)
	for _, s := range iso004Steps() {
		if names[s.Name] {
			t.Errorf("duplicate step name %s", s.Name)
		}
		names[s.Name] = true
		if !selectRe.MatchString(s.Lookup) {
			t.Errorf("%s: Lookup is not a SELECT", s.Name)
		}
		if !selectRe.MatchString(s.OwnerByID) {
			t.Errorf("%s: OwnerByID is not a SELECT", s.Name)
		}
		for _, p := range paramRe.FindAllString(s.OwnerByID, -1) {
			if p != "$1" {
				t.Errorf("%s: OwnerByID uses %s; only $1 is substituted in the printed verification SQL", s.Name, p)
			}
		}
		if s.Expect == nil || s.LookupArgs == nil || s.InsertArgs == nil {
			t.Errorf("%s: missing function fields", s.Name)
		}
	}
	for _, required := range []string{"forecast_a", "forecast_b", "accounting_period_b", "variance_rule_b", "variance_snapshot_b", "ap_policy_b", "po_b", "grn_b", "ap_invoice_b_missing", "ap_invoice_b_exception", "ap_invoice_b_matched", "ocr_job_forged", "payslip_b"} {
		if !names[required] {
			t.Errorf("missing fixture step %s", required)
		}
	}
}

func iso004FakeRefs() *iso004Refs {
	r := newISO004Refs("18123456789")
	for name, id := range map[string]int64{"company_a": 1, "company_b": 2, "branch_a": 11, "branch_b": 12, "admin_a": 21, "unit": 31, "classification_a": 41, "tax_rule": 51, "bpjs_rule": 52} {
		r.ids[name] = id
	}
	r.strs["currency_b"] = "IDR"
	r.strs["payslip_email"] = "iso004-payslip-18123456789@staging.invalid"
	r.strs["ap_invoice_b_missing.number"] = r.code("ISO004-AP-B-MISSING")
	r.strs["ap_invoice_b_exception.number"] = r.code("ISO004-AP-B-EXCEPTION")
	r.strs["ap_invoice_b_matched.number"] = r.code("ISO004-AP-B-MATCHED")
	return r
}

// TestISO004DryRunPlanPrintsInserts renders the dry-run plan for a database
// where nothing exists yet and checks every planned insert is printed with
// its SQL, arguments (placeholders for rows created earlier in the plan) and
// expected owner.
func TestISO004DryRunPlanPrintsInserts(t *testing.T) {
	res := &iso004Result{
		Target:   iso004Target{Host: "staging-db.internal", Confirm: iso004Confirmation{Database: "odyssey_staging", Host: "staging-db.internal"}},
		DryRun:   true,
		Refs:     iso004FakeRefs(),
		Warnings: []string{"not satisfied: example advisory"},
	}
	for _, s := range iso004Steps() {
		status := "would-insert"
		if iso004HasPlaceholder(s.LookupArgs(res.Refs)) {
			status = "would-insert (parent pending)"
		}
		res.Steps = append(res.Steps, iso004StepResult{Step: s, Status: status})
	}
	var buf bytes.Buffer
	printISO004Plan(&buf, res)
	out := buf.String()

	for _, s := range iso004Steps() {
		if !strings.Contains(out, s.Name+" ["+s.Table+"]") {
			t.Errorf("plan does not list step %s", s.Name)
		}
		if !strings.Contains(out, s.Insert) {
			t.Errorf("plan does not print insert SQL for %s", s.Name)
		}
	}
	for _, want := range []string{
		"DRY RUN (no rows written)",
		"WARNING: not satisfied: example advisory",
		"$1=<period_b.id>",
		"ISO004-AP-B-MATCHED-18123456789",
		"owner:  must equal company:2",
		"owner:  must equal company:2/version-company:1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan missing %q", want)
		}
	}
}

func TestISO004JSONOutput(t *testing.T) {
	r := iso004FakeRefs()
	for i, s := range iso004Steps() {
		r.ids[s.Name] = int64(100 + i)
	}
	res := &iso004Result{Refs: r, PayslipAt: "2026-10-04T04:41:31.730731Z", APCreatedBy: 7}
	out, err := iso004JSON(iso004Vars(res))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	for k := range decoded {
		if !strings.HasPrefix(k, "STAGING_CERT_ISO004_") {
			t.Errorf("unexpected key %s", k)
		}
	}
	for _, k := range []string{
		"STAGING_CERT_ISO004_ADMIN_USER_ID", "STAGING_CERT_ISO004_AP_INVOICE_B_CREATED_BY",
		"STAGING_CERT_ISO004_AP_INVOICE_B_MISSING_ID", "STAGING_CERT_ISO004_AP_INVOICE_B_EXCEPTION_ID", "STAGING_CERT_ISO004_AP_INVOICE_B_MATCHED_ID",
		"STAGING_CERT_ISO004_PAYSLIP_B_ID", "STAGING_CERT_ISO004_PAYSLIP_B_EMAIL", "STAGING_CERT_ISO004_PAYSLIP_B_CREATED_AT",
		"STAGING_CERT_ISO004_OCR_JOB_FORGED_ID", "STAGING_CERT_ISO004_VARIANCE_SNAPSHOT_B_ID",
		"STAGING_CERT_ISO004_FORECAST_SCENARIO_A_ID", "STAGING_CERT_ISO004_FORECAST_SCENARIO_B_ID",
	} {
		if _, ok := decoded[k]; !ok {
			t.Errorf("missing %s", k)
		}
	}
	if decoded["STAGING_CERT_ISO004_AP_INVOICE_B_CREATED_BY"] != float64(7) {
		t.Errorf("created_by = %v", decoded["STAGING_CERT_ISO004_AP_INVOICE_B_CREATED_BY"])
	}
	if decoded["STAGING_CERT_ISO004_PAYSLIP_B_EMAIL"] != "iso004-payslip-18123456789@staging.invalid" {
		t.Errorf("email = %v", decoded["STAGING_CERT_ISO004_PAYSLIP_B_EMAIL"])
	}
}

// TestBaseSummaryStillHasTwelveVariables guards the existing JSON contract.
func TestBaseSummaryStillHasTwelveVariables(t *testing.T) {
	old := os.Stdout
	rd, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	printSummary(&Fixtures{CompanyID: 1, BranchID: 2, OtherCompanyID: 3, OtherBranchID: 4, CustomerID: 5, SupplierID: 6, ProductID: 7, WarehouseID: 8, GRNID: 9, DocumentCategoryID: 10, DocumentClassID: 11, Amount: "100000"}, false, false, true)
	_ = w.Close()
	os.Stdout = old
	raw, err := io.ReadAll(rd)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	want := []string{"STAGING_CERT_COMPANY_ID", "STAGING_CERT_BRANCH_ID", "STAGING_CERT_OTHER_COMPANY_ID", "STAGING_CERT_OTHER_BRANCH_ID", "STAGING_CERT_CUSTOMER_ID", "STAGING_CERT_SUPPLIER_ID", "STAGING_CERT_PRODUCT_ID", "STAGING_CERT_WAREHOUSE_ID", "STAGING_CERT_GRN_ID", "STAGING_CERT_DOCUMENT_CATEGORY_ID", "STAGING_CERT_DOCUMENT_CLASSIFICATION_ID", "STAGING_CERT_AMOUNT"}
	if len(decoded) != len(want) {
		t.Fatalf("base summary has %d keys, want %d", len(decoded), len(want))
	}
	for _, k := range want {
		if _, ok := decoded[k]; !ok {
			t.Errorf("base summary missing %s", k)
		}
	}
}

func TestISO004VerificationSQLInterpolatesOnlyIDs(t *testing.T) {
	r := iso004FakeRefs()
	res := &iso004Result{Refs: r}
	for i, s := range iso004Steps() {
		r.ids[s.Name] = int64(500 + i)
		res.Steps = append(res.Steps, iso004StepResult{Step: s, Status: "created", ID: int64(500 + i)})
	}
	sql := iso004VerificationSQL(res)
	if strings.Contains(sql, "$1") {
		t.Fatal("verification SQL still contains $1")
	}
	if !strings.HasPrefix(sql, "SELECT fixture") || !strings.Contains(sql, "owner = expected AS ok") {
		t.Fatalf("unexpected verification SQL shape:\n%s", sql)
	}
	if got := strings.Count(sql, "UNION ALL"); got != len(res.Steps)-1 {
		t.Fatalf("UNION ALL count = %d, want %d", got, len(res.Steps)-1)
	}
}
