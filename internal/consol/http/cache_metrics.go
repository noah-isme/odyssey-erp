package http

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	cacheMetricsMu          sync.Mutex
	cacheMetricsInitialized bool

	cacheHitCounter   *prometheus.CounterVec
	cacheMissCounter  *prometheus.CounterVec
	vmBuildHistogram  *prometheus.HistogramVec
	cacheMetricsError error
)

// SetupCacheMetrics registers Prometheus metrics used to observe the consolidated view-model cache.
// The registration is performed once and subsequent calls are ignored.
func SetupCacheMetrics(reg prometheus.Registerer) error {
	cacheMetricsMu.Lock()
	defer cacheMetricsMu.Unlock()
	if cacheMetricsInitialized {
		return cacheMetricsError
	}
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	cacheHitCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "odyssey_consol_cache_hits_total",
		Help: "Number of cache hits for consolidated view models.",
	}, []string{"report", "group", "period"})
	cacheMissCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "odyssey_consol_cache_miss_total",
		Help: "Number of cache misses for consolidated view models.",
	}, []string{"report", "group", "period"})
	vmBuildHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "odyssey_consol_vm_build_duration_seconds",
		Help:    "Duration required to build consolidated view models.",
		Buckets: prometheus.DefBuckets,
	}, []string{"report", "group", "period"})

	for _, collector := range []prometheus.Collector{cacheHitCounter, cacheMissCounter, vmBuildHistogram} {
		if err := reg.Register(collector); err != nil {
			var already prometheus.AlreadyRegisteredError
			if errors.As(err, &already) {
				switch c := already.ExistingCollector.(type) {
				case *prometheus.CounterVec:
					if collector == cacheHitCounter {
						cacheHitCounter = c
					} else {
						cacheMissCounter = c
					}
				case *prometheus.HistogramVec:
					vmBuildHistogram = c
				default:
					cacheMetricsError = fmt.Errorf("consol cache metrics: unexpected collector type %T", c)
				}
				continue
			}
			cacheMetricsError = err
			cacheHitCounter = nil
			cacheMissCounter = nil
			vmBuildHistogram = nil
			cacheMetricsInitialized = true
			return cacheMetricsError
		}
	}

	cacheMetricsInitialized = true
	return cacheMetricsError
}

func recordCacheHit(report string, groupID int64, period string) {
	if cacheHitCounter == nil {
		return
	}
	cacheHitCounter.WithLabelValues(report, strconv.FormatInt(groupID, 10), period).Inc()
}

func recordCacheMiss(report string, groupID int64, period string) {
	if cacheMissCounter == nil {
		return
	}
	cacheMissCounter.WithLabelValues(report, strconv.FormatInt(groupID, 10), period).Inc()
}

func observeVMBuildDuration(report string, groupID int64, period string, duration time.Duration) {
	if vmBuildHistogram == nil {
		return
	}
	vmBuildHistogram.WithLabelValues(report, strconv.FormatInt(groupID, 10), period).Observe(duration.Seconds())
}
