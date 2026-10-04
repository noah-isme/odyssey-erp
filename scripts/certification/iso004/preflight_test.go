package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckReleaseIdentity(t *testing.T) {
	const good = "tag=v0.10.0-rc.9\ncommit=" + testSHA + "\nprofile=v0.10-core\nmigration_ceiling=000124_scoped_rbac_global_compatibility\n"
	cases := []struct {
		name    string
		body    string
		wantErr []string
	}{
		{"exact match", good, nil},
		{"crlf line endings", strings.ReplaceAll(good, "\n", "\r\n"), nil},
		{"tag mismatch", strings.Replace(good, "rc.9", "rc.8", 1), []string{`tag="v0.10.0-rc.8"`}},
		{"tag prefix is not a match", strings.Replace(good, "rc.9", "rc.90", 1), []string{"tag="}},
		{"commit mismatch", strings.Replace(good, testSHA, strings.Repeat("f", 40), 1), []string{"commit="}},
		{"indented line is not exact", strings.Replace(good, "tag=", " tag=", 1), []string{"tag="}},
		{"both missing", "profile=v0.10-core\n", []string{"tag=", "commit="}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ident, err := checkReleaseIdentity("/ri", "v0.10.0-rc.9", testSHA, fakeFS(map[string]string{"/ri": tc.body}))
			if tc.wantErr == nil {
				require.NoError(t, err)
				assert.Equal(t, "v0.10-core", ident["profile"])
				return
			}
			require.Error(t, err)
			for _, w := range tc.wantErr {
				assert.Contains(t, err.Error(), w)
			}
		})
	}
	_, err := checkReleaseIdentity("/absent", "v0.10.0-rc.9", testSHA, fakeFS(nil))
	assert.ErrorContains(t, err, "read RELEASE_IDENTITY /absent")
}

func TestReadOnlyPoolConfig(t *testing.T) {
	pc, err := readOnlyPoolConfig("postgres://ro:secret@127.0.0.1:5432/odyssey?sslmode=disable")
	require.NoError(t, err)
	assert.Equal(t, "on", pc.ConnConfig.RuntimeParams["default_transaction_read_only"])
	assert.Equal(t, "iso004-certification", pc.ConnConfig.RuntimeParams["application_name"])

	_, err = readOnlyPoolConfig("postgres://ro:secret@127.0.0.1:notaport/odyssey")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret")
}

// startWorker runs an asynq server consuming "default" on miniredis so
// Inspector.Servers() reports it.
func startWorker(t *testing.T, mr *miniredis.Miniredis) {
	t.Helper()
	srv := asynq.NewServer(asynq.RedisClientOpt{Addr: mr.Addr()}, asynq.Config{
		Concurrency: 1,
		Queues:      map[string]int{workerQueue: 1},
		LogLevel:    asynq.FatalLevel,
	})
	require.NoError(t, srv.Start(asynq.NewServeMux()))
	t.Cleanup(srv.Shutdown)
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = insp.Close() })
	require.Eventually(t, func() bool {
		s, err := insp.Servers()
		return err == nil && len(s) == 1
	}, 5*time.Second, 20*time.Millisecond)
}

func checkByName(p *Preflight, name string) (Check, bool) {
	for _, c := range p.Checks {
		if c.Name == name {
			return c, true
		}
	}
	return Check{}, false
}

func TestRedisChecks(t *testing.T) {
	cfg := &Config{RunID: "777"}

	t.Run("worker present, prefix unused", func(t *testing.T) {
		mr := miniredis.RunT(t)
		startWorker(t, mr)
		// A task of another run must not trip the prefix check.
		client := asynq.NewClient(asynq.RedisClientOpt{Addr: mr.Addr()})
		t.Cleanup(func() { _ = client.Close() })
		_, err := client.Enqueue(asynq.NewTask("iso004:unregistered", nil), asynq.TaskID("iso004:7770:S01-unregistered-type:1"), asynq.Queue(workerQueue))
		require.NoError(t, err)

		cfg.RedisAddr = mr.Addr()
		p := &Preflight{Unmet: map[Requirement]string{}}
		redisChecks(context.Background(), cfg, p)
		assert.True(t, p.Passed(), "%v", p.Failures)
		require.Len(t, p.Servers, 1)
		assert.Equal(t, 1, p.Servers[0].Concurrency)
		assert.Equal(t, 1, p.Servers[0].Queues[workerQueue])
		assert.NotZero(t, p.Servers[0].PID)
		require.NotNil(t, p.Queue)
		assert.Equal(t, 1, p.Queue.Pending)
	})

	t.Run("existing task with the run prefix", func(t *testing.T) {
		mr := miniredis.RunT(t)
		startWorker(t, mr)
		client := asynq.NewClient(asynq.RedisClientOpt{Addr: mr.Addr()})
		t.Cleanup(func() { _ = client.Close() })
		_, err := client.Enqueue(asynq.NewTask("iso004:unregistered", nil), asynq.TaskID("iso004:777:S01-unregistered-type:1"), asynq.Queue("other"))
		require.NoError(t, err)

		cfg.RedisAddr = mr.Addr()
		p := &Preflight{Unmet: map[Requirement]string{}}
		redisChecks(context.Background(), cfg, p)
		assert.False(t, p.Passed())
		c, ok := checkByName(p, "redis.run_prefix_unused")
		require.True(t, ok)
		assert.Equal(t, statusFail, c.Status)
		assert.Equal(t, []string{"asynq:{other}:t:iso004:777:S01-unregistered-type:1"}, c.Data)
	})

	t.Run("no worker consuming default", func(t *testing.T) {
		mr := miniredis.RunT(t)
		cfg.RedisAddr = mr.Addr()
		p := &Preflight{Unmet: map[Requirement]string{}}
		redisChecks(context.Background(), cfg, p)
		assert.False(t, p.Passed())
		c, _ := checkByName(p, "redis.servers")
		assert.Equal(t, statusFail, c.Status)
		assert.Contains(t, c.Detail, `no active asynq server consumes queue "default"`)
		q, _ := checkByName(p, "redis.queue_info")
		assert.Equal(t, statusPass, q.Status, "a not-yet-created queue is not a failure")
	})

	t.Run("redis unreachable", func(t *testing.T) {
		mr := miniredis.RunT(t)
		addr := mr.Addr()
		mr.Close()
		cfg.RedisAddr = "redis://:topsecret@" + addr
		p := &Preflight{Unmet: map[Requirement]string{}}
		redisChecks(context.Background(), cfg, p)
		assert.False(t, p.Passed())
		require.Len(t, p.Failures, 1)
		assert.Contains(t, p.Failures[0], "redis.ping")
		assert.NotContains(t, p.Failures[0], "topsecret")
	})
}

// testDeps returns preflight dependencies with no real network effects
// besides the given Redis; OpenDB fails unless overridden.
func testDeps(t *testing.T, env map[string]string, files map[string]string) preflightDeps {
	t.Helper()
	return preflightDeps{
		Getenv: envMap(env),
		ReadFile: func(p string) ([]byte, error) {
			if v, ok := files[p]; ok {
				return []byte(v), nil
			}
			return os.ReadFile(p)
		},
		Now: func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) },
		OpenDB: func(context.Context, string) (*pgxpool.Pool, error) {
			return nil, errors.New("connect: connection refused")
		},
		Email: emailDeps{
			Run: func(context.Context, string, ...string) ([]byte, error) {
				return nil, errors.New("systemctl unavailable in tests")
			},
			ReadFile: fakeFS(nil),
			Lookup:   func(context.Context, string) ([]string, error) { return []string{"127.0.0.1"}, nil },
			HTTP:     &http.Client{Timeout: time.Second},
			Timeout:  200 * time.Millisecond,
		},
	}
}

const identityPath = "/opt/test/RELEASE_IDENTITY"

func goodIdentity() map[string]string {
	return map[string]string{identityPath: "tag=v0.10.0-rc.9\ncommit=" + testSHA + "\nprofile=v0.10-core\n"}
}

func TestRunPreflightFailureWritesReasonAndExitsNonZero(t *testing.T) {
	mr := miniredis.RunT(t)
	out := filepath.Join(t.TempDir(), "bundle")
	args := []string{
		"--redis", mr.Addr(),
		"--dsn", "postgres://ro:dbsecret@127.0.0.1:1/odyssey",
		"--run-id", "9000000001",
		"--out", out,
		"--candidate-tag", "v0.10.0-rc.9",
		"--candidate-sha", testSHA,
		"--release-identity", identityPath,
		"--fixtures", sampleFixtures,
	}
	var stdout, stderr bytes.Buffer
	code := runWithDeps(context.Background(), args, &stdout, &stderr, testDeps(t, nil, goodIdentity()))
	assert.Equal(t, exitFail, code)
	assert.Contains(t, stderr.String(), "preflight FAILED; nothing was enqueued")

	raw, err := os.ReadFile(filepath.Join(out, preflightFileName))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "dbsecret")
	var p Preflight
	require.NoError(t, json.Unmarshal(raw, &p))
	assert.Equal(t, "FAIL", p.Result)
	joined := strings.Join(p.Failures, "\n")
	assert.Contains(t, joined, "redis.servers: no active asynq server")
	assert.Contains(t, joined, "db.connect: connect: connection refused")
	assert.Equal(t, "postgres://127.0.0.1:1/odyssey", p.Config.DSN)
	assert.Equal(t, "v0.10-core", p.ReleaseIdentity["profile"])
	c, ok := checkByName(&p, "release_identity")
	require.True(t, ok)
	assert.Equal(t, statusPass, c.Status)
	assert.Equal(t, "--allow-email not set", p.Unmet[ReqEmail])
	require.Len(t, p.Scenarios, len(scenarioRegistry))
	for _, sp := range p.Scenarios {
		if sp.ID == "S11-forged-recipient-mail" || sp.ID == "S12-duplicate-delivery-payslip" {
			assert.False(t, sp.Enabled, sp.ID)
			assert.Contains(t, sp.Excluded, "email-gate unmet: --allow-email not set")
		} else {
			assert.True(t, sp.Enabled, sp.ID)
		}
	}

	// The bundle directory is never reused.
	code = runWithDeps(context.Background(), args, io.Discard, &stderr, testDeps(t, nil, goodIdentity()))
	assert.Equal(t, exitUsage, code)
	assert.Contains(t, stderr.String(), "is not empty")
}

func TestRunPreflightReleaseIdentityMismatchFails(t *testing.T) {
	mr := miniredis.RunT(t)
	startWorker(t, mr)
	out := filepath.Join(t.TempDir(), "bundle")
	files := map[string]string{identityPath: "tag=v0.10.0-rc.8\ncommit=" + testSHA + "\n"}
	args := []string{"--redis", mr.Addr(), "--dsn", "postgres://x@127.0.0.1:1/db", "--run-id", "9000000001", "--out", out,
		"--candidate-tag", "v0.10.0-rc.9", "--candidate-sha", testSHA, "--release-identity", identityPath, "--fixtures", sampleFixtures}
	code := runWithDeps(context.Background(), args, io.Discard, io.Discard, testDeps(t, nil, files))
	assert.Equal(t, exitFail, code)
	raw, err := os.ReadFile(filepath.Join(out, preflightFileName))
	require.NoError(t, err)
	var p Preflight
	require.NoError(t, json.Unmarshal(raw, &p))
	assert.Contains(t, strings.Join(p.Failures, "\n"), `release_identity: RELEASE_IDENTITY /opt/test/RELEASE_IDENTITY has tag="v0.10.0-rc.8", want "v0.10.0-rc.9"`)
}

func TestRunPreflightFixtureProblemsAreFatal(t *testing.T) {
	mr := miniredis.RunT(t)
	out := filepath.Join(t.TempDir(), "bundle")
	args := []string{"--redis", mr.Addr(), "--dsn", "postgres://x@127.0.0.1:1/db", "--run-id", "1234", "--out", out,
		"--candidate-tag", "v0.10.0-rc.9", "--candidate-sha", testSHA, "--release-identity", identityPath, "--fixtures", sampleFixtures}
	code := runWithDeps(context.Background(), args, io.Discard, io.Discard, testDeps(t, nil, goodIdentity()))
	assert.Equal(t, exitFail, code)
	raw, err := os.ReadFile(filepath.Join(out, preflightFileName))
	require.NoError(t, err)
	assert.Contains(t, string(raw), `does not equal --run-id \"1234\"`)
}

func TestDryRunDoesNotTouchRedisOrDB(t *testing.T) {
	mr := miniredis.RunT(t)
	out := filepath.Join(t.TempDir(), "bundle")
	deps := testDeps(t, nil, nil)
	deps.OpenDB = func(context.Context, string) (*pgxpool.Pool, error) {
		t.Fatal("dry run must not open Postgres")
		return nil, nil
	}
	deps.Email.Run = func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("dry run must not inspect the worker unit")
		return nil, nil
	}
	args := []string{"--dry-run", "--redis", mr.Addr(), "--dsn", "postgres://x@127.0.0.1:1/db", "--run-id", "9000000001", "--out", out,
		"--candidate-tag", "v0.10.0-rc.9", "--candidate-sha", testSHA, "--release-identity", filepath.Join(t.TempDir(), "absent"),
		"--fixtures", sampleFixtures, "--allow-email", "--mail-api", "http://127.0.0.1:1"}
	var stdout, stderr bytes.Buffer
	before := mr.TotalConnectionCount()
	code := runWithDeps(context.Background(), args, &stdout, &stderr, deps)
	assert.Equal(t, exitOK, code, stderr.String())
	assert.Equal(t, before, mr.TotalConnectionCount(), "no Redis connection in dry run")
	assert.Equal(t, 0, mr.CommandCount())
	_, err := os.Stat(out)
	assert.True(t, os.IsNotExist(err), "dry run writes no files")

	plan := stdout.String()
	for _, s := range scenarioRegistry {
		assert.Contains(t, plan, "["+s.ID+"]")
	}
	for _, sp := range buildPlan(&Config{RunID: "9000000001"}, nil, nil) {
		for _, task := range sp.Tasks {
			assert.Contains(t, plan, "task_id="+task.TaskID)
		}
	}
	assert.Contains(t, plan, `payload: {"snapshot_id":2}`)
	assert.Contains(t, plan, `payload: {"invoice_id":4,"created_by":6}`)
	assert.Contains(t, plan, "release_identity")
	assert.Contains(t, plan, "fixtures.validate  pass")
	assert.NotContains(t, plan, "postgres://x@")
}

func TestDryRunWithoutFixturesExitsNonZero(t *testing.T) {
	args := []string{"--dry-run", "--run-id", "9000000001", "--out", "o", "--candidate-tag", "v0.10.0-rc.9", "--candidate-sha", testSHA}
	var stdout, stderr bytes.Buffer
	code := runWithDeps(context.Background(), args, &stdout, &stderr, testDeps(t, nil, nil))
	assert.Equal(t, exitFail, code)
	assert.Contains(t, stdout.String(), "[S01-unregistered-type]", "the plan is still printed")
	assert.Contains(t, stdout.String(), "STAGING_CERT_ISO004_KEY is missing")
}

func TestDispatch(t *testing.T) {
	var stderr bytes.Buffer
	assert.Equal(t, exitUsage, dispatch(context.Background(), []string{"bogus"}, io.Discard, &stderr))
	assert.Contains(t, stderr.String(), `unknown subcommand "bogus" (available: run)`)

	stderr.Reset()
	assert.Equal(t, exitUsage, dispatch(context.Background(), []string{"run", "--run-id", "x"}, io.Discard, &stderr))
	assert.Contains(t, stderr.String(), "--candidate-tag")

	stderr.Reset()
	assert.Equal(t, exitUsage, dispatch(context.Background(), nil, io.Discard, &stderr), "no arguments selects run and fails validation")
}

func TestBuildPlanRequirements(t *testing.T) {
	fx, _, err := loadFixtures(sampleFixtures, envMap(nil))
	require.NoError(t, err)
	cfg := &Config{RunID: "9000000001", MaxRetry: 2}
	unmet := map[Requirement]string{
		ReqNoConnectorConnections: "1 row",
		ReqNoGlobalAPPolicy:       "1 policy",
		ReqEmail:                  "--allow-email not set",
	}
	plan := buildPlan(cfg, fx, unmet)
	byID := map[string]ScenarioPlan{}
	for _, sp := range plan {
		byID[sp.ID] = sp
	}
	assert.False(t, byID["S10-forged-company-bi-export"].Enabled)
	assert.False(t, byID["S11-forged-recipient-mail"].Enabled)
	assert.False(t, byID["S12-duplicate-delivery-payslip"].Enabled)
	s09 := byID["S09-duplicate-delivery-ap"]
	assert.True(t, s09.Enabled, "only the MISSING path is skipped")
	assert.Len(t, s09.Tasks, 9)
	assert.Len(t, s09.Skipped, 3)
	for id, why := range s09.Skipped {
		assert.True(t, strings.HasPrefix(id, "iso004:9000000001:S09-duplicate-delivery-ap:"))
		assert.Contains(t, why, "FAIL-by-precondition")
	}
	for _, sp := range plan {
		for _, task := range sp.Tasks {
			assert.True(t, strings.HasPrefix(task.TaskID, cfg.TaskIDPrefix()) || task.TaskID == "iso004:9000000001:S11", task.TaskID)
		}
	}
	assert.NotContains(t, byID, "S04-duplicate-forecast", "S04 is retired")
}

func TestScenarioRegistryIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range scenarioRegistry {
		assert.False(t, seen[s.ID], s.ID)
		seen[s.ID] = true
		assert.NotContains(t, s.ID, ":")
		assert.NotEmpty(t, s.Tasks)
	}
	assert.Equal(t, fmt.Sprint(scenarioIDs()), fmt.Sprint([]string{
		"S01-unregistered-type", "S02-malformed-payload", "S03-object-not-found", "S05-forged-object-variance",
		"S06-forged-crossscope-ocr", "S07-duplicate-delivery-variance", "S08-unregistered-under-profile",
		"S09-duplicate-delivery-ap", "S09b-forged-actor-ap", "S10-forged-company-bi-export",
		"S11-forged-recipient-mail", "S12-duplicate-delivery-payslip",
	}))
}
