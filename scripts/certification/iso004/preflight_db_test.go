package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DB-backed preflight tests. They run only when ISO004_PG_DSN points at a
// local, migrated Postgres holding the ISO-004 fixtures (seeded with
// `--iso004 --iso004-key 9000000001`). ISO004_FIXTURES_FILE may point at
// the seed's --json output; testdata/fixtures.json is used otherwise.
// The tests only read: the one write attempt below must be rejected.

func iso004DSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ISO004_PG_DSN")
	if dsn == "" {
		t.Skip("ISO004_PG_DSN not set; skipping DB-backed preflight tests")
	}
	cfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	if ip := net.ParseIP(cfg.Host); cfg.Host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Skipf("ISO004_PG_DSN host %q is not loopback", cfg.Host)
	}
	return dsn
}

func iso004FixturesFile() string {
	if f := os.Getenv("ISO004_FIXTURES_FILE"); f != "" {
		return f
	}
	return sampleFixtures
}

func TestDBReadOnlyPoolRejectsWrites(t *testing.T) {
	dsn := iso004DSN(t)
	ctx := context.Background()
	pool, err := openReadOnlyPool(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	sess, err := readOnlySession(ctx, pool)
	require.NoError(t, err)
	assert.Equal(t, "on", sess.DefaultTransactionReadOn)
	assert.Equal(t, "on", sess.TransactionReadOnly)
	assert.NotEmpty(t, sess.Role)

	// Every pooled connection is read-only, not only the first one.
	for i := 0; i < 3; i++ {
		_, err = pool.Exec(ctx, `CREATE TEMP TABLE iso004_should_fail (id int)`)
		var pgErr *pgconn.PgError
		require.True(t, errors.As(err, &pgErr), "write must fail, got %v", err)
		assert.Equal(t, "25006", pgErr.Code, "read_only_sql_transaction")
	}
}

func TestDBFixtureOwnership(t *testing.T) {
	dsn := iso004DSN(t)
	ctx := context.Background()
	fx, _, err := loadFixtures(iso004FixturesFile(), os.Getenv)
	require.NoError(t, err)
	pool, err := openReadOnlyPool(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	results, err := verifyFixtures(ctx, pool, fixtureChecks(fx))
	require.NoError(t, err)
	require.Len(t, results, 20)
	for _, r := range results {
		assert.True(t, r.OK, "%s: got %s want %s", r.Name, r.Got, r.Expect)
	}

	// Swapping the companies must be detected by every company-scoped probe.
	swapped := *fx
	swapped.CompanyA, swapped.CompanyB = fx.CompanyB, fx.CompanyA
	results, err = verifyFixtures(ctx, pool, fixtureChecks(&swapped))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fixture ownership mismatch")
	bad := 0
	for _, r := range results {
		if !r.OK {
			bad++
		}
	}
	assert.GreaterOrEqual(t, bad, 15)

	// A nonexistent ID is reported as missing, not as a crash.
	missing := *fx
	missing.PayslipB = 1 << 40
	_, err = verifyFixtures(ctx, pool, fixtureChecks(&missing))
	assert.ErrorContains(t, err, `payslip_b: got "<missing>"`)
}

func TestDBFullPreflightPassesAndEnqueues(t *testing.T) {
	dsn := iso004DSN(t)
	run := func(t *testing.T) (string, int, string) {
		mr := miniredis.RunT(t)
		startWorker(t, mr)
		out := filepath.Join(t.TempDir(), "bundle")
		deps := testDeps(t, nil, goodIdentity())
		deps.OpenDB = openReadOnlyPool
		args := []string{"--redis", mr.Addr(), "--dsn", dsn, "--run-id", "9000000001", "--out", out,
			"--candidate-tag", "v0.10.0-rc.9", "--candidate-sha", testSHA, "--release-identity", identityPath,
			"--fixtures", iso004FixturesFile()}
		var stderr bytes.Buffer
		code := runWithDeps(context.Background(), args, &bytes.Buffer{}, &stderr, deps)
		return out, code, stderr.String()
	}

	t.Run("without executor nothing is enqueued", func(t *testing.T) {
		saved := runExecutor
		runExecutor = nil
		t.Cleanup(func() { runExecutor = saved })
		_, code, stderr := run(t)
		assert.Equal(t, exitNoExecutor, code, stderr)
		assert.Contains(t, stderr, "nothing was enqueued")
	})

	out, code, stderr := run(t)
	assert.Equal(t, exitNotEvaluated, code, stderr)
	assert.Contains(t, stderr, "no scenario was evaluated")

	raw, err := os.ReadFile(filepath.Join(out, preflightFileName))
	require.NoError(t, err)
	var p Preflight
	require.NoError(t, json.Unmarshal(raw, &p))
	assert.Equal(t, "PASS", p.Result, strings.Join(p.Failures, "\n"))
	require.NotNil(t, p.DB)
	assert.Equal(t, "on", p.DB.DefaultTransactionReadOn)
	assert.Len(t, p.FixtureChecks, 20)
	cc, _ := checkByName(&p, "db.connector_connections")
	assert.Equal(t, statusPass, cc.Status)
	gp, _ := checkByName(&p, "db.global_ap_policy")
	assert.Equal(t, statusPass, gp.Status)
	assert.Empty(t, p.Failures)
	if pw := passwordOf(dsn); pw != "" {
		assert.NotContains(t, string(raw), ":"+pw+"@", "DSN credentials are redacted")
		assert.NotContains(t, string(raw), "password="+pw)
	}

	// Dynamic IDs were resolved by read-only SELECTs against the database.
	var rec EnqueueRecord
	data, err := os.ReadFile(filepath.Join(out, "scenarios", "S03-object-not-found", "enqueue.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &rec))
	require.Len(t, rec.DynamicValues, 3)
	require.Len(t, rec.Submissions, 3)
	for _, sub := range rec.Submissions {
		assert.Equal(t, outcomeEnqueued, sub.Outcome)
		assert.NotContains(t, sub.Payload, "<", "no placeholder left: %s", sub.Payload)
		require.NotNil(t, sub.TaskInfo)
		assert.Equal(t, sub.Payload, sub.TaskInfo.Payload)
	}
	var maxID int64
	require.NoError(t, pool(t, dsn).QueryRow(context.Background(), `SELECT COALESCE(MAX(id), 0) + 100000 FROM variance_snapshots`).Scan(&maxID))
	assert.Equal(t, fmt.Sprintf(`{"snapshot_id":%d}`, maxID), rec.Submissions[0].Payload)
}

func pool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	p, err := openReadOnlyPool(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return p
}

func passwordOf(dsn string) string {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return ""
	}
	return cfg.Password
}
