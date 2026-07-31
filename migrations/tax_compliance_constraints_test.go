package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestTaxComplianceMigrationEnforcesRulesIdentitiesCodesAndMappings(t *testing.T) {
	data, err := os.ReadFile("000048_tax_compliance.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"tax_rule_versions_no_reviewed_overlap",
		"reviewed_at IS NOT NULL",
		"tax_vat_rates",
		"tax_withholding_types",
		"tax_codes",
		"company_tax_identities",
		"npwp TEXT NOT NULL",
		"nitku TEXT NOT NULL",
		"tax_account_mappings",
		"tax_account_mappings_no_overlap",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("tax migration missing %q", want)
		}
	}
}

func TestTaxComplianceMigrationEnforcesImmutableHashesAndLockedPeriods(t *testing.T) {
	data, err := os.ReadFile("000048_tax_compliance.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"source_hash TEXT NOT NULL",
		"tax_documents_immutable",
		"tax_withholding_immutable",
		"tax_ledger_immutable",
		"tax_exports_immutable",
		"guard_locked_tax_period",
		"tax period is locked",
		"UNIQUE(category, source_type, source_id)",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("tax immutability migration missing %q", want)
		}
	}
}

func TestTaxCaptureOutboxIsPostOnlyAndDuplicateSafe(t *testing.T) {
	data, err := os.ReadFile("000049_tax_capture_outbox.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"CREATE TABLE tax_capture_outbox",
		"UNIQUE(source_type,source_id)",
		"AFTER UPDATE OF status",
		"NEW.status IN ('POSTED','PAID')",
		"ON CONFLICT(source_type,source_id) DO NOTHING",
		"AFTER INSERT ON ap_payment_allocations",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("tax outbox migration missing %q", want)
		}
	}
}

func TestTaxCaptureSelectsReviewedEffectiveIdentityCodeAndMapping(t *testing.T) {
	data, err := os.ReadFile("../internal/tax/repository.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"company_tax_identities WHERE company_id=$1 AND effective_from<=",
		"tax_codes tc JOIN tax_rule_versions rv",
		"rv.reviewed_at IS NOT NULL",
		"ORDER BY rv.effective_from DESC,tc.id LIMIT 1",
		"tax_account_mappings WHERE company_id=$1 AND category=$2",
		"effective_to IS NULL OR effective_to >=",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("tax capture selection missing %q", want)
		}
	}
}
