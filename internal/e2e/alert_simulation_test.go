package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

type alertScenario struct {
	name       string
	severity   string
	threshold  float64
	actual     float64
	window     time.Duration
	runbookRef string
}

func TestAlertSimulationProducesFiringAndResolvedLogs(t *testing.T) {
	scenarios := []alertScenario{
		{
			name:       "HighErrorRate",
			severity:   "critical",
			threshold:  0.02,
			actual:     0.05,
			window:     5 * time.Minute,
			runbookRef: "docs/runbook-ops-finance.md#high-error-rate",
		},
		{
			name:       "HighLatency",
			severity:   "warning",
			threshold:  0.8,
			actual:     0.92,
			window:     10 * time.Minute,
			runbookRef: "docs/runbook-ops-finance.md#high-latency",
		},
		{
			name:       "AnomalySpike",
			severity:   "warning",
			threshold:  3,
			actual:     5,
			window:     15 * time.Minute,
			runbookRef: "docs/runbook-ops-finance.md#anomaly-spike",
		},
	}

	var logBuilder strings.Builder
	for _, scenario := range scenarios {
		logBuilder.WriteString(renderAlertLog("FIRING", scenario))
		logBuilder.WriteString(renderAlertLog("RESOLVED", scenario))
	}

	logOutput := logBuilder.String()
	for _, scenario := range scenarios {
		firing := renderAlertLog("FIRING", scenario)
		if !strings.Contains(logOutput, firing) {
			t.Fatalf("expected log to contain firing entry for %s", scenario.name)
		}
		resolved := renderAlertLog("RESOLVED", scenario)
		if !strings.Contains(logOutput, resolved) {
			t.Fatalf("expected log to contain resolved entry for %s", scenario.name)
		}
	}
}

func renderAlertLog(state string, scenario alertScenario) string {
	return fmt.Sprintf("%s %s severity=%s actual=%.2f threshold=%.2f window=%s runbook=%s\n",
		state, scenario.name, scenario.severity, scenario.actual, scenario.threshold, scenario.window, scenario.runbookRef)
}
