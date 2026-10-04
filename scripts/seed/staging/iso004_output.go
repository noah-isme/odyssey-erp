package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// iso004Var is one exported STAGING_CERT_ISO004_* value. Numeric values are
// emitted as JSON numbers, everything else as strings, matching the style of
// the existing 12 STAGING_CERT_* variables.
type iso004Var struct {
	Name    string
	Value   string
	Numeric bool
}

func iso004Vars(res *iso004Result) []iso004Var {
	r := res.Refs
	num := func(name, ref string) iso004Var {
		return iso004Var{Name: name, Value: strconv.FormatInt(r.ids[ref], 10), Numeric: true}
	}
	return []iso004Var{
		{Name: "STAGING_CERT_ISO004_KEY", Value: r.key},
		num("STAGING_CERT_ISO004_COMPANY_A_ID", "company_a"),
		num("STAGING_CERT_ISO004_COMPANY_B_ID", "company_b"),
		num("STAGING_CERT_ISO004_ADMIN_USER_ID", "admin_a"),
		num("STAGING_CERT_ISO004_ACTOR_B_USER_ID", "actor_b"),
		num("STAGING_CERT_ISO004_FORECAST_SCENARIO_A_ID", "forecast_a"),
		num("STAGING_CERT_ISO004_FORECAST_SCENARIO_B_ID", "forecast_b"),
		num("STAGING_CERT_ISO004_ACCOUNTING_PERIOD_B_ID", "accounting_period_b"),
		num("STAGING_CERT_ISO004_VARIANCE_RULE_B_ID", "variance_rule_b"),
		num("STAGING_CERT_ISO004_VARIANCE_SNAPSHOT_B_ID", "variance_snapshot_b"),
		num("STAGING_CERT_ISO004_AP_SUPPLIER_B_NOPOLICY_ID", "supplier_b_nopolicy"),
		num("STAGING_CERT_ISO004_AP_SUPPLIER_B_POLICY_ID", "supplier_b_policy"),
		num("STAGING_CERT_ISO004_AP_POLICY_ID", "ap_policy_b"),
		num("STAGING_CERT_ISO004_AP_PO_B_ID", "po_b"),
		num("STAGING_CERT_ISO004_AP_GRN_B_ID", "grn_b"),
		num("STAGING_CERT_ISO004_AP_INVOICE_B_MISSING_ID", "ap_invoice_b_missing"),
		num("STAGING_CERT_ISO004_AP_INVOICE_B_EXCEPTION_ID", "ap_invoice_b_exception"),
		num("STAGING_CERT_ISO004_AP_INVOICE_B_MATCHED_ID", "ap_invoice_b_matched"),
		{Name: "STAGING_CERT_ISO004_AP_INVOICE_B_CREATED_BY", Value: strconv.FormatInt(res.APCreatedBy, 10), Numeric: true},
		num("STAGING_CERT_ISO004_DOCUMENT_VERSION_A_ID", "document_version_a"),
		num("STAGING_CERT_ISO004_OCR_JOB_FORGED_ID", "ocr_job_forged"),
		num("STAGING_CERT_ISO004_PAYSLIP_B_ID", "payslip_b"),
		{Name: "STAGING_CERT_ISO004_PAYSLIP_B_EMAIL", Value: r.str("payslip_email")},
		{Name: "STAGING_CERT_ISO004_PAYSLIP_B_CREATED_AT", Value: res.PayslipAt},
	}
}

func iso004JSON(vars []iso004Var) (string, error) {
	var b strings.Builder
	b.WriteString("{\n")
	for i, v := range vars {
		key, err := json.Marshal(v.Name)
		if err != nil {
			return "", fmt.Errorf("encode %s: %w", v.Name, err)
		}
		var val []byte
		if v.Numeric {
			val = []byte(v.Value)
		} else if val, err = json.Marshal(v.Value); err != nil {
			return "", fmt.Errorf("encode %s: %w", v.Name, err)
		}
		sep := ","
		if i == len(vars)-1 {
			sep = ""
		}
		fmt.Fprintf(&b, "  %s: %s%s\n", key, val, sep)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// printISO004Plan prints the dry-run plan: the guard result, resolved
// prerequisites and every insert the real run would perform.
func printISO004Plan(w io.Writer, res *iso004Result) {
	fmt.Fprintln(w, "================================================================================")
	fmt.Fprintln(w, "          ISO-004 FIXTURE EXTENSION - DRY RUN (no rows written)                ")
	fmt.Fprintln(w, "================================================================================")
	fmt.Fprintf(w, "Target: database %q on host %q (guard passed)\n", res.Target.Confirm.Database, res.Target.Host)
	fmt.Fprintf(w, "Run key: %s\n", res.Refs.key)
	fmt.Fprintln(w, "Resolved prerequisites (SELECT only):")
	for _, name := range []string{"company_a", "company_b", "branch_a", "branch_b", "admin_a", "unit", "classification_a", "tax_rule", "bpjs_rule"} {
		fmt.Fprintf(w, "  %-18s = %d\n", name, res.Refs.ids[name])
	}
	fmt.Fprintf(w, "  %-18s = %s\n", "currency_b", res.Refs.str("currency_b"))
	fmt.Fprintf(w, "  %-18s = %s\n", "matched invoice no", res.Refs.str("ap_invoice_b_matched.number"))
	iso004PrintWarnings(w, res)
	fmt.Fprintln(w, "Planned fixture rows:")
	for i, s := range res.Steps {
		fmt.Fprintf(w, "\n%2d. %s [%s] %s\n", i+1, s.Step.Name, s.Step.Table, s.Step.Purpose)
		if s.ID != 0 {
			fmt.Fprintf(w, "    status: %s (id=%d, ownership %s verified)\n", s.Status, s.ID, s.Step.Expect(res.Refs))
			continue
		}
		fmt.Fprintf(w, "    status: %s\n", s.Status)
		fmt.Fprintf(w, "    sql:    %s\n", s.Step.Insert)
		fmt.Fprintf(w, "    args:   %s\n", iso004FormatArgs(s.Step.InsertArgs(res.Refs)))
		fmt.Fprintf(w, "    owner:  must equal %s\n", s.Step.Expect(res.Refs))
	}
}

func iso004FormatArgs(args []any) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = fmt.Sprintf("$%d=%v", i+1, a)
	}
	return strings.Join(parts, " ")
}

func iso004PrintWarnings(w io.Writer, res *iso004Result) {
	for _, warn := range res.Warnings {
		fmt.Fprintf(w, "WARNING: %s\n", warn)
	}
}

// printISO004Summary prints the result in the same styles as printSummary:
// JSON (stdout only, so it can be redirected to fixtures.json), the text
// table, shell exports and gh commands. In JSON mode the verification SQL and
// warnings go to stderr.
func printISO004Summary(stdout, stderr io.Writer, res *iso004Result, applied, export, jsonOut bool) error {
	vars := iso004Vars(res)
	if jsonOut {
		out, err := iso004JSON(vars)
		if err != nil {
			return err
		}
		fmt.Fprint(stdout, out)
		iso004PrintWarnings(stderr, res)
		fmt.Fprintln(stderr, "-- ISO-004 fixture ownership verification (read-only):")
		fmt.Fprintln(stderr, res.VerifySQL)
		return nil
	}

	fmt.Fprintln(stdout, "================================================================================")
	fmt.Fprintln(stdout, "          ODYSSEY ISO-004 WORKER INJECTION FIXTURES (--iso004)                  ")
	fmt.Fprintln(stdout, "================================================================================")
	for i, v := range vars {
		fmt.Fprintf(stdout, "%2d. %-46s = %s\n", i+1, v.Name, v.Value)
	}
	fmt.Fprintln(stdout, "--------------------------------------------------------------------------------")
	created, reused := 0, 0
	for _, s := range res.Steps {
		if s.Status == "created" {
			created++
		} else {
			reused++
		}
	}
	fmt.Fprintf(stdout, "Fixture rows: %d created, %d reused (ownership verified for all %d)\n", created, reused, len(res.Verification))
	iso004PrintWarnings(stdout, res)
	fmt.Fprintln(stdout, "\n-- Ownership verification SQL (read-only; every row must show ok = t):")
	fmt.Fprintln(stdout, res.VerifySQL)
	fmt.Fprintln(stdout, "================================================================================")

	if export {
		fmt.Fprintln(stdout, "\n# Shell export commands:")
		for _, v := range vars {
			fmt.Fprintf(stdout, "export %s=%q\n", v.Name, v.Value)
		}
	}
	if !applied {
		fmt.Fprintf(stdout, "\nTo apply these %d ISO-004 fixture variables to GitHub staging environment:\n", len(vars))
		for _, v := range vars {
			fmt.Fprintf(stdout, "gh variable set %s --body %q --env staging\n", v.Name, v.Value)
		}
		fmt.Fprintln(stdout, "\nOr run with flag: go run ./scripts/seed/staging --iso004 ... --apply-gh")
	}
	return nil
}

func applyISO004ToGitHub(stdout io.Writer, res *iso004Result) error {
	vars := iso004Vars(res)
	fmt.Fprintf(stdout, "\n→ Applying %d ISO-004 variables to GitHub environment 'staging'...\n", len(vars))
	for _, v := range vars {
		cmd := exec.Command("gh", "variable", "set", v.Name, "--body", v.Value, "--env", "staging")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("gh variable set %s: %w, output: %s", v.Name, err, string(output))
		}
		fmt.Fprintf(stdout, "  ✓ Set %s = %s\n", v.Name, v.Value)
	}
	return nil
}
