package main

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Table-driven tests of the pure evaluation functions over synthetic
// snapshots, plus the source scan proving the tool issues no write SQL.

func num(s string) json.Number { return json.Number(s) }

func snap(label string, queries ...SQLQueryResult) *SQLSnapshot {
	return &SQLSnapshot{Label: label, TransactionReadOnly: "on", Queries: queries}
}

func qr(name string, rows ...map[string]any) SQLQueryResult {
	if rows == nil {
		rows = []map[string]any{}
	}
	return SQLQueryResult{Name: name, Rows: rows}
}

func row(kv ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

func snaps(ss ...*SQLSnapshot) map[string]*SQLSnapshot {
	out := map[string]*SQLSnapshot{}
	for _, s := range ss {
		out[s.Label] = s
	}
	return out
}

func TestEvaluateAssertion(t *testing.T) {
	countBefore := snap(snapBefore, qr("total", row("n", num("7"))), qr("rows_a", row("id", num("1"), "updated_at", "2026-10-04T01:00:00Z")))
	countAfterSame := snap(snapAfter, qr("total", row("n", num("7"))), qr("rows_a", row("id", num("1"), "updated_at", "2026-10-04T01:00:00Z")),
		qr("one", row("n", num("1"), "status", "FAILED", "posted_by", nil, "rule_id", num("2"), "flag", true, "msg", "OCR job company does not match document version")))
	countAfterChanged := snap(snapAfter, qr("total", row("n", num("8"))), qr("rows_a", row("id", num("1"), "updated_at", "2026-10-04T02:00:00Z")))
	twoRows := snap(snapAfter, qr("one", row("n", num("1")), row("n", num("2"))))
	failedQuery := snap(snapAfter, SQLQueryResult{Name: "one", Error: "relation does not exist"})

	stable1 := snap("after-phase-1", qr("payslip", row("delivered_at", "2026-10-04T05:00:00.123456Z")))
	stable2 := snap(snapAfter, qr("payslip", row("delivered_at", "2026-10-04T05:00:00.123456Z")))
	moved := snap(snapAfter, qr("payslip", row("delivered_at", "2026-10-04T05:09:00Z")))

	cases := []struct {
		name   string
		a      RenderedAssertion
		sql    map[string]*SQLSnapshot
		unmet  map[Requirement]string
		status string
		reason string
	}{
		{"unchanged pass", RenderedAssertion{ID: "u", Kind: AssertUnchanged, Query: "total"}, snaps(countBefore, countAfterSame), nil, resultPass, ""},
		{"unchanged count changed", RenderedAssertion{ID: "u", Kind: AssertUnchanged, Query: "total"}, snaps(countBefore, countAfterChanged), nil, resultFail, "rows changed"},
		{"unchanged updated_at changed", RenderedAssertion{ID: "u", Kind: AssertUnchanged, Query: "rows_a"}, snaps(countBefore, countAfterChanged), nil, resultFail, "rows changed"},
		{"unchanged missing after", RenderedAssertion{ID: "u", Kind: AssertUnchanged, Query: "total"}, snaps(countBefore), nil, resultFail, "needs sql-before.json and sql-after.json"},
		{"unchanged query missing", RenderedAssertion{ID: "u", Kind: AssertUnchanged, Query: "nope"}, snaps(countBefore, countAfterSame), nil, resultFail, "missing from a snapshot"},
		{"equals number vs int literal", RenderedAssertion{ID: "e", Kind: AssertEquals, Query: "one", Column: "n", Want: 1}, snaps(countAfterSame), nil, resultPass, ""},
		{"equals number vs int64 fixture", RenderedAssertion{ID: "e", Kind: AssertEquals, Query: "one", Column: "rule_id", Want: int64(2)}, snaps(countAfterSame), nil, resultPass, ""},
		{"equals mismatch", RenderedAssertion{ID: "e", Kind: AssertEquals, Query: "one", Column: "n", Want: 0}, snaps(countAfterSame), nil, resultFail, "observed 1, want 0"},
		{"equals text", RenderedAssertion{ID: "e", Kind: AssertEquals, Query: "one", Column: "status", Want: "FAILED"}, snaps(countAfterSame), nil, resultPass, ""},
		{"equals text is not number", RenderedAssertion{ID: "e", Kind: AssertEquals, Query: "one", Column: "rule_id", Want: "2"}, snaps(countAfterSame), nil, resultFail, `want "2"`},
		{"equals bool", RenderedAssertion{ID: "e", Kind: AssertEquals, Query: "one", Column: "flag", Want: true}, snaps(countAfterSame), nil, resultPass, ""},
		{"equals NULL vs id", RenderedAssertion{ID: "e", Kind: AssertEquals, Query: "one", Column: "posted_by", Want: int64(6)}, snaps(countAfterSame), nil, resultFail, "observed null, want 6"},
		{"equals needs one row", RenderedAssertion{ID: "e", Kind: AssertEquals, Query: "one", Column: "n", Want: 1}, snaps(twoRows), nil, resultFail, "returned 2 rows"},
		{"equals failed query", RenderedAssertion{ID: "e", Kind: AssertEquals, Query: "one", Column: "n", Want: 1}, snaps(failedQuery), nil, resultFail, "relation does not exist"},
		{"equals missing column", RenderedAssertion{ID: "e", Kind: AssertEquals, Query: "one", Column: "zzz", Want: 1}, snaps(countAfterSame), nil, resultFail, "no column zzz"},
		{"equals at before", RenderedAssertion{ID: "e", Kind: AssertEquals, Query: "total", Column: "n", At: snapBefore, Want: 7}, snaps(countBefore), nil, resultPass, ""},
		{"not null pass", RenderedAssertion{ID: "nn", Kind: AssertNotNull, Query: "one", Column: "status"}, snaps(countAfterSame), nil, resultPass, ""},
		{"not null fail", RenderedAssertion{ID: "nn", Kind: AssertNotNull, Query: "one", Column: "posted_by"}, snaps(countAfterSame), nil, resultFail, "NULL"},
		{"contains pass case-insensitive", RenderedAssertion{ID: "c", Kind: AssertContains, Query: "one", Column: "msg", Want: "Does Not Match Document Version"}, snaps(countAfterSame), nil, resultPass, ""},
		{"contains fail", RenderedAssertion{ID: "c", Kind: AssertContains, Query: "one", Column: "msg", Want: "actor"}, snaps(countAfterSame), nil, resultFail, "does not contain"},
		{"contains on NULL", RenderedAssertion{ID: "c", Kind: AssertContains, Query: "one", Column: "posted_by", Want: "x"}, snaps(countAfterSame), nil, resultFail, "is not text"},
		{"stable pass", RenderedAssertion{ID: "s", Kind: AssertStableAfterFirst, Query: "payslip", Column: "delivered_at"}, snaps(stable1, stable2), nil, resultPass, ""},
		{"stable changed", RenderedAssertion{ID: "s", Kind: AssertStableAfterFirst, Query: "payslip", Column: "delivered_at"}, snaps(stable1, moved), nil, resultFail, "differs across after-phase-1, after"},
		{"stable no snapshot", RenderedAssertion{ID: "s", Kind: AssertStableAfterFirst, Query: "payslip", Column: "delivered_at"}, snaps(countBefore), nil, resultFail, "no snapshot taken after phase 1"},
		{"precondition unmet", RenderedAssertion{ID: "p", Kind: AssertEquals, Query: "one", Column: "n", Want: 1, Requires: []Requirement{ReqNoGlobalAPPolicy}}, snaps(countAfterSame),
			map[Requirement]string{ReqNoGlobalAPPolicy: "1 policy"}, resultFail, "FAIL-by-precondition: no-global-ap-policy unmet: 1 policy"},
		{"precondition met", RenderedAssertion{ID: "p", Kind: AssertEquals, Query: "one", Column: "n", Want: 1, Requires: []Requirement{ReqNoGlobalAPPolicy}}, snaps(countAfterSame),
			map[Requirement]string{ReqEmail: "off"}, resultPass, ""},
		{"unknown kind", RenderedAssertion{ID: "x", Kind: "bogus"}, snaps(countAfterSame), nil, resultFail, "unknown assertion kind"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := evaluateAssertion(c.a, c.sql, nil, c.unmet)
			assert.Equal(t, c.status, r.Status, r.Reason)
			if c.reason != "" {
				assert.Contains(t, r.Reason, c.reason)
			}
			assert.Equal(t, c.a.ID, r.ID)
		})
	}
}

func TestEvaluateMailCount(t *testing.T) {
	one := 1
	msg := func(id, created string, att int, subj string) MailMessage {
		return MailMessage{ID: id, To: []string{"r@staging.invalid"}, Created: created, Attachments: att, Subject: subj}
	}
	mk := func(label string, conv bool, q MailQuery) map[string]*MailSnapshot {
		return map[string]*MailSnapshot{label: {Label: label, AfterConvergence: conv, Queries: []MailQuery{q}}}
	}
	base := RenderedAssertion{ID: "m", Kind: AssertMailCount, At: snapAfter, Recipient: "r@staging.invalid", Want: 1}
	since := base
	since.Since, since.Attachments = "2026-10-04T04:41:31.730731Z", &one
	subj := base
	subj.Want, subj.SubjectContains = 2, "ISO-004 9000000001"

	cases := []struct {
		name   string
		a      RenderedAssertion
		mail   map[string]*MailSnapshot
		status string
		reason string
	}{
		{"exact count", base, mk(snapAfter, true, MailQuery{Recipient: "R@staging.invalid", Messages: []MailMessage{msg("a", "", 0, "")}}), resultPass, ""},
		{"duplicate email", base, mk(snapAfter, true, MailQuery{Recipient: "r@staging.invalid", Messages: []MailMessage{msg("a", "", 0, ""), msg("b", "", 0, "")}}), resultFail, "2 messages to r@staging.invalid, want 1"},
		{"none", base, mk(snapAfter, true, MailQuery{Recipient: "r@staging.invalid", Messages: []MailMessage{}}), resultFail, "0 messages"},
		{"since filters older mail", since, mk(snapAfter, true, MailQuery{Recipient: "r@staging.invalid", Messages: []MailMessage{
			msg("old", "2026-10-03T00:00:00Z", 1, ""), msg("new", "2026-10-04T05:00:00.5Z", 1, "")}}), resultPass, ""},
		{"attachment count", since, mk(snapAfter, true, MailQuery{Recipient: "r@staging.invalid", Messages: []MailMessage{msg("new", "2026-10-04T05:00:00Z", 0, "")}}), resultFail, "0 attachments, want 1"},
		{"unparseable created", since, mk(snapAfter, true, MailQuery{Recipient: "r@staging.invalid", Messages: []MailMessage{msg("x", "yesterday", 1, "")}}), resultFail, "unparseable created"},
		{"subject", subj, mk(snapAfter, true, MailQuery{Recipient: "r@staging.invalid", Messages: []MailMessage{msg("a", "", 0, "ISO-004 9000000001"), msg("b", "", 0, "other")}}), resultFail, `subject "other"`},
		{"query error", base, mk(snapAfter, true, MailQuery{Recipient: "r@staging.invalid", Error: "status 500"}), resultFail, "status 500"},
		{"not after convergence", base, mk(snapAfter, false, MailQuery{Recipient: "r@staging.invalid", Messages: []MailMessage{msg("a", "", 0, "")}}), resultFail, "not taken after convergence"},
		{"recipient not queried", base, mk(snapAfter, true, MailQuery{Recipient: "other@staging.invalid"}), resultFail, "not queried"},
		{"snapshot missing", base, map[string]*MailSnapshot{}, resultFail, "mail-after.json missing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := evaluateAssertion(c.a, nil, c.mail, nil)
			assert.Equal(t, c.status, r.Status, r.Reason)
			if c.reason != "" {
				assert.Contains(t, r.Reason, c.reason)
			}
		})
	}
}

func TestEvaluateTask(t *testing.T) {
	sub := func(outcome string, exp Expectation) Submission {
		return Submission{N: 1, TaskID: "iso004:r:S:1", Type: "t", Outcome: outcome, Expectation: exp, Options: EnqueueOptions{MaxRetry: 2}}
	}
	final := func(state string, retried, maxRetry int, lastErr string) *FinalTasks {
		return &FinalTasks{Tasks: []FinalTask{{TaskID: "iso004:r:S:1", State: state, Converged: isConvergedState(state),
			Info: &TaskInfoRecord{ID: "iso004:r:S:1", State: state, Retried: retried, MaxRetry: maxRetry, LastErr: lastErr}}}}
	}
	archivedMax := Expectation{States: []string{stateArchived}, Retried: RetriedMax, LastErrAnyOf: []string{"handler not found"}}
	archivedZero := Expectation{States: []string{stateArchived}, Retried: RetriedZero}
	anyConverged := Expectation{States: converged, Retried: RetriedAny}
	conflict := Expectation{Conflict: true}

	cases := []struct {
		name   string
		sub    Submission
		final  *FinalTasks
		status string
		reason string
	}{
		{"archived after max retries", sub(outcomeEnqueued, archivedMax), final(stateArchived, 2, 2, "handler not found for task iso004:unregistered"), resultPass, ""},
		{"retried short of max", sub(outcomeEnqueued, archivedMax), final(stateArchived, 1, 2, "handler not found"), resultFail, "Retried 1, expected MaxRetry 2"},
		{"wrong LastErr", sub(outcomeEnqueued, archivedMax), final(stateArchived, 2, 2, "no connection found"), resultFail, "contains none of"},
		{"LastErr case-insensitive", sub(outcomeEnqueued, archivedMax), final(stateArchived, 2, 2, "Handler Not Found"), resultPass, ""},
		{"skip retry", sub(outcomeEnqueued, archivedZero), final(stateArchived, 0, 2, "skip retry"), resultPass, ""},
		{"retried instead of skip", sub(outcomeEnqueued, archivedZero), final(stateArchived, 2, 2, "not found"), resultFail, "Retried 2, expected 0 (SkipRetry)"},
		{"completed where archived expected", sub(outcomeEnqueued, archivedZero), final(stateCompleted, 0, 2, ""), resultFail, "final state completed"},
		{"any converged completed", sub(outcomeEnqueued, anyConverged), final(stateCompleted, 0, 2, ""), resultPass, ""},
		{"any converged archived", sub(outcomeEnqueued, anyConverged), final(stateArchived, 2, 2, "x"), resultPass, ""},
		{"still retrying", sub(outcomeEnqueued, anyConverged), final("retry", 1, 2, "x"), resultFail, "did not converge: final state retry"},
		{"max retry differs", sub(outcomeEnqueued, anyConverged), final(stateCompleted, 0, 25, ""), resultFail, "MaxRetry 25 differs"},
		{"no final observation", sub(outcomeEnqueued, anyConverged), &FinalTasks{}, resultFail, "no final observation"},
		{"nil final", sub(outcomeEnqueued, anyConverged), nil, resultFail, "no final observation"},
		{"unreadable final", sub(outcomeEnqueued, anyConverged), &FinalTasks{Tasks: []FinalTask{{TaskID: "iso004:r:S:1", State: stateNotFound, Error: "asynq: task not found"}}}, resultFail, "task not found"},
		{"expected conflict", sub(outcomeConflict, conflict), nil, resultPass, ""},
		{"unexpected conflict", sub(outcomeConflict, anyConverged), nil, resultFail, "unexpected TaskID conflict"},
		{"conflict expected but enqueued", sub(outcomeEnqueued, conflict), nil, resultFail, "expected ErrTaskIDConflict"},
		{"enqueue error", Submission{Outcome: outcomeError, Error: "dial tcp"}, nil, resultFail, "enqueue error: dial tcp"},
		{"skipped by precondition", Submission{Outcome: outcomeSkipped, Reason: "FAIL-by-precondition: x"}, nil, resultFail, "FAIL-by-precondition"},
		{"not submitted", Submission{Outcome: outcomeNotSubmitted, Reason: "phase 1 did not converge"}, nil, resultFail, "phase 1 did not converge"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tc := evaluateTask(c.sub, c.final)
			assert.Equal(t, c.status, tc.Status, tc.Reasons)
			if c.reason != "" {
				assert.Contains(t, strings.Join(tc.Reasons, "; "), c.reason)
			}
		})
	}
}

func TestEvaluateScenario(t *testing.T) {
	plan := ScenarioPlan{ID: "S", Observations: []string{"obs"}, Assertions: []RenderedAssertion{{ID: "u", Kind: AssertUnchanged, Query: "total"}}}
	subOK := Submission{N: 1, TaskID: "id1", Outcome: outcomeEnqueued, Expectation: Expectation{States: converged, Retried: RetriedAny}, Options: EnqueueOptions{MaxRetry: 2}}
	finalOK := &FinalTasks{Converged: true, Tasks: []FinalTask{{TaskID: "id1", State: stateCompleted, Converged: true, Info: &TaskInfoRecord{State: stateCompleted, MaxRetry: 2}}}}
	same := snaps(snap(snapBefore, qr("total", row("n", num("1")))), snap(snapAfter, qr("total", row("n", num("1")))))
	changed := snaps(snap(snapBefore, qr("total", row("n", num("1")))), snap(snapAfter, qr("total", row("n", num("2")))))
	rec := func(mut func(*EnqueueRecord)) *EnqueueRecord {
		r := &EnqueueRecord{Scenario: "S", Enabled: true, Converged: true, Submissions: []Submission{subOK}, Plan: plan}
		if mut != nil {
			mut(r)
		}
		return r
	}

	cases := []struct {
		name   string
		in     EvalInput
		result string
		reason string
	}{
		{"pass", EvalInput{Record: rec(nil), Final: finalOK, SQL: same}, resultPass, ""},
		{"assertion fails", EvalInput{Record: rec(nil), Final: finalOK, SQL: changed}, resultFail, "assertion u: rows changed"},
		{"excluded", EvalInput{Record: rec(func(r *EnqueueRecord) { r.Enabled, r.Excluded = false, "email-gate unmet: off" }), SQL: same}, resultExcluded, "excluded: email-gate unmet"},
		{"not converged", EvalInput{Record: rec(func(r *EnqueueRecord) { r.Converged = false }), Final: finalOK, SQL: same}, resultFail, "did not converge"},
		{"timed out", EvalInput{Record: rec(func(r *EnqueueRecord) { r.Converged = false }), Final: &FinalTasks{TimedOut: true, Error: "timeout after 15m0s: not converged: id1=retry",
			Tasks: []FinalTask{{TaskID: "id1", State: "retry", Info: &TaskInfoRecord{State: "retry", MaxRetry: 2}}}}, SQL: same}, resultFail, "convergence timeout: timeout after 15m0s"},
		{"executor error", EvalInput{Record: rec(func(r *EnqueueRecord) { r.Errors = []string{"sql after: boom"} }), Final: finalOK, SQL: same}, resultFail, "sql after: boom"},
		{"load error", EvalInput{Record: rec(nil), Final: finalOK, SQL: same, LoadErrors: []string{"decode sql-after.json: bad"}}, resultFail, "decode sql-after.json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.in.MaxRetry = 2
			res, af := evaluateScenario(c.in)
			assert.Equal(t, c.result, res.Result, res.Reasons)
			if c.reason != "" {
				assert.Contains(t, strings.Join(res.Reasons, "\n"), c.reason)
			} else {
				assert.Empty(t, res.Reasons)
			}
			assert.Equal(t, 2, res.Timing.ToolMaxRetry)
			assert.Contains(t, res.Timing.Note, "3-25")
			assert.Equal(t, []string{"obs"}, res.Observations)
			if c.result != resultExcluded {
				assert.Len(t, af.Tasks, 1)
				assert.Len(t, af.Assertions, 1)
			}
		})
	}
}

func TestAfterLabelsOrder(t *testing.T) {
	sql := snaps(snap(snapBefore), snap(snapAfter), snap("after-phase-10"), snap("after-phase-2"), snap("after-phase-1"), snap("after-phase-x"))
	assert.Equal(t, []string{"after-phase-1", "after-phase-2", "after-phase-10", "after"}, afterLabels(sql))
}

func TestNormalizeValueMatchesFileRoundTrip(t *testing.T) {
	ts := time.Date(2026, 10, 4, 12, 0, 0, 123456000, time.FixedZone("WIB", 7*3600))
	vals := []any{nil, int64(42), int32(7), "text", true, ts, []byte("abc"), []byte{0xff, 0x00}, 3.5}
	row := map[string]any{}
	for i, v := range vals {
		nv, err := normalizeValue(v)
		require.NoError(t, err)
		row[strconv.Itoa(i)] = nv
	}
	assert.Equal(t, "2026-10-04T05:00:00.123456Z", row["5"])
	assert.Equal(t, "abc", row["6"])
	assert.Equal(t, "base64:/wA=", row["7"])

	dir := t.TempDir()
	require.NoError(t, writeJSONFile(filepath.Join(dir, "s.json"), row))
	var back map[string]any
	_, err := readJSONFile(filepath.Join(dir, "s.json"), &back)
	require.NoError(t, err)
	assert.Equal(t, canon(row), canon(back), "in-memory and on-disk snapshots compare identically")
	assert.Equal(t, canon(int64(42)), canon(back["1"]))
}

func TestBundleEvaluatorReadsScenarioDir(t *testing.T) {
	dir := t.TempDir()
	rec := &EnqueueRecord{Scenario: "S", Enabled: true, Converged: true,
		Submissions: []Submission{{N: 1, TaskID: "id1", Outcome: outcomeEnqueued, Expectation: Expectation{States: converged, Retried: RetriedAny}, Options: EnqueueOptions{MaxRetry: 2}}},
		Plan: ScenarioPlan{ID: "S", Assertions: []RenderedAssertion{
			{ID: "u", Kind: AssertUnchanged, Query: "total"},
			{ID: "m", Kind: AssertMailCount, At: snapAfter, Recipient: "r@x.invalid", Want: 1},
		}}}
	require.NoError(t, writeJSONFile(filepath.Join(dir, finalTasksFileName), FinalTasks{Converged: true,
		Tasks: []FinalTask{{TaskID: "id1", State: stateCompleted, Converged: true, Info: &TaskInfoRecord{State: stateCompleted, MaxRetry: 2}}}}))
	for _, l := range []string{snapBefore, "after-phase-1", snapAfter} {
		require.NoError(t, writeJSONFile(filepath.Join(dir, sqlFileName(l)), snap(l, qr("total", row("n", int64(3))))))
	}
	require.NoError(t, writeJSONFile(filepath.Join(dir, "mail-after.json"), MailSnapshot{Label: snapAfter, AfterConvergence: true,
		Queries: []MailQuery{{Recipient: "r@x.invalid", Messages: []MailMessage{{ID: "m1"}}}}}))

	ev := &bundleEvaluator{MaxRetry: 2}
	res, err := ev.Evaluate(context.Background(), rec, dir)
	require.NoError(t, err)
	assert.Equal(t, resultPass, res.Result, res.Reasons)
	assert.Equal(t, []string{"sql-after-phase-1.json", "sql-after.json", "sql-before.json"}, res.SQLSnapshots)
	assert.Equal(t, []string{"mail-after.json"}, res.MailSnapshots)
	assert.Equal(t, map[string]string{"id1": stateCompleted}, res.FinalStates)

	var onDisk ScenarioResult
	_, err = readJSONFile(filepath.Join(dir, resultFileName), &onDisk)
	require.NoError(t, err)
	assert.Equal(t, resultPass, onDisk.Result)
	var af AssertionsFile
	_, err = readJSONFile(filepath.Join(dir, assertionsFileName), &af)
	require.NoError(t, err)
	require.Len(t, af.Assertions, 2)
	require.Len(t, af.Tasks, 1)

	// A corrupt snapshot is a FAIL, never skipped.
	require.NoError(t, os.WriteFile(filepath.Join(dir, sqlFileName(snapAfter)), []byte("{"), 0o644))
	res, err = ev.Evaluate(context.Background(), rec, dir)
	require.NoError(t, err)
	assert.Equal(t, resultFail, res.Result)
	assert.Contains(t, strings.Join(res.Reasons, "\n"), "decode sql-after.json")

	// No final-tasks.json: the enqueued task has no observation.
	require.NoError(t, os.Remove(filepath.Join(dir, finalTasksFileName)))
	res, _ = ev.Evaluate(context.Background(), rec, dir)
	assert.Contains(t, strings.Join(res.Reasons, "\n"), "no final observation")
}

// writeSQL matches statements that write or lock, wherever they appear in a
// string literal of the tool's non-test sources.
var writeSQL = regexp.MustCompile(`(?is)\b(INSERT\s+INTO|UPDATE\s+[a-z_."]+\s+SET|DELETE\s+FROM|TRUNCATE\b|DROP\s+(TABLE|SCHEMA|INDEX|VIEW|FUNCTION|ROLE|DATABASE)|ALTER\s+(TABLE|ROLE|SYSTEM|DATABASE|SEQUENCE)|CREATE\s+(TABLE|INDEX|SCHEMA|ROLE|FUNCTION|VIEW|EXTENSION|SEQUENCE|TEMP)|MERGE\s+INTO|COPY\s+[a-z_."(]+|GRANT\s|REVOKE\s|FOR\s+(NO\s+KEY\s+)?UPDATE|FOR\s+(KEY\s+)?SHARE|NEXTVAL\s*\(|SETVAL\s*\(|PG_ADVISORY|SET\s+(SESSION|LOCAL|ROLE|TRANSACTION)|ON\s+CONFLICT|RETURNING\b|LOCK\s+TABLE)`)

// sqlStart matches a literal that is a SQL statement.
var sqlStart = regexp.MustCompile(`(?is)^\s*(SELECT|SHOW|WITH|INSERT|UPDATE|DELETE|MERGE|TRUNCATE|DROP|ALTER|CREATE|COPY|SET|BEGIN|COMMIT|GRANT|REVOKE|CALL|DO|LOCK|VACUUM|REFRESH)\b`)

// TestNoWriteSQLAnywhereInTool parses every non-test Go file of the tool and
// checks every string literal (raw and interpreted): no write or locking SQL
// anywhere, and every literal that is a statement is a single SELECT or SHOW.
// It also forbids Exec calls, the pgx entry point for statements without
// result rows.
func TestNoWriteSQLAnywhereInTool(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	statements, literals := 0, 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		require.NoError(t, err, f)
		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if x.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(x.Value)
				require.NoError(t, err)
				literals++
				assert.False(t, writeSQL.MatchString(s), "%s: write SQL in literal %q", fset.Position(x.Pos()), s)
				if m := sqlStart.FindStringSubmatch(s); m != nil && looksLikeSQL(s) {
					statements++
					kw := strings.ToUpper(m[1])
					assert.Contains(t, []string{"SELECT", "SHOW"}, kw, "%s: statement is not SELECT/SHOW: %q", fset.Position(x.Pos()), s)
					assert.NotContains(t, s, ";", "%s: single statement only", fset.Position(x.Pos()))
				}
			case *ast.SelectorExpr:
				assert.NotEqual(t, "Exec", x.Sel.Name, "%s: Exec call in the tool", fset.Position(x.Pos()))
				assert.NotEqual(t, "CopyFrom", x.Sel.Name, "%s: CopyFrom call in the tool", fset.Position(x.Pos()))
				assert.NotEqual(t, "SendBatch", x.Sel.Name, "%s: SendBatch call in the tool", fset.Position(x.Pos()))
			}
			return true
		})
	}
	assert.Greater(t, literals, 100)
	assert.GreaterOrEqual(t, statements, 30, "the scan must see the tool's SQL")
}

// looksLikeSQL separates statements from prose that starts with a keyword
// (e.g. "set ..." in a message): a statement has FROM, or is SHOW.
func looksLikeSQL(s string) bool {
	u := strings.ToUpper(s)
	return strings.Contains(u, " FROM ") || strings.HasPrefix(strings.TrimSpace(u), "SHOW ") || strings.HasPrefix(strings.TrimSpace(u), "SELECT ")
}

func TestWriteSQLPatternCatchesWrites(t *testing.T) {
	for _, s := range []string{
		"INSERT INTO t (a) VALUES (1)", "update ap_invoices set status = 'X'", "DELETE FROM t", "TRUNCATE t",
		"SELECT id FROM t FOR UPDATE", "SELECT id FROM t FOR NO KEY UPDATE", "SELECT nextval('s')", "SELECT pg_advisory_lock(1)",
		"WITH x AS (DELETE FROM t RETURNING id) SELECT * FROM x", "CREATE TEMP TABLE x (a int)", "COPY t FROM STDIN", "SET SESSION characteristics",
	} {
		assert.True(t, writeSQL.MatchString(s), s)
	}
	for _, s := range []string{
		"SELECT COUNT(*) AS n FROM ap_matching_runs WHERE ap_invoice_id = $1",
		"SELECT i.id, i.status, i.posted_by FROM ap_invoices i WHERE i.id = $1",
		"SHOW transaction_read_only", "updated_at unchanged", "no company-A snapshot changed (status, updated_at)",
	} {
		assert.False(t, writeSQL.MatchString(s), s)
	}
}
