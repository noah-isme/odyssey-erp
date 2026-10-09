package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/app"
	"github.com/odyssey-erp/odyssey-erp/internal/platform/db"
	"github.com/odyssey-erp/odyssey-erp/internal/storage"
	"github.com/odyssey-erp/odyssey-erp/internal/tax"
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
		{Type: jobs.TaskFinanceAutomationDispatch},
		{Type: jobs.TaskConnectorOutboxSweep},
	}
	inProfile := []string{jobs.TaskProcessAPInvoice, jobs.TaskBoardPackGenerate, jobs.TaskConnectorOutboxSweep}
	full := append(append([]string(nil), inProfile...),
		jobs.TypeBankFeedsSync, jobs.TypeBankFeedsEvent, jobs.TypeCashForecastRefresh, jobs.TaskFinanceAutomationDispatch)

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

func TestWorkerCronForProfile(t *testing.T) {
	cron := func(spec, taskType string) jobs.CronRegistration {
		return jobs.CronRegistration{Spec: spec, Task: asynq.NewTask(taskType, nil)}
	}
	all := []jobs.CronRegistration{
		cron("* * * * *", jobs.TaskConnectorOutboxSweep),
		cron("* * * * *", jobs.TaskFinanceAutomationDispatch),
		cron("*/5 * * * *", jobs.TaskPayrollPayslipDispatch),
		{Spec: "0 * * * *"}, // nil task is passed through for NewWorker to skip
	}
	scheduled := func(profile app.ReleaseProfile) []string {
		var got []string
		for _, c := range workerCronForProfile(profile, all) {
			if c.Task == nil {
				got = append(got, "<nil task>")
				continue
			}
			got = append(got, c.Task.Type())
		}
		return got
	}

	inProfile := []string{jobs.TaskConnectorOutboxSweep, jobs.TaskPayrollPayslipDispatch, "<nil task>"}
	full := []string{jobs.TaskConnectorOutboxSweep, jobs.TaskFinanceAutomationDispatch, jobs.TaskPayrollPayslipDispatch, "<nil task>"}

	tests := []struct {
		profile app.ReleaseProfile
		want    []string
	}{
		{app.ReleaseProfileV010Core, inProfile},
		{app.ReleaseProfileV011Finance, full},
		{app.ReleaseProfileFull, full},
	}
	for _, tc := range tests {
		t.Run(string(tc.profile), func(t *testing.T) {
			require.Equal(t, tc.want, scheduled(tc.profile))
		})
	}
}

func TestTaskTypeEnabledForProfile(t *testing.T) {
	gated := []string{jobs.TypeBankFeedsSync, jobs.TypeBankFeedsEvent, jobs.TypeCashForecastRefresh, jobs.TaskFinanceAutomationDispatch}
	always := []string{jobs.TaskProcessAPInvoice, jobs.TaskConnectorOutboxSweep, jobs.TaskTaxCaptureDispatch}

	for _, taskType := range gated {
		require.False(t, taskTypeEnabledForProfile(app.ReleaseProfileV010Core, taskType), taskType)
		require.True(t, taskTypeEnabledForProfile(app.ReleaseProfileV011Finance, taskType), taskType)
		require.True(t, taskTypeEnabledForProfile(app.ReleaseProfileFull, taskType), taskType)
	}
	for _, taskType := range always {
		for _, profile := range []app.ReleaseProfile{app.ReleaseProfileV010Core, app.ReleaseProfileV011Finance, app.ReleaseProfileFull} {
			require.True(t, taskTypeEnabledForProfile(profile, taskType), "%s under %s", taskType, profile)
		}
	}
}

func TestNewWorkerAPStackWiresLedgerAndAudit(t *testing.T) {
	apService, orchestrator := newWorkerAPStack(nil, tax.NewService(tax.NewRepository(nil), nil))
	require.NotNil(t, orchestrator)

	tests := []struct {
		name string
		got  bool
	}{
		{name: "integration hooks", got: apService.IntegrationHandlerConfigured()},
		{name: "audit logger", got: apService.AuditLoggerConfigured()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.True(t, tc.got, "worker AP service has no %s: MATCHED invoices would post without it", tc.name)
		})
	}
}

// TestDocumentStorageUsesSharedResolver pins the documents storage wiring of
// this binary to app.OpenDocumentStorage, the function the other binary
// (cmd/odyssey and cmd/worker) uses too, so web-written blobs are readable by
// the worker's OCR and disposition jobs. It also checks the behaviour: with
// DOCUMENT_STORAGE_DIR set, a blob lands in that directory and not under the
// working directory.
func TestDocumentStorageUsesSharedResolver(t *testing.T) {
	if reflect.ValueOf(openDocumentStorage).Pointer() != reflect.ValueOf(app.OpenDocumentStorage).Pointer() {
		t.Errorf("openDocumentStorage is not app.OpenDocumentStorage")
	}

	workDir := t.TempDir()
	t.Chdir(workDir)
	sharedDir := filepath.Join(t.TempDir(), "shared-storage")
	t.Setenv("APP_ENV", "staging")
	t.Setenv("RELEASE_PROFILE", string(app.ReleaseProfileV010Core))
	t.Setenv("SESSION_SECRET", "test-session-secret")
	t.Setenv("CSRF_SECRET", "test-csrf-secret")
	t.Setenv("DOCUMENT_STORAGE_DRIVER", "local")
	t.Setenv("DOCUMENT_STORAGE_DIR", sharedDir)
	cfg, err := app.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	ctx := context.Background()
	store, dir, err := openDocumentStorage(ctx, cfg)
	if err != nil {
		t.Fatalf("openDocumentStorage() error = %v", err)
	}
	if dir != sharedDir {
		t.Errorf("openDocumentStorage() dir = %q, want DOCUMENT_STORAGE_DIR %q", dir, sharedDir)
	}
	key, err := store.Put(ctx, storage.PutInput{CompanyID: 1, Data: strings.NewReader("blob"), Size: 4})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(sharedDir, filepath.FromSlash(key))); err != nil {
		t.Fatalf("blob %q is not under DOCUMENT_STORAGE_DIR %q: %v", key, sharedDir, err)
	}
	entries, err := os.ReadDir(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("documents storage wrote %d entries under the working directory", len(entries))
	}
}
