package main

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSHA = "0123456789abcdef0123456789abcdef01234567"

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func requiredArgs(extra ...string) []string {
	return append([]string{
		"--run-id", "9000000001",
		"--out", "/tmp/iso004-out",
		"--candidate-tag", "v0.10.0-rc.9",
		"--candidate-sha", testSHA,
	}, extra...)
}

func TestParseRunConfigDefaults(t *testing.T) {
	env := envMap(map[string]string{
		"REDIS_ADDR":          "127.0.0.1:6379",
		"STAGING_CERT_PG_DSN": "postgres://ro@db.staging/odyssey",
		"PG_DSN":              "postgres://rw@db.staging/odyssey",
	})
	cfg, err := parseRunConfig(requiredArgs(), env, io.Discard)
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:6379", cfg.RedisAddr)
	assert.Equal(t, "postgres://ro@db.staging/odyssey", cfg.DSN, "STAGING_CERT_PG_DSN wins over PG_DSN")
	assert.Equal(t, defaultReleaseIdentity, cfg.ReleaseIdentity)
	assert.Equal(t, "/opt/odyssey-staging/current/RELEASE_IDENTITY", cfg.ReleaseIdentity)
	assert.Equal(t, 2, cfg.MaxRetry)
	assert.Equal(t, 15*time.Minute, cfg.Timeout)
	assert.Equal(t, 2*time.Second, cfg.Poll)
	assert.Equal(t, "odyssey-staging-worker.service", cfg.WorkerUnit)
	assert.Empty(t, cfg.WorkerEnv)
	assert.Empty(t, cfg.MailAPI)
	assert.False(t, cfg.AllowEmail, "email is off by default")
	assert.False(t, cfg.DryRun)
	assert.Empty(t, cfg.Scenarios, "empty selection means all Tier 1 scenarios")
	assert.Len(t, selectedScenarios(cfg), len(scenarioRegistry))
	assert.Equal(t, "iso004:9000000001:", cfg.TaskIDPrefix())
	assert.Equal(t, "iso004:9000000001:S01-unregistered-type:3", cfg.TaskID("S01-unregistered-type", 3))
}

func TestParseRunConfigDSNFallsBackToPGDSN(t *testing.T) {
	cfg, err := parseRunConfig(requiredArgs("--redis", "r:6379"), envMap(map[string]string{"PG_DSN": "postgres://x@h/db"}), io.Discard)
	require.NoError(t, err)
	assert.Equal(t, "postgres://x@h/db", cfg.DSN)
}

func TestParseRunConfigRequiresConnectionsOutsideDryRun(t *testing.T) {
	_, err := parseRunConfig(requiredArgs(), envMap(nil), io.Discard)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUsage))
	assert.Contains(t, err.Error(), "--redis")
	assert.Contains(t, err.Error(), "--dsn")

	cfg, err := parseRunConfig(requiredArgs("--dry-run"), envMap(nil), io.Discard)
	require.NoError(t, err, "dry run needs neither Redis nor Postgres")
	assert.True(t, cfg.DryRun)
}

func TestCandidateTagValidation(t *testing.T) {
	cases := []struct {
		tag string
		ok  bool
	}{
		{"v0.10.0-rc.9", true},
		{"v0.10.0-rc.1", true},
		{"v0.10.0-rc.10", true},
		{"v0.10.0-rc.123", true},
		{"v0.10.0-rc.0", false},
		{"v0.10.0-rc.09", false},
		{"v0.10.0", false},
		{"v0.10.1-rc.1", false},
		{"v0.11.0-rc.1", false},
		{"0.10.0-rc.9", false},
		{"v0.10.0-rc.9 ", false},
		{"v0.10.0-rc.9\n", false},
		{"v0.10.0-RC.9", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.tag, func(t *testing.T) {
			err := validateCandidateTag(tc.tag)
			if tc.ok {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			args := []string{"--dry-run", "--run-id", "1", "--out", "o", "--candidate-tag", tc.tag, "--candidate-sha", testSHA}
			_, perr := parseRunConfig(args, envMap(nil), io.Discard)
			assert.Equal(t, tc.ok, perr == nil, "parseRunConfig agrees: %v", perr)
		})
	}
}

func TestParseRunConfigRejectsInvalidValues(t *testing.T) {
	base := func(over map[string]string, extra ...string) []string {
		vals := map[string]string{"--run-id": "9000000001", "--out": "o", "--candidate-tag": "v0.10.0-rc.9", "--candidate-sha": testSHA}
		for k, v := range over {
			vals[k] = v
		}
		args := []string{"--dry-run"}
		for _, k := range []string{"--run-id", "--out", "--candidate-tag", "--candidate-sha"} {
			args = append(args, k, vals[k])
		}
		return append(args, extra...)
	}
	cases := map[string]struct {
		args []string
		want string
	}{
		"uppercase sha":         {base(map[string]string{"--candidate-sha": strings.ToUpper(testSHA)}), "--candidate-sha"},
		"short sha":             {base(map[string]string{"--candidate-sha": "abc123"}), "--candidate-sha"},
		"run id with colon":     {base(map[string]string{"--run-id": "1:2"}), "--run-id"},
		"run id with glob":      {base(map[string]string{"--run-id": "1*"}), "--run-id"},
		"empty run id":          {base(map[string]string{"--run-id": ""}), "--run-id"},
		"empty out":             {base(map[string]string{"--out": ""}), "--out"},
		"max retry zero":        {base(nil, "--max-retry", "0"), "--max-retry"},
		"max retry too high":    {base(nil, "--max-retry", "26"), "--max-retry"},
		"poll beyond timeout":   {base(nil, "--timeout", "1s", "--poll", "2s"), "--poll"},
		"email without api":     {base(nil, "--allow-email"), "--mail-api is required"},
		"api not http":          {base(nil, "--mail-api", "ftp://x"), "--mail-api"},
		"api with credentials":  {base(nil, "--mail-api", "http://u:p@127.0.0.1:8025"), "credentials"},
		"unknown scenario":      {base(nil, "--scenarios", "S01-unregistered-type,S99-nope"), "S99-nope"},
		"duplicate scenario":    {base(nil, "--scenarios", "S01-unregistered-type,S01-unregistered-type"), "twice"},
		"positional argument":   {append(base(nil), "extra"), "unexpected arguments"},
		"retired scenario S04":  {base(nil, "--scenarios", "S04-duplicate-forecast"), "S04"},
		"no unit and no envset": {base(nil, "--worker-unit", ""), "--worker-unit"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseRunConfig(tc.args, envMap(nil), io.Discard)
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUsage))
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestParseRunConfigScenarioSelectionAndWorkerEnv(t *testing.T) {
	cfg, err := parseRunConfig(requiredArgs("--dry-run",
		"--scenarios", " S10-forged-company-bi-export, S01-unregistered-type ",
		"--worker-env", "a.env", "--worker-env", "b.env",
		"--allow-email", "--mail-api", "http://127.0.0.1:8025"), envMap(nil), io.Discard)
	require.NoError(t, err)
	assert.Equal(t, []string{"S10-forged-company-bi-export", "S01-unregistered-type"}, cfg.Scenarios)
	assert.Equal(t, []string{"a.env", "b.env"}, cfg.WorkerEnv)
	sel := selectedScenarios(cfg)
	require.Len(t, sel, 2)
	assert.Equal(t, "S01-unregistered-type", sel[0].ID, "registry order is kept")
}

func TestRedactAddr(t *testing.T) {
	cases := map[string]string{
		"postgres://user:secret@db.staging:5432/odyssey?sslmode=require": "postgres://db.staging:5432/odyssey",
		"redis://:secret@10.0.0.2:6379/0":                                "redis://10.0.0.2:6379/0",
		"127.0.0.1:6379":                                                 "127.0.0.1:6379",
		"host=db user=ro password=secret dbname=odyssey":                 "host=db dbname=odyssey",
		"user:secret@host":                                               "<redacted>",
		"":                                                               "",
	}
	for in, want := range cases {
		got := redactAddr(in)
		assert.Equal(t, want, got, in)
		assert.NotContains(t, got, "secret")
	}
	cfg := &Config{RedisAddr: "redis://:secret@r:6379", DSN: "postgres://u:secret@h/db"}
	r := cfg.redacted()
	assert.NotContains(t, r.Redis+r.DSN, "secret")
}
