package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/odyssey-erp/odyssey-erp/internal/platform/cache"
)

// Check statuses.
const (
	statusPass = "pass"
	statusFail = "fail"
	statusSkip = "skip"
	statusWarn = "warn"
)

// Check is one preflight check. A failed Fatal check aborts the run before
// any enqueue; a failed non-fatal check only marks requirements unmet.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Fatal  bool   `json:"fatal"`
	Detail string `json:"detail,omitempty"`
	Data   any    `json:"data,omitempty"`
}

// ServerSummary is the credential-free view of an asynq server.
type ServerSummary struct {
	ID          string         `json:"id"`
	Host        string         `json:"host"`
	PID         int            `json:"pid"`
	Concurrency int            `json:"concurrency"`
	Queues      map[string]int `json:"queues"`
	Status      string         `json:"status"`
	Started     time.Time      `json:"started"`
	Active      int            `json:"active_workers"`
}

// DBSession records the read-only session preflight observed.
type DBSession struct {
	Target                   string `json:"target"`
	Role                     string `json:"role"`
	Database                 string `json:"database"`
	DefaultTransactionReadOn string `json:"default_transaction_read_only"`
	TransactionReadOnly      string `json:"transaction_read_only"`
	InRecovery               bool   `json:"in_recovery"`
}

// FixtureResult is the outcome of one ownership probe.
type FixtureResult struct {
	Name   string `json:"name"`
	Expect string `json:"expect"`
	Got    string `json:"got"`
	OK     bool   `json:"ok"`
}

// Preflight is written to <out>/preflight.json.
type Preflight struct {
	Tool            string                 `json:"tool"`
	SchemaVersion   int                    `json:"schema_version"`
	StartedUTC      time.Time              `json:"started_utc"`
	FinishedUTC     time.Time              `json:"finished_utc"`
	Result          string                 `json:"result"`
	Failures        []string               `json:"failures"`
	Config          redactedConfig         `json:"config"`
	ReleaseIdentity map[string]string      `json:"release_identity,omitempty"`
	Servers         []ServerSummary        `json:"servers,omitempty"`
	Queue           *asynq.QueueInfo       `json:"queue,omitempty"`
	DB              *DBSession             `json:"db,omitempty"`
	Fixtures        *Fixtures              `json:"fixtures,omitempty"`
	FixtureSources  FixtureSource          `json:"fixture_sources,omitempty"`
	FixtureChecks   []FixtureResult        `json:"fixture_checks,omitempty"`
	EmailGate       EmailGate              `json:"email_gate"`
	Unmet           map[Requirement]string `json:"unmet_requirements"`
	Checks          []Check                `json:"checks"`
	Scenarios       []ScenarioPlan         `json:"scenarios"`
}

func (p *Preflight) add(c Check) {
	p.Checks = append(p.Checks, c)
	if c.Status == statusFail && c.Fatal {
		p.Failures = append(p.Failures, c.Name+": "+c.Detail)
	}
}

// Passed reports whether every fatal check passed.
func (p *Preflight) Passed() bool { return len(p.Failures) == 0 }

// preflightDeps are the injectable effects of preflight.
type preflightDeps struct {
	Getenv   func(string) string
	ReadFile func(string) ([]byte, error)
	Email    emailDeps
	Now      func() time.Time
	// OpenDB opens the read-only pool; nil uses openReadOnlyPool.
	OpenDB func(ctx context.Context, dsn string) (*pgxpool.Pool, error)
}

func defaultPreflightDeps() preflightDeps {
	return preflightDeps{
		Getenv:   os.Getenv,
		ReadFile: os.ReadFile,
		Now:      time.Now,
		OpenDB:   openReadOnlyPool,
		Email: emailDeps{
			Run:      execRunner,
			ReadFile: os.ReadFile,
			Lookup:   netDefaultLookup,
			HTTP:     &http.Client{Timeout: 10 * time.Second},
			Timeout:  5 * time.Second,
		},
	}
}

// staticChecks run without network access: fixture loading/validation and
// RELEASE_IDENTITY. They are shared by --dry-run (informational) and the real
// preflight (fatal).
func staticChecks(cfg *Config, d preflightDeps, p *Preflight) bool {
	fixturesOK := true
	fx, src, err := loadFixtures(cfg.FixturesFile, d.Getenv)
	if err != nil {
		p.add(Check{Name: "fixtures.load", Status: statusFail, Fatal: true, Detail: err.Error()})
		fixturesOK = false
	} else {
		p.Fixtures, p.FixtureSources = fx, src
		if problems := validateFixtures(fx, cfg.RunID); len(problems) > 0 {
			p.add(Check{Name: "fixtures.validate", Status: statusFail, Fatal: true, Detail: fmt.Sprintf("%v: %s", errFixtures, strings.Join(problems, "; "))})
			fixturesOK = false
		} else {
			p.add(Check{Name: "fixtures.validate", Status: statusPass, Fatal: true, Detail: fmt.Sprintf("%d fixture values present and consistent", len(fixtureFields()))})
		}
	}

	ident, err := checkReleaseIdentity(cfg.ReleaseIdentity, cfg.CandidateTag, cfg.CandidateSHA, d.ReadFile)
	p.ReleaseIdentity = ident
	if err != nil {
		p.add(Check{Name: "release_identity", Status: statusFail, Fatal: true, Detail: err.Error()})
	} else {
		p.add(Check{Name: "release_identity", Status: statusPass, Fatal: true, Detail: fmt.Sprintf("%s contains tag=%s and commit=%s", cfg.ReleaseIdentity, cfg.CandidateTag, cfg.CandidateSHA)})
	}
	return fixturesOK
}

// parseReleaseIdentity parses key=value lines.
func parseReleaseIdentity(data []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if k, v, ok := strings.Cut(line, "="); ok && k != "" {
			out[k] = v
		}
	}
	return out
}

// checkReleaseIdentity requires the exact lines tag=<tag> and commit=<sha>
// (the same `grep -Fx` rule the deploy and certify workflows use).
func checkReleaseIdentity(path, tag, sha string, readFile func(string) ([]byte, error)) (map[string]string, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, fmt.Errorf("read RELEASE_IDENTITY %s: %w", path, err)
	}
	ident := parseReleaseIdentity(data)
	lines := map[string]bool{}
	for _, l := range strings.Split(string(data), "\n") {
		lines[strings.TrimRight(l, "\r")] = true
	}
	var errs []error
	if !lines["tag="+tag] {
		errs = append(errs, fmt.Errorf("RELEASE_IDENTITY %s has tag=%q, want %q", path, ident["tag"], tag))
	}
	if !lines["commit="+sha] {
		errs = append(errs, fmt.Errorf("RELEASE_IDENTITY %s has commit=%q, want %q", path, ident["commit"], sha))
	}
	return ident, errors.Join(errs...)
}

// runPreflight executes every check, records it, and returns the result. It
// performs only reads: Redis PING/SCAN/inspector reads and SELECT statements
// over a pool whose sessions default to read-only transactions.
func runPreflight(ctx context.Context, cfg *Config, d preflightDeps) *Preflight {
	p := &Preflight{
		Tool:          "iso004",
		SchemaVersion: 1,
		StartedUTC:    d.Now().UTC(),
		Config:        cfg.redacted(),
		Unmet:         map[Requirement]string{},
		Failures:      []string{},
	}
	fixturesOK := staticChecks(cfg, d, p)
	redisChecks(ctx, cfg, p)
	dbChecks(ctx, cfg, d, p, fixturesOK)

	p.EmailGate = evaluateEmailGate(ctx, cfg, d.Email)
	if p.EmailGate.Enabled {
		p.add(Check{Name: "email_gate", Status: statusPass, Detail: "SMTP sink verified by loopback host, banner and API"})
	} else {
		reason := strings.Join(p.EmailGate.Reasons, "; ")
		p.Unmet[ReqEmail] = reason
		status := statusWarn
		if cfg.AllowEmail {
			status = statusFail
		}
		p.add(Check{Name: "email_gate", Status: status, Detail: "email scenarios disabled: " + reason})
	}

	p.Scenarios = buildPlan(cfg, p.Fixtures, p.Unmet)
	p.FinishedUTC = d.Now().UTC()
	if p.Passed() {
		p.Result = "PASS"
	} else {
		p.Result = "FAIL"
	}
	return p
}

func redisChecks(ctx context.Context, cfg *Config, p *Preflight) {
	opt, err := cache.AsynqOptions(cfg.RedisAddr)
	if err != nil {
		p.add(Check{Name: "redis.config", Status: statusFail, Fatal: true, Detail: fmt.Sprintf("parse redis address %s: %v", redactAddr(cfg.RedisAddr), err)})
		return
	}
	client, ok := opt.MakeRedisClient().(redis.UniversalClient)
	if !ok {
		p.add(Check{Name: "redis.config", Status: statusFail, Fatal: true, Detail: "unexpected redis client type"})
		return
	}
	defer client.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = client.Ping(pingCtx).Err()
	cancel()
	if err != nil {
		p.add(Check{Name: "redis.ping", Status: statusFail, Fatal: true, Detail: fmt.Sprintf("redis %s unreachable: %v", redactAddr(cfg.RedisAddr), err)})
		return
	}
	p.add(Check{Name: "redis.ping", Status: statusPass, Fatal: true, Detail: "redis " + redactAddr(cfg.RedisAddr) + " reachable"})

	insp := asynq.NewInspectorFromRedisClient(client)
	servers, err := insp.Servers()
	if err != nil {
		p.add(Check{Name: "redis.servers", Status: statusFail, Fatal: true, Detail: fmt.Sprintf("list asynq servers: %v", err)})
	} else {
		consuming := 0
		for _, s := range servers {
			p.Servers = append(p.Servers, ServerSummary{ID: s.ID, Host: s.Host, PID: s.PID, Concurrency: s.Concurrency, Queues: s.Queues, Status: s.Status, Started: s.Started.UTC(), Active: len(s.ActiveWorkers)})
			if s.Queues[workerQueue] > 0 && s.Status == "active" {
				consuming++
			}
		}
		sort.Slice(p.Servers, func(i, j int) bool { return p.Servers[i].ID < p.Servers[j].ID })
		if consuming == 0 {
			p.add(Check{Name: "redis.servers", Status: statusFail, Fatal: true, Detail: fmt.Sprintf("no active asynq server consumes queue %q (%d servers listed)", workerQueue, len(servers))})
		} else {
			p.add(Check{Name: "redis.servers", Status: statusPass, Fatal: true, Detail: fmt.Sprintf("%d active server(s) consume queue %q", consuming, workerQueue)})
		}
	}

	qi, err := insp.GetQueueInfo(workerQueue)
	switch {
	case err != nil && strings.Contains(err.Error(), "does not exist"):
		p.add(Check{Name: "redis.queue_info", Status: statusPass, Fatal: true, Detail: fmt.Sprintf("queue %q has no tasks yet (not created)", workerQueue)})
	case err != nil:
		p.add(Check{Name: "redis.queue_info", Status: statusFail, Fatal: true, Detail: fmt.Sprintf("queue info %q: %v", workerQueue, err)})
	default:
		p.Queue = qi
		p.add(Check{Name: "redis.queue_info", Status: statusPass, Fatal: true, Detail: fmt.Sprintf("queue %q snapshot: size=%d pending=%d active=%d retry=%d archived=%d completed=%d paused=%t", workerQueue, qi.Size, qi.Pending, qi.Active, qi.Retry, qi.Archived, qi.Completed, qi.Paused)})
		if qi.Paused {
			p.add(Check{Name: "redis.queue_paused", Status: statusFail, Fatal: true, Detail: fmt.Sprintf("queue %q is paused; tasks would never converge", workerQueue)})
		}
	}

	ids, err := existingRunTaskKeys(ctx, client, cfg.TaskIDPrefix())
	switch {
	case err != nil:
		p.add(Check{Name: "redis.run_prefix_unused", Status: statusFail, Fatal: true, Detail: err.Error()})
	case len(ids) > 0:
		p.add(Check{Name: "redis.run_prefix_unused", Status: statusFail, Fatal: true, Detail: fmt.Sprintf("%d task(s) with prefix %q already exist; use a new run ID", len(ids), cfg.TaskIDPrefix()), Data: ids})
	default:
		p.add(Check{Name: "redis.run_prefix_unused", Status: statusPass, Fatal: true, Detail: fmt.Sprintf("no task with prefix %q in any queue", cfg.TaskIDPrefix())})
	}
}

// existingRunTaskKeys scans asynq task hashes ("asynq:{<queue>}:t:<id>") of
// every queue for IDs starting with prefix. prefix is built from a validated
// run ID and contains no glob metacharacters.
func existingRunTaskKeys(ctx context.Context, client redis.UniversalClient, prefix string) ([]string, error) {
	if strings.ContainsAny(prefix, "*?[]\\") {
		return nil, fmt.Errorf("task ID prefix %q contains glob metacharacters", prefix)
	}
	pattern := "asynq:{*}:t:" + prefix + "*"
	var out []string
	var cursor uint64
	for {
		keys, next, err := client.Scan(ctx, cursor, pattern, 1000).Result()
		if err != nil {
			return nil, fmt.Errorf("scan redis for run prefix: %w", err)
		}
		out = append(out, keys...)
		if next == 0 {
			break
		}
		cursor = next
	}
	sort.Strings(out)
	return out, nil
}

// openReadOnlyPool opens a pool whose every connection starts with
// default_transaction_read_only=on as a startup parameter, so no statement on
// any pooled connection can write, even without an explicit transaction.
func openReadOnlyPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pc, err := readOnlyPoolConfig(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("open read-only pool: %w", err)
	}
	return pool, nil
}

func readOnlyPoolConfig(dsn string) (*pgxpool.Config, error) {
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// pgx redacts the password in parse errors; the DSN itself is not echoed.
		return nil, fmt.Errorf("parse DSN %s: %w", redactAddr(dsn), err)
	}
	if pc.ConnConfig.RuntimeParams == nil {
		pc.ConnConfig.RuntimeParams = map[string]string{}
	}
	pc.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pc.ConnConfig.RuntimeParams["application_name"] = "iso004-certification"
	pc.MaxConns = 2
	return pc, nil
}

// queryRower is the subset of pgxpool.Pool the checks need.
type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func dbChecks(ctx context.Context, cfg *Config, d preflightDeps, p *Preflight, fixturesOK bool) {
	open := d.OpenDB
	if open == nil {
		open = openReadOnlyPool
	}
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pool, err := open(connectCtx, cfg.DSN)
	if err != nil {
		p.add(Check{Name: "db.connect", Status: statusFail, Fatal: true, Detail: err.Error()})
		return
	}
	defer pool.Close()

	sess, err := readOnlySession(connectCtx, pool)
	if err != nil {
		p.add(Check{Name: "db.connect", Status: statusFail, Fatal: true, Detail: err.Error()})
		return
	}
	sess.Target = redactAddr(cfg.DSN)
	p.DB = sess
	p.add(Check{Name: "db.connect", Status: statusPass, Fatal: true, Detail: fmt.Sprintf("connected as role %q to database %q", sess.Role, sess.Database)})
	if sess.DefaultTransactionReadOn != "on" || sess.TransactionReadOnly != "on" {
		p.add(Check{Name: "db.read_only", Status: statusFail, Fatal: true, Detail: fmt.Sprintf("session is not read-only (default_transaction_read_only=%s, transaction_read_only=%s)", sess.DefaultTransactionReadOn, sess.TransactionReadOnly)})
		return
	}
	p.add(Check{Name: "db.read_only", Status: statusPass, Fatal: true, Detail: "default_transaction_read_only=on on a fresh pooled connection"})

	if !fixturesOK || p.Fixtures == nil {
		p.add(Check{Name: "db.fixtures", Status: statusSkip, Fatal: true, Detail: "fixture validation failed; ownership probes skipped"})
		return
	}
	fx := p.Fixtures
	results, err := verifyFixtures(ctx, pool, fixtureChecks(fx))
	p.FixtureChecks = results
	if err != nil {
		p.add(Check{Name: "db.fixtures", Status: statusFail, Fatal: true, Detail: err.Error()})
	} else {
		p.add(Check{Name: "db.fixtures", Status: statusPass, Fatal: true, Detail: fmt.Sprintf("%d fixture ownership probes matched", len(results))})
	}

	var connectors int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM connector_connections WHERE company_id = ANY($1::bigint[])`, []int64{fx.CompanyA, fx.CompanyB}).Scan(&connectors); err != nil {
		p.add(Check{Name: "db.connector_connections", Status: statusFail, Fatal: true, Detail: fmt.Sprintf("count connector_connections: %v", err)})
	} else if connectors > 0 {
		p.Unmet[ReqNoConnectorConnections] = fmt.Sprintf("%d connector_connections row(s) for companies %d/%d", connectors, fx.CompanyA, fx.CompanyB)
		p.add(Check{Name: "db.connector_connections", Status: statusFail, Detail: p.Unmet[ReqNoConnectorConnections] + "; connector scenarios excluded", Data: connectors})
	} else {
		p.add(Check{Name: "db.connector_connections", Status: statusPass, Detail: "no connector_connections for A or B", Data: connectors})
	}

	var globalPolicies int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM ap_matching_policies WHERE company_id IS NULL AND supplier_id IS NULL AND effective_from <= CURRENT_DATE AND (effective_to IS NULL OR effective_to >= CURRENT_DATE)`).Scan(&globalPolicies); err != nil {
		p.add(Check{Name: "db.global_ap_policy", Status: statusFail, Fatal: true, Detail: fmt.Sprintf("count global AP matching policies: %v", err)})
	} else if globalPolicies > 0 {
		p.Unmet[ReqNoGlobalAPPolicy] = fmt.Sprintf("%d active global AP matching policy row(s)", globalPolicies)
		p.add(Check{Name: "db.global_ap_policy", Status: statusFail, Detail: p.Unmet[ReqNoGlobalAPPolicy] + "; S09 MISSING_MAPPING path is FAIL-by-precondition", Data: globalPolicies})
	} else {
		p.add(Check{Name: "db.global_ap_policy", Status: statusPass, Detail: "no active global AP matching policy", Data: globalPolicies})
	}
}

// readOnlySession records the session settings on a freshly acquired
// connection. transaction_read_only is read inside the implicit transaction
// of the statement, proving the default applies.
func readOnlySession(ctx context.Context, pool *pgxpool.Pool) (*DBSession, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()
	s := &DBSession{}
	if err := conn.QueryRow(ctx, `SHOW default_transaction_read_only`).Scan(&s.DefaultTransactionReadOn); err != nil {
		return nil, fmt.Errorf("show default_transaction_read_only: %w", err)
	}
	if err := conn.QueryRow(ctx, `SELECT current_user::text, current_database()::text, current_setting('transaction_read_only'), pg_is_in_recovery()`).Scan(&s.Role, &s.Database, &s.TransactionReadOnly, &s.InRecovery); err != nil {
		return nil, fmt.Errorf("read session identity: %w", err)
	}
	return s, nil
}

// verifyFixtures runs every ownership probe and reports all mismatches.
func verifyFixtures(ctx context.Context, q queryRower, checks []fixtureCheck) ([]FixtureResult, error) {
	var results []FixtureResult
	var errs []error
	for _, c := range checks {
		r := FixtureResult{Name: c.Name, Expect: c.Expect}
		var got *string
		err := q.QueryRow(ctx, c.SQL, c.Args...).Scan(&got)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			r.Got = "<missing>"
		case err != nil:
			return results, fmt.Errorf("fixture probe %s: %w", c.Name, err)
		case got == nil:
			r.Got = "<null>"
		default:
			r.Got = *got
		}
		r.OK = r.Got == r.Expect
		if !r.OK {
			errs = append(errs, fmt.Errorf("%s: got %q, want %q", c.Name, r.Got, r.Expect))
		}
		results = append(results, r)
	}
	if len(errs) > 0 {
		return results, fmt.Errorf("fixture ownership mismatch: %w", errors.Join(errs...))
	}
	return results, nil
}

// writePreflight writes preflight.json into dir.
func writePreflight(dir string, p *Preflight) (string, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode preflight.json: %w", err)
	}
	path := filepath.Join(dir, "preflight.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("write preflight.json: %w", err)
	}
	return path, nil
}

// prepareOutDir creates dir, refusing an existing non-empty directory so an
// earlier run's evidence is never overwritten.
func prepareOutDir(dir string) error {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create --out %s: %w", dir, err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("read --out %s: %w", dir, err)
	case len(entries) > 0:
		return fmt.Errorf("--out %s is not empty; never overwrite an earlier run's evidence", dir)
	}
	return nil
}
