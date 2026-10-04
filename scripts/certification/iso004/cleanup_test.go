package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const otherTestQueue = "iso004test-other"

func cleanupArgs(runID, dir, redis string, extra ...string) []string {
	return append([]string{"--run-id", runID, "--out", dir, "--redis", redis}, extra...)
}

func runCleanupCmd(t *testing.T, args []string, deps cleanupDeps) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	getenv := func(string) string { return "" }
	code := cleanupWithDeps(context.Background(), args, getenv, &stdout, &stderr, deps)
	return code, stdout.String(), stderr.String()
}

// noRedisDeps fails the test if the cleanup reaches Redis: refusals must
// happen before any connection.
func noRedisDeps(t *testing.T) cleanupDeps {
	return cleanupDeps{
		Open: func(string) (TaskCleaner, func() error, error) {
			t.Fatal("cleanup opened Redis on a refusal path")
			return nil, nil, errors.New("unreachable")
		},
		Now: time.Now,
	}
}

func cleanupJSONPath(bundle, runID string) string {
	return filepath.Join(filepath.Dir(bundle), cleanupDirName, runID, cleanupFileName)
}

func readCleanupFile(t *testing.T, path string) CleanupFile {
	t.Helper()
	var rec CleanupFile
	ok, err := readJSONFile(path, &rec)
	require.NoError(t, err)
	require.True(t, ok, "%s exists", path)
	return rec
}

func taskState(t *testing.T, insp *asynq.Inspector, queue, id string) string {
	t.Helper()
	info, err := insp.GetTaskInfo(queue, id)
	if errors.Is(err, asynq.ErrTaskNotFound) {
		return "deleted"
	}
	require.NoError(t, err, id)
	return info.State.String()
}

// TestCleanupDeletesOnlyTheRunPrefix runs the real subcommand against
// miniredis: retained tasks of run "12" are deleted from the archived and
// completed sets of the selected queues; tasks of runs "123" and "1", other
// prefixes, unselected queues and non-retained states are untouched; the
// sealed bundle is not modified.
func TestCleanupDeletesOnlyTheRunPrefix(t *testing.T) {
	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })
	insp := asynq.NewInspector(opt)
	t.Cleanup(func() { _ = insp.Close() })

	type task struct{ queue, id, want string }
	// Processed by the stub worker on the default queue.
	processed := []struct{ typ, id string }{
		{stubOK, "iso004:12:S01:1"}, {stubError, "iso004:12:S01:2"}, {stubSkip, "iso004:12:S09b:1"},
		{stubOK, "iso004:123:S01:1"}, {stubError, "iso004:123:S01:2"},
		{stubOK, "iso004:1:S01:1"}, {stubError, "iso004:1:S01:2"},
		{stubOK, "xiso004:12:S01:1"}, {stubError, "other:12:S01:1"},
	}
	for _, p := range processed {
		enqueueStub(t, client, p.typ, p.id)
	}
	startStubWorker(t, mr, stubMux(nil))
	require.Eventually(t, func() bool {
		for _, p := range processed {
			info, err := insp.GetTaskInfo(workerQueue, p.id)
			if err != nil || (info.State != asynq.TaskStateCompleted && info.State != asynq.TaskStateArchived) {
				return false
			}
		}
		return true
	}, 30*time.Second, 20*time.Millisecond)

	// A run-12 task that is still scheduled is not retained and must stay.
	_, err := client.Enqueue(asynq.NewTask(stubOK, []byte(`{}`)), asynq.Queue(workerQueue), asynq.TaskID("iso004:12:S07:9"), asynq.ProcessIn(time.Hour))
	require.NoError(t, err)
	// Archived tasks on a queue no worker consumes: run 12 on the selected
	// second queue and run 123 there as well.
	for _, id := range []string{"iso004:12:S02:1", "iso004:123:S02:1"} {
		_, err := client.Enqueue(asynq.NewTask(stubOK, []byte(`{}`)), asynq.Queue(otherTestQueue), asynq.TaskID(id))
		require.NoError(t, err)
		require.NoError(t, insp.ArchiveTask(otherTestQueue, id))
	}
	// An archived run-12 task on a queue that is not selected stays.
	_, err = client.Enqueue(asynq.NewTask(stubOK, []byte(`{}`)), asynq.Queue("unselected"), asynq.TaskID("iso004:12:S03:1"))
	require.NoError(t, err)
	require.NoError(t, insp.ArchiveTask("unselected", "iso004:12:S03:1"))

	all := []task{
		{workerQueue, "iso004:12:S01:1", "deleted"},
		{workerQueue, "iso004:12:S01:2", "deleted"},
		{workerQueue, "iso004:12:S09b:1", "deleted"},
		{otherTestQueue, "iso004:12:S02:1", "deleted"},
		{workerQueue, "iso004:12:S07:9", "scheduled"},
		{"unselected", "iso004:12:S03:1", "archived"},
		{workerQueue, "iso004:123:S01:1", "completed"},
		{workerQueue, "iso004:123:S01:2", "archived"},
		{otherTestQueue, "iso004:123:S02:1", "archived"},
		{workerQueue, "iso004:1:S01:1", "completed"},
		{workerQueue, "iso004:1:S01:2", "archived"},
		{workerQueue, "xiso004:12:S01:1", "completed"},
		{workerQueue, "other:12:S01:1", "archived"},
	}

	bundle := makeSealedBundle(t, "12")
	sumsBefore := readSums(t, bundle)
	frozen := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	deps := defaultCleanupDeps()
	deps.Now = func() time.Time { return frozen }

	code, stdout, stderr := runCleanupCmd(t, cleanupArgs("12", bundle, mr.Addr(), "--queue", workerQueue, "--queue", otherTestQueue), deps)
	require.Equal(t, exitOK, code, stderr)
	assert.Contains(t, stdout, "matched 4, deleted 4, failed 0")

	for _, tk := range all {
		assert.Equal(t, tk.want, taskState(t, insp, tk.queue, tk.id), "%s/%s", tk.queue, tk.id)
	}

	// The bundle is unchanged and still verifies; the record is a sibling.
	assert.Equal(t, sumsBefore, readSums(t, bundle))
	assertSumsVerify(t, bundle)
	_, err = os.Stat(filepath.Join(bundle, cleanupFileName))
	assert.ErrorIs(t, err, os.ErrNotExist)

	rec := readCleanupFile(t, cleanupJSONPath(bundle, "12"))
	assert.Equal(t, "12", rec.RunID)
	assert.Equal(t, "iso004:12:", rec.TaskIDPrefix)
	assert.Equal(t, []string{workerQueue, otherTestQueue}, rec.Queues)
	assert.Equal(t, CleanupCounts{Matched: 4, Deleted: 4, Failed: 0}, rec.Counts)
	assert.Empty(t, rec.Failed)
	assert.Equal(t, frozen, rec.StartedUTC)
	sumsHash, err := fileSHA256(filepath.Join(bundle, sha256SumsFileName))
	require.NoError(t, err)
	assert.Equal(t, sumsHash, rec.BundleSumsSHA256)
	var deleted []string
	for _, d := range rec.Deleted {
		deleted = append(deleted, d.Queue+"/"+d.State+"/"+d.ID)
	}
	assert.ElementsMatch(t, []string{
		workerQueue + "/completed/iso004:12:S01:1",
		workerQueue + "/archived/iso004:12:S01:2",
		workerQueue + "/archived/iso004:12:S09b:1",
		otherTestQueue + "/archived/iso004:12:S02:1",
	}, deleted)
	require.Len(t, rec.Scans, 4)
	for _, s := range rec.Scans {
		assert.Empty(t, s.Error)
	}

	// A second cleanup of the same run refuses instead of overwriting the
	// first record.
	code, _, stderr = runCleanupCmd(t, cleanupArgs("12", bundle, mr.Addr()), noRedisDeps(t))
	assert.Equal(t, exitFail, code)
	assert.Contains(t, stderr, "already exists")
}

// TestCleanupDefaultQueue checks that without --queue only "default" is
// cleaned, and that run "12" never matches run "123".
func TestCleanupDefaultQueue(t *testing.T) {
	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })
	insp := asynq.NewInspector(opt)
	t.Cleanup(func() { _ = insp.Close() })
	for _, qt := range []struct{ queue, id string }{
		{workerQueue, "iso004:12:S01:1"}, {workerQueue, "iso004:123:S01:1"}, {otherTestQueue, "iso004:12:S02:1"},
	} {
		_, err := client.Enqueue(asynq.NewTask(stubOK, nil), asynq.Queue(qt.queue), asynq.TaskID(qt.id))
		require.NoError(t, err)
		require.NoError(t, insp.ArchiveTask(qt.queue, qt.id))
	}

	bundle := makeSealedBundle(t, "12")
	code, _, stderr := runCleanupCmd(t, cleanupArgs("12", bundle, mr.Addr()), defaultCleanupDeps())
	require.Equal(t, exitOK, code, stderr)
	assert.Equal(t, "deleted", taskState(t, insp, workerQueue, "iso004:12:S01:1"))
	assert.Equal(t, "archived", taskState(t, insp, workerQueue, "iso004:123:S01:1"))
	assert.Equal(t, "archived", taskState(t, insp, otherTestQueue, "iso004:12:S02:1"))
	rec := readCleanupFile(t, cleanupJSONPath(bundle, "12"))
	assert.Equal(t, []string{workerQueue}, rec.Queues)
	assert.Equal(t, CleanupCounts{Matched: 1, Deleted: 1}, rec.Counts)
}

func TestCleanupRefusesUnsealedOrTamperedBundle(t *testing.T) {
	cases := []struct {
		name   string
		runID  string
		mutate func(t *testing.T, dir string)
		want   string
	}{
		{"no SHA256SUMS", "12", func(t *testing.T, dir string) {
			require.NoError(t, os.Remove(filepath.Join(dir, sha256SumsFileName)))
		}, "bundle is not sealed"},
		{"tampered SHA256SUMS digest", "12", func(t *testing.T, dir string) {
			sums := readSums(t, dir)
			if sums[0] == '0' {
				sums[0] = '1'
			} else {
				sums[0] = '0'
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, sha256SumsFileName), sums, 0o644))
		}, "does not verify"},
		{"SHA256SUMS lists a missing file", "12", func(t *testing.T, dir string) {
			sums := append(readSums(t, dir), []byte("0000000000000000000000000000000000000000000000000000000000000000  forged.txt\n")...)
			require.NoError(t, os.WriteFile(filepath.Join(dir, sha256SumsFileName), sums, 0o644))
		}, "missing: forged.txt"},
		{"malformed SHA256SUMS", "12", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, sha256SumsFileName), []byte("garbage\n"), 0o644))
		}, "malformed"},
		{"bundle file modified after sealing", "12", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, summaryJSONFileName), []byte(`{"result":"FAIL"}`), 0o644))
		}, "changed: " + summaryJSONFileName},
		{"journal added but not sealed", "12", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, workerJournalFileName), []byte("journal\n"), 0o644))
		}, "not listed: " + workerJournalFileName},
		{"bundle of another run", "123", func(*testing.T, string) {}, `is for run "12", not --run-id "123"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bundle := makeSealedBundle(t, "12")
			tc.mutate(t, bundle)
			code, stdout, stderr := runCleanupCmd(t, cleanupArgs(tc.runID, bundle, "127.0.0.1:1"), noRedisDeps(t))
			assert.Equal(t, exitFail, code)
			assert.Empty(t, stdout)
			assert.Contains(t, stderr, "refusing")
			assert.Contains(t, stderr, tc.want)
			_, err := os.Stat(filepath.Join(filepath.Dir(bundle), cleanupDirName))
			assert.ErrorIs(t, err, os.ErrNotExist, "nothing is written on refusal")
		})
	}
}

func TestCleanupUsage(t *testing.T) {
	bundle := makeSealedBundle(t, "12")
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"missing run id", []string{"--out", bundle, "--redis", "127.0.0.1:1"}, "--run-id"},
		{"run id with colon", cleanupArgs("12:S01", bundle, "127.0.0.1:1"), "--run-id"},
		{"run id with glob", cleanupArgs("1*", bundle, "127.0.0.1:1"), "--run-id"},
		{"missing out", []string{"--run-id", "12", "--redis", "127.0.0.1:1"}, "--out is required"},
		{"missing redis", []string{"--run-id", "12", "--out", bundle}, "--redis"},
		{"bad queue", cleanupArgs("12", bundle, "127.0.0.1:1", "--queue", "a:b"), "--queue"},
		{"extra args", append(cleanupArgs("12", bundle, "127.0.0.1:1"), "extra"), "unexpected arguments"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := runCleanupCmd(t, tc.args, noRedisDeps(t))
			assert.Equal(t, exitUsage, code)
			assert.Contains(t, stderr, tc.want)
		})
	}
	// Dispatch reaches the subcommand.
	var stderr bytes.Buffer
	assert.Equal(t, exitUsage, dispatch(context.Background(), []string{"cleanup"}, io.Discard, &stderr))
	assert.Contains(t, stderr.String(), "iso004 cleanup:")
}

// fakeCleaner serves fixed listings and fails selected deletions.
type fakeCleaner struct {
	archived, completed []*asynq.TaskInfo
	failDelete          map[string]bool
	deleted             []string
}

func (f *fakeCleaner) ListArchivedTasks(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error) {
	return f.archived, nil
}

func (f *fakeCleaner) ListCompletedTasks(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error) {
	return f.completed, nil
}

func (f *fakeCleaner) DeleteTask(_, id string) error {
	if f.failDelete[id] {
		return errors.New("stub delete failure")
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func TestCleanupPrefixIsExact(t *testing.T) {
	ids := func(s ...string) []*asynq.TaskInfo {
		out := make([]*asynq.TaskInfo, 0, len(s))
		for _, id := range s {
			out = append(out, &asynq.TaskInfo{ID: id})
		}
		return out
	}
	f := &fakeCleaner{
		archived:  ids("iso004:12:S01:2", "iso004:12", "iso004:120:S01:1", "iso004:123:S01:1", "xiso004:12:a", "ISO004:12:a"),
		completed: ids("iso004:12:S01:1", "iso004:1:S01:1", "iso004:12-b:S01:1"),
	}
	cfg := &CleanupConfig{RunID: "12", Queues: []string{workerQueue}}
	rec := &CleanupFile{States: []string{stateArchived, stateCompleted}}
	runCleanup(context.Background(), f, cfg, rec)
	assert.ElementsMatch(t, []string{"iso004:12:S01:2", "iso004:12:S01:1"}, f.deleted)
	assert.Equal(t, CleanupCounts{Matched: 2, Deleted: 2}, rec.Counts)
}

func TestCleanupRecordsDeleteFailures(t *testing.T) {
	bundle := makeSealedBundle(t, "12")
	f := &fakeCleaner{
		archived:   []*asynq.TaskInfo{{ID: "iso004:12:S01:1"}, {ID: "iso004:12:S01:2"}},
		failDelete: map[string]bool{"iso004:12:S01:2": true},
	}
	deps := cleanupDeps{Open: func(string) (TaskCleaner, func() error, error) { return f, nil, nil }, Now: time.Now}
	code, _, stderr := runCleanupCmd(t, cleanupArgs("12", bundle, "127.0.0.1:1"), deps)
	assert.Equal(t, exitFail, code, stderr)
	rec := readCleanupFile(t, cleanupJSONPath(bundle, "12"))
	assert.Equal(t, CleanupCounts{Matched: 2, Deleted: 1, Failed: 1}, rec.Counts)
	require.Len(t, rec.Failed, 1)
	assert.Equal(t, "iso004:12:S01:2", rec.Failed[0].ID)
	assert.Contains(t, rec.Failed[0].Error, "stub delete failure")
	raw, err := os.ReadFile(cleanupJSONPath(bundle, "12"))
	require.NoError(t, err)
	assert.True(t, json.Valid(raw))
}
