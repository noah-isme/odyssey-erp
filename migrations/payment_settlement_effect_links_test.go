package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestPaymentSettlementEffectLinksMigrationKeepsCompanyScopedDurableLinks(t *testing.T) {
	data, err := os.ReadFile("000127_payment_settlement_effect_links.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, fragment := range []string{
		"CREATE TABLE payment_settlement_effect_links",
		"FOREIGN KEY (company_id, effect_key)",
		"REFERENCES payment_settlement_effects (company_id, effect_key)",
		"amount NUMERIC",
		"metadata JSONB NOT NULL DEFAULT '{}'::jsonb",
		"UNIQUE (\n        company_id, effect_key, link_type, entity_type, entity_id",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("settlement effect-links migration missing %q", fragment)
		}
	}
}
