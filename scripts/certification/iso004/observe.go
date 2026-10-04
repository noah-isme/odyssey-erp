package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hibiken/asynq"
)

// Convergence poller (plan Step 4). It polls Inspector.GetTaskInfo for every
// task of a scenario phase, appends each distinct observation to
// scenarios/<id>/timeline.jsonl and writes the latest state of every task to
// scenarios/<id>/final-tasks.json. It returns only when every task is
// completed or archived, or with an error on --timeout / cancellation; the
// executor then records the scenario as not converged and the evaluator turns
// that into FAIL with the recorded states. A timeout is never silent.

const (
	timelineFileName   = "timeline.jsonl"
	finalTasksFileName = "final-tasks.json"
	// stateNotFound marks a GetTaskInfo that returned ErrTaskNotFound.
	stateNotFound = "not_found"
)

// Production producers set MaxRetry between 3 and 25 (25 is asynq's default
// when a producer sets none). The tool enqueues with --max-retry (default 2),
// so a failing task archives sooner than it would in production: the
// retry/dead-letter path is the same, only the timing differs.
const (
	productionMaxRetryMin = 3
	productionMaxRetryMax = 25
)

// TimingFidelity is recorded in every result.json for the Step 5 summary.
type TimingFidelity struct {
	ToolMaxRetry          int    `json:"tool_max_retry"`
	ProductionMaxRetryMin int    `json:"production_max_retry_min"`
	ProductionMaxRetryMax int    `json:"production_max_retry_max"`
	RetryDelay            string `json:"retry_delay"`
	Note                  string `json:"note"`
}

func timingFidelity(toolMaxRetry int) TimingFidelity {
	return TimingFidelity{
		ToolMaxRetry:          toolMaxRetry,
		ProductionMaxRetryMin: productionMaxRetryMin,
		ProductionMaxRetryMax: productionMaxRetryMax,
		RetryDelay:            "worker default (asynq DefaultRetryDelayFunc: n^4 + 15 + rand(30)*(n+1) seconds)",
		Note: fmt.Sprintf("tasks were enqueued with MaxRetry(%d); production producers use MaxRetry %d-%d with the same worker backoff, "+
			"so in production the same retry/dead-letter path is taken and only the time to archive differs (timing, not path)",
			toolMaxRetry, productionMaxRetryMin, productionMaxRetryMax),
	}
}

// TimelineEntry is one line of timeline.jsonl: a task observation that
// differs from the previous observation of the same task.
type TimelineEntry struct {
	ObservedUTC   time.Time  `json:"observed_utc"`
	Wait          int        `json:"wait"` // 1-based WaitConverged call of the scenario (one per phase)
	TaskID        string     `json:"task_id"`
	State         string     `json:"state"`
	Retried       int        `json:"retried"`
	MaxRetry      int        `json:"max_retry"`
	LastErr       string     `json:"last_err,omitempty"`
	LastFailedAt  *time.Time `json:"last_failed_at,omitempty"`
	NextProcessAt *time.Time `json:"next_process_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	Error         string     `json:"error,omitempty"` // GetTaskInfo error
}

// key identifies an observation for de-duplication: (State, Retried,
// LastErr, LastFailedAt, NextProcessAt). asynq reports "now" as the
// NextProcessAt of a pending task, so it is ignored for pending tasks;
// otherwise every poll of a pending task would be a new entry.
func (e TimelineEntry) key() string {
	ts := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Format(time.RFC3339Nano)
	}
	next := ts(e.NextProcessAt)
	if e.State == "pending" {
		next = ""
	}
	return strings.Join([]string{e.State, fmt.Sprint(e.Retried), e.LastErr, ts(e.LastFailedAt), next, e.Error}, "\x1f")
}

// FinalTask is the last observation of one task.
type FinalTask struct {
	TaskID    string          `json:"task_id"`
	Converged bool            `json:"converged"`
	State     string          `json:"state"`
	Error     string          `json:"error,omitempty"`
	Info      *TaskInfoRecord `json:"task_info,omitempty"`
}

// FinalTasks is written to final-tasks.json after every wait (the last wait
// of a scenario leaves the final states of every enqueued task).
type FinalTasks struct {
	Scenario    string      `json:"scenario"`
	ObservedUTC time.Time   `json:"observed_utc"`
	Wait        int         `json:"wait"`
	Poll        string      `json:"poll"`
	Timeout     string      `json:"timeout"`
	DeadlineUTC time.Time   `json:"deadline_utc"`
	Polls       int         `json:"polls"`
	Converged   bool        `json:"converged"`
	TimedOut    bool        `json:"timed_out"`
	Cancelled   bool        `json:"cancelled"`
	Error       string      `json:"error,omitempty"`
	Tasks       []FinalTask `json:"tasks"`
}

// Get returns the final observation of a task.
func (f *FinalTasks) Get(id string) (FinalTask, bool) {
	if f == nil {
		return FinalTask{}, false
	}
	for _, t := range f.Tasks {
		if t.TaskID == id {
			return t, true
		}
	}
	return FinalTask{}, false
}

func isConvergedState(s string) bool { return s == stateCompleted || s == stateArchived }

// Poller implements ConvergenceWaiter over asynq.Inspector.
type Poller struct {
	Inspect TaskInspector
	Poll    time.Duration
	// Timeout bounds the convergence wait of one scenario: the clock starts
	// at the scenario's first wait and is shared by its later phases.
	Timeout time.Duration
	Now     func() time.Time

	mu        sync.Mutex
	deadlines map[string]time.Time
	waits     map[string]int
	last      map[string]map[string]string // scenario dir -> task ID -> last observation key
}

func (p *Poller) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}

// begin registers one wait of the scenario stored in dir.
func (p *Poller) begin(dir string) (deadline time.Time, wait int, last map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.deadlines == nil {
		p.deadlines, p.waits, p.last = map[string]time.Time{}, map[string]int{}, map[string]map[string]string{}
	}
	if _, ok := p.deadlines[dir]; !ok {
		p.deadlines[dir] = p.now().Add(p.Timeout)
		p.last[dir] = map[string]string{}
	}
	p.waits[dir]++
	return p.deadlines[dir], p.waits[dir], p.last[dir]
}

// WaitConverged polls every task until all are completed or archived. It
// returns an error on timeout or cancellation after recording the states.
func (p *Poller) WaitConverged(ctx context.Context, sp *ScenarioPlan, dir string, taskIDs []string) error {
	if p.Inspect == nil {
		return errors.New("poller has no inspector")
	}
	if p.Poll <= 0 || p.Timeout <= 0 {
		return fmt.Errorf("poller needs positive poll (%s) and timeout (%s)", p.Poll, p.Timeout)
	}
	deadline, wait, last := p.begin(dir)
	ids := append([]string(nil), taskIDs...)
	sort.Strings(ids)

	tl, err := os.OpenFile(filepath.Join(dir, timelineFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", timelineFileName, err)
	}
	defer tl.Close()
	enc := json.NewEncoder(tl)

	final := &FinalTasks{
		Scenario: sp.ID, Wait: wait, Poll: p.Poll.String(), Timeout: p.Timeout.String(), DeadlineUTC: deadline,
	}
	for {
		final.Polls++
		final.ObservedUTC = p.now()
		final.Tasks = final.Tasks[:0]
		all := true
		for _, id := range ids {
			ft, entry := p.observe(id, wait, final.ObservedUTC)
			final.Tasks = append(final.Tasks, ft)
			all = all && ft.Converged
			if k := entry.key(); last[id] != k {
				last[id] = k
				if err := enc.Encode(entry); err != nil {
					return fmt.Errorf("append %s: %w", timelineFileName, err)
				}
			}
		}
		if all {
			final.Converged = true
			return writeJSONFile(filepath.Join(dir, finalTasksFileName), final)
		}
		remaining := deadline.Sub(p.now())
		if remaining <= 0 {
			final.TimedOut = true
			final.Error = fmt.Sprintf("timeout after %s: not converged: %s", p.Timeout, pendingSummary(final.Tasks))
			return errors.Join(errors.New(final.Error), writeJSONFile(filepath.Join(dir, finalTasksFileName), final))
		}
		timer := time.NewTimer(min(p.Poll, remaining))
		select {
		case <-ctx.Done():
			timer.Stop()
			final.Cancelled = true
			final.Error = fmt.Sprintf("cancelled: %v; not converged: %s", ctx.Err(), pendingSummary(final.Tasks))
			return errors.Join(errors.New(final.Error), writeJSONFile(filepath.Join(dir, finalTasksFileName), final))
		case <-timer.C:
		}
	}
}

// observe reads one task. Read errors (including not found) are recorded as
// data; such a task never counts as converged.
func (p *Poller) observe(id string, wait int, at time.Time) (FinalTask, TimelineEntry) {
	entry := TimelineEntry{ObservedUTC: at, Wait: wait, TaskID: id}
	info, err := p.Inspect.GetTaskInfo(workerQueue, id)
	if err != nil {
		entry.Error = err.Error()
		entry.State = "unreadable"
		if errors.Is(err, asynq.ErrTaskNotFound) {
			entry.State = stateNotFound
		}
		return FinalTask{TaskID: id, State: entry.State, Error: entry.Error}, entry
	}
	rec := taskInfoRecord(info)
	entry.State, entry.Retried, entry.MaxRetry, entry.LastErr = rec.State, rec.Retried, rec.MaxRetry, rec.LastErr
	entry.LastFailedAt, entry.NextProcessAt, entry.CompletedAt = rec.LastFailedAt, rec.NextProcessAt, rec.CompletedAt
	return FinalTask{TaskID: id, State: rec.State, Converged: isConvergedState(rec.State), Info: rec}, entry
}

func pendingSummary(tasks []FinalTask) string {
	var parts []string
	for _, t := range tasks {
		if !t.Converged {
			parts = append(parts, t.TaskID+"="+t.State)
		}
	}
	return strings.Join(parts, ", ")
}

// readFinalTasks loads final-tasks.json; a missing file returns (nil, nil).
func readFinalTasks(dir string) (*FinalTasks, error) {
	var f FinalTasks
	ok, err := readJSONFile(filepath.Join(dir, finalTasksFileName), &f)
	if !ok || err != nil {
		return nil, err
	}
	return &f, nil
}

// readJSONFile decodes path with json.Number preserved. It reports false
// when the file does not exist.
func readJSONFile(path string, v any) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return true, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return true, nil
}
