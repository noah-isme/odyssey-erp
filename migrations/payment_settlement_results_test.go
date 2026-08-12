package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestPaymentSettlementResultsMigrationKeepsResultIdentityAndEffectState(t *testing.T) {
	data, err := os.ReadFile("000125_payment_settlement_results.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, fragment := range []string{
		"CREATE TABLE payment_settlement_results",
		"UNIQUE (company_id, result_id)",
		"effect_applied BOOLEAN NOT NULL DEFAULT FALSE",
		"payload JSONB NOT NULL",
		"REFERENCES connector_connections(id)",
		"CREATE TABLE payment_settlement_effects",
		"UNIQUE (company_id, effect_key)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("settlement result migration missing %q", fragment)
		}
	}
}
