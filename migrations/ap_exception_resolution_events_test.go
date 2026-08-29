package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestAPExceptionResolutionEventsMigrationIsTenantScopedAndImmutable(t *testing.T) {
	data, err := os.ReadFile("000129_ap_exception_resolution_events.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, fragment := range []string{
		"CREATE TABLE ap_exception_resolution_events",
		"ap_exception_id BIGINT NOT NULL REFERENCES ap_exceptions(id) ON DELETE RESTRICT",
		"company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT",
		"from_status TEXT NOT NULL CHECK (from_status IN ('OPEN', 'IN_REVIEW'))",
		"to_status TEXT NOT NULL CHECK (to_status IN ('RESOLVED', 'REJECTED'))",
		"CONSTRAINT ap_exception_resolution_events_one_per_exception",
		"CREATE INDEX idx_ap_exception_resolution_events_company_time",
		"CREATE OR REPLACE FUNCTION validate_ap_exception_resolution_event()",
		"COALESCE(i.company_id, s.company_id)",
		"CREATE OR REPLACE FUNCTION prevent_ap_exception_resolution_event_mutation()",
		"BEFORE UPDATE OR DELETE ON ap_exception_resolution_events",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("AP exception resolution migration missing %q", fragment)
		}
	}
}

func TestAPExceptionResolutionEventsMigrationDownRemovesGuards(t *testing.T) {
	data, err := os.ReadFile("000129_ap_exception_resolution_events.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, fragment := range []string{
		"DROP TRIGGER IF EXISTS trg_prevent_ap_exception_resolution_event_mutation",
		"DROP TRIGGER IF EXISTS trg_validate_ap_exception_resolution_event",
		"DROP FUNCTION IF EXISTS prevent_ap_exception_resolution_event_mutation()",
		"DROP FUNCTION IF EXISTS validate_ap_exception_resolution_event()",
		"DROP TABLE IF EXISTS ap_exception_resolution_events",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("AP exception resolution down migration missing %q", fragment)
		}
	}
}
