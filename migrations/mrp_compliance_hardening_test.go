package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestMRPComplianceHardeningMigrationProtectsEvidence(t *testing.T) {
	contents, err := os.ReadFile("000118_mrp_compliance_hardening.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(contents))
	for _, fragment := range []string{
		"create table if not exists mrp_record_snapshots",
		"record_hash char(64)",
		"retention_until",
		"unique (company_id, record_type, record_id, record_version)",
		"before update or delete on mrp_record_snapshots",
		"before update or delete on compliance_decisions",
		"before update or delete on audit_events",
		"signer_id bigint",
		"reauthentication_method",
		"approver_roles",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("hardening migration missing %q", fragment)
		}
	}
}

func TestMRPComplianceHardeningRollbackRemovesEvidenceGuards(t *testing.T) {
	contents, err := os.ReadFile("000118_mrp_compliance_hardening.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(contents))
	for _, fragment := range []string{
		"drop trigger if exists trg_prevent_mrp_snapshot_mutation",
		"drop function if exists prevent_mrp_compliance_mutation",
		"drop table if exists mrp_record_snapshots",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("hardening rollback missing %q", fragment)
		}
	}
}
