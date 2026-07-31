package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestPayrollMigrationsEnforceReviewedRuleAndPolicyBoundaries(t *testing.T) {
	data, err := os.ReadFile("000047_payroll_review_fixes.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"CREATE EXTENSION IF NOT EXISTS btree_gist",
		"payroll_rule_versions_no_reviewed_overlap",
		"reviewed_at IS NOT NULL",
		"payroll_company_policies_no_overlap",
		"daterange(effective_from",
		"CREATE TABLE payroll_run_events",
		"event_type TEXT NOT NULL CHECK (event_type IN ('REJECTED'))",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("payroll review migration missing %q", want)
		}
	}
}

func TestPayrollMigrationMakesPostedRunsAndLinesImmutable(t *testing.T) {
	data, err := os.ReadFile("000046_payroll_engine.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"payroll_runs_posted_immutable",
		"prevent_posted_payroll_run_mutation",
		"payroll_lines_posted_immutable",
		"prevent_posted_payroll_line_mutation",
		"payroll_one_posted_regular_run",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("payroll engine migration missing %q", want)
		}
	}
}

func TestPayrollDraftSelectionRequiresReviewedRulesAndLatestEffectiveVersion(t *testing.T) {
	data, err := os.ReadFile("../internal/payroll/repository.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"reviewed_at IS NOT NULL",
		"effective_from <= (SELECT pay_date FROM p)",
		"effective_to IS NULL OR effective_to >= (SELECT pay_date FROM p)",
		"ORDER BY effective_from DESC LIMIT 1",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("payroll draft rule selection missing %q", want)
		}
	}
}
