package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestCRMConstraintsIndexesAndCompanyOwnership(t *testing.T) {
	data, err := os.ReadFile("000050_crm.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"CREATE TABLE crm_pipeline_stages",
		"UNIQUE(company_id,name)",
		"UNIQUE(company_id,position)",
		"CREATE TABLE crm_leads",
		"CREATE INDEX idx_crm_leads_visibility ON crm_leads(company_id,owner_id,status)",
		"UNIQUE INDEX uq_crm_contacts_company_email",
		"CREATE TABLE crm_opportunities",
		"expected_value NUMERIC(18,2)",
		"CREATE INDEX idx_crm_opportunities_pipeline",
		"CREATE TABLE crm_activities",
		"CREATE INDEX idx_crm_activities_due",
		"CREATE INDEX idx_crm_events_timeline",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("CRM migration missing %q", want)
		}
	}
}

func TestCRMQuotationLinkIsRetrySafeAndRestrictsDeletion(t *testing.T) {
	data, err := os.ReadFile("000051_crm_review_fixes.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"crm_opportunity_id BIGINT REFERENCES crm_opportunities(id) ON DELETE RESTRICT",
		"uq_quotations_crm_opportunity",
		"WHERE crm_opportunity_id IS NOT NULL",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("CRM review migration missing %q", want)
		}
	}
}

func TestCRMRepositoryUsesTransactionalQualificationAndConversion(t *testing.T) {
	data, err := os.ReadFile("../internal/crm/repository.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"pgx.BeginTxFunc(ctx, r.pool",
		"INSERT INTO crm_contacts",
		"INSERT INTO crm_opportunities",
		"UPDATE crm_leads SET status='QUALIFIED'",
		"UPDATE crm_opportunities o SET customer_id",
		"WHERE NOT EXISTS (SELECT 1 FROM crm_events",
		"ORDER BY created_at DESC",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("CRM repository missing transactional guarantee %q", want)
		}
	}
}
