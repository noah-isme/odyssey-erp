package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Enqueue-only tests on miniredis. No asynq server runs, so every enqueued
// task stays pending and its options can be inspected exactly.

type fakeRow struct {
	v   any
	err error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	switch d := dest[0].(type) {
	case *int64:
		*d = r.v.(int64)
	case *string:
		*d = r.v.(string)
	default:
		return errors.New("unexpected scan target")
	}
	return nil
}

type fakeDB struct {
	mu    sync.Mutex
	calls []string
	err   error
}

func (f *fakeDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	f.mu.Lock()
	f.calls = append(f.calls, sql)
	f.mu.Unlock()
	if f.err != nil {
		return fakeRow{err: f.err}
	}
	if strings.Contains(sql, "to_char") {
		return fakeRow{v: "2026-10"}
	}
	return fakeRow{v: int64(500000)}
}

type fakeMail struct {
	calls [][]string
	err   error
}

func (f *fakeMail) Snapshot(_ context.Context, recipients []string) (*MailSnapshot, error) {
	f.calls = append(f.calls, recipients)
	snap := &MailSnapshot{Sink: "mailpit", API: "http://127.0.0.1:8025", FetchedUTC: time.Now().UTC()}
	for _, r := range recipients {
		snap.Queries = append(snap.Queries, MailQuery{Recipient: r, Messages: []MailMessage{}})
	}
	return snap, f.err
}

type waitCall struct {
	Scenario string
	TaskIDs  []string
}

type fakeWaiter struct {
	calls []waitCall
	err   error
}

func (f *fakeWaiter) WaitConverged(_ context.Context, sp *ScenarioPlan, dir string, ids []string) error {
	f.calls = append(f.calls, waitCall{Scenario: sp.ID, TaskIDs: ids})
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	return f.err
}

type fakeSQL struct{ labels map[string][]string }

func (f *fakeSQL) Snapshot(_ context.Context, sp *ScenarioPlan, _ string, label string) error {
	if f.labels == nil {
		f.labels = map[string][]string{}
	}
	f.labels[sp.ID] = append(f.labels[sp.ID], label)
	return nil
}

// passEval is an evaluator that passes every scenario, for tests about
// enqueue behavior only (sqlobs_test.go covers the real evaluator).
type passEval struct{}

func (passEval) Evaluate(_ context.Context, rec *EnqueueRecord, _ string) (*ScenarioResult, error) {
	return &ScenarioResult{Scenario: rec.Scenario, Result: resultPass}, nil
}

type execHarness struct {
	mr   *miniredis.Miniredis
	insp *asynq.Inspector
	ex   *Executor
	out  string
}

func newHarness(t *testing.T, unmet map[Requirement]string) *execHarness {
	t.Helper()
	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })
	insp := asynq.NewInspector(opt)
	t.Cleanup(func() { _ = insp.Close() })
	fx, _, err := loadFixtures(sampleFixtures, envMap(nil))
	require.NoError(t, err)
	if unmet == nil {
		unmet = map[Requirement]string{}
	}
	out := filepath.Join(t.TempDir(), "bundle")
	require.NoError(t, prepareOutDir(out))
	return &execHarness{mr: mr, insp: insp, out: out, ex: &Executor{
		Cfg:      &Config{RunID: "9000000001", MaxRetry: 3, OutDir: out},
		Fixtures: fx, Unmet: unmet, OutDir: out,
		Enqueue: client, Inspect: insp, DB: &fakeDB{}, Mail: &fakeMail{},
	}}
}

func readEnqueueJSON(t *testing.T, out, id string) EnqueueRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(out, "scenarios", id, "enqueue.json"))
	require.NoError(t, err)
	var rec EnqueueRecord
	require.NoError(t, json.Unmarshal(data, &rec))
	return rec
}

func submissionsByN(rec EnqueueRecord) map[int]Submission {
	out := map[int]Submission{}
	for _, s := range rec.Submissions {
		out[s.N] = s
	}
	return out
}

func TestExecutorEnqueuesEveryScenarioWithOptions(t *testing.T) {
	h := newHarness(t, nil)
	waiter, sql := &fakeWaiter{}, &fakeSQL{}
	h.ex.Wait, h.ex.SQL, h.ex.Eval = waiter, sql, passEval{}

	records, code, err := h.ex.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, exitOK, code)
	require.Len(t, records, len(scenarioRegistry))

	enqueued, conflicts := 0, 0
	for _, s := range scenarioRegistry {
		rec := readEnqueueJSON(t, h.out, s.ID)
		assert.True(t, rec.Enabled, s.ID)
		assert.Empty(t, rec.Errors, s.ID)
		assert.True(t, rec.Converged, s.ID)
		assert.Equal(t, "iso004:9000000001:", rec.TaskIDPrefix)
		require.Len(t, rec.Submissions, len(s.Tasks), s.ID)
		assert.Equal(t, s.ID, rec.Plan.ID)
		assert.Len(t, rec.Plan.Queries, len(s.Queries), s.ID)
		assert.Len(t, rec.Plan.Assertions, len(s.Assertions), s.ID)

		for _, sub := range rec.Submissions {
			assert.False(t, sub.Unexpected, "%s/%d: %s %s", s.ID, sub.N, sub.Reason, sub.Error)
			raw, err := base64.StdEncoding.DecodeString(sub.PayloadBase64)
			require.NoError(t, err)
			assert.Equal(t, sub.Payload, string(raw), "exact payload bytes")
			assert.Equal(t, EnqueueOptions{Queue: "default", MaxRetry: 3, Timeout: "2m0s", TimeoutSeconds: 120, Retention: "72h0m0s", RetentionSeconds: 259200, TaskID: sub.TaskID}, sub.Options)
			assert.Equal(t, []string{`Queue("default")`, "MaxRetry(3)", "Timeout(2m0s)", "Retention(72h0m0s)", `TaskID("` + sub.TaskID + `")`}, sub.OptionStrings)
			assert.True(t, strings.HasPrefix(sub.TaskID, "iso004:9000000001:"), sub.TaskID)
			require.NotNil(t, sub.SubmittedUTC)

			switch sub.Outcome {
			case outcomeEnqueued:
				enqueued++
				require.NotNil(t, sub.TaskInfo, "TaskInfo returned by Enqueue is recorded")
				assert.Equal(t, sub.TaskID, sub.TaskInfo.ID)
				assert.Equal(t, "pending", sub.TaskInfo.State)
				assert.Equal(t, sub.Payload, sub.TaskInfo.Payload)
				assert.Equal(t, int64(259200), sub.TaskInfo.RetentionSeconds)

				ti, err := h.insp.GetTaskInfo("default", sub.TaskID)
				require.NoError(t, err, sub.TaskID)
				assert.Equal(t, asynq.TaskStatePending, ti.State)
				assert.Equal(t, "default", ti.Queue)
				assert.Equal(t, sub.Type, ti.Type)
				assert.Equal(t, sub.Payload, string(ti.Payload))
				assert.Equal(t, 3, ti.MaxRetry)
				assert.Equal(t, 2*time.Minute, ti.Timeout)
				assert.Equal(t, 72*time.Hour, ti.Retention)
			case outcomeConflict:
				conflicts++
				assert.True(t, sub.Expectation.Conflict)
				assert.Contains(t, sub.Error, asynq.ErrTaskIDConflict.Error())
				assert.Nil(t, sub.TaskInfo)
				require.NotNil(t, sub.ConflictingTask, "the blocking task is recorded")
				assert.Equal(t, sub.TaskID, sub.ConflictingTask.ID)
			default:
				t.Errorf("%s/%d: unexpected outcome %s", s.ID, sub.N, sub.Outcome)
			}
		}
	}
	assert.Equal(t, 2, conflicts, "S07 n=4 and S11 n=3")
	total := 0
	for _, s := range scenarioRegistry {
		total += len(s.Tasks)
	}
	assert.Equal(t, total-2, enqueued)
	qi, err := h.insp.GetQueueInfo("default")
	require.NoError(t, err)
	assert.Equal(t, enqueued, qi.Pending)

	// S07: the conflict probe reused the TaskID of n=1.
	s07 := submissionsByN(readEnqueueJSON(t, h.out, "S07-duplicate-delivery-variance"))
	assert.Equal(t, s07[1].TaskID, s07[4].TaskID)
	assert.Equal(t, outcomeConflict, s07[4].Outcome)
	assert.True(t, s07[1].Concurrent && s07[2].Concurrent)

	// Phases are awaited in order; SQL is snapshotted around them.
	var s07Waits []waitCall
	for _, c := range waiter.calls {
		if c.Scenario == "S07-duplicate-delivery-variance" {
			s07Waits = append(s07Waits, c)
		}
	}
	require.Len(t, s07Waits, 2)
	assert.ElementsMatch(t, []string{s07[1].TaskID, s07[2].TaskID}, s07Waits[0].TaskIDs)
	assert.ElementsMatch(t, []string{s07[1].TaskID, s07[2].TaskID, s07[3].TaskID}, s07Waits[1].TaskIDs)
	assert.Equal(t, []string{"before", "after-phase-1", "after"}, sql.labels["S07-duplicate-delivery-variance"])
	assert.Equal(t, []string{"before", "after"}, sql.labels["S01-unregistered-type"])

	// S03: dynamic IDs resolved by SELECT and recorded with their SQL.
	s03 := readEnqueueJSON(t, h.out, "S03-object-not-found")
	require.Len(t, s03.DynamicValues, 3)
	assert.Equal(t, `{"snapshot_id":500000}`, s03.Submissions[0].Payload)
	assert.Contains(t, s03.DynamicValues[dynVarianceUnknown].SQL, "FROM variance_snapshots")

	// Email scenarios record mail-before.json and mail-after.json; others do not.
	for _, s := range scenarioRegistry {
		dir := filepath.Join(h.out, "scenarios", s.ID)
		for _, name := range []string{"mail-before.json", "mail-after.json"} {
			_, err := os.Stat(filepath.Join(dir, name))
			if len(s.MailRecipients) > 0 {
				assert.NoError(t, err, "%s/%s", s.ID, name)
			} else {
				assert.True(t, os.IsNotExist(err), "%s/%s", s.ID, name)
			}
		}
	}
	var after MailSnapshot
	data, err := os.ReadFile(filepath.Join(h.out, "scenarios", "S12-duplicate-delivery-payslip", "mail-after.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &after))
	assert.Equal(t, "after", after.Label)
	assert.True(t, after.AfterConvergence)
	require.Len(t, after.Queries, 1)
	assert.Equal(t, "iso004-payslip-9000000001@staging.invalid", after.Queries[0].Recipient)
}

func TestExecutorWithoutObserversEnqueuesFirstPhaseOnly(t *testing.T) {
	h := newHarness(t, nil)
	_, code, err := h.ex.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, exitNotEvaluated, code)

	s07 := readEnqueueJSON(t, h.out, "S07-duplicate-delivery-variance")
	assert.False(t, s07.Converged)
	assert.Contains(t, s07.NotEvaluated, "no convergence waiter")
	by := submissionsByN(s07)
	assert.Equal(t, outcomeEnqueued, by[1].Outcome)
	assert.Equal(t, outcomeEnqueued, by[2].Outcome)
	assert.Equal(t, outcomeConflict, by[4].Outcome)
	assert.Equal(t, outcomeNotSubmitted, by[3].Outcome, "phase 2 waits for convergence")
	_, err = h.insp.GetTaskInfo("default", by[3].TaskID)
	assert.ErrorIs(t, err, asynq.ErrTaskNotFound)

	s01 := readEnqueueJSON(t, h.out, "S01-unregistered-type")
	assert.Contains(t, s01.NotEvaluated, "Step 4")
	assert.Equal(t, outcomeEnqueued, s01.Submissions[0].Outcome)
}

func TestExecutorCapturesUnexpectedConflictAsData(t *testing.T) {
	h := newHarness(t, nil)
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: h.mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	_, err := client.Enqueue(asynq.NewTask("other", nil), asynq.TaskID("iso004:9000000001:S01-unregistered-type:1"), asynq.Queue("default"))
	require.NoError(t, err)

	h.ex.Wait, h.ex.SQL = &fakeWaiter{}, &fakeSQL{}
	records, code, err := h.ex.Run(context.Background())
	require.NoError(t, err, "a conflict is data, not an error")
	assert.Equal(t, exitFail, code)
	require.Len(t, records, len(scenarioRegistry), "later scenarios still ran")

	s01 := readEnqueueJSON(t, h.out, "S01-unregistered-type")
	sub := s01.Submissions[0]
	assert.Equal(t, outcomeConflict, sub.Outcome)
	assert.True(t, sub.Unexpected)
	assert.Equal(t, "unexpected TaskID conflict", sub.Reason)
	require.NotNil(t, sub.ConflictingTask)
	assert.Equal(t, "other", sub.ConflictingTask.Type)

	s02 := readEnqueueJSON(t, h.out, "S02-malformed-payload")
	assert.Equal(t, outcomeEnqueued, s02.Submissions[0].Outcome)
}

func TestExecutorMissingExpectedConflictIsUnexpected(t *testing.T) {
	h := newHarness(t, nil)
	h.ex.Cfg.Scenarios = []string{"S07-duplicate-delivery-variance"}
	// An enqueuer that never reports conflicts (e.g. TaskIDs dropped).
	h.ex.Enqueue = enqueueFunc(func(ctx context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
		return &asynq.TaskInfo{ID: "x", Queue: "default", Type: task.Type(), Payload: task.Payload()}, nil
	})
	_, code, err := h.ex.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, exitFail, code)
	by := submissionsByN(readEnqueueJSON(t, h.out, "S07-duplicate-delivery-variance"))
	assert.True(t, by[4].Unexpected)
	assert.Contains(t, by[4].Reason, "expected ErrTaskIDConflict")
}

type enqueueFunc func(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)

func (f enqueueFunc) EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return f(ctx, task, opts...)
}

func TestExecutorReleasesConcurrentGroupTogether(t *testing.T) {
	h := newHarness(t, nil)
	h.ex.Cfg.Scenarios = []string{"S09-duplicate-delivery-ap"}
	real := h.ex.Enqueue
	var inflight, peak int32
	h.ex.Enqueue = enqueueFunc(func(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
		n := atomic.AddInt32(&inflight, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&inflight, -1)
		return real.EnqueueContext(ctx, task, opts...)
	})
	h.ex.Wait, h.ex.SQL, h.ex.Eval = &fakeWaiter{}, &fakeSQL{}, passEval{}
	_, code, err := h.ex.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, exitOK, code)
	assert.Equal(t, int32(6), atomic.LoadInt32(&peak), "phase 1 releases the six concurrent deliveries together")
}

func TestExecutorExcludedAndSkipped(t *testing.T) {
	h := newHarness(t, map[Requirement]string{
		ReqEmail:            "--allow-email not set",
		ReqNoGlobalAPPolicy: "1 policy",
	})
	mail := &fakeMail{}
	h.ex.Mail = mail
	h.ex.Wait, h.ex.SQL, h.ex.Eval = &fakeWaiter{}, &fakeSQL{}, passEval{}
	_, code, err := h.ex.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, exitOK, code, "exclusions are recorded, not enqueue failures")
	assert.Empty(t, mail.calls)

	for _, id := range []string{"S11-forged-recipient-mail", "S12-duplicate-delivery-payslip"} {
		rec := readEnqueueJSON(t, h.out, id)
		assert.False(t, rec.Enabled)
		assert.Contains(t, rec.Excluded, "email-gate unmet")
		for _, sub := range rec.Submissions {
			assert.Equal(t, outcomeExcluded, sub.Outcome)
			_, err := h.insp.GetTaskInfo("default", sub.TaskID)
			assert.ErrorIs(t, err, asynq.ErrTaskNotFound)
		}
		_, err := os.Stat(filepath.Join(h.out, "scenarios", id, "mail-before.json"))
		assert.True(t, os.IsNotExist(err))
	}
	s09 := submissionsByN(readEnqueueJSON(t, h.out, "S09-duplicate-delivery-ap"))
	for n := 1; n <= 9; n++ {
		if n <= 3 {
			assert.Equal(t, outcomeSkipped, s09[n].Outcome, n)
			assert.Contains(t, s09[n].Reason, "FAIL-by-precondition")
		} else {
			assert.Equal(t, outcomeEnqueued, s09[n].Outcome, n)
		}
	}
}

func TestExecutorMailBeforeFailureAbortsScenario(t *testing.T) {
	h := newHarness(t, nil)
	h.ex.Cfg.Scenarios = []string{"S11-forged-recipient-mail"}
	h.ex.Mail = &fakeMail{err: errors.New("status 500")}
	h.ex.Wait, h.ex.SQL = &fakeWaiter{}, &fakeSQL{}
	_, code, err := h.ex.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, exitFail, code)
	rec := readEnqueueJSON(t, h.out, "S11-forged-recipient-mail")
	require.Len(t, rec.Errors, 1)
	assert.Contains(t, rec.Errors[0], "mail-before.json: status 500")
	for _, sub := range rec.Submissions {
		assert.Equal(t, outcomeNotSubmitted, sub.Outcome)
	}
	_, err = os.Stat(filepath.Join(h.out, "scenarios", "S11-forged-recipient-mail", "mail-before.json"))
	assert.NoError(t, err, "the failed API response is still recorded")
	_, err = h.insp.GetTaskInfo("default", "iso004:9000000001:S11")
	assert.Error(t, err, "nothing was enqueued (the queue does not even exist)")
}

func TestExecutorDynamicFailureAbortsScenario(t *testing.T) {
	h := newHarness(t, nil)
	h.ex.Cfg.Scenarios = []string{"S03-object-not-found", "S05-forged-object-variance"}
	h.ex.DB = &fakeDB{err: errors.New("connection reset")}
	h.ex.Wait, h.ex.SQL = &fakeWaiter{}, &fakeSQL{}
	_, code, err := h.ex.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, exitFail, code)
	s03 := readEnqueueJSON(t, h.out, "S03-object-not-found")
	assert.Contains(t, s03.Errors[0], "connection reset")
	for _, sub := range s03.Submissions {
		assert.Equal(t, outcomeNotSubmitted, sub.Outcome)
	}
	s05 := readEnqueueJSON(t, h.out, "S05-forged-object-variance")
	assert.Equal(t, outcomeEnqueued, s05.Submissions[0].Outcome, "scenarios without dynamic values need no DB")
}

func TestExecutorWaitTimeoutStopsLaterPhases(t *testing.T) {
	h := newHarness(t, nil)
	h.ex.Cfg.Scenarios = []string{"S12-duplicate-delivery-payslip"}
	h.ex.Wait, h.ex.SQL = &fakeWaiter{err: errors.New("timeout after 15m0s")}, &fakeSQL{}
	_, code, err := h.ex.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, exitFail, code)
	rec := readEnqueueJSON(t, h.out, "S12-duplicate-delivery-payslip")
	assert.False(t, rec.Converged)
	by := submissionsByN(rec)
	assert.Equal(t, outcomeNotSubmitted, by[3].Outcome)
	assert.Equal(t, "phase 1 did not converge", by[3].Reason)
	require.Len(t, rec.Phases, 1)
	assert.Contains(t, rec.Phases[0].WaitError, "timeout")
	var after MailSnapshot
	data, err := os.ReadFile(filepath.Join(h.out, "scenarios", "S12-duplicate-delivery-payslip", "mail-after.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &after))
	assert.False(t, after.AfterConvergence)
}
