package migrations

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPaymentReconciliationOperationsMigrationHasDurableRecoveryState(t *testing.T) {
	data, err := os.ReadFile("000120_payment_reconciliation_operations.up.sql")
	require.NoError(t, err)
	text := string(data)
	for _, fragment := range []string{
		"CREATE TABLE payment_reconciliation_runs",
		"recovered_count INT NOT NULL DEFAULT 0",
		"CREATE TABLE payment_reconciliation_issues",
		"UNIQUE(company_id, connection_id, provider_reference, issue_type)",
		"CREATE TABLE connector_dead_letter_events",
		"UNIQUE(command_id)",
		"WHERE replayed_at IS NULL",
	} {
		require.Contains(t, text, fragment)
	}
}
