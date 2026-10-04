package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Poller tests against an in-process asynq.Server on miniredis with stub
// handlers and a 10ms RetryDelayFunc.

const (
	stubOK       = "iso004test:ok"
	stubError    = "iso004test:error"
	stubSkip     = "iso004test:skip"
	stubBlock    = "iso004test:block"
	stubUnknown  = "iso004test:unregistered"
	testMaxRetry = 2
)

// startStubWorker runs an asynq server consuming the worker queue with the
// given mux, fast retries and fast forwarding of retry tasks.
func startStubWorker(t *testing.T, mr *miniredis.Miniredis, mux *asynq.ServeMux) {
	t.Helper()
	srv := asynq.NewServer(asynq.RedisClientOpt{Addr: mr.Addr()}, asynq.Config{
		Concurrency:              5,
		Queues:                   map[string]int{workerQueue: 1},
		LogLevel:                 asynq.FatalLevel,
		RetryDelayFunc:           func(int, error, *asynq.Task) time.Duration { return 10 * time.Millisecond },
		DelayedTaskCheckInterval: 50 * time.Millisecond,
		TaskCheckInterval:        20 * time.Millisecond,
	})
	require.NoError(t, srv.Start(mux))
	t.Cleanup(srv.Shutdown)
	// Wait for the heartbeat so preflight's redis.servers check sees it.
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = insp.Close() })
	require.Eventually(t, func() bool {
		s, err := insp.Servers()
		return err == nil && len(s) == 1
	}, 5*time.Second, 10*time.Millisecond)
}

func stubMux(release <-chan struct{}) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(stubOK, func(context.Context, *asynq.Task) error { return nil })
	mux.HandleFunc(stubError, func(context.Context, *asynq.Task) error { return errors.New("stub failure") })
	mux.HandleFunc(stubSkip, func(context.Context, *asynq.Task) error {
		return fmt.Errorf("stub invalid payload: %w", asynq.SkipRetry)
	})
	mux.HandleFunc(stubBlock, func(ctx context.Context, _ *asynq.Task) error {
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	return mux
}

func enqueueStub(t *testing.T, client *asynq.Client, typ, id string) {
	t.Helper()
	_, err := client.Enqueue(asynq.NewTask(typ, []byte(`{}`)),
		asynq.Queue(workerQueue), asynq.MaxRetry(testMaxRetry), asynq.Timeout(time.Minute), asynq.Retention(time.Hour), asynq.TaskID(id))
	require.NoError(t, err)
}

func readTimeline(t *testing.T, dir string) []TimelineEntry {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, timelineFileName))
	require.NoError(t, err)
	defer f.Close()
	var out []TimelineEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e TimelineEntry
		require.NoError(t, json.Unmarshal(sc.Bytes(), &e))
		out = append(out, e)
	}
	require.NoError(t, sc.Err())
	return out
}

func TestPollerConvergencePaths(t *testing.T) {
	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })
	insp := asynq.NewInspector(opt)
	t.Cleanup(func() { _ = insp.Close() })

	ids := map[string]string{
		stubOK: "iso004:t:P:1", stubError: "iso004:t:P:2", stubSkip: "iso004:t:P:3", stubUnknown: "iso004:t:P:4",
	}
	for typ, id := range ids {
		enqueueStub(t, client, typ, id)
	}
	startStubWorker(t, mr, stubMux(nil))

	dir := t.TempDir()
	p := &Poller{Inspect: insp, Poll: 20 * time.Millisecond, Timeout: 30 * time.Second}
	all := []string{ids[stubOK], ids[stubError], ids[stubSkip], ids[stubUnknown]}
	require.NoError(t, p.WaitConverged(context.Background(), &ScenarioPlan{ID: "P"}, dir, all))

	final, err := readFinalTasks(dir)
	require.NoError(t, err)
	require.NotNil(t, final)
	assert.True(t, final.Converged)
	assert.False(t, final.TimedOut)
	assert.Equal(t, "P", final.Scenario)
	require.Len(t, final.Tasks, 4)

	get := func(typ string) FinalTask {
		ft, ok := final.Get(ids[typ])
		require.True(t, ok, typ)
		require.NotNil(t, ft.Info, typ)
		assert.True(t, ft.Converged, typ)
		return ft
	}
	ok := get(stubOK)
	assert.Equal(t, stateCompleted, ok.State, "success -> completed (kept by Retention)")
	assert.Equal(t, int64(3600), ok.Info.RetentionSeconds)
	assert.NotNil(t, ok.Info.CompletedAt)
	assert.Equal(t, 0, ok.Info.Retried)

	er := get(stubError)
	assert.Equal(t, stateArchived, er.State, "error -> archived after N retries")
	assert.Equal(t, testMaxRetry, er.Info.Retried)
	assert.Equal(t, testMaxRetry, er.Info.MaxRetry)
	assert.Contains(t, er.Info.LastErr, "stub failure")
	assert.NotNil(t, er.Info.LastFailedAt)

	sk := get(stubSkip)
	assert.Equal(t, stateArchived, sk.State, "SkipRetry -> archived")
	assert.Equal(t, 0, sk.Info.Retried)
	assert.Contains(t, sk.Info.LastErr, "skip retry")

	un := get(stubUnknown)
	assert.Equal(t, stateArchived, un.State, "unknown type -> archived")
	assert.Equal(t, testMaxRetry, un.Info.Retried)
	assert.Contains(t, un.Info.LastErr, "handler not found")

	// Every recorded line differs from the previous line of the same task,
	// and the error task's retries are visible in the timeline.
	timeline := readTimeline(t, dir)
	require.NotEmpty(t, timeline)
	lastKey := map[string]string{}
	retriedSeen := map[int]bool{}
	for _, e := range timeline {
		assert.NotEqual(t, lastKey[e.TaskID], e.key(), "duplicate consecutive entry for %s", e.TaskID)
		lastKey[e.TaskID] = e.key()
		assert.Equal(t, 1, e.Wait)
		if e.TaskID == ids[stubError] {
			retriedSeen[e.Retried] = true
		}
	}
	assert.True(t, retriedSeen[testMaxRetry], "final retried count recorded")
	for typ, id := range ids {
		var states []string
		for _, e := range timeline {
			if e.TaskID == id {
				states = append(states, e.State)
			}
		}
		require.NotEmpty(t, states, typ)
		assert.True(t, isConvergedState(states[len(states)-1]), "%s ends converged: %v", typ, states)
	}
}

func TestPollerTimeoutIsRecordedNeverSilent(t *testing.T) {
	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })
	insp := asynq.NewInspector(opt)
	t.Cleanup(func() { _ = insp.Close() })

	release := make(chan struct{})
	enqueueStub(t, client, stubOK, "iso004:t:T:1")
	enqueueStub(t, client, stubBlock, "iso004:t:T:2")
	startStubWorker(t, mr, stubMux(release))
	t.Cleanup(func() { close(release) })

	dir := t.TempDir()
	p := &Poller{Inspect: insp, Poll: 20 * time.Millisecond, Timeout: 700 * time.Millisecond}
	start := time.Now()
	err := p.WaitConverged(context.Background(), &ScenarioPlan{ID: "T"}, dir, []string{"iso004:t:T:1", "iso004:t:T:2", "iso004:t:T:missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout after 700ms")
	assert.Contains(t, err.Error(), "iso004:t:T:2=active")
	assert.Contains(t, err.Error(), "iso004:t:T:missing="+stateNotFound)
	assert.Less(t, time.Since(start), 5*time.Second)

	final, rerr := readFinalTasks(dir)
	require.NoError(t, rerr)
	require.NotNil(t, final)
	assert.True(t, final.TimedOut)
	assert.False(t, final.Converged)
	ok, _ := final.Get("iso004:t:T:1")
	assert.Equal(t, stateCompleted, ok.State)
	blocked, _ := final.Get("iso004:t:T:2")
	assert.Equal(t, "active", blocked.State)
	assert.False(t, blocked.Converged)
	missing, _ := final.Get("iso004:t:T:missing")
	assert.Equal(t, stateNotFound, missing.State)
	assert.Contains(t, missing.Error, "not found")

	// A second wait of the same scenario shares the deadline: it fails at
	// once instead of granting another --timeout.
	err = p.WaitConverged(context.Background(), &ScenarioPlan{ID: "T"}, dir, []string{"iso004:t:T:2"})
	require.Error(t, err)
	final, _ = readFinalTasks(dir)
	assert.Equal(t, 2, final.Wait)
	assert.Equal(t, 1, final.Polls)
	timeline := readTimeline(t, dir)
	for _, e := range timeline {
		if e.Wait == 2 {
			t.Errorf("unchanged observations must not be re-recorded on a later wait: %+v", e)
		}
	}
}

func TestPollerCancellation(t *testing.T) {
	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })
	insp := asynq.NewInspector(opt)
	t.Cleanup(func() { _ = insp.Close() })
	enqueueStub(t, client, stubOK, "iso004:t:C:1") // no worker: stays pending

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	dir := t.TempDir()
	p := &Poller{Inspect: insp, Poll: 20 * time.Millisecond, Timeout: time.Minute}
	err := p.WaitConverged(ctx, &ScenarioPlan{ID: "C"}, dir, []string{"iso004:t:C:1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
	final, _ := readFinalTasks(dir)
	require.NotNil(t, final)
	assert.True(t, final.Cancelled)
	assert.Equal(t, "pending", final.Tasks[0].State)
	// A pending task is polled many times but recorded once.
	assert.Len(t, readTimeline(t, dir), 1)
}

func TestTimingFidelityNote(t *testing.T) {
	tf := timingFidelity(2)
	assert.Equal(t, 2, tf.ToolMaxRetry)
	assert.Equal(t, 3, tf.ProductionMaxRetryMin)
	assert.Equal(t, 25, tf.ProductionMaxRetryMax)
	assert.Contains(t, tf.Note, "MaxRetry 3-25")
	assert.Contains(t, tf.Note, "timing, not path")
}
