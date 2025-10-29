package perf

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	jobmetrics "github.com/odyssey-erp/odyssey-erp/internal/jobs"
)

func TestAnalyticsJobThroughputAndReliability(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := jobmetrics.NewMetrics(reg)

	// Simulate cached jobs finishing fast and mostly successful.
	for i := 0; i < 60; i++ {
		tracker := metrics.Track("analytics.cached_insights")
		time.Sleep(12 * time.Millisecond)
		if err := tracker.End(nil); err != nil {
			t.Fatalf("unexpected error ending cached tracker: %v", err)
		}
	}

	// Simulate cold jobs that are slower but still within 2s budget.
	for i := 0; i < 15; i++ {
		tracker := metrics.Track("analytics.cold_refresh")
		time.Sleep(40 * time.Millisecond)
		if err := tracker.End(nil); err != nil {
			t.Fatalf("unexpected error ending cold tracker: %v", err)
		}
	}

	// Inject a couple of failures to ensure alerts fire correctly.
	for i := 0; i < 3; i++ {
		tracker := metrics.Track("analytics.cached_insights")
		time.Sleep(15 * time.Millisecond)
		if err := tracker.End(errors.New("timeout")); err == nil {
			t.Fatal("expected error to propagate")
		}
	}

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	success := metricValue(t, families, "odyssey_jobs_total", map[string]string{"job": "analytics.cached_insights", "status": "success"})
	failure := metricValue(t, families, "odyssey_jobs_total", map[string]string{"job": "analytics.cached_insights", "status": "failure"})
	if success+failure == 0 {
		t.Fatal("no cached job executions recorded")
	}
	ratio := success / (success + failure)
	if ratio < 0.9 {
		t.Fatalf("cached job success ratio too low: %f", ratio)
	}

	coldDuration := histogramMean(t, families, "odyssey_job_duration_seconds", map[string]string{"job": "analytics.cold_refresh"})
	if coldDuration > 2.0 {
		t.Fatalf("cold refresh duration above budget: %f", coldDuration)
	}

	cachedDuration := histogramMean(t, families, "odyssey_job_duration_seconds", map[string]string{"job": "analytics.cached_insights"})
	if cachedDuration > 0.5 {
		t.Fatalf("cached duration above budget: %f", cachedDuration)
	}
}

func metricValue(t *testing.T, families []*dto.MetricFamily, name string, labels map[string]string) float64 {
	t.Helper()
	for _, fam := range families {
		if fam.GetName() != name {
			continue
		}
		for _, metric := range fam.GetMetric() {
			if hasLabels(metric, labels) {
				if fam.GetType() == dto.MetricType_COUNTER {
					return metric.GetCounter().GetValue()
				}
				if fam.GetType() == dto.MetricType_GAUGE {
					return metric.GetGauge().GetValue()
				}
			}
		}
	}
	t.Fatalf("metric %s with labels %v not found", name, labels)
	return 0
}

func histogramMean(t *testing.T, families []*dto.MetricFamily, name string, labels map[string]string) float64 {
	t.Helper()
	for _, fam := range families {
		if fam.GetName() != name {
			continue
		}
		for _, metric := range fam.GetMetric() {
			if hasLabels(metric, labels) {
				hist := metric.GetHistogram()
				if hist == nil || hist.GetSampleCount() == 0 {
					t.Fatalf("histogram %s missing samples", name)
				}
				return hist.GetSampleSum() / float64(hist.GetSampleCount())
			}
		}
	}
	t.Fatalf("histogram %s with labels %v not found", name, labels)
	return 0
}

func hasLabels(metric *dto.Metric, labels map[string]string) bool {
	for _, lp := range metric.GetLabel() {
		if val, ok := labels[lp.GetName()]; ok {
			if lp.GetValue() != val {
				return false
			}
		}
	}
	for key := range labels {
		found := false
		for _, lp := range metric.GetLabel() {
			if lp.GetName() == key {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
