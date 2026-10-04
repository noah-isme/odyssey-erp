package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// evidenceLineRe is the bash regex of scripts/staging-certification-
// evidence.sh (record parser, lines 187-205), unchanged: RE2 supports the
// POSIX classes it uses.
var evidenceLineRe = regexp.MustCompile(`CERTIFICATION_EVIDENCE[[:space:]]+evidence_id=([A-Z][A-Z0-9-]+)[[:space:]]+(.+)$`)

// closeoutUTCRe is is_utc_timestamp of scripts/staging-certification-closeout.sh.
var closeoutUTCRe = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?Z$`)

// parseEvidenceLog applies the evidence script's per-line rules: the regex,
// then the jq shape check
//
//	(type == "object") and (.evidence_id == $id) and
//	(.result == "PASS" or .result == "FAIL" or .result == "N/A") and
//	(.collected_utc | type == "string") and (.details | type == "string")
//
// It returns the records keyed by ID and fails on duplicates or bad shapes.
func parseEvidenceLog(t *testing.T, log string) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, line := range strings.Split(log, "\n") {
		m := evidenceLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		id, payload := m[1], m[2]
		require.NotContains(t, out, id, "duplicate certification evidence ID")
		var v any
		require.NoError(t, json.Unmarshal([]byte(payload), &v), "payload is JSON")
		obj, ok := v.(map[string]any)
		require.True(t, ok, "payload is an object")
		assert.Equal(t, id, obj["evidence_id"])
		assert.Contains(t, []any{"PASS", "FAIL", "N/A"}, obj["result"])
		_, ok = obj["collected_utc"].(string)
		assert.True(t, ok, "collected_utc is a string")
		_, ok = obj["details"].(string)
		assert.True(t, ok, "details is a string")
		out[id] = obj
	}
	return out
}

// bundleRun runs scenarios end to end and writes the bundle through
// finishRun, the same function executeRun uses.
func bundleRun(t *testing.T, mr *miniredis.Miniredis, runID, reviewer string, scenarios []string) (e2eRun, int, string) {
	t.Helper()
	run := runE2E(t, mr, runID, scenarios)
	identity := filepath.Join(t.TempDir(), "RELEASE_IDENTITY")
	require.NoError(t, os.WriteFile(identity, []byte("tag=v0.10.0-rc.9\ncommit="+testSHA+"\nprofile=v0.10-core\n"), 0o644))
	run.cfg.CandidateTag, run.cfg.CandidateSHA, run.cfg.ReleaseIdentity = "v0.10.0-rc.9", testSHA, identity
	run.cfg.Reviewer = reviewer
	run.cfg.DSN = "postgres://ro:dbsecret@127.0.0.1:5432/odyssey"
	run.cfg.RedisAddr = "redis://:redissecret@127.0.0.1:6379/0"
	pf := &Preflight{
		StartedUTC: time.Now().UTC().Add(-time.Minute), Fixtures: run.fx, Unmet: map[Requirement]string{},
		Servers: []ServerSummary{{ID: "srv", Host: "worker-host", PID: 42, Concurrency: 5, Queues: map[string]int{workerQueue: 1}, Status: "active"}},
	}
	var stderr bytes.Buffer
	rc := &RunContext{Cfg: run.cfg, Preflight: pf, Stderr: &stderr}
	code, err := finishRun(context.Background(), rc, run.records, run.code, nil, run.insp)
	require.NoError(t, err)
	return run, code, stderr.String()
}

func readEvidenceRecord(t *testing.T, out string) (string, map[string]any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(out, evidenceRecordName))
	require.NoError(t, err)
	log := string(raw)
	require.Equal(t, 1, strings.Count(log, "\n"), "exactly one line")
	require.Equal(t, 1, strings.Count(log, "CERTIFICATION_EVIDENCE"), "exactly one record")
	recs := parseEvidenceLog(t, log)
	require.Contains(t, recs, evidenceID)
	return strings.TrimSuffix(log, "\n"), recs[evidenceID]
}

// TestBundleSubsetRunIsFailRehearsal: a --scenarios subset can never be
// certification evidence. Even when every scenario it ran passed, the record
// is FAIL with a non-zero exit, and it says it is a rehearsal and names the
// scenarios that were not run. The real evidence script accepts the FAIL
// record and fails the collection.
func TestBundleSubsetRunIsFailRehearsal(t *testing.T) {
	mr := miniredis.RunT(t)
	startStubWorker(t, mr, iso004StubMux(nil))
	run, code, stderr := bundleRun(t, mr, "9000000011", "", []string{"S01-unregistered-type"})
	assert.Equal(t, exitFail, code, stderr)
	assert.Contains(t, stderr, "ISO-004 evidence: FAIL")

	var notRun []string
	for _, id := range scenarioIDs() {
		if id != "S01-unregistered-type" {
			notRun = append(notRun, id)
		}
	}
	require.NotEmpty(t, notRun)

	line, rec := readEvidenceRecord(t, run.out)
	assert.True(t, strings.HasPrefix(line, `CERTIFICATION_EVIDENCE evidence_id=ISO-004 {"evidence_id":"ISO-004","result":"FAIL","run_id":"9000000011","collected_utc":"`), line)
	assert.Equal(t, "FAIL", rec["result"])
	assert.Equal(t, "9000000011", rec["run_id"])
	assert.Regexp(t, closeoutUTCRe, rec["collected_utc"])
	details := rec["details"].(string)
	assert.True(t, strings.HasPrefix(details, "iso-004-worker-injection/summary.json reviewed by REVIEWER_PENDING; "), details)
	for _, want := range []string{
		"candidate v0.10.0-rc.9 sha " + testSHA,
		"S01-unregistered-type=PASS",
		"scenarios (subset 1 of",
		"REHEARSAL/SUBSET RUN, NOT CERTIFICATION EVIDENCE (result is never PASS); scenarios not run: " + strings.Join(notRun, ", "),
		"task ID prefix iso004:9000000011:",
		"archived tasks with prefix: 1",
		"company_a_id=3", "company_b_id=", "key=",
		"worker worker-host pid 42 concurrency 5",
		"deferred (not executed, recorded as deferred, not N/A): boardpack:generate full generation, documents:ocr legitimate duplicate, bankfeeds:event legitimate delivery",
		"branch: N/A (no worker payload carries branch_id at " + testSHA,
		"timing: MaxRetry(2) vs production 3-25",
	} {
		assert.Contains(t, details, want)
	}
	assert.NotContains(t, details, "all Tier 1")
	assert.NotContains(t, details, "@", "no recipient address in details")

	// Every other bundle file exists and SHA256SUMS covers them.
	for _, f := range []string{runFileName, summaryJSONFileName, summaryMDFileName, archivedTasksFileName, candidateFileName, evidenceRecordName, sha256SumsFileName} {
		_, err := os.Stat(filepath.Join(run.out, f))
		assert.NoError(t, err, f)
	}
	cand, err := os.ReadFile(filepath.Join(run.out, candidateFileName))
	require.NoError(t, err)
	assert.Equal(t, "candidate=v0.10.0-rc.9\ntag=v0.10.0-rc.9\nsha="+testSHA+"\nexpected_sha="+testSHA+"\n", string(cand))

	var archived ArchivedTasksFile
	_, err = readJSONFile(filepath.Join(run.out, archivedTasksFileName), &archived)
	require.NoError(t, err)
	require.Len(t, archived.Tasks, 1)
	assert.Equal(t, "iso004:9000000011:S01-unregistered-type:1", archived.Tasks[0].ID)
	assert.Empty(t, archived.Error)

	raw, err := os.ReadFile(filepath.Join(run.out, runFileName))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "dbsecret")
	assert.NotContains(t, string(raw), "redissecret")
	var rf RunFile
	require.NoError(t, json.Unmarshal(raw, &rf))
	assert.Equal(t, "FAIL", rf.Result)
	assert.Equal(t, exitFail, rf.ExitCode)
	assert.Contains(t, rf.Errors, subsetReason(notRun))
	assert.Equal(t, "postgres://127.0.0.1:5432/odyssey", rf.Config.DSN)
	assert.Equal(t, "v0.10-core", rf.ReleaseIdentity.Parsed["profile"])
	assert.Contains(t, rf.ReleaseIdentity.Contents, "commit="+testSHA)
	assert.NotEmpty(t, rf.Build.GitDescribe)
	assert.Len(t, rf.Servers, 1)
	assert.False(t, rf.StartedUTC.IsZero())
	assert.False(t, rf.FinishedUTC.Before(rf.StartedUTC))
	require.NotNil(t, rf.Fixtures)

	var sum SummaryFile
	_, err = readJSONFile(filepath.Join(run.out, summaryJSONFileName), &sum)
	require.NoError(t, err)
	assert.Equal(t, "FAIL", sum.Result)
	assert.Equal(t, []string{subsetReason(notRun)}, sum.Reasons, "the only reason is the subset: the scenario itself passed")
	require.Len(t, sum.Scenarios, 1)
	row := sum.Scenarios[0]
	assert.Equal(t, resultPass, row.Result, "the scenario row keeps its own verdict")
	assert.Equal(t, []string{"iso004:9000000011:S01-unregistered-type:1"}, row.TaskIDs)
	assert.Equal(t, stateArchived, row.FinalStates["iso004:9000000011:S01-unregistered-type:1"])
	assert.NotNil(t, row.Assertions)
	assert.Contains(t, row.Timing, "timing, not path")

	md, err := os.ReadFile(filepath.Join(run.out, summaryMDFileName))
	require.NoError(t, err)
	assert.Contains(t, string(md), "# ISO-004 worker injection: FAIL")
	assert.Contains(t, string(md), "## Why the result is not PASS")
	assert.Contains(t, string(md), "rehearsal/subset run, not certification evidence")
	assert.Contains(t, string(md), "| S01-unregistered-type | PASS |")

	assertSumsVerify(t, run.out)
	runEvidenceScript(t, run.out, line, "FAIL")
}

// TestBundleFullTier1RunIsPass: the only way to PASS is a run in which every
// Tier 1 scenario ran and passed.
func TestBundleFullTier1RunIsPass(t *testing.T) {
	out := t.TempDir()
	cfg := &Config{RunID: "9000000015", OutDir: out, MaxRetry: 2, CandidateTag: "v0.10.0-rc.9", CandidateSHA: testSHA,
		ReleaseIdentity: filepath.Join(out, "absent")}
	var recs []*EnqueueRecord
	for _, id := range scenarioIDs() {
		recs = append(recs, &EnqueueRecord{Scenario: id, Result: &ScenarioResult{Result: resultPass}})
	}
	result, code, err := writeBundle(context.Background(), &Bundle{Cfg: cfg, Records: recs, ExitCode: exitOK,
		Archived: fakeArchived{}, GitDescribe: func() (string, error) { return "v", nil }})
	require.NoError(t, err)
	assert.Equal(t, resultPass, result)
	assert.Equal(t, exitOK, code)

	line, rec := readEvidenceRecord(t, out)
	assert.Equal(t, "PASS", rec["result"])
	details := rec["details"].(string)
	assert.Contains(t, details, fmt.Sprintf("scenarios (all Tier 1): %s=PASS", scenarioIDs()[0]))
	assert.NotContains(t, details, "subset")
	assert.NotContains(t, details, "REHEARSAL")

	var sum SummaryFile
	_, err = readJSONFile(filepath.Join(out, summaryJSONFileName), &sum)
	require.NoError(t, err)
	assert.Equal(t, "PASS", sum.Result)
	assert.Empty(t, sum.Reasons)
	assert.Len(t, sum.Scenarios, len(scenarioRegistry))
	assertSumsVerify(t, out)
	runEvidenceScript(t, out, line, "PASS")
}

// TestBundleSubsetWithFailingScenarioKeepsBothReasons: a failing scenario in a
// subset is reported next to the subset reason, and the details name every
// scenario not run.
func TestBundleSubsetWithFailingScenarioKeepsBothReasons(t *testing.T) {
	out := t.TempDir()
	cfg := &Config{RunID: "9000000016", OutDir: out, MaxRetry: 2, CandidateTag: "v0.10.0-rc.9", CandidateSHA: testSHA,
		ReleaseIdentity: filepath.Join(out, "absent")}
	ids := scenarioIDs()
	require.GreaterOrEqual(t, len(ids), 3)
	recs := []*EnqueueRecord{
		{Scenario: ids[0], Result: &ScenarioResult{Result: resultPass}},
		{Scenario: ids[1], Result: &ScenarioResult{Result: resultFail, Reasons: []string{"assertion failed"}}},
	}
	result, code, err := writeBundle(context.Background(), &Bundle{Cfg: cfg, Records: recs, ExitCode: exitFail,
		Archived: fakeArchived{}, GitDescribe: func() (string, error) { return "v", nil }})
	require.NoError(t, err)
	assert.Equal(t, resultFail, result)
	assert.Equal(t, exitFail, code)

	var sum SummaryFile
	_, err = readJSONFile(filepath.Join(out, summaryJSONFileName), &sum)
	require.NoError(t, err)
	assert.Contains(t, sum.Reasons, subsetReason(ids[2:]))
	assert.Contains(t, sum.Reasons, "scenario "+ids[1]+" is FAIL")
	_, rec := readEvidenceRecord(t, out)
	assert.Contains(t, rec["details"], "scenarios not run: "+strings.Join(ids[2:], ", "))
}

func TestBundleFailScenarioYieldsFailRecordAndNonZeroExit(t *testing.T) {
	mr := miniredis.RunT(t)
	// The stub variance handler retries a malformed payload: S02 FAILs.
	startStubWorker(t, mr, iso004StubMux(fmt.Errorf("decode payload: invalid snapshot_id")))
	run, code, stderr := bundleRun(t, mr, "9000000012", "Jane Reviewer", []string{"S01-unregistered-type", "S02-malformed-payload"})
	assert.Equal(t, exitFail, code, stderr)
	assert.Contains(t, stderr, "ISO-004 evidence: FAIL")

	line, rec := readEvidenceRecord(t, run.out)
	assert.Equal(t, "FAIL", rec["result"])
	details := rec["details"].(string)
	assert.True(t, strings.HasPrefix(details, "iso-004-worker-injection/summary.json reviewed by Jane Reviewer; "), details)
	assert.Contains(t, details, "S01-unregistered-type=PASS, S02-malformed-payload=FAIL")

	var sum SummaryFile
	_, err := readJSONFile(filepath.Join(run.out, summaryJSONFileName), &sum)
	require.NoError(t, err)
	assert.Equal(t, "FAIL", sum.Result)
	assert.Contains(t, sum.Reasons, "scenario S02-malformed-payload is FAIL")
	require.Len(t, sum.Scenarios, 2)
	require.Len(t, sum.Scenarios[1].Assertions, 2, "S02 assertion outcomes are summarized")
	for _, a := range sum.Scenarios[1].Assertions {
		assert.Equal(t, resultPass, a.Status, a.ID)
	}
	assert.NotEmpty(t, sum.Scenarios[1].Reasons)
	md, err := os.ReadFile(filepath.Join(run.out, summaryMDFileName))
	require.NoError(t, err)
	assert.Contains(t, string(md), "### S02-malformed-payload reasons")

	assertSumsVerify(t, run.out)
	runEvidenceScript(t, run.out, line, "FAIL")
}

func TestBundleExcludedScenarioIsFail(t *testing.T) {
	out := t.TempDir()
	cfg := &Config{RunID: "9000000013", OutDir: out, MaxRetry: 2, CandidateTag: "v0.10.0-rc.9", CandidateSHA: testSHA,
		ReleaseIdentity: filepath.Join(out, "absent")}
	recs := []*EnqueueRecord{
		{Scenario: "S01-unregistered-type", Result: &ScenarioResult{Result: resultPass}},
		{Scenario: "S11-forged-recipient-mail", Result: &ScenarioResult{Result: resultExcluded, Reasons: []string{"excluded: email-gate unmet"}}},
	}
	// Even an exit code of 0 cannot make an EXCLUDED scenario PASS.
	result, code, err := writeBundle(context.Background(), &Bundle{Cfg: cfg, Records: recs, ExitCode: exitOK,
		Archived: fakeArchived{}, GitDescribe: func() (string, error) { return "", errors.New("git: not found") }})
	require.NoError(t, err)
	assert.Equal(t, resultFail, result)
	assert.Equal(t, exitFail, code)
	_, rec := readEvidenceRecord(t, out)
	assert.Equal(t, "FAIL", rec["result"])
	assert.Contains(t, rec["details"], "S11-forged-recipient-mail=EXCLUDED")
	assert.Contains(t, rec["details"], "scenarios (subset 2 of")
	assert.NotContains(t, rec["details"], "=N/A", "no scenario is ever N/A")

	var rf RunFile
	_, err = readJSONFile(filepath.Join(out, runFileName), &rf)
	require.NoError(t, err)
	assert.Equal(t, "unknown", rf.Build.GitDescribe, "a missing git is tolerated")
	assert.Contains(t, rf.Build.GitDescribeError, "git: not found")
	assert.NotEmpty(t, rf.ReleaseIdentity.ReadError, "an unreadable RELEASE_IDENTITY is recorded")
	assert.Equal(t, exitFail, rf.ExitCode)
	assertSumsVerify(t, out)
}

func TestBundleNoScenarioOrListingErrorIsFail(t *testing.T) {
	out := t.TempDir()
	cfg := &Config{RunID: "9000000014", OutDir: out, MaxRetry: 2, CandidateTag: "v0.10.0-rc.9", CandidateSHA: testSHA}
	result, code, err := writeBundle(context.Background(), &Bundle{Cfg: cfg, ExitCode: exitOK,
		Archived: fakeArchived{err: errors.New("redis down")}, GitDescribe: func() (string, error) { return "v", nil }})
	require.NoError(t, err)
	assert.Equal(t, resultFail, result)
	assert.Equal(t, exitFail, code)
	var sum SummaryFile
	_, err = readJSONFile(filepath.Join(out, summaryJSONFileName), &sum)
	require.NoError(t, err)
	assert.Contains(t, sum.Reasons, "no scenario was executed")
	assert.Contains(t, strings.Join(sum.Reasons, "\n"), "archived task listing failed")

	// The executor's own non-zero code is kept.
	out2 := t.TempDir()
	cfg.OutDir = out2
	recs := []*EnqueueRecord{{Scenario: "S01-unregistered-type", Result: &ScenarioResult{Result: resultPass}}}
	_, code, err = writeBundle(context.Background(), &Bundle{Cfg: cfg, Records: recs, ExitCode: exitNotEvaluated,
		Archived: fakeArchived{}, GitDescribe: func() (string, error) { return "v", nil }})
	require.NoError(t, err)
	assert.Equal(t, exitNotEvaluated, code)
	_, rec := readEvidenceRecord(t, out2)
	assert.Equal(t, "FAIL", rec["result"])
}

// fakeArchived pages through a fixed task list.
type fakeArchived struct {
	tasks []*asynq.TaskInfo
	err   error
	calls *int
}

func (f fakeArchived) ListArchivedTasks(_ string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error) {
	if f.calls != nil {
		*f.calls++
	}
	if f.err != nil {
		return nil, f.err
	}
	page := 1
	if f.calls != nil {
		page = *f.calls
	}
	start := (page - 1) * archivedPageSize
	if start >= len(f.tasks) {
		return nil, nil
	}
	return f.tasks[start:min(start+archivedPageSize, len(f.tasks))], nil
}

func TestListRunArchivedFiltersPrefixAcrossPages(t *testing.T) {
	var tasks []*asynq.TaskInfo
	for i := 0; i < archivedPageSize+3; i++ {
		id := fmt.Sprintf("iso004:other:S01:%d", i)
		if i%100 == 0 || i == archivedPageSize+2 {
			id = fmt.Sprintf("iso004:run1:S01:%d", i)
		}
		tasks = append(tasks, &asynq.TaskInfo{ID: id, State: asynq.TaskStateArchived})
	}
	calls := 0
	f := listRunArchived(context.Background(), fakeArchived{tasks: tasks, calls: &calls}, "iso004:run1:", time.Now())
	assert.Empty(t, f.Error)
	assert.Equal(t, 2, calls)
	assert.Equal(t, archivedPageSize+3, f.Scanned)
	require.Len(t, f.Tasks, 7)
	for _, tk := range f.Tasks {
		assert.True(t, strings.HasPrefix(tk.ID, "iso004:run1:"), tk.ID)
	}

	f = listRunArchived(context.Background(), fakeArchived{err: fmt.Errorf("asynq: %w", asynq.ErrQueueNotFound)}, "iso004:run1:", time.Now())
	assert.Empty(t, f.Error, "a queue that does not exist has no archived tasks")
	assert.Empty(t, f.Tasks)
}

func TestSHA256SUMSRoundTripAndTamper(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "scenarios", "S01"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "run.json"), []byte("{}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scenarios", "S01", "result.json"), []byte("{\"a\":1}\n"), 0o644))
	require.NoError(t, writeSHA256SUMS(dir))
	raw, err := os.ReadFile(filepath.Join(dir, sha256SumsFileName))
	require.NoError(t, err)
	entries, err := parseSums(raw)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "run.json", entries[0].Path)
	assert.Equal(t, "scenarios/S01/result.json", entries[1].Path)
	assertSumsVerify(t, dir)

	// Rewriting is idempotent and SHA256SUMS never lists itself.
	require.NoError(t, writeSHA256SUMS(dir))
	raw2, err := os.ReadFile(filepath.Join(dir, sha256SumsFileName))
	require.NoError(t, err)
	assert.Equal(t, raw, raw2)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "run.json"), []byte("{\"tampered\":true}\n"), 0o644))
	if _, err := exec.LookPath("sha256sum"); err == nil {
		cmd := exec.Command("sha256sum", "-c", "--quiet", sha256SumsFileName)
		cmd.Dir = dir
		assert.Error(t, cmd.Run(), "sha256sum -c detects a changed file")
	}

	_, err = parseSums([]byte("nothex  run.json\n"))
	assert.Error(t, err)
	require.NoError(t, os.Symlink("run.json", filepath.Join(dir, "link")))
	assert.ErrorContains(t, writeSHA256SUMS(dir), "not a regular file")
}

func TestReviewerValidation(t *testing.T) {
	cfg, err := parseRunConfig(requiredArgs("--redis", "r:1", "--dsn", "postgres://h/db"), envMap(nil), &bytes.Buffer{})
	require.NoError(t, err)
	assert.Equal(t, "REVIEWER_PENDING", cfg.Reviewer)
	for _, bad := range []string{"", " ", "a;b", "<reviewer>", "a\nb", strings.Repeat("x", maxReviewerLen+1)} {
		_, err := parseRunConfig(requiredArgs("--redis", "r:1", "--dsn", "postgres://h/db", "--reviewer", bad), envMap(nil), &bytes.Buffer{})
		assert.Error(t, err, "%q", bad)
	}
	cfg, err = parseRunConfig(requiredArgs("--redis", "r:1", "--dsn", "postgres://h/db", "--reviewer", "Jane Doe"), envMap(nil), &bytes.Buffer{})
	require.NoError(t, err)
	assert.Equal(t, "Jane Doe", cfg.Reviewer)
}

// assertSumsVerify checks SHA256SUMS in Go and, when available, with
// `sha256sum -c`.
func assertSumsVerify(t *testing.T, dir string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, sha256SumsFileName))
	require.NoError(t, err)
	listed, err := parseSums(raw)
	require.NoError(t, err)
	actual, err := computeSums(dir)
	require.NoError(t, err)
	assert.Equal(t, actual, listed, "SHA256SUMS lists every other file with its current digest")
	if _, err := exec.LookPath("sha256sum"); err != nil {
		t.Log("sha256sum not installed; Go verification only")
		return
	}
	cmd := exec.Command("sha256sum", "-c", "--strict", sha256SumsFileName)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "sha256sum -c: %s", out)
}

// runEvidenceScript registers the bundle the way the procedure does (copy as
// iso-004-worker-injection/ and append the record to staging-
// certification.log) and runs the real evidence script for ISO-004 alone.
// It is skipped when bash or jq is missing.
func runEvidenceScript(t *testing.T, bundle, line, wantResult string) {
	t.Helper()
	for _, tool := range []string{"bash", "jq", "sha256sum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Logf("%s not installed; skipping the evidence script check", tool)
			return
		}
	}
	script, err := filepath.Abs("../../staging-certification-evidence.sh")
	require.NoError(t, err)
	contract, err := filepath.Abs("../../staging-certification-contract.json")
	require.NoError(t, err)

	root := t.TempDir()
	candidate := filepath.Join(root, "candidate")
	require.NoError(t, os.MkdirAll(candidate, 0o755))
	require.NoError(t, os.CopyFS(filepath.Join(candidate, evidenceBundleDir), os.DirFS(bundle)))
	require.NoError(t, os.WriteFile(filepath.Join(candidate, "staging-certification.log"), []byte(line+"\n"), 0o644))
	evidence := filepath.Join(root, "evidence")

	cmd := exec.Command("bash", script, "--candidate", candidate, "--evidence", evidence, "--lane", "operator",
		"--contract", contract, "--expected-evidence-ids", evidenceID, "--local-only")
	cmd.Env = append(os.Environ(), "GITHUB_RUN_ID=9000000099", "EVIDENCE_S3_BUCKET=", "EVIDENCE_S3_PREFIX=", "EXPECTED_EVIDENCE_IDS=",
		"CERTIFICATION_LANE=", "TMPDIR="+root)
	out, err := cmd.CombinedOutput()
	if wantResult == "PASS" {
		require.NoError(t, err, "%s", out)
	} else {
		require.Error(t, err, "a FAIL record fails the collection: %s", out)
	}

	var idx struct {
		Collection struct {
			Result string `json:"result"`
		} `json:"collection"`
		Entries []struct {
			EvidenceID string `json:"evidence_id"`
			Result     string `json:"result"`
			Details    string `json:"details"`
		} `json:"entries"`
		Validation struct {
			Errors []string `json:"errors"`
		} `json:"validation"`
	}
	raw, err := os.ReadFile(filepath.Join(evidence, "evidence-index.json"))
	require.NoError(t, err, "%s", out)
	require.NoError(t, json.Unmarshal(raw, &idx))
	require.Len(t, idx.Entries, 1)
	assert.Equal(t, evidenceID, idx.Entries[0].EvidenceID)
	assert.Equal(t, wantResult, idx.Entries[0].Result)
	assert.Equal(t, wantResult, idx.Collection.Result)
	assert.Contains(t, idx.Entries[0].Details, "reviewed by ")
	for _, e := range idx.Validation.Errors {
		assert.NotContains(t, e, "invalid certification evidence payload")
		assert.NotContains(t, e, "missing certification evidence")
		assert.NotContains(t, e, "unexpected certification evidence")
	}
	if wantResult == "PASS" {
		assert.Empty(t, idx.Validation.Errors)
	} else {
		assert.Equal(t, []string{"certification evidence ISO-004 has result FAIL"}, idx.Validation.Errors)
	}

	// The copied bundle's own manifest still verifies inside the evidence dir.
	cmd = exec.Command("sha256sum", "-c", "--strict", sha256SumsFileName)
	cmd.Dir = filepath.Join(evidence, evidenceBundleDir)
	vout, err := cmd.CombinedOutput()
	require.NoError(t, err, "bundle SHA256SUMS after evidence collection: %s", vout)
}
