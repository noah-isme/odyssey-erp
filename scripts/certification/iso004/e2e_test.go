package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/jobs"
)

// End-to-end: the real executor, poller, SQL observation and evaluator
// against miniredis and an in-process asynq server with stub handlers. With
// ISO004_PG_DSN (loopback) the real read-only SQL observer runs against the
// local database; otherwise a file-writing fake observer stands in.

// fileSQL writes synthetic, identical snapshots (one row n=7 per query).
type fileSQL struct{}

func (fileSQL) Snapshot(_ context.Context, sp *ScenarioPlan, dir, label string) error {
	s := &SQLSnapshot{Scenario: sp.ID, Label: label, TakenUTC: time.Now().UTC(), TransactionReadOnly: "on", Queries: []SQLQueryResult{}}
	for _, q := range sp.Queries {
		s.Queries = append(s.Queries, SQLQueryResult{Name: q.Name, SQL: q.SQL, Args: q.Args, Columns: []string{"n"}, Rows: []map[string]any{{"n": int64(7)}}})
	}
	return writeJSONFile(filepath.Join(dir, sqlFileName(label)), s)
}

// e2eDSN returns ISO004_PG_DSN when it points at a loopback host, else "".
func e2eDSN(t *testing.T) string {
	dsn := os.Getenv("ISO004_PG_DSN")
	if dsn == "" {
		return ""
	}
	cfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	if ip := net.ParseIP(cfg.Host); cfg.Host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return ""
	}
	return dsn
}

// iso004StubMux mimics rc.9 handler outcomes for the scenario types the
// tests run; types without a handler converge as "handler not found".
func iso004StubMux(varianceErr error) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(jobs.TaskVarianceSnapshotProcess, func(context.Context, *asynq.Task) error { return varianceErr })
	mux.HandleFunc(jobs.TaskBoardPackGenerate, func(context.Context, *asynq.Task) error {
		return fmt.Errorf("board pack not found: %w", asynq.SkipRetry)
	})
	mux.HandleFunc(jobs.TaskDocumentOCR, func(context.Context, *asynq.Task) error { return fmt.Errorf("ocr job not found") })
	return mux
}

type e2eRun struct {
	out     string
	code    int
	records []*EnqueueRecord
	cfg     *Config
	insp    *asynq.Inspector
	fx      *Fixtures
}

func runE2E(t *testing.T, mr *miniredis.Miniredis, runID string, scenarios []string) e2eRun {
	t.Helper()
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })
	insp := asynq.NewInspector(opt)
	t.Cleanup(func() { _ = insp.Close() })

	fixturesFile := sampleFixtures
	var sqlObs SQLObserver = fileSQL{}
	var db queryRower = &fakeDB{}
	if dsn := e2eDSN(t); dsn != "" {
		fixturesFile = iso004FixturesFile()
		p := pool(t, dsn)
		sqlObs, db = &sqlObserver{DB: p}, p
	}
	fx, _, err := loadFixtures(fixturesFile, envMap(nil))
	require.NoError(t, err)

	out := filepath.Join(t.TempDir(), "bundle")
	require.NoError(t, prepareOutDir(out))
	cfg := &Config{RunID: runID, MaxRetry: 2, OutDir: out, Scenarios: scenarios, Poll: 20 * time.Millisecond, Timeout: 30 * time.Second}
	rc := &RunContext{Cfg: cfg, Preflight: &Preflight{Unmet: map[Requirement]string{}}}
	obs := newObservers(rc, insp, nil)
	ex := &Executor{
		Cfg: cfg, Fixtures: fx, Unmet: map[Requirement]string{}, OutDir: out,
		Enqueue: client, Inspect: insp, DB: db,
		Wait: obs.Wait, SQL: sqlObs, Eval: obs.Eval,
	}
	records, code, err := ex.Run(context.Background())
	require.NoError(t, err)
	return e2eRun{out: out, code: code, records: records, cfg: cfg, insp: insp, fx: fx}
}

func readResult(t *testing.T, out, id string) (ScenarioResult, AssertionsFile) {
	t.Helper()
	dir := filepath.Join(out, "scenarios", id)
	var res ScenarioResult
	ok, err := readJSONFile(filepath.Join(dir, resultFileName), &res)
	require.True(t, ok, "%s/result.json exists", id)
	require.NoError(t, err)
	var af AssertionsFile
	ok, err = readJSONFile(filepath.Join(dir, assertionsFileName), &af)
	require.True(t, ok)
	require.NoError(t, err)
	return res, af
}

func TestEndToEndPassingAndFailingScenarios(t *testing.T) {
	mr := miniredis.RunT(t)
	// The stub variance handler returns a plain error, so the malformed
	// payload of S02 is retried instead of SkipRetry: S02 must FAIL.
	startStubWorker(t, mr, iso004StubMux(fmt.Errorf("decode payload: invalid snapshot_id")))

	run := runE2E(t, mr, "9000000002", []string{"S01-unregistered-type", "S02-malformed-payload"})
	assert.Equal(t, exitFail, run.code)
	require.Len(t, run.records, 2)

	for _, id := range []string{"S01-unregistered-type", "S02-malformed-payload"} {
		for _, f := range []string{"enqueue.json", timelineFileName, finalTasksFileName, "sql-before.json", "sql-after.json", assertionsFileName, resultFileName} {
			_, err := os.Stat(filepath.Join(run.out, "scenarios", id, f))
			assert.NoError(t, err, "%s/%s", id, f)
		}
	}

	s01, s01a := readResult(t, run.out, "S01-unregistered-type")
	assert.Equal(t, resultPass, s01.Result, s01.Reasons)
	assert.Empty(t, s01.Reasons)
	assert.True(t, s01.Converged)
	assert.Equal(t, map[string]string{"iso004:9000000002:S01-unregistered-type:1": stateArchived}, s01.FinalStates)
	require.Len(t, s01a.Tasks, 1)
	assert.Equal(t, 2, *s01a.Tasks[0].Retried)
	assert.Contains(t, s01a.Tasks[0].LastErr, "handler not found")
	assert.Contains(t, s01.Timing.Note, "MaxRetry 3-25")
	assert.Equal(t, resultPass, run.records[0].Result.Result)

	s02, s02a := readResult(t, run.out, "S02-malformed-payload")
	assert.Equal(t, resultFail, s02.Result)
	assert.Equal(t, 1, s02.TasksFailed)
	assert.Equal(t, 1, s02.TasksPassed, "the board pack SkipRetry task met its expectation")
	assert.Equal(t, 2, s02.AssertionsPassed, "no rows changed")
	assert.Contains(t, strings.Join(s02.Reasons, "\n"), "Retried 2, expected 0 (SkipRetry)")
	require.Len(t, s02a.Tasks, 2)
	assert.Equal(t, resultFail, s02a.Tasks[0].Status)
	assert.Equal(t, resultPass, s02a.Tasks[1].Status)

	if e2eDSN(t) != "" {
		var before SQLSnapshot
		_, err := readJSONFile(filepath.Join(run.out, "scenarios", "S02-malformed-payload", "sql-before.json"), &before)
		require.NoError(t, err)
		assert.Equal(t, "on", before.TransactionReadOnly)
		assert.Equal(t, "repeatable read", before.Isolation)
		require.Len(t, before.Queries, 2)
		for _, q := range before.Queries {
			assert.Empty(t, q.Error, q.Name)
			assert.Len(t, q.Rows, 1, q.Name)
		}
	}

	// A run of only the passing scenario exits 0.
	pass := runE2E(t, mr, "9000000003", []string{"S01-unregistered-type"})
	assert.Equal(t, exitOK, pass.code)
	res, _ := readResult(t, pass.out, "S01-unregistered-type")
	assert.Equal(t, resultPass, res.Result)
}

func TestEndToEndExcludedScenarioIsNotPass(t *testing.T) {
	mr := miniredis.RunT(t)
	startStubWorker(t, mr, iso004StubMux(nil))
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })
	insp := asynq.NewInspector(opt)
	t.Cleanup(func() { _ = insp.Close() })
	fx, _, err := loadFixtures(sampleFixtures, envMap(nil))
	require.NoError(t, err)
	out := filepath.Join(t.TempDir(), "bundle")
	require.NoError(t, prepareOutDir(out))
	unmet := map[Requirement]string{ReqEmail: "--allow-email not set"}
	cfg := &Config{RunID: "9000000004", MaxRetry: 2, OutDir: out, Scenarios: []string{"S01-unregistered-type", "S11-forged-recipient-mail"}, Poll: 20 * time.Millisecond, Timeout: 30 * time.Second}
	obs := newObservers(&RunContext{Cfg: cfg, Preflight: &Preflight{Unmet: unmet}}, insp, nil)
	ex := &Executor{Cfg: cfg, Fixtures: fx, Unmet: unmet, OutDir: out, Enqueue: client, Inspect: insp, DB: &fakeDB{},
		Wait: obs.Wait, SQL: fileSQL{}, Eval: obs.Eval}
	_, code, err := ex.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, exitFail, code, "an excluded scenario never yields exit 0")
	s01, _ := readResult(t, out, "S01-unregistered-type")
	assert.Equal(t, resultPass, s01.Result)
	s11, _ := readResult(t, out, "S11-forged-recipient-mail")
	assert.Equal(t, resultExcluded, s11.Result)
	assert.Contains(t, s11.Reasons[0], "email-gate unmet: --allow-email not set")
}
