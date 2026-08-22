package observability

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

func TestPaymentRecoveryMetricsExposeRecoverySignals(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPaymentRecoveryMetrics(registry)
	metrics.ObserveReconciliationRun("PARTIAL")
	metrics.ObserveCandidate("midtrans", "PENDING")
	metrics.ObserveRecovery("midtrans", "PENDING", "SETTLED")
	metrics.ObserveIssue("midtrans", "LOOKUP_FAILED")
	metrics.ObserveRefund("midtrans", "REFUNDED")
	metrics.ObserveDeadLetter("payment.refund")
	metrics.ObserveFinanceExecution("midtrans_iris", "AMBIGUOUS")
	metrics.SetAmbiguousAge("midtrans_iris", 5*time.Minute)
	metrics.SetUnappliedEffects("midtrans_iris", "SETTLED", 2)
	metrics.SetUnmatchedSettlements("midtrans_iris", 1)
	metrics.ObserveFinanceDeadLetter("payment.execute")
	metrics.ObserveRecoveryAttempt("alert", "attempted")
	metrics.ObserveRecoverySuccess("alert")
	metrics.ObserveProviderLookup("midtrans_iris", 250*time.Millisecond)

	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var builder strings.Builder
	for _, family := range families {
		if _, err := expfmt.MetricFamilyToText(&builder, family); err != nil {
			t.Fatal(err)
		}
	}
	body := builder.String()
	for _, name := range []string{
		"odyssey_payment_reconciliation_runs_total",
		"odyssey_payment_reconciliation_candidates_total",
		"odyssey_payment_recovery_transitions_total",
		"odyssey_payment_reconciliation_issues_total",
		"odyssey_payment_refund_status_total",
		"odyssey_connector_dead_letters_total",
		"odyssey_finance_payment_execution_outcomes_total",
		"odyssey_finance_payment_ambiguous_age_seconds",
		"odyssey_finance_payment_unapplied_effects",
		"odyssey_finance_payment_unmatched_settlements",
		"odyssey_finance_payment_dead_letters_total",
		"odyssey_finance_payment_recovery_attempts_total",
		"odyssey_finance_payment_recovery_success_total",
		"odyssey_finance_payment_provider_lookup_latency_seconds",
	} {
		if !strings.Contains(body, name) {
			t.Fatalf("metrics output missing %s: %s", name, body)
		}
	}
}
