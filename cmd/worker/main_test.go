package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/platform/db"
	"github.com/odyssey-erp/odyssey-erp/jobs"
)

func TestWorkerPoolUndersized(t *testing.T) {
	tests := []struct {
		name        string
		maxConns    int32
		concurrency int
		want        bool
	}{
		{name: "pgx default 4 is undersized", maxConns: 4, concurrency: 5, want: true},
		{name: "one below threshold", maxConns: 14, concurrency: 5, want: true},
		{name: "exactly threshold", maxConns: 15, concurrency: 5, want: false},
		{name: "worker default", maxConns: workerDefaultPoolMaxConns, concurrency: jobs.WorkerConcurrency, want: false},
		{name: "explicit large pool", maxConns: 40, concurrency: 5, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, workerPoolUndersized(tc.maxConns, tc.concurrency))
		})
	}
}

func TestWorkerPoolDefaultsFromDSN(t *testing.T) {
	tests := []struct {
		name      string
		dsn       string
		wantConns int32
		wantWarn  bool
	}{
		{name: "no pool_max_conns uses worker default", dsn: "postgres://u:p@localhost/db?sslmode=disable", wantConns: 16, wantWarn: false},
		{name: "explicit small pool warns", dsn: "postgres://u:p@localhost/db?pool_max_conns=8", wantConns: 8, wantWarn: true},
		{name: "explicit sufficient pool", dsn: "postgres://u:p@localhost/db?pool_max_conns=15", wantConns: 15, wantWarn: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := db.ConfigWithDefaults(tc.dsn, workerDefaultPoolMaxConns)
			require.NoError(t, err)
			require.Equal(t, tc.wantConns, cfg.MaxConns)
			require.Equal(t, tc.wantWarn, workerPoolUndersized(cfg.MaxConns, jobs.WorkerConcurrency))
		})
	}
}
