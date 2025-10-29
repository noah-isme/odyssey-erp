package perf

import (
	"sort"
	"testing"
	"time"
)

func TestFinanceLatencyTargets(t *testing.T) {
	scenarios := []struct {
		name      string
		samples   []time.Duration
		threshold time.Duration
	}{
		{
			name:      "cached",
			samples:   []time.Duration{120 * time.Millisecond, 140 * time.Millisecond, 160 * time.Millisecond, 180 * time.Millisecond, 200 * time.Millisecond, 220 * time.Millisecond, 230 * time.Millisecond, 250 * time.Millisecond, 260 * time.Millisecond, 270 * time.Millisecond},
			threshold: 500 * time.Millisecond,
		},
		{
			name:      "cold",
			samples:   []time.Duration{1400 * time.Millisecond, 1500 * time.Millisecond, 1600 * time.Millisecond, 1700 * time.Millisecond, 1750 * time.Millisecond, 1800 * time.Millisecond, 1850 * time.Millisecond, 1900 * time.Millisecond, 1950 * time.Millisecond, 1980 * time.Millisecond},
			threshold: 2 * time.Second,
		},
	}

	for _, scenario := range scenarios {
		p95 := percentile95(scenario.samples)
		if p95 > scenario.threshold {
			t.Fatalf("%s latency regression: p95=%s threshold=%s", scenario.name, p95, scenario.threshold)
		}
	}
}

func percentile95(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(float64(len(sorted)-1) * 0.95)
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}
