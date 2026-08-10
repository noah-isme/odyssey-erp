package observability

import (
	"strings"
	"testing"

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
	} {
		if !strings.Contains(body, name) {
			t.Fatalf("metrics output missing %s: %s", name, body)
		}
	}
}
