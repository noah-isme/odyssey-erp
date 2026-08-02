package migrations

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFinanceAutomationFoundationMigrationHasIsolationAndSafeDefaults(t *testing.T) {
	data, err := os.ReadFile("000076_finance_automation_foundation.up.sql")
	require.NoError(t, err)
	text := string(data)

	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS finance_automation_settings",
		"company_id BIGINT PRIMARY KEY REFERENCES companies(id) ON DELETE CASCADE",
		"bank_feed_auto_sync_enabled BOOLEAN NOT NULL DEFAULT FALSE",
		"payment_execution_enabled BOOLEAN NOT NULL DEFAULT FALSE",
		"CHECK (NOT payment_execution_enabled OR payment_scheduling_enabled)",
		"CREATE TABLE IF NOT EXISTS finance_automation_outbox",
		"UNIQUE (company_id, operation, idempotency_key)",
		"status TEXT NOT NULL DEFAULT 'PENDING'",
		"WHERE status = 'PENDING'",
		"trg_company_finance_automation_settings",
		"finance.payment.propose",
		"procurement.p2p_exception.resolve",
		"fixedassets.warranty.manage",
	} {
		require.Contains(t, text, want)
	}
}
