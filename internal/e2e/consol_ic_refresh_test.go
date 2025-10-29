package e2e

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	jobmetrics "github.com/odyssey-erp/odyssey-erp/internal/jobs"
	"github.com/odyssey-erp/odyssey-erp/jobs"
)

type stubConsolService struct {
	calls []struct {
		group  int64
		period string
	}
	err error
}

func (s *stubConsolService) RebuildConsolidation(_ context.Context, groupID int64, period string) error {
	s.calls = append(s.calls, struct {
		group  int64
		period string
	}{group: groupID, period: period})
	return s.err
}

type stubConsolRepo struct {
	groups []int64
	period string
	err    error
}

func (s *stubConsolRepo) ListGroupIDs(_ context.Context) ([]int64, error) {
	return append([]int64(nil), s.groups...), nil
}

func (s *stubConsolRepo) ActiveConsolidationPeriod(_ context.Context) (string, error) {
	return s.period, s.err
}

func TestConsolidateRefreshJob(t *testing.T) {
	repo := &stubConsolRepo{groups: []int64{11, 22, 33}, period: "2024-02"}
	service := &stubConsolService{}
	reg := prometheus.NewRegistry()
	metrics := jobmetrics.NewMetrics(reg)

	job := jobs.NewConsolidateRefreshJob(service, repo, nil, metrics)
	task, err := jobs.NewConsolidateRefreshTask("all", "active")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := job.Handle(context.Background(), task); err != nil {
		t.Fatalf("job handle: %v", err)
	}
	if len(service.calls) != 3 {
		t.Fatalf("expected 3 refresh calls, got %d", len(service.calls))
	}
	for _, call := range service.calls {
		if call.period != "2024-02" {
			t.Fatalf("expected period 2024-02, got %s", call.period)
		}
	}

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	if !assertCounter(t, families, "odyssey_jobs_total", map[string]string{"job": jobs.TaskConsolidateRefresh, "status": "success"}, 1) {
		t.Fatalf("expected odyssey_jobs_total increment for consolidate refresh")
	}
	if !metricExists(families, "odyssey_job_duration_seconds") {
		t.Fatalf("expected odyssey_job_duration_seconds to be recorded")
	}
}

func assertCounter(t *testing.T, families []*dto.MetricFamily, name string, labels map[string]string, expected float64) bool {
	t.Helper()
	for _, fam := range families {
		if fam.GetName() != name {
			continue
		}
		for _, metric := range fam.GetMetric() {
			if matchLabels(metric.GetLabel(), labels) {
				if metric.GetCounter() == nil {
					return false
				}
				if metric.GetCounter().GetValue() == expected {
					return true
				}
			}
		}
	}
	return false
}

func metricExists(families []*dto.MetricFamily, name string) bool {
	for _, fam := range families {
		if fam.GetName() == name {
			return true
		}
	}
	return false
}

func matchLabels(pairs []*dto.LabelPair, expected map[string]string) bool {
	if len(expected) == 0 {
		return true
	}
	seen := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		seen[pair.GetName()] = pair.GetValue()
	}
	for k, v := range expected {
		if seen[k] != v {
			return false
		}
	}
	return true
}
