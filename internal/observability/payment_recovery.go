package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// PaymentRecoveryMetrics exposes bounded-cardinality metrics for payment
// reconciliation, refund recovery, and connector dead letters.
type PaymentRecoveryMetrics struct {
	runs                  *prometheus.CounterVec
	candidates            *prometheus.CounterVec
	recoveries            *prometheus.CounterVec
	issues                *prometheus.CounterVec
	refunds               *prometheus.CounterVec
	deadLetters           *prometheus.CounterVec
	financeOutcomes       *prometheus.CounterVec
	ambiguousAge          *prometheus.GaugeVec
	unappliedEffects      *prometheus.GaugeVec
	unmatchedSettlements  *prometheus.GaugeVec
	financeDeadLetters    *prometheus.CounterVec
	recoveryAttempts      *prometheus.CounterVec
	recoverySuccess       *prometheus.CounterVec
	providerLookupLatency *prometheus.HistogramVec
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
		financeOutcomes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "odyssey_finance_payment_execution_outcomes_total",
			Help: "Finance payment execution outcomes observed by provider and state.",
		}, []string{"provider", "state"}),
		ambiguousAge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "odyssey_finance_payment_ambiguous_age_seconds",
			Help: "Age in seconds of the oldest unresolved ambiguous finance payment.",
		}, []string{"provider"}),
		unappliedEffects: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "odyssey_finance_payment_unapplied_effects",
			Help: "Unresolved confirmed finance settlement results whose accounting effects are not applied.",
		}, []string{"provider", "state"}),
		unmatchedSettlements: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "odyssey_finance_payment_unmatched_settlements",
			Help: "Confirmed finance settlements without a linked bank reconciliation record.",
		}, []string{"provider"}),
		financeDeadLetters: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "odyssey_finance_payment_dead_letters_total",
			Help: "Finance payment outbox commands observed in the dead-letter state.",
		}, []string{"operation"}),
		recoveryAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "odyssey_finance_payment_recovery_attempts_total",
			Help: "Finance payment recovery and alert attempts by action and outcome.",
		}, []string{"action", "outcome"}),
		recoverySuccess: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "odyssey_finance_payment_recovery_success_total",
			Help: "Successful finance payment recovery actions.",
		}, []string{"action"}),
		providerLookupLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "odyssey_finance_payment_provider_lookup_latency_seconds",
			Help:    "Provider lookup latency for finance payment recovery.",
			Buckets: prometheus.DefBuckets,
		}, []string{"provider"}),
	}
	registerer.MustRegister(
		m.runs,
		m.candidates,
		m.recoveries,
		m.issues,
		m.refunds,
		m.deadLetters,
		m.financeOutcomes,
		m.ambiguousAge,
		m.unappliedEffects,
		m.unmatchedSettlements,
		m.financeDeadLetters,
		m.recoveryAttempts,
		m.recoverySuccess,
		m.providerLookupLatency,
	)
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

// ObserveFinanceExecution records a bounded finance payment outcome. Provider
// and state are normalized by callers to a small allow-list; no instruction or
// provider object IDs are labels.
func (m *PaymentRecoveryMetrics) ObserveFinanceExecution(provider, state string) {
	if m == nil {
		return
	}
	m.financeOutcomes.WithLabelValues(provider, state).Inc()
}

// SetAmbiguousAge updates the oldest unresolved ambiguous payment age for a
// provider. A gauge is used because this is current backlog state, not an
// unbounded event counter.
func (m *PaymentRecoveryMetrics) SetAmbiguousAge(provider string, age time.Duration) {
	if m == nil {
		return
	}
	if age < 0 {
		age = 0
	}
	m.ambiguousAge.WithLabelValues(provider).Set(age.Seconds())
}

// SetUnappliedEffects updates the current confirmed-result backlog by provider
// and state.
func (m *PaymentRecoveryMetrics) SetUnappliedEffects(provider, state string, count int) {
	if m == nil {
		return
	}
	if count < 0 {
		count = 0
	}
	m.unappliedEffects.WithLabelValues(provider, state).Set(float64(count))
}

// SetUnmatchedSettlements updates the current bank-reconciliation backlog.
func (m *PaymentRecoveryMetrics) SetUnmatchedSettlements(provider string, count int) {
	if m == nil {
		return
	}
	if count < 0 {
		count = 0
	}
	m.unmatchedSettlements.WithLabelValues(provider).Set(float64(count))
}

func (m *PaymentRecoveryMetrics) ObserveFinanceDeadLetter(operation string) {
	if m == nil {
		return
	}
	m.financeDeadLetters.WithLabelValues(operation).Inc()
}

func (m *PaymentRecoveryMetrics) ObserveRecoveryAttempt(action, outcome string) {
	if m == nil {
		return
	}
	m.recoveryAttempts.WithLabelValues(action, outcome).Inc()
}

func (m *PaymentRecoveryMetrics) ObserveRecoverySuccess(action string) {
	if m == nil {
		return
	}
	m.recoverySuccess.WithLabelValues(action).Inc()
}

func (m *PaymentRecoveryMetrics) ObserveProviderLookup(provider string, latency time.Duration) {
	if m == nil {
		return
	}
	if latency < 0 {
		latency = 0
	}
	m.providerLookupLatency.WithLabelValues(provider).Observe(latency.Seconds())
}
