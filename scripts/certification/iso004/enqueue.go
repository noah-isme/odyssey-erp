package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/platform/cache"
)

// exitNotEvaluated: tasks were enqueued and recorded, but the executor had no
// convergence waiter, SQL observer or evaluator, so no scenario was
// evaluated. A real run always has them (observerFactory); the code remains
// for executors built without observers (tests).
const exitNotEvaluated = 4

// Outcomes of one submission in enqueue.json.
const (
	outcomeEnqueued     = "enqueued"
	outcomeConflict     = "conflict"      // asynq.ErrTaskIDConflict, captured as data
	outcomeError        = "error"         // any other enqueue error
	outcomeSkipped      = "skipped"       // task-level requirement unmet (FAIL-by-precondition)
	outcomeExcluded     = "excluded"      // scenario-level requirement unmet
	outcomeNotSubmitted = "not_submitted" // an earlier phase could not be awaited, or the scenario aborted
)

// Enqueuer submits tasks; *asynq.Client satisfies it.
type Enqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// TaskInspector reads a task's current state; *asynq.Inspector satisfies it.
// The executor uses it only to record the task that blocked a TaskID.
type TaskInspector interface {
	GetTaskInfo(queue, id string) (*asynq.TaskInfo, error)
}

// ConvergenceWaiter blocks until every given task is completed or archived
// (Step 4, observe.go). It returns an error on --timeout; the executor then
// stops submitting later phases of the scenario.
type ConvergenceWaiter interface {
	WaitConverged(ctx context.Context, sp *ScenarioPlan, scenarioDir string, taskIDs []string) error
}

// SQLObserver runs the scenario's read-only snapshot queries (Step 4,
// sqlobs.go) under a label: "before", "after-phase-<k>" for intermediate
// phases, and "after" once the last phase converged.
type SQLObserver interface {
	Snapshot(ctx context.Context, sp *ScenarioPlan, scenarioDir, label string) error
}

// Observers are the Step 4 components of a real run.
type Observers struct {
	Wait ConvergenceWaiter
	SQL  SQLObserver
	Eval ScenarioEvaluator
}

// observerFactory supplies the observers of a real run (observe.go,
// sqlobs.go). Nil means enqueue-only: the run exits with exitNotEvaluated.
var observerFactory func(rc *RunContext, insp *asynq.Inspector, db *pgxpool.Pool) Observers

func init() {
	runExecutor = executeRun
	observerFactory = func(rc *RunContext, insp *asynq.Inspector, db *pgxpool.Pool) Observers {
		return newObservers(rc, insp, db)
	}
}

// EnqueueOptions are the asynq options applied to one submission.
type EnqueueOptions struct {
	Queue            string `json:"queue"`
	MaxRetry         int    `json:"max_retry"`
	Timeout          string `json:"timeout"`
	TimeoutSeconds   int64  `json:"timeout_seconds"`
	Retention        string `json:"retention"`
	RetentionSeconds int64  `json:"retention_seconds"`
	TaskID           string `json:"task_id"`
}

// TaskInfoRecord is the JSON form of asynq.TaskInfo.
type TaskInfoRecord struct {
	ID               string     `json:"id"`
	Queue            string     `json:"queue"`
	Type             string     `json:"type"`
	Payload          string     `json:"payload"`
	PayloadBase64    string     `json:"payload_base64"`
	State            string     `json:"state"`
	MaxRetry         int        `json:"max_retry"`
	Retried          int        `json:"retried"`
	LastErr          string     `json:"last_err,omitempty"`
	LastFailedAt     *time.Time `json:"last_failed_at,omitempty"`
	Timeout          string     `json:"timeout"`
	TimeoutSeconds   int64      `json:"timeout_seconds"`
	Deadline         *time.Time `json:"deadline,omitempty"`
	Group            string     `json:"group,omitempty"`
	NextProcessAt    *time.Time `json:"next_process_at,omitempty"`
	IsOrphaned       bool       `json:"is_orphaned"`
	Retention        string     `json:"retention"`
	RetentionSeconds int64      `json:"retention_seconds"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	ResultBase64     string     `json:"result_base64,omitempty"`
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

// taskStateString is asynq.TaskState.String without its panic on unknown
// values (a recorder must never crash on unexpected data).
func taskStateString(s asynq.TaskState) string {
	switch s {
	case asynq.TaskStateActive, asynq.TaskStatePending, asynq.TaskStateScheduled, asynq.TaskStateRetry,
		asynq.TaskStateArchived, asynq.TaskStateCompleted, asynq.TaskStateAggregating:
		return s.String()
	}
	return fmt.Sprintf("unknown(%d)", int(s))
}

func taskInfoRecord(ti *asynq.TaskInfo) *TaskInfoRecord {
	if ti == nil {
		return nil
	}
	r := &TaskInfoRecord{
		ID: ti.ID, Queue: ti.Queue, Type: ti.Type,
		Payload: string(ti.Payload), PayloadBase64: base64.StdEncoding.EncodeToString(ti.Payload),
		State: taskStateString(ti.State), MaxRetry: ti.MaxRetry, Retried: ti.Retried, LastErr: ti.LastErr,
		LastFailedAt: timePtr(ti.LastFailedAt), Timeout: ti.Timeout.String(), TimeoutSeconds: int64(ti.Timeout / time.Second),
		Deadline: timePtr(ti.Deadline), Group: ti.Group, NextProcessAt: timePtr(ti.NextProcessAt), IsOrphaned: ti.IsOrphaned,
		Retention: ti.Retention.String(), RetentionSeconds: int64(ti.Retention / time.Second), CompletedAt: timePtr(ti.CompletedAt),
	}
	if len(ti.Result) > 0 {
		r.ResultBase64 = base64.StdEncoding.EncodeToString(ti.Result)
	}
	return r
}

// Submission is one entry of enqueue.json.
type Submission struct {
	N              int            `json:"n"`
	Phase          int            `json:"phase"`
	Concurrent     bool           `json:"concurrent"`
	Type           string         `json:"type"`
	TaskID         string         `json:"task_id"`
	ReusesTaskIDOf int            `json:"reuses_task_id_of,omitempty"`
	Note           string         `json:"note,omitempty"`
	Expectation    Expectation    `json:"expectation"`
	Payload        string         `json:"payload"`
	PayloadBase64  string         `json:"payload_base64"`
	PayloadSHA256  string         `json:"payload_sha256"`
	Options        EnqueueOptions `json:"options"`
	OptionStrings  []string       `json:"option_strings"`
	Outcome        string         `json:"outcome"`
	// Unexpected is true when the outcome contradicts the expectation: an
	// enqueue error, a conflict that was not expected, or an expected
	// conflict that did not happen.
	Unexpected      bool            `json:"unexpected"`
	Error           string          `json:"error,omitempty"`
	Reason          string          `json:"reason,omitempty"`
	SubmittedUTC    *time.Time      `json:"submitted_utc,omitempty"`
	ReturnedUTC     *time.Time      `json:"returned_utc,omitempty"`
	TaskInfo        *TaskInfoRecord `json:"task_info"`
	ConflictingTask *TaskInfoRecord `json:"conflicting_task,omitempty"`
}

// PhaseRecord records one phase boundary.
type PhaseRecord struct {
	Phase     int        `json:"phase"`
	TaskIDs   []string   `json:"awaited_task_ids"`
	Waited    bool       `json:"waited"`
	WaitError string     `json:"wait_error,omitempty"`
	SQLLabel  string     `json:"sql_label,omitempty"`
	SQLError  string     `json:"sql_error,omitempty"`
	EndedUTC  *time.Time `json:"ended_utc,omitempty"`
}

// EnqueueRecord is written to scenarios/<id>/enqueue.json.
type EnqueueRecord struct {
	Scenario      string                  `json:"scenario"`
	Title         string                  `json:"title"`
	RunID         string                  `json:"run_id"`
	TaskIDPrefix  string                  `json:"task_id_prefix"`
	Enabled       bool                    `json:"enabled"`
	Excluded      string                  `json:"excluded_reason,omitempty"`
	StartedUTC    time.Time               `json:"started_utc"`
	FinishedUTC   time.Time               `json:"finished_utc"`
	DynamicValues map[string]DynamicValue `json:"dynamic_values"`
	Submissions   []Submission            `json:"submissions"`
	Phases        []PhaseRecord           `json:"phases"`
	MailBefore    string                  `json:"mail_before,omitempty"`
	MailAfter     string                  `json:"mail_after,omitempty"`
	// Converged is true when every phase was awaited by a ConvergenceWaiter.
	Converged bool `json:"converged"`
	// Result is the evaluator's verdict (result.json); not part of
	// enqueue.json.
	Result *ScenarioResult `json:"-"`
	// NotEvaluated explains why convergence/SQL were not observed.
	NotEvaluated string   `json:"not_evaluated,omitempty"`
	Errors       []string `json:"errors"`
	// Plan is the rendered scenario (queries, assertions, recipients) Step 4
	// evaluates.
	Plan ScenarioPlan `json:"plan"`
}

// unexpected reports whether any submission contradicted its expectation
// or the scenario recorded an error.
func (r *EnqueueRecord) unexpected() bool {
	if len(r.Errors) > 0 {
		return true
	}
	for _, s := range r.Submissions {
		if s.Unexpected {
			return true
		}
	}
	return false
}

// Executor enqueues the selected scenarios and records every submission.
type Executor struct {
	Cfg      *Config
	Fixtures *Fixtures
	Unmet    map[Requirement]string
	OutDir   string
	Enqueue  Enqueuer
	Inspect  TaskInspector // optional
	DB       queryRower    // read-only; resolves dynamic values
	Mail     MailRecorder  // required by scenarios with mail recipients
	Wait     ConvergenceWaiter
	SQL      SQLObserver
	Eval     ScenarioEvaluator
	Now      func() time.Time
	Log      io.Writer
}

func (e *Executor) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

func (e *Executor) logf(format string, args ...any) {
	if e.Log != nil {
		fmt.Fprintf(e.Log, format+"\n", args...)
	}
}

// Run executes every selected scenario in registry order and, with an
// evaluator, writes each scenario's assertions.json and result.json. It
// returns the records and the exit code:
//   - exitOK (0): every selected scenario evaluated PASS;
//   - exitFail (1): any scenario FAIL or EXCLUDED (an excluded scenario is
//     never a pass), any unexpected submission, or an output error;
//   - exitNotEvaluated (4): no waiter, SQL observer or evaluator was wired
//     (enqueue-only executor) and nothing else failed.
//
// Enqueue errors are data, never a crash.
func (e *Executor) Run(ctx context.Context) ([]*EnqueueRecord, int, error) {
	var records []*EnqueueRecord
	var errs []error
	for _, s := range selectedScenarios(e.Cfg) {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		rec := e.runScenario(ctx, s)
		if err := writeJSONFile(filepath.Join(e.scenarioDir(s.ID), "enqueue.json"), rec); err != nil {
			errs = append(errs, err)
		}
		if e.Eval != nil {
			res, err := e.Eval.Evaluate(ctx, rec, e.scenarioDir(s.ID))
			if err != nil {
				errs = append(errs, fmt.Errorf("evaluate %s: %w", s.ID, err))
			}
			rec.Result = res
			if res != nil {
				e.logf("[%s] %s %s", s.ID, res.Result, strings.Join(res.Reasons, "; "))
			}
		}
		records = append(records, rec)
	}
	code := exitOK
	if e.Wait == nil || e.SQL == nil || e.Eval == nil {
		code = exitNotEvaluated
	}
	for _, r := range records {
		if r.unexpected() {
			code = exitFail
		}
		if e.Eval != nil && (r.Result == nil || r.Result.Result != resultPass) {
			code = exitFail
		}
	}
	if len(errs) > 0 {
		code = exitFail
	}
	return records, code, errors.Join(errs...)
}

func (e *Executor) scenarioDir(id string) string {
	return filepath.Join(e.OutDir, "scenarios", id)
}

func (e *Executor) taskOptions(taskID string) ([]asynq.Option, EnqueueOptions) {
	opts := []asynq.Option{
		asynq.Queue(workerQueue),
		asynq.MaxRetry(e.Cfg.MaxRetry),
		asynq.Timeout(taskTimeout),
		asynq.Retention(taskRetention),
		asynq.TaskID(taskID),
	}
	return opts, EnqueueOptions{
		Queue: workerQueue, MaxRetry: e.Cfg.MaxRetry,
		Timeout: taskTimeout.String(), TimeoutSeconds: int64(taskTimeout / time.Second),
		Retention: taskRetention.String(), RetentionSeconds: int64(taskRetention / time.Second),
		TaskID: taskID,
	}
}

func (e *Executor) newSubmission(t PlannedTask) Submission {
	sum := sha256.Sum256([]byte(t.Payload))
	opts, eo := e.taskOptions(t.TaskID)
	strs := make([]string, len(opts))
	for i, o := range opts {
		strs[i] = o.String()
	}
	return Submission{
		N: t.N, Phase: t.Phase, Concurrent: t.Concurrent, Type: t.Type, TaskID: t.TaskID,
		ReusesTaskIDOf: t.ReusesTaskIDOf, Note: t.Note, Expectation: t.Expectation,
		Payload: t.Payload, PayloadBase64: base64.StdEncoding.EncodeToString([]byte(t.Payload)), PayloadSHA256: hex.EncodeToString(sum[:]),
		Options: eo, OptionStrings: strs,
	}
}

// unmetFor returns the reason of the first unmet requirement, or "".
func (e *Executor) unmetFor(reqs []Requirement) string {
	for _, r := range reqs {
		if reason, bad := e.Unmet[r]; bad {
			return fmt.Sprintf("FAIL-by-precondition: %s unmet: %s", r, reason)
		}
	}
	return ""
}

func (e *Executor) runScenario(ctx context.Context, s Scenario) *EnqueueRecord {
	rec := &EnqueueRecord{
		Scenario: s.ID, Title: s.Title, RunID: e.Cfg.RunID, TaskIDPrefix: e.Cfg.TaskIDPrefix(),
		Enabled: true, StartedUTC: e.now(), DynamicValues: map[string]DynamicValue{},
		Submissions: []Submission{}, Phases: []PhaseRecord{}, Errors: []string{},
	}
	defer func() { rec.FinishedUTC = e.now() }()
	dir := e.scenarioDir(s.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		rec.Errors = append(rec.Errors, fmt.Sprintf("create %s: %v", dir, err))
		return rec
	}

	if reason := e.unmetFor(s.Requires); reason != "" {
		rec.Enabled = false
		rec.Excluded = strings.TrimPrefix(reason, "FAIL-by-precondition: ")
		rec.Plan = planScenario(renderEnv{Cfg: e.Cfg, Fixtures: e.Fixtures}, s, e.Unmet)
		for _, t := range rec.Plan.Tasks {
			sub := e.newSubmission(t)
			sub.Outcome, sub.Reason = outcomeExcluded, rec.Excluded
			rec.Submissions = append(rec.Submissions, sub)
		}
		e.logf("[%s] excluded: %s", s.ID, rec.Excluded)
		return rec
	}

	dynamic, err := resolveDynamics(ctx, e.DB, s.dynamicRefs())
	rec.DynamicValues = dynamic
	if err != nil {
		rec.Errors = append(rec.Errors, err.Error())
		rec.Plan = planScenario(renderEnv{Cfg: e.Cfg, Fixtures: e.Fixtures}, s, e.Unmet)
		e.abortAll(rec, "dynamic values could not be resolved")
		return rec
	}
	sp := planScenario(renderEnv{Cfg: e.Cfg, Fixtures: e.Fixtures, Dynamic: dynamic}, s, e.Unmet)
	rec.Plan = sp
	if sp.RenderError != "" {
		rec.Errors = append(rec.Errors, sp.RenderError)
		e.abortAll(rec, "scenario did not render")
		return rec
	}

	if len(sp.MailRecipients) > 0 {
		if e.Mail == nil {
			rec.Errors = append(rec.Errors, "scenario has mail recipients but no mail recorder is configured")
			e.abortAll(rec, "no mail recorder")
			return rec
		}
		rec.MailBefore = "mail-before.json"
		if err := e.mailSnapshot(ctx, dir, snapBefore, sp.MailRecipients, false); err != nil {
			rec.Errors = append(rec.Errors, err.Error())
			e.abortAll(rec, "mail-before snapshot failed; email scenario not submitted")
			return rec
		}
	}

	if e.SQL != nil {
		if err := e.SQL.Snapshot(ctx, &sp, dir, snapBefore); err != nil {
			rec.Errors = append(rec.Errors, "sql before: "+err.Error())
			e.abortAll(rec, "sql-before snapshot failed")
			return rec
		}
	}

	phases := phaseNumbers(sp.Tasks)
	var enqueued []string
	allWaited := true
	for i, ph := range phases {
		if err := ctx.Err(); err != nil {
			rec.Errors = append(rec.Errors, err.Error())
			allWaited = false
			e.markRemaining(rec, sp.Tasks, ph, "run cancelled")
			break
		}
		for _, sub := range e.submitPhase(ctx, sp.Tasks, ph) {
			rec.Submissions = append(rec.Submissions, sub)
			if sub.Outcome == outcomeEnqueued {
				enqueued = append(enqueued, sub.TaskID)
			}
		}
		last := i == len(phases)-1
		pr := PhaseRecord{Phase: ph, TaskIDs: append([]string(nil), enqueued...)}
		if e.Wait == nil {
			allWaited = false
			rec.NotEvaluated = "no convergence waiter in this build (Step 4); later phases are not submitted"
			if !last {
				pr.WaitError = rec.NotEvaluated
				pr.EndedUTC = timePtr(e.now())
				rec.Phases = append(rec.Phases, pr)
				e.markRemaining(rec, sp.Tasks, phases[i+1], "phase "+fmt.Sprint(ph)+" was not awaited: no convergence waiter in this build")
				break
			}
		} else if err := e.Wait.WaitConverged(ctx, &sp, dir, pr.TaskIDs); err != nil {
			allWaited = false
			pr.WaitError = err.Error()
			rec.Errors = append(rec.Errors, fmt.Sprintf("phase %d: %v", ph, err))
			pr.EndedUTC = timePtr(e.now())
			rec.Phases = append(rec.Phases, pr)
			if !last {
				e.markRemaining(rec, sp.Tasks, phases[i+1], fmt.Sprintf("phase %d did not converge", ph))
			}
			break
		} else {
			pr.Waited = true
		}
		if e.SQL != nil {
			pr.SQLLabel = snapAfter
			if !last {
				pr.SQLLabel = fmt.Sprintf("after-phase-%d", ph)
			}
			if err := e.SQL.Snapshot(ctx, &sp, dir, pr.SQLLabel); err != nil {
				pr.SQLError = err.Error()
				rec.Errors = append(rec.Errors, fmt.Sprintf("sql %s: %v", pr.SQLLabel, err))
			}
		}
		pr.EndedUTC = timePtr(e.now())
		rec.Phases = append(rec.Phases, pr)
	}
	rec.Converged = allWaited && e.Wait != nil
	if e.SQL == nil && rec.NotEvaluated == "" {
		rec.NotEvaluated = "no SQL observer in this build (Step 4)"
	}

	if len(sp.MailRecipients) > 0 {
		rec.MailAfter = "mail-after.json"
		if err := e.mailSnapshot(ctx, dir, snapAfter, sp.MailRecipients, rec.Converged); err != nil {
			rec.Errors = append(rec.Errors, err.Error())
		}
	}
	sort.SliceStable(rec.Submissions, func(i, j int) bool { return rec.Submissions[i].N < rec.Submissions[j].N })
	e.logf("[%s] %s", s.ID, submissionSummary(rec))
	return rec
}

func submissionSummary(rec *EnqueueRecord) string {
	counts := map[string]int{}
	unexpected := 0
	for _, s := range rec.Submissions {
		counts[s.Outcome]++
		if s.Unexpected {
			unexpected++
		}
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)+1)
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, counts[k]))
	}
	parts = append(parts, fmt.Sprintf("unexpected=%d", unexpected))
	if len(rec.Errors) > 0 {
		parts = append(parts, "errors: "+strings.Join(rec.Errors, "; "))
	}
	return strings.Join(parts, " ")
}

// abortAll records every planned task as not submitted.
func (e *Executor) abortAll(rec *EnqueueRecord, reason string) {
	for _, t := range rec.Plan.Tasks {
		sub := e.newSubmission(t)
		sub.Outcome, sub.Reason = outcomeNotSubmitted, reason
		rec.Submissions = append(rec.Submissions, sub)
	}
	e.logf("[%s] aborted: %s (%s)", rec.Scenario, reason, strings.Join(rec.Errors, "; "))
}

// markRemaining records the tasks of phases >= from as not submitted.
func (e *Executor) markRemaining(rec *EnqueueRecord, tasks []PlannedTask, from int, reason string) {
	for _, t := range tasks {
		if t.Phase >= from {
			sub := e.newSubmission(t)
			sub.Outcome, sub.Reason = outcomeNotSubmitted, reason
			rec.Submissions = append(rec.Submissions, sub)
		}
	}
}

func phaseNumbers(tasks []PlannedTask) []int {
	seen := map[int]bool{}
	var out []int
	for _, t := range tasks {
		if !seen[t.Phase] {
			seen[t.Phase] = true
			out = append(out, t.Phase)
		}
	}
	sort.Ints(out)
	return out
}

// submitPhase submits one phase: the concurrent group is released together
// from a barrier, then the sequential tasks follow in N order.
func (e *Executor) submitPhase(ctx context.Context, tasks []PlannedTask, phase int) []Submission {
	var concurrent, sequential []PlannedTask
	var out []Submission
	for _, t := range tasks {
		if t.Phase != phase {
			continue
		}
		if reason := e.unmetFor(t.Requires); reason != "" {
			sub := e.newSubmission(t)
			sub.Outcome, sub.Reason = outcomeSkipped, reason
			out = append(out, sub)
			continue
		}
		if t.Concurrent {
			concurrent = append(concurrent, t)
		} else {
			sequential = append(sequential, t)
		}
	}
	if len(concurrent) > 0 {
		results := make([]Submission, len(concurrent))
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i, t := range concurrent {
			wg.Add(1)
			go func(i int, t PlannedTask) {
				defer wg.Done()
				<-start
				results[i] = e.submit(ctx, t)
			}(i, t)
		}
		close(start)
		wg.Wait()
		out = append(out, results...)
	}
	for _, t := range sequential {
		out = append(out, e.submit(ctx, t))
	}
	return out
}

// submit enqueues one task and classifies the outcome against its
// expectation. ErrTaskIDConflict is recorded as data.
func (e *Executor) submit(ctx context.Context, t PlannedTask) Submission {
	sub := e.newSubmission(t)
	opts, _ := e.taskOptions(t.TaskID)
	sub.SubmittedUTC = timePtr(e.now())
	info, err := e.Enqueue.EnqueueContext(ctx, asynq.NewTask(t.Type, []byte(t.Payload)), opts...)
	sub.ReturnedUTC = timePtr(e.now())
	sub.TaskInfo = taskInfoRecord(info)
	switch {
	case err == nil:
		sub.Outcome = outcomeEnqueued
		if t.Expectation.Conflict {
			sub.Unexpected = true
			sub.Reason = "expected ErrTaskIDConflict but the task was enqueued"
		}
	case errors.Is(err, asynq.ErrTaskIDConflict):
		sub.Outcome, sub.Error = outcomeConflict, err.Error()
		if !t.Expectation.Conflict {
			sub.Unexpected = true
			sub.Reason = "unexpected TaskID conflict"
		}
		if e.Inspect != nil {
			if existing, ierr := e.Inspect.GetTaskInfo(workerQueue, t.TaskID); ierr == nil {
				sub.ConflictingTask = taskInfoRecord(existing)
			} else {
				sub.Reason = strings.TrimPrefix(sub.Reason+"; conflicting task not readable: "+ierr.Error(), "; ")
			}
		}
	default:
		sub.Outcome, sub.Error, sub.Unexpected = outcomeError, err.Error(), true
	}
	return sub
}

func (e *Executor) mailSnapshot(ctx context.Context, dir, label string, recipients []string, converged bool) error {
	snap, err := e.Mail.Snapshot(ctx, recipients)
	if snap == nil {
		snap = &MailSnapshot{FetchedUTC: e.now(), Queries: []MailQuery{}}
	}
	snap.Label = label
	snap.AfterConvergence = converged
	name := "mail-" + label + ".json"
	if werr := writeJSONFile(filepath.Join(dir, name), snap); werr != nil {
		return werr
	}
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// resolveDynamics runs the read-only SELECT of every named dynamic value.
func resolveDynamics(ctx context.Context, db queryRower, names []string) (map[string]DynamicValue, error) {
	out := map[string]DynamicValue{}
	if len(names) == 0 {
		return out, nil
	}
	if db == nil {
		return out, fmt.Errorf("dynamic values %s need a database connection", strings.Join(names, ", "))
	}
	for _, name := range names {
		spec, ok := dynamicSpec(name)
		if !ok {
			return out, fmt.Errorf("unknown dynamic value %q", name)
		}
		var v any
		switch spec.Kind {
		case "int":
			var n int64
			if err := db.QueryRow(ctx, spec.SQL).Scan(&n); err != nil {
				return out, fmt.Errorf("resolve %s: %w", name, err)
			}
			v = n
		case "text":
			var s string
			if err := db.QueryRow(ctx, spec.SQL).Scan(&s); err != nil {
				return out, fmt.Errorf("resolve %s: %w", name, err)
			}
			v = s
		default:
			return out, fmt.Errorf("dynamic value %s has unknown kind %q", name, spec.Kind)
		}
		out[name] = DynamicValue{SQL: spec.SQL, Value: v}
	}
	return out, nil
}

func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// executeRun is the runExecutor of a real run: it connects to Redis and the
// read-only pool, wires the mail recorder when the email gate passed and the
// Step 4 observers when registered, and runs the executor.
func executeRun(ctx context.Context, rc *RunContext) (int, error) {
	cfg := rc.Cfg
	opt, err := cache.AsynqOptions(cfg.RedisAddr)
	if err != nil {
		return exitFail, fmt.Errorf("parse redis address %s: %w", redactAddr(cfg.RedisAddr), err)
	}
	client := asynq.NewClient(opt)
	defer client.Close()
	insp := asynq.NewInspector(opt)
	defer insp.Close()

	open := rc.OpenDB
	if open == nil {
		open = openReadOnlyPool
	}
	pool, err := open(ctx, cfg.DSN)
	if err != nil {
		return exitFail, fmt.Errorf("open read-only pool: %w", err)
	}
	defer pool.Close()

	ex := &Executor{
		Cfg: cfg, Fixtures: rc.Preflight.Fixtures, Unmet: rc.Preflight.Unmet, OutDir: cfg.OutDir,
		Enqueue: client, Inspect: insp, DB: pool, Log: rc.Stderr,
	}
	if g := rc.Preflight.EmailGate; g.Enabled && g.Probe != nil {
		httpClient := rc.HTTP
		if httpClient == nil {
			httpClient = &http.Client{Timeout: 30 * time.Second}
		}
		ex.Mail = &mailAPIRecorder{Base: cfg.MailAPI, Sink: g.Probe.APISink, HTTP: httpClient}
	}
	if observerFactory != nil {
		obs := observerFactory(rc, insp, pool)
		ex.Wait, ex.SQL, ex.Eval = obs.Wait, obs.SQL, obs.Eval
	}
	records, code, err := ex.Run(ctx)
	if code == exitNotEvaluated {
		fmt.Fprintln(rc.Stderr, "iso004: tasks enqueued and recorded under scenarios/<id>/enqueue.json; this build has no convergence poller or SQL observer, so no scenario was evaluated")
	}
	if ex.Eval != nil {
		printResults(rc.Stderr, records, cfg.MaxRetry)
	}
	return code, err
}

// printResults prints one line per evaluated scenario.
func printResults(w io.Writer, records []*EnqueueRecord, maxRetry int) {
	fmt.Fprintln(w, "ISO-004 scenario results:")
	for _, r := range records {
		result := "NOT EVALUATED"
		if r.Result != nil {
			result = r.Result.Result
		}
		fmt.Fprintf(w, "  %-34s %s\n", r.Scenario, result)
	}
	fmt.Fprintf(w, "  timing: %s\n", timingFidelity(maxRetry).Note)
}
