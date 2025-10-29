package jobmetrics

import (
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics exposes Prometheus collectors for background jobs.
type Metrics struct {
	runs      *prometheus.CounterVec
	failures  *prometheus.CounterVec
	duration  *prometheus.HistogramVec
	anomalies *prometheus.CounterVec
}

var (
	defaultOnce    sync.Once
	defaultMetrics *Metrics
)

// NewMetrics registers the job metrics against the provided registerer. When the
// registerer is nil the default Prometheus registerer is used.
func NewMetrics(registerer prometheus.Registerer) *Metrics {
	if registerer == nil {
		defaultOnce.Do(func() {
			defaultMetrics = buildMetrics(prometheus.DefaultRegisterer)
		})
		return defaultMetrics
	}
	return buildMetrics(registerer)
}

// Tracker provides lifecycle instrumentation helpers for a single job run.
type Tracker struct {
	metrics *Metrics
	job     string
	start   time.Time
}

// Track spawns a tracker for the given job name.
func (m *Metrics) Track(job string) *Tracker {
	if m == nil {
		return &Tracker{job: job, start: time.Now()}
	}
	return &Tracker{metrics: m, job: job, start: time.Now()}
}

// End finalises the tracker, recording duration, success/failure counts and
// returning the provided error untouched.
func (t *Tracker) End(err error) error {
	if t == nil || t.metrics == nil || t.job == "" {
		return err
	}
	status := "success"
	if err != nil {
		status = "failure"
		t.metrics.failures.WithLabelValues(t.job).Inc()
	}
	t.metrics.runs.WithLabelValues(t.job, status).Inc()
	t.metrics.duration.WithLabelValues(t.job).Observe(time.Since(t.start).Seconds())
	return err
}

// AddAnomalies increments the anomaly counter for the supplied severity and
// company scope.
func (m *Metrics) AddAnomalies(severity string, companyID, branchID int64, count int) {
	if m == nil || count <= 0 {
		return
	}
	company := ""
	branch := ""
	if companyID > 0 {
		company = formatInt(companyID)
	} else {
		company = "0"
	}
	if branchID > 0 {
		branch = formatInt(branchID)
	} else {
		branch = "0"
	}
	m.anomalies.WithLabelValues(severity, company, branch).Add(float64(count))
}

func formatInt(v int64) string {
	return strconv.FormatInt(v, 10)
}

func buildMetrics(registerer prometheus.Registerer) *Metrics {
	runs := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "odyssey_jobs_total",
		Help: "Total job executions partitioned by job name and status.",
	}, []string{"job", "status"})
	failures := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "odyssey_jobs_failures_total",
		Help: "Total failures observed for background jobs.",
	}, []string{"job"})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "odyssey_job_duration_seconds",
		Help:    "Duration in seconds of background job executions.",
		Buckets: prometheus.DefBuckets,
	}, []string{"job"})
	anomalies := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "odyssey_finance_anomalies_total",
		Help: "Detected finance anomalies grouped by severity and scope.",
	}, []string{"severity", "company", "branch"})
	registerer.MustRegister(runs, failures, duration, anomalies)
	return &Metrics{runs: runs, failures: failures, duration: duration, anomalies: anomalies}
}
