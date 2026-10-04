package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/app"
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

func TestWorkerPoolSizePrecedence(t *testing.T) {
	const plainDSN = "postgres://u:p@localhost/db?sslmode=disable"
	tests := []struct {
		name      string
		dsn       string
		envConns  int32
		wantConns int32
		wantWarn  bool
	}{
		{name: "nothing set uses worker default", dsn: plainDSN, wantConns: 16, wantWarn: false},
		{name: "env var beats default", dsn: plainDSN, envConns: 24, wantConns: 24, wantWarn: false},
		{name: "env var beats dsn param", dsn: plainDSN + "&pool_max_conns=40", envConns: 20, wantConns: 20, wantWarn: false},
		{name: "small env var beats large dsn param and warns", dsn: plainDSN + "&pool_max_conns=40", envConns: 8, wantConns: 8, wantWarn: true},
		{name: "large env var beats small dsn param and clears the warning", dsn: plainDSN + "&pool_max_conns=4", envConns: 15, wantConns: 15, wantWarn: false},
		{name: "dsn param honored without env var", dsn: plainDSN + "&pool_max_conns=8", wantConns: 8, wantWarn: true},
		{name: "sufficient dsn param honored without env var", dsn: plainDSN + "&pool_max_conns=15", wantConns: 15, wantWarn: false},
		{name: "unset env var (zero) falls through to dsn param", dsn: plainDSN + "&pool_max_conns=9", envConns: 0, wantConns: 9, wantWarn: true},
		{name: "env var one below threshold warns", dsn: plainDSN, envConns: 14, wantConns: 14, wantWarn: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := db.ConfigWithMaxConns(tc.dsn, tc.envConns, workerDefaultPoolMaxConns)
			require.NoError(t, err)
			require.Equal(t, tc.wantConns, cfg.MaxConns)
			require.Equal(t, tc.wantWarn, workerPoolUndersized(cfg.MaxConns, jobs.WorkerConcurrency))
		})
	}
}

func TestWorkerHandlersForProfile(t *testing.T) {
	all := []jobs.TaskHandler{
		{Type: jobs.TaskProcessAPInvoice},
		{Type: jobs.TaskBoardPackGenerate},
		{Type: jobs.TypeBankFeedsSync},
		{Type: jobs.TypeBankFeedsEvent},
		{Type: jobs.TypeCashForecastRefresh},
		{Type: jobs.TaskConnectorOutboxSweep},
	}
	inProfile := []string{jobs.TaskProcessAPInvoice, jobs.TaskBoardPackGenerate, jobs.TaskConnectorOutboxSweep}
	full := append(append([]string(nil), inProfile...), jobs.TypeBankFeedsSync, jobs.TypeBankFeedsEvent, jobs.TypeCashForecastRefresh)

	tests := []struct {
		profile  app.ReleaseProfile
		want     []string
		biExport bool
	}{
		{app.ReleaseProfileV010Core, inProfile, false},
		{app.ReleaseProfileV011Finance, full, true},
		{app.ReleaseProfileFull, full, true},
	}
	for _, tc := range tests {
		t.Run(string(tc.profile), func(t *testing.T) {
			var got []string
			for _, h := range workerHandlersForProfile(tc.profile, all) {
				got = append(got, h.Type)
			}
			require.ElementsMatch(t, tc.want, got)
			require.Equal(t, tc.biExport, registerBIExportForProfile(tc.profile))
		})
	}
}
