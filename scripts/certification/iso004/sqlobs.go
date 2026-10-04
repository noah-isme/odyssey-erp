package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// SQL observation and scenario evaluation (plan Step 4).
//
// sqlObserver runs a scenario's rendered snapshot queries (scenarios.go; all
// single SELECT statements) inside one READ ONLY, REPEATABLE READ transaction
// on the read-only pool and writes sql-<label>.json. bundleEvaluator then
// reads the scenario directory back (enqueue record, final-tasks.json,
// sql-*.json, mail-*.json) and calls evaluateScenario, a pure function, so the
// verdict is computed from exactly the files that ship in the evidence
// bundle. It writes assertions.json and result.json.

// Scenario results.
const (
	resultPass     = "PASS"
	resultFail     = "FAIL"
	resultExcluded = "EXCLUDED" // a scenario-level requirement was unmet; never counts as PASS
)

const (
	assertionsFileName = "assertions.json"
	resultFileName     = "result.json"
)

// SQLQueryResult is one query of a snapshot. Values are normalized to JSON
// (timestamps as UTC RFC3339Nano strings, numbers as JSON numbers).
type SQLQueryResult struct {
	Name    string           `json:"name"`
	SQL     string           `json:"sql"`
	Args    []any            `json:"args"`
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
	Error   string           `json:"error,omitempty"`
}

// SQLSnapshot is written to sql-<label>.json.
type SQLSnapshot struct {
	Scenario            string           `json:"scenario"`
	Label               string           `json:"label"`
	TakenUTC            time.Time        `json:"taken_utc"`
	TransactionReadOnly string           `json:"transaction_read_only"`
	Isolation           string           `json:"transaction_isolation"`
	Queries             []SQLQueryResult `json:"queries"`
}

// Query returns the named query result.
func (s *SQLSnapshot) Query(name string) (SQLQueryResult, bool) {
	if s == nil {
		return SQLQueryResult{}, false
	}
	for _, q := range s.Queries {
		if q.Name == name {
			return q, true
		}
	}
	return SQLQueryResult{}, false
}

func sqlFileName(label string) string { return "sql-" + label + ".json" }

// txBeginner is the subset of pgxpool.Pool the observer needs.
type txBeginner interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

// sqlObserver implements SQLObserver.
type sqlObserver struct {
	DB  txBeginner
	Now func() time.Time
}

func (o *sqlObserver) now() time.Time {
	if o.Now != nil {
		return o.Now().UTC()
	}
	return time.Now().UTC()
}

// Snapshot runs every query of the scenario in one read-only transaction and
// writes sql-<label>.json. A query error is recorded in the file and
// returned; the transaction is always rolled back.
func (o *sqlObserver) Snapshot(ctx context.Context, sp *ScenarioPlan, dir, label string) error {
	snap := &SQLSnapshot{Scenario: sp.ID, Label: label, TakenUTC: o.now(), Queries: []SQLQueryResult{}}
	err := o.collect(ctx, sp, snap)
	if werr := writeJSONFile(filepath.Join(dir, sqlFileName(label)), snap); werr != nil {
		return errors.Join(err, werr)
	}
	return err
}

func (o *sqlObserver) collect(ctx context.Context, sp *ScenarioPlan, snap *SQLSnapshot) error {
	if o.DB == nil {
		return errors.New("sql observer has no database")
	}
	tx, err := o.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin read-only transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := tx.QueryRow(ctx, `SHOW transaction_read_only`).Scan(&snap.TransactionReadOnly); err != nil {
		return fmt.Errorf("show transaction_read_only: %w", err)
	}
	if err := tx.QueryRow(ctx, `SHOW transaction_isolation`).Scan(&snap.Isolation); err != nil {
		return fmt.Errorf("show transaction_isolation: %w", err)
	}
	if snap.TransactionReadOnly != "on" {
		return fmt.Errorf("snapshot transaction is not read-only (transaction_read_only=%s)", snap.TransactionReadOnly)
	}
	var errs []error
	for _, q := range sp.Queries {
		r := SQLQueryResult{Name: q.Name, SQL: q.SQL, Args: q.Args, Columns: []string{}, Rows: []map[string]any{}}
		if r.Args == nil {
			r.Args = []any{}
		}
		if err := runSnapshotQuery(ctx, tx, &r); err != nil {
			r.Error = err.Error()
			errs = append(errs, fmt.Errorf("query %s: %w", q.Name, err))
		}
		snap.Queries = append(snap.Queries, r)
		if r.Error != "" {
			// A failed statement aborts the transaction; later queries cannot
			// run. Their absence fails every assertion that needs them.
			break
		}
	}
	return errors.Join(errs...)
}

func runSnapshotQuery(ctx context.Context, tx pgx.Tx, r *SQLQueryResult) error {
	if !isSelectStatement(r.SQL) {
		return errors.New("refusing a statement that is not a single SELECT")
	}
	rows, err := tx.Query(ctx, r.SQL, r.Args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for _, fd := range rows.FieldDescriptions() {
		r.Columns = append(r.Columns, fd.Name)
	}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return err
		}
		row := make(map[string]any, len(vals))
		for i, v := range vals {
			nv, err := normalizeValue(v)
			if err != nil {
				return fmt.Errorf("column %s: %w", r.Columns[i], err)
			}
			row[r.Columns[i]] = nv
		}
		r.Rows = append(r.Rows, row)
	}
	return rows.Err()
}

// isSelectStatement is a runtime guard in addition to the read-only
// transaction: one statement, starting with SELECT.
func isSelectStatement(sql string) bool {
	s := strings.TrimSpace(sql)
	return strings.HasPrefix(strings.ToUpper(s), "SELECT") && !strings.Contains(s, ";")
}

// normalizeValue converts a pgx value into the JSON form stored in the
// snapshot file (so in-memory and on-disk snapshots compare identically).
func normalizeValue(v any) (any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano), nil
	case []byte:
		if utf8.Valid(x) {
			return string(x), nil
		}
		return "base64:" + base64.StdEncoding.EncodeToString(x), nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v), nil
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// canon is the comparison form of a JSON-normalized value: its JSON
// encoding (map keys sorted; json.Number, int64 and float64 1 all encode
// as 1).
func canon(v any) string {
	if t, ok := v.(time.Time); ok {
		v = t.UTC().Format(time.RFC3339Nano)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%#v", v)
	}
	return string(b)
}

// ---- evaluation (pure) ---------------------------------------------------

// TaskCheck is the evaluation of one submission against its Expectation.
type TaskCheck struct {
	N           int         `json:"n"`
	TaskID      string      `json:"task_id"`
	Type        string      `json:"type"`
	Outcome     string      `json:"outcome"`
	Expectation Expectation `json:"expectation"`
	FinalState  string      `json:"final_state,omitempty"`
	Retried     *int        `json:"retried,omitempty"`
	MaxRetry    *int        `json:"max_retry,omitempty"`
	LastErr     string      `json:"last_err,omitempty"`
	Status      string      `json:"status"`
	Reasons     []string    `json:"reasons,omitempty"`
}

// AssertionResult is the evaluation of one RenderedAssertion.
type AssertionResult struct {
	ID          string     `json:"id"`
	Kind        AssertKind `json:"kind"`
	Query       string     `json:"query,omitempty"`
	Column      string     `json:"column,omitempty"`
	At          string     `json:"at,omitempty"`
	Description string     `json:"description"`
	Want        any        `json:"want,omitempty"`
	Observed    any        `json:"observed,omitempty"`
	Before      any        `json:"before,omitempty"`
	Status      string     `json:"status"`
	Reason      string     `json:"reason,omitempty"`
}

// AssertionsFile is written to assertions.json.
type AssertionsFile struct {
	Scenario   string            `json:"scenario"`
	Tasks      []TaskCheck       `json:"tasks"`
	Assertions []AssertionResult `json:"assertions"`
}

// ScenarioResult is written to result.json.
type ScenarioResult struct {
	Scenario         string            `json:"scenario"`
	Title            string            `json:"title"`
	Result           string            `json:"result"`
	Reasons          []string          `json:"reasons"`
	EvaluatedUTC     time.Time         `json:"evaluated_utc"`
	Converged        bool              `json:"converged"`
	TimedOut         bool              `json:"timed_out"`
	TaskIDs          []string          `json:"task_ids"`
	FinalStates      map[string]string `json:"final_states"`
	TasksPassed      int               `json:"tasks_passed"`
	TasksFailed      int               `json:"tasks_failed"`
	AssertionsPassed int               `json:"assertions_passed"`
	AssertionsFailed int               `json:"assertions_failed"`
	SQLSnapshots     []string          `json:"sql_snapshots"`
	MailSnapshots    []string          `json:"mail_snapshots,omitempty"`
	Observations     []string          `json:"observations,omitempty"`
	Timing           TimingFidelity    `json:"timing"`
}

// EvalInput is everything evaluateScenario needs; it is built from the
// scenario directory by bundleEvaluator.
type EvalInput struct {
	Record     *EnqueueRecord
	Final      *FinalTasks
	SQL        map[string]*SQLSnapshot  // by label
	Mail       map[string]*MailSnapshot // by label ("before", "after")
	Unmet      map[Requirement]string
	MaxRetry   int
	LoadErrors []string
	Now        time.Time
}

func pass() (string, string) { return resultPass, "" }
func fail(format string, a ...any) (string, string) {
	return resultFail, fmt.Sprintf(format, a...)
}

// evaluateTask compares one submission with its Expectation and the task's
// final observation.
func evaluateTask(sub Submission, final *FinalTasks) TaskCheck {
	tc := TaskCheck{N: sub.N, TaskID: sub.TaskID, Type: sub.Type, Outcome: sub.Outcome, Expectation: sub.Expectation, Status: resultPass}
	bad := func(format string, a ...any) { tc.Reasons = append(tc.Reasons, fmt.Sprintf(format, a...)) }
	exp := sub.Expectation
	switch sub.Outcome {
	case outcomeSkipped, outcomeExcluded:
		bad("not submitted: %s", sub.Reason)
	case outcomeNotSubmitted:
		bad("not submitted: %s", sub.Reason)
	case outcomeError:
		bad("enqueue error: %s", sub.Error)
	case outcomeConflict:
		if !exp.Conflict {
			bad("unexpected TaskID conflict: %s", sub.Error)
		}
	case outcomeEnqueued:
		if exp.Conflict {
			bad("expected ErrTaskIDConflict but the task was enqueued")
			break
		}
		ft, ok := final.Get(sub.TaskID)
		if !ok {
			bad("no final observation recorded for the task")
			break
		}
		tc.FinalState = ft.State
		if ft.Info == nil {
			bad("final state %s: %s", ft.State, ft.Error)
			break
		}
		retried, maxRetry := ft.Info.Retried, ft.Info.MaxRetry
		tc.Retried, tc.MaxRetry, tc.LastErr = &retried, &maxRetry, ft.Info.LastErr
		if !ft.Converged {
			bad("did not converge: final state %s", ft.State)
		}
		if len(exp.States) > 0 && !containsString(exp.States, ft.State) {
			bad("final state %s, expected one of %s", ft.State, strings.Join(exp.States, "/"))
		}
		if maxRetry != sub.Options.MaxRetry {
			bad("task MaxRetry %d differs from the enqueued MaxRetry(%d)", maxRetry, sub.Options.MaxRetry)
		}
		switch exp.Retried {
		case RetriedZero:
			if retried != 0 {
				bad("Retried %d, expected 0 (SkipRetry)", retried)
			}
		case RetriedMax:
			if retried != sub.Options.MaxRetry {
				bad("Retried %d, expected MaxRetry %d", retried, sub.Options.MaxRetry)
			}
		case RetriedAny, "":
		default:
			bad("unknown Retried expectation %q", exp.Retried)
		}
		if len(exp.LastErrAnyOf) > 0 && !containsFold(ft.Info.LastErr, exp.LastErrAnyOf) {
			bad("LastErr %q contains none of %q", ft.Info.LastErr, exp.LastErrAnyOf)
		}
	default:
		bad("unknown outcome %q", sub.Outcome)
	}
	if len(tc.Reasons) > 0 {
		tc.Status = resultFail
	}
	return tc
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func containsFold(s string, subs []string) bool {
	ls := strings.ToLower(s)
	for _, sub := range subs {
		if strings.Contains(ls, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

// singleValue returns Column of the query's only row in the snapshot.
func singleValue(snap *SQLSnapshot, label, query, column string) (any, error) {
	if snap == nil {
		return nil, fmt.Errorf("snapshot %s missing", sqlFileName(label))
	}
	q, ok := snap.Query(query)
	if !ok {
		return nil, fmt.Errorf("query %s missing from %s", query, sqlFileName(label))
	}
	if q.Error != "" {
		return nil, fmt.Errorf("query %s failed in %s: %s", query, sqlFileName(label), q.Error)
	}
	if len(q.Rows) != 1 {
		return nil, fmt.Errorf("query %s returned %d rows in %s, expected exactly 1", query, len(q.Rows), sqlFileName(label))
	}
	v, ok := q.Rows[0][column]
	if !ok {
		return nil, fmt.Errorf("query %s has no column %s", query, column)
	}
	return v, nil
}

// afterLabels returns the labels of the snapshots taken after phase 1
// (after-phase-<k> ascending, then after) that exist in sql.
func afterLabels(sql map[string]*SQLSnapshot) []string {
	type lk struct {
		label string
		k     int
	}
	var phases []lk
	for label := range sql {
		var k int
		if _, err := fmt.Sscanf(label, "after-phase-%d", &k); err == nil && label == fmt.Sprintf("after-phase-%d", k) {
			phases = append(phases, lk{label, k})
		}
	}
	sort.Slice(phases, func(i, j int) bool { return phases[i].k < phases[j].k })
	var out []string
	for _, p := range phases {
		out = append(out, p.label)
	}
	if _, ok := sql[snapAfter]; ok {
		out = append(out, snapAfter)
	}
	return out
}

// evaluateAssertion evaluates one assertion over the snapshots.
func evaluateAssertion(a RenderedAssertion, sql map[string]*SQLSnapshot, mail map[string]*MailSnapshot, unmet map[Requirement]string) AssertionResult {
	at := a.At
	if at == "" {
		at = snapAfter
	}
	r := AssertionResult{ID: a.ID, Kind: a.Kind, Query: a.Query, Column: a.Column, At: at, Description: a.Description, Want: a.Want}
	for _, req := range a.Requires {
		if reason, bad := unmet[req]; bad {
			r.Status, r.Reason = fail("FAIL-by-precondition: %s unmet: %s", req, reason)
			return r
		}
	}
	switch a.Kind {
	case AssertUnchanged:
		before, after := sql[snapBefore], sql[snapAfter]
		qb, okb := before.Query(a.Query)
		qa, oka := after.Query(a.Query)
		switch {
		case before == nil || after == nil:
			r.Status, r.Reason = fail("needs %s and %s", sqlFileName(snapBefore), sqlFileName(snapAfter))
		case !okb || !oka:
			r.Status, r.Reason = fail("query %s missing from a snapshot", a.Query)
		case qb.Error != "" || qa.Error != "":
			r.Status, r.Reason = fail("query %s failed: before=%q after=%q", a.Query, qb.Error, qa.Error)
		default:
			r.Before, r.Observed = qb.Rows, qa.Rows
			if canon(qb.Rows) == canon(qa.Rows) {
				r.Status, r.Reason = pass()
			} else {
				r.Status, r.Reason = fail("rows changed")
			}
		}
	case AssertEquals, AssertNotNull, AssertContains:
		v, err := singleValue(sql[at], at, a.Query, a.Column)
		if err != nil {
			r.Status, r.Reason = fail("%v", err)
			break
		}
		r.Observed = v
		switch a.Kind {
		case AssertEquals:
			if canon(v) == canon(a.Want) {
				r.Status, r.Reason = pass()
			} else {
				r.Status, r.Reason = fail("observed %s, want %s", canon(v), canon(a.Want))
			}
		case AssertNotNull:
			if v != nil {
				r.Status, r.Reason = pass()
			} else {
				r.Status, r.Reason = fail("value is NULL")
			}
		default:
			s, ok := v.(string)
			want := fmt.Sprint(a.Want)
			switch {
			case !ok:
				r.Status, r.Reason = fail("value %s is not text", canon(v))
			case strings.Contains(strings.ToLower(s), strings.ToLower(want)):
				r.Status, r.Reason = pass()
			default:
				r.Status, r.Reason = fail("%q does not contain %q", s, want)
			}
		}
	case AssertStableAfterFirst:
		labels := afterLabels(sql)
		if len(labels) == 0 {
			r.Status, r.Reason = fail("no snapshot taken after phase 1")
			break
		}
		seen := map[string]any{}
		var first string
		var errs []string
		stable := true
		for i, l := range labels {
			v, err := singleValue(sql[l], l, a.Query, a.Column)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			seen[l] = v
			if i == 0 || first == "" {
				first = canon(v)
			} else if canon(v) != first {
				stable = false
			}
		}
		r.Observed = seen
		switch {
		case len(errs) > 0:
			r.Status, r.Reason = fail("%s", strings.Join(errs, "; "))
		case !stable:
			r.Status, r.Reason = fail("value differs across %s", strings.Join(labels, ", "))
		default:
			r.Status, r.Reason = pass()
		}
	case AssertMailCount:
		r.Status, r.Reason, r.Observed = evaluateMailCount(a, mail[at], at)
	default:
		r.Status, r.Reason = fail("unknown assertion kind %q", a.Kind)
	}
	return r
}

// MailCountObservation is the Observed value of a mail_count assertion.
type MailCountObservation struct {
	Counted  int      `json:"counted"`
	Messages []string `json:"message_ids"`
}

func evaluateMailCount(a RenderedAssertion, snap *MailSnapshot, at string) (string, string, any) {
	if snap == nil {
		s, r := fail("mail-%s.json missing", at)
		return s, r, nil
	}
	if at == snapAfter && !snap.AfterConvergence {
		s, r := fail("mail-after.json was not taken after convergence")
		return s, r, nil
	}
	var q *MailQuery
	for i := range snap.Queries {
		if strings.EqualFold(snap.Queries[i].Recipient, a.Recipient) {
			q = &snap.Queries[i]
			break
		}
	}
	if q == nil {
		s, r := fail("recipient %s not queried in mail-%s.json", a.Recipient, at)
		return s, r, nil
	}
	if q.Error != "" {
		s, r := fail("mail query for %s failed: %s", a.Recipient, q.Error)
		return s, r, nil
	}
	var since time.Time
	if a.Since != "" {
		t, err := time.Parse(time.RFC3339Nano, a.Since)
		if err != nil {
			s, r := fail("since %q is not RFC3339: %v", a.Since, err)
			return s, r, nil
		}
		since = t
	}
	obs := MailCountObservation{Messages: []string{}}
	var problems []string
	for _, m := range q.Messages {
		if !since.IsZero() {
			created, err := time.Parse(time.RFC3339Nano, m.Created)
			if err != nil {
				problems = append(problems, fmt.Sprintf("message %s has unparseable created %q", m.ID, m.Created))
				continue
			}
			if created.Before(since) {
				continue
			}
		}
		obs.Counted++
		obs.Messages = append(obs.Messages, m.ID)
		if a.Attachments != nil && m.Attachments != *a.Attachments {
			problems = append(problems, fmt.Sprintf("message %s has %d attachments, want %d", m.ID, m.Attachments, *a.Attachments))
		}
		if a.SubjectContains != "" && !strings.Contains(m.Subject, a.SubjectContains) {
			problems = append(problems, fmt.Sprintf("message %s subject %q does not contain %q", m.ID, m.Subject, a.SubjectContains))
		}
	}
	if want := canon(a.Want); canon(obs.Counted) != want {
		problems = append([]string{fmt.Sprintf("%d messages to %s, want %s", obs.Counted, a.Recipient, want)}, problems...)
	}
	if len(problems) > 0 {
		s, r := fail("%s", strings.Join(problems, "; "))
		return s, r, obs
	}
	s, r := pass()
	return s, r, obs
}

// evaluateScenario is the pure scenario verdict: PASS only when the scenario
// ran, every phase converged, every submission met its expectation, every
// assertion passed and nothing failed to load. A scenario excluded by an
// unmet scenario-level requirement is EXCLUDED (never PASS).
func evaluateScenario(in EvalInput) (ScenarioResult, AssertionsFile) {
	rec := in.Record
	res := ScenarioResult{
		Scenario: rec.Scenario, Title: rec.Title, EvaluatedUTC: in.Now, Converged: rec.Converged,
		Reasons: []string{}, TaskIDs: []string{}, FinalStates: map[string]string{}, SQLSnapshots: []string{},
		Observations: rec.Plan.Observations, Timing: timingFidelity(in.MaxRetry),
	}
	af := AssertionsFile{Scenario: rec.Scenario, Tasks: []TaskCheck{}, Assertions: []AssertionResult{}}
	for label := range in.SQL {
		res.SQLSnapshots = append(res.SQLSnapshots, sqlFileName(label))
	}
	sort.Strings(res.SQLSnapshots)
	for label := range in.Mail {
		res.MailSnapshots = append(res.MailSnapshots, "mail-"+label+".json")
	}
	sort.Strings(res.MailSnapshots)
	if in.Final != nil {
		res.TimedOut = in.Final.TimedOut
		for _, t := range in.Final.Tasks {
			res.FinalStates[t.TaskID] = t.State
		}
	}
	seen := map[string]bool{}
	for _, s := range rec.Submissions {
		if !seen[s.TaskID] {
			seen[s.TaskID] = true
			res.TaskIDs = append(res.TaskIDs, s.TaskID)
		}
	}

	if !rec.Enabled {
		res.Result = resultExcluded
		res.Reasons = append(res.Reasons, "excluded: "+rec.Excluded)
		return res, af
	}

	res.Reasons = append(res.Reasons, rec.Errors...)
	res.Reasons = append(res.Reasons, in.LoadErrors...)
	if !rec.Converged {
		res.Reasons = append(res.Reasons, "scenario did not converge (see final-tasks.json and timeline.jsonl)")
	}
	if in.Final != nil && in.Final.TimedOut {
		res.Reasons = append(res.Reasons, "convergence timeout: "+in.Final.Error)
	}
	for _, s := range rec.Submissions {
		tc := evaluateTask(s, in.Final)
		af.Tasks = append(af.Tasks, tc)
		if tc.Status == resultPass {
			res.TasksPassed++
		} else {
			res.TasksFailed++
			res.Reasons = append(res.Reasons, fmt.Sprintf("task %d (%s): %s", tc.N, tc.TaskID, strings.Join(tc.Reasons, "; ")))
		}
	}
	for _, a := range rec.Plan.Assertions {
		ar := evaluateAssertion(a, in.SQL, in.Mail, in.Unmet)
		af.Assertions = append(af.Assertions, ar)
		if ar.Status == resultPass {
			res.AssertionsPassed++
		} else {
			res.AssertionsFailed++
			res.Reasons = append(res.Reasons, fmt.Sprintf("assertion %s: %s", ar.ID, ar.Reason))
		}
	}
	res.Result = resultPass
	if len(res.Reasons) > 0 {
		res.Result = resultFail
	}
	return res, af
}

// ---- evaluator (I/O around evaluateScenario) -----------------------------

// ScenarioEvaluator turns a finished scenario directory into a verdict.
type ScenarioEvaluator interface {
	Evaluate(ctx context.Context, rec *EnqueueRecord, dir string) (*ScenarioResult, error)
}

// bundleEvaluator reads the scenario directory and writes assertions.json and
// result.json.
type bundleEvaluator struct {
	MaxRetry int
	Unmet    map[Requirement]string
	Now      func() time.Time
}

func (b *bundleEvaluator) Evaluate(_ context.Context, rec *EnqueueRecord, dir string) (*ScenarioResult, error) {
	now := time.Now
	if b.Now != nil {
		now = b.Now
	}
	in := EvalInput{Record: rec, SQL: map[string]*SQLSnapshot{}, Mail: map[string]*MailSnapshot{}, Unmet: b.Unmet, MaxRetry: b.MaxRetry, Now: now().UTC()}
	var err error
	if in.Final, err = readFinalTasks(dir); err != nil {
		in.LoadErrors = append(in.LoadErrors, err.Error())
	}
	files, err := filepath.Glob(filepath.Join(dir, "sql-*.json"))
	if err != nil {
		in.LoadErrors = append(in.LoadErrors, err.Error())
	}
	for _, f := range files {
		var s SQLSnapshot
		if _, err := readJSONFile(f, &s); err != nil {
			in.LoadErrors = append(in.LoadErrors, err.Error())
			continue
		}
		label := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "sql-"), ".json")
		if s.Label != label {
			in.LoadErrors = append(in.LoadErrors, fmt.Sprintf("%s holds label %q", filepath.Base(f), s.Label))
			continue
		}
		in.SQL[label] = &s
	}
	for _, label := range []string{snapBefore, snapAfter} {
		var m MailSnapshot
		ok, err := readJSONFile(filepath.Join(dir, "mail-"+label+".json"), &m)
		if err != nil {
			in.LoadErrors = append(in.LoadErrors, err.Error())
		}
		if ok && err == nil {
			in.Mail[label] = &m
		}
	}
	res, af := evaluateScenario(in)
	werr := errors.Join(
		writeJSONFile(filepath.Join(dir, assertionsFileName), af),
		writeJSONFile(filepath.Join(dir, resultFileName), res),
	)
	if werr != nil {
		res.Result = resultFail
		res.Reasons = append(res.Reasons, werr.Error())
	}
	return &res, werr
}

// newObservers wires the Step 4 observers of a real run: the poller over the
// run's inspector, the SQL observer over the read-only pool and the bundle
// evaluator.
func newObservers(rc *RunContext, insp TaskInspector, db txBeginner) Observers {
	var unmet map[Requirement]string
	if rc.Preflight != nil {
		unmet = rc.Preflight.Unmet
	}
	return Observers{
		Wait: &Poller{Inspect: insp, Poll: rc.Cfg.Poll, Timeout: rc.Cfg.Timeout},
		SQL:  &sqlObserver{DB: db},
		Eval: &bundleEvaluator{MaxRetry: rc.Cfg.MaxRetry, Unmet: unmet},
	}
}
