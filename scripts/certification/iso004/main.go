// Command iso004 executes certification row ISO-004 (forged, retried and
// duplicate worker inputs) against the staging asynq worker. It enqueues
// tasks into Redis and observes Postgres strictly read-only.
//
// Usage:
//
//	go run ./scripts/certification/iso004 [run] --run-id ... --out DIR ... [--dry-run]
//
// "run" is the default subcommand. Later steps register further subcommands
// (seal, cleanup) in the commands table and the scenario executor through
// runExecutor.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Exit codes.
const (
	exitOK              = 0
	exitFail            = 1 // preflight or scenario failure
	exitUsage           = 2
	exitNoExecutor      = 3 // preflight passed but this build cannot enqueue
	defaultSubcommand   = "run"
	preflightFileName   = "preflight.json"
	noExecutorExplainer = "preflight passed; this build has no scenario executor registered, nothing was enqueued"
)

// command is a subcommand entry point.
type command func(ctx context.Context, args []string, stdout, stderr io.Writer) int

// commands is the subcommand table. Step 6 adds "seal" and "cleanup".
var commands = map[string]command{
	defaultSubcommand: cmdRun,
}

// RunContext is handed to the scenario executor after preflight passed.
type RunContext struct {
	Cfg       *Config
	Preflight *Preflight
	Stdout    io.Writer
	Stderr    io.Writer
	// OpenDB opens the read-only pool (nil: openReadOnlyPool).
	OpenDB func(ctx context.Context, dsn string) (*pgxpool.Pool, error)
	// HTTP is the client for the mail sink API.
	HTTP *http.Client
	// StartedUTC is when the run began (before preflight); run.json records it.
	StartedUTC time.Time
	// ReadFile reads RELEASE_IDENTITY for run.json (nil: os.ReadFile).
	ReadFile func(string) ([]byte, error)
}

// runExecutor enqueues and observes the enabled scenarios. enqueue.go
// registers executeRun from its init function; with nil the run stops after
// preflight with exitNoExecutor.
var runExecutor func(ctx context.Context, rc *RunContext) (exitCode int, err error)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := dispatch(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// dispatch selects the subcommand. Arguments starting with "-" (or none)
// select the default "run" subcommand.
func dispatch(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	name := defaultSubcommand
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name, args = args[0], args[1:]
	}
	cmd, ok := commands[name]
	if !ok {
		names := make([]string, 0, len(commands))
		for n := range commands {
			names = append(names, n)
		}
		sort.Strings(names)
		fmt.Fprintf(stderr, "iso004: unknown subcommand %q (available: %s)\n", name, strings.Join(names, ", "))
		return exitUsage
	}
	return cmd(ctx, args, stdout, stderr)
}

func cmdRun(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runWithDeps(ctx, args, stdout, stderr, defaultPreflightDeps())
}

func runWithDeps(ctx context.Context, args []string, stdout, stderr io.Writer, deps preflightDeps) int {
	cfg, err := parseRunConfig(args, deps.Getenv, stderr)
	if err != nil {
		if errors.Is(err, errUsage) {
			fmt.Fprintf(stderr, "iso004: %v\n", err)
			return exitUsage
		}
		fmt.Fprintf(stderr, "iso004: %v\n", err)
		return exitFail
	}

	if cfg.DryRun {
		return dryRun(cfg, deps, stdout, stderr)
	}

	if err := prepareOutDir(cfg.OutDir); err != nil {
		fmt.Fprintf(stderr, "iso004: %v\n", err)
		return exitUsage
	}
	started := deps.Now().UTC()
	p := runPreflight(ctx, cfg, deps)
	path, err := writePreflight(cfg.OutDir, p)
	if err != nil {
		fmt.Fprintf(stderr, "iso004: %v\n", err)
		return exitFail
	}
	printPreflightSummary(stderr, p, path)
	if !p.Passed() {
		fmt.Fprintf(stderr, "iso004: preflight FAILED; nothing was enqueued (see %s)\n", path)
		return exitFail
	}
	if runExecutor == nil {
		fmt.Fprintf(stderr, "iso004: %s\n", noExecutorExplainer)
		return exitNoExecutor
	}
	code, err := runExecutor(ctx, &RunContext{Cfg: cfg, Preflight: p, Stdout: stdout, Stderr: stderr, OpenDB: deps.OpenDB, HTTP: deps.Email.HTTP, StartedUTC: started, ReadFile: deps.ReadFile})
	if err != nil {
		fmt.Fprintf(stderr, "iso004: %v\n", err)
		if code == exitOK {
			code = exitFail
		}
	}
	return code
}

// dryRun prints the plan. It runs only the static checks (fixtures file/env
// and RELEASE_IDENTITY when readable); it never contacts Redis or Postgres
// and writes no files. Invalid fixtures make the plan incomplete, so they
// exit non-zero; an unreadable RELEASE_IDENTITY is expected off the worker
// host and is reported only.
func dryRun(cfg *Config, deps preflightDeps, stdout, stderr io.Writer) int {
	p := &Preflight{Unmet: map[Requirement]string{}}
	fixturesOK := staticChecks(cfg, deps, p)
	if err := printPlan(stdout, cfg, buildPlan(cfg, p.Fixtures, nil)); err != nil {
		fmt.Fprintf(stderr, "iso004: write plan: %v\n", err)
		return exitFail
	}
	fmt.Fprintln(stdout, "\nstatic checks (no network):")
	for _, c := range p.Checks {
		fmt.Fprintf(stdout, "  %-18s %-4s %s\n", c.Name, c.Status, c.Detail)
	}
	fmt.Fprintln(stdout, "not run in dry-run: redis.*, db.*, email_gate (preflight.json is written only by a real run)")
	if !fixturesOK {
		fmt.Fprintln(stderr, "iso004: dry run: fixtures are incomplete or inconsistent; the plan above uses zero IDs where values are missing")
		return exitFail
	}
	return exitOK
}

func printPreflightSummary(w io.Writer, p *Preflight, path string) {
	fmt.Fprintf(w, "ISO-004 preflight: %s (%s)\n", p.Result, path)
	for _, c := range p.Checks {
		fatal := ""
		if c.Fatal {
			fatal = " [fatal]"
		}
		fmt.Fprintf(w, "  %-26s %-4s%s %s\n", c.Name, c.Status, fatal, c.Detail)
	}
	for _, sp := range p.Scenarios {
		if !sp.Enabled {
			fmt.Fprintf(w, "  scenario %s excluded: %s\n", sp.ID, sp.Excluded)
		}
		for id, why := range sp.Skipped {
			fmt.Fprintf(w, "  task %s skipped: %s\n", id, why)
		}
	}
}
