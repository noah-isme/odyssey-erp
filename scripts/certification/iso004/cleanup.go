package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	"github.com/odyssey-erp/odyssey-erp/internal/platform/cache"
)

// Cleanup subcommand (plan Step 6). `iso004 cleanup --run-id X --out DIR`
// deletes the run's retained tasks (TaskID prefix "iso004:X:") from the
// archived and completed sets so their TaskIDs are freed. It runs only
// against a sealed bundle (DIR/SHA256SUMS exists and verifies, and run.json
// names the same run), never modifies the bundle, and writes its record to
// <parent of DIR>/cleanup/<run-id>/cleanup.json. Fixture rows are left in
// place; this subcommand has no database access.

const (
	cleanupDirName    = "cleanup"
	cleanupFileName   = "cleanup.json"
	cleanupTempPrefix = ".cleanup.json.tmp-*"
	cleanupPageSize   = 500
)

// TaskCleaner lists and deletes retained tasks; *asynq.Inspector satisfies it.
type TaskCleaner interface {
	ListArchivedTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
	ListCompletedTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
	DeleteTask(queue, id string) error
}

// CleanupConfig holds the validated flags of the cleanup command.
type CleanupConfig struct {
	RunID     string
	OutDir    string
	RedisAddr string
	Queues    []string
}

// TaskIDPrefix is the exact prefix of the run's TaskIDs. runIDRe forbids
// ':' so run "12" can never match a TaskID of run "123".
func (c *CleanupConfig) TaskIDPrefix() string { return taskIDRoot + ":" + c.RunID + ":" }

// CleanupTask is one task the cleanup acted on.
type CleanupTask struct {
	Queue string `json:"queue"`
	State string `json:"state"`
	ID    string `json:"id"`
	Error string `json:"error,omitempty"`
}

// CleanupScan records one listing of a queue's set.
type CleanupScan struct {
	Queue   string `json:"queue"`
	State   string `json:"state"`
	Scanned int    `json:"scanned"`
	Matched int    `json:"matched"`
	Error   string `json:"error,omitempty"`
}

// CleanupCounts summarizes the cleanup.
type CleanupCounts struct {
	Matched int `json:"matched"`
	Deleted int `json:"deleted"`
	Failed  int `json:"failed"`
}

// CleanupFile is written to cleanup.json.
type CleanupFile struct {
	Tool             string        `json:"tool"`
	EvidenceID       string        `json:"evidence_id"`
	RunID            string        `json:"run_id"`
	TaskIDPrefix     string        `json:"task_id_prefix"`
	Bundle           string        `json:"bundle"`
	BundleSumsSHA256 string        `json:"bundle_sha256sums_sha256"`
	BundleFiles      int           `json:"bundle_files"`
	Redis            string        `json:"redis"`
	Queues           []string      `json:"queues"`
	States           []string      `json:"states"`
	StartedUTC       time.Time     `json:"started_utc"`
	FinishedUTC      time.Time     `json:"finished_utc"`
	Scans            []CleanupScan `json:"scans"`
	Deleted          []CleanupTask `json:"deleted"`
	Failed           []CleanupTask `json:"failed"`
	Counts           CleanupCounts `json:"counts"`
	FixtureRows      string        `json:"fixture_rows"`
}

// cleanupDeps lets tests replace Redis and the clock.
type cleanupDeps struct {
	// Open returns the cleaner and a close function for the Redis address.
	Open func(addr string) (TaskCleaner, func() error, error)
	Now  func() time.Time
}

func defaultCleanupDeps() cleanupDeps {
	return cleanupDeps{
		Open: func(addr string) (TaskCleaner, func() error, error) {
			opt, err := cache.AsynqOptions(addr)
			if err != nil {
				return nil, nil, fmt.Errorf("parse redis address %s: %w", redactAddr(addr), err)
			}
			insp := asynq.NewInspector(opt)
			return insp, insp.Close, nil
		},
		Now: time.Now,
	}
}

func cmdCleanup(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return cleanupWithDeps(ctx, args, os.Getenv, stdout, stderr, defaultCleanupDeps())
}

func parseCleanupConfig(args []string, getenv func(string) string, stderr io.Writer) (*CleanupConfig, error) {
	cfg := &CleanupConfig{}
	var queues stringList
	fs := flag.NewFlagSet("iso004 cleanup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.RunID, "run-id", "", "run ID of the sealed bundle (required); only TaskIDs starting with iso004:<run-id>: are deleted")
	fs.StringVar(&cfg.OutDir, "out", "", "sealed evidence bundle directory of that run (required; read-only)")
	fs.StringVar(&cfg.RedisAddr, "redis", getenv("REDIS_ADDR"), "staging Redis address (host:port or redis:// URL); default $REDIS_ADDR")
	fs.Var(&queues, "queue", "queue the run used (repeatable; default \""+workerQueue+"\")")
	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("%w: %w", errUsage, err)
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("%w: unexpected arguments: %s", errUsage, strings.Join(fs.Args(), " "))
	}
	cfg.Queues = dedupe(queues)
	if len(cfg.Queues) == 0 {
		cfg.Queues = []string{workerQueue}
	}
	var errs []error
	if !runIDRe.MatchString(cfg.RunID) {
		errs = append(errs, fmt.Errorf("--run-id %q must match %s", cfg.RunID, runIDRe))
	}
	if strings.TrimSpace(cfg.OutDir) == "" {
		errs = append(errs, errors.New("--out is required"))
	}
	if cfg.RedisAddr == "" {
		errs = append(errs, errors.New("--redis (or REDIS_ADDR) is required"))
	}
	for _, q := range cfg.Queues {
		if strings.TrimSpace(q) != q || strings.ContainsAny(q, ":*?[] ") {
			errs = append(errs, fmt.Errorf("--queue %q is not a valid queue name", q))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("%w: %w", errUsage, err)
	}
	return cfg, nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// cleanupOutputDir is <parent of the bundle>/cleanup/<run-id>; it is never
// inside the bundle.
func cleanupOutputDir(bundle, runID string) (string, error) {
	abs, err := filepath.Abs(bundle)
	if err != nil {
		return "", fmt.Errorf("resolve --out %s: %w", bundle, err)
	}
	out := filepath.Join(filepath.Dir(abs), cleanupDirName, runID)
	if rel, err := filepath.Rel(abs, out); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cleanup directory %s would be inside the bundle %s", out, abs)
	}
	return out, nil
}

// checkBundleRun requires the bundle's run.json to name the same run, so a
// sealed bundle of another run cannot authorize this deletion.
func checkBundleRun(dir, runID string) error {
	var rf struct {
		RunID string `json:"run_id"`
	}
	ok, err := readJSONFile(filepath.Join(dir, runFileName), &rf)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("bundle has no %s", runFileName)
	}
	if rf.RunID != runID {
		return fmt.Errorf("bundle %s is for run %q, not --run-id %q", runFileName, rf.RunID, runID)
	}
	return nil
}

func cleanupWithDeps(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer, deps cleanupDeps) int {
	cfg, err := parseCleanupConfig(args, getenv, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "iso004 cleanup: %v\n", err)
		return exitUsage
	}
	if st, err := os.Stat(cfg.OutDir); err != nil || !st.IsDir() {
		fmt.Fprintf(stderr, "iso004 cleanup: refusing: --out %s is not a directory\n", cfg.OutDir)
		return exitFail
	}

	// Refusal paths: nothing is deleted and nothing is written.
	listed, err := verifyBundle(cfg.OutDir)
	if err != nil {
		fmt.Fprintf(stderr, "iso004 cleanup: refusing: bundle is not sealed: %v (run `iso004 seal --out %s` after adding %s)\n", err, cfg.OutDir, workerJournalFileName)
		return exitFail
	}
	if err := checkBundleRun(cfg.OutDir, cfg.RunID); err != nil {
		fmt.Fprintf(stderr, "iso004 cleanup: refusing: %v\n", err)
		return exitFail
	}
	sumsHash, err := fileSHA256(filepath.Join(cfg.OutDir, sha256SumsFileName))
	if err != nil {
		fmt.Fprintf(stderr, "iso004 cleanup: refusing: %v\n", err)
		return exitFail
	}
	outDir, err := cleanupOutputDir(cfg.OutDir, cfg.RunID)
	if err != nil {
		fmt.Fprintf(stderr, "iso004 cleanup: refusing: %v\n", err)
		return exitFail
	}
	outPath := filepath.Join(outDir, cleanupFileName)
	if _, err := os.Stat(outPath); err == nil {
		fmt.Fprintf(stderr, "iso004 cleanup: refusing: %s already exists; move it aside to keep the earlier record\n", outPath)
		return exitFail
	}

	cleaner, closeFn, err := deps.Open(cfg.RedisAddr)
	if err != nil {
		fmt.Fprintf(stderr, "iso004 cleanup: %v\n", err)
		return exitFail
	}
	if closeFn != nil {
		defer func() { _ = closeFn() }()
	}

	abs, _ := filepath.Abs(cfg.OutDir)
	rec := CleanupFile{
		Tool: "iso004 cleanup", EvidenceID: evidenceID, RunID: cfg.RunID, TaskIDPrefix: cfg.TaskIDPrefix(),
		Bundle: abs, BundleSumsSHA256: sumsHash, BundleFiles: len(listed), Redis: redactAddr(cfg.RedisAddr),
		Queues: cfg.Queues, States: []string{stateArchived, stateCompleted}, StartedUTC: deps.Now().UTC(),
		Scans: []CleanupScan{}, Deleted: []CleanupTask{}, Failed: []CleanupTask{},
		FixtureRows: "left in place (staging-only fixtures; this subcommand has no database access)",
	}
	runCleanup(ctx, cleaner, cfg, &rec)
	rec.FinishedUTC = deps.Now().UTC()

	if err := writeCleanupFile(outDir, rec); err != nil {
		fmt.Fprintf(stderr, "iso004 cleanup: %v\n", err)
		return exitFail
	}
	fmt.Fprintf(stdout, "iso004 cleanup: prefix %s: matched %d, deleted %d, failed %d (%s)\n",
		rec.TaskIDPrefix, rec.Counts.Matched, rec.Counts.Deleted, rec.Counts.Failed, outPath)
	scanFailed := false
	for _, s := range rec.Scans {
		if s.Error != "" {
			scanFailed = true
			fmt.Fprintf(stderr, "iso004 cleanup: list %s %s: %s\n", s.Queue, s.State, s.Error)
		}
	}
	if rec.Counts.Failed > 0 || scanFailed {
		return exitFail
	}
	return exitOK
}

// runCleanup lists every set first and deletes afterwards, so deletions do
// not shift the pages being listed.
func runCleanup(ctx context.Context, c TaskCleaner, cfg *CleanupConfig, rec *CleanupFile) {
	prefix := cfg.TaskIDPrefix()
	var targets []CleanupTask
	for _, q := range cfg.Queues {
		for _, state := range rec.States {
			ids, scan := listRunTasks(ctx, c, q, state, prefix)
			rec.Scans = append(rec.Scans, scan)
			for _, id := range ids {
				targets = append(targets, CleanupTask{Queue: q, State: state, ID: id})
			}
		}
	}
	for _, t := range targets {
		if !strings.HasPrefix(t.ID, prefix) { // defense in depth; listRunTasks already filtered
			continue
		}
		if err := ctx.Err(); err != nil {
			t.Error = err.Error()
			rec.Failed = append(rec.Failed, t)
			continue
		}
		if err := c.DeleteTask(t.Queue, t.ID); err != nil {
			t.Error = err.Error()
			rec.Failed = append(rec.Failed, t)
			continue
		}
		rec.Deleted = append(rec.Deleted, t)
	}
	rec.Counts = CleanupCounts{Matched: len(targets), Deleted: len(rec.Deleted), Failed: len(rec.Failed)}
}

func listRunTasks(ctx context.Context, c TaskCleaner, queue, state, prefix string) ([]string, CleanupScan) {
	scan := CleanupScan{Queue: queue, State: state}
	list := c.ListArchivedTasks
	if state == stateCompleted {
		list = c.ListCompletedTasks
	}
	var ids []string
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			scan.Error = err.Error()
			break
		}
		tasks, err := list(queue, asynq.Page(page), asynq.PageSize(cleanupPageSize))
		if errors.Is(err, asynq.ErrQueueNotFound) {
			break
		}
		if err != nil {
			scan.Error = fmt.Sprintf("page %d: %v", page, err)
			break
		}
		scan.Scanned += len(tasks)
		for _, t := range tasks {
			if strings.HasPrefix(t.ID, prefix) {
				ids = append(ids, t.ID)
			}
		}
		if len(tasks) < cleanupPageSize {
			break
		}
	}
	sort.Strings(ids)
	scan.Matched = len(ids)
	return ids, scan
}

func writeCleanupFile(dir string, rec CleanupFile) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", cleanupFileName, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	return writeFileAtomic(dir, cleanupFileName, cleanupTempPrefix, append(data, '\n'))
}
