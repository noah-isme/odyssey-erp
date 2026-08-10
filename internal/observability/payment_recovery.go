package observability

import "github.com/prometheus/client_golang/prometheus"

// PaymentRecoveryMetrics exposes bounded-cardinality metrics for payment
// reconciliation, refund recovery, and connector dead letters.
type PaymentRecoveryMetrics struct {
	runs        *prometheus.CounterVec
	candidates  *prometheus.CounterVec
	recoveries  *prometheus.CounterVec
	issues      *prometheus.CounterVec
	refunds     *prometheus.CounterVec
	deadLetters *prometheus.CounterVec
}

func NewPaymentRecoveryMetrics(registerer prometheus.Registerer) *PaymentRecoveryMetrics {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	m := &PaymentRecoveryMetrics{
		runs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "odyssey_payment_reconciliation_runs_total",
			Help: "Payment reconciliation runs by terminal status.",
		}, []string{"status"}),
		candidates: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "odyssey_payment_reconciliation_candidates_total",
			Help: "Payment intents examined by provider and local status.",
		}, []string{"provider", "local_status"}),
		recoveries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "odyssey_payment_recovery_transitions_total",
			Help: "Payment states advanced by provider reconciliation.",
		}, []string{"provider", "from_status", "to_status"}),
		issues: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "odyssey_payment_reconciliation_issues_total",
			Help: "Payment reconciliation issues opened or refreshed.",
		}, []string{"provider", "issue_type"}),
		refunds: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "odyssey_payment_refund_status_total",
			Help: "Refund lifecycle statuses persisted from provider responses.",
		}, []string{"provider", "status"}),
		deadLetters: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "odyssey_connector_dead_letters_total",
			Help: "Connector commands observed in the dead-letter state.",
		}, []string{"command_type"}),
	}
	registerer.MustRegister(m.runs, m.candidates, m.recoveries, m.issues, m.refunds, m.deadLetters)
	return m
}

func (m *PaymentRecoveryMetrics) ObserveReconciliationRun(status string) {
	if m == nil {
		return
	}
	m.runs.WithLabelValues(status).Inc()
}

func (m *PaymentRecoveryMetrics) ObserveCandidate(provider, localStatus string) {
	if m == nil {
		return
	}
	m.candidates.WithLabelValues(provider, localStatus).Inc()
}

func (m *PaymentRecoveryMetrics) ObserveRecovery(provider, fromStatus, toStatus string) {
	if m == nil {
		return
	}
	m.recoveries.WithLabelValues(provider, fromStatus, toStatus).Inc()
}

func (m *PaymentRecoveryMetrics) ObserveIssue(provider, issueType string) {
	if m == nil {
		return
	}
	m.issues.WithLabelValues(provider, issueType).Inc()
}

func (m *PaymentRecoveryMetrics) ObserveRefund(provider, status string) {
	if m == nil {
		return
	}
	m.refunds.WithLabelValues(provider, status).Inc()
}

func (m *PaymentRecoveryMetrics) ObserveDeadLetter(commandType string) {
	if m == nil {
		return
	}
	m.deadLetters.WithLabelValues(commandType).Inc()
}
