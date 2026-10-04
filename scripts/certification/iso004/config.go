package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Defaults for the run command. They are referenced by tests so a change here
// is a deliberate, reviewed change to the operator contract.
const (
	defaultReleaseIdentity = "/opt/odyssey-staging/current/RELEASE_IDENTITY"
	defaultWorkerUnit      = "odyssey-staging-worker.service"
	defaultMaxRetry        = 2
	defaultTimeout         = 15 * time.Minute
	defaultPoll            = 2 * time.Second
	defaultSMTPHost        = "127.0.0.1" // internal/app/config.go SMTP_HOST default
	defaultSMTPPort        = "1025"      // internal/app/config.go SMTP_PORT default
	workerQueue            = "default"
	taskIDRoot             = "iso004"
)

var (
	candidateTagRe = regexp.MustCompile(`^v0\.10\.0-rc\.[1-9][0-9]*$`)
	candidateSHARe = regexp.MustCompile(`^[0-9a-f]{40}$`)
	// runIDRe keeps the run ID safe inside a TaskID ("iso004:<run>:...") and
	// inside a Redis SCAN glob: no ':' and no glob metacharacters.
	runIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

// errUsage marks configuration errors (exit code 2).
var errUsage = errors.New("usage error")

// stringList is a repeatable string flag.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	if strings.TrimSpace(v) == "" {
		return errors.New("empty value")
	}
	*s = append(*s, v)
	return nil
}

// Config holds the parsed and validated flags of the run command.
type Config struct {
	RedisAddr       string
	DSN             string
	RunID           string
	OutDir          string
	CandidateTag    string
	CandidateSHA    string
	ReleaseIdentity string
	FixturesFile    string
	Scenarios       []string
	MaxRetry        int
	Timeout         time.Duration
	Poll            time.Duration
	WorkerUnit      string
	WorkerEnv       []string
	MailAPI         string
	AllowEmail      bool
	DryRun          bool
}

// TaskIDPrefix is the prefix every TaskID of this run carries.
func (c *Config) TaskIDPrefix() string { return taskIDRoot + ":" + c.RunID + ":" }

// TaskID builds "iso004:<run-id>:<scenario>:<n>".
func (c *Config) TaskID(scenario string, n int) string {
	return fmt.Sprintf("%s%s:%d", c.TaskIDPrefix(), scenario, n)
}

// parseRunConfig parses the run command's flags. getenv supplies the
// environment defaults so tests do not depend on the process environment.
func parseRunConfig(args []string, getenv func(string) string, stderr io.Writer) (*Config, error) {
	cfg := &Config{}
	var scenarios string
	var workerEnv stringList

	fs := flag.NewFlagSet("iso004", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.RedisAddr, "redis", getenv("REDIS_ADDR"), "staging Redis address (host:port or redis:// URL); default $REDIS_ADDR")
	fs.StringVar(&cfg.DSN, "dsn", firstNonEmpty(getenv("STAGING_CERT_PG_DSN"), getenv("PG_DSN")), "read-only Postgres DSN; default $STAGING_CERT_PG_DSN then $PG_DSN (every pooled connection is forced read-only)")
	fs.StringVar(&cfg.RunID, "run-id", "", "automated certification workflow GITHUB_RUN_ID (required; also the --iso004-key used by the seed)")
	fs.StringVar(&cfg.OutDir, "out", "", "output directory for the evidence bundle (required; must not exist or be empty)")
	fs.StringVar(&cfg.CandidateTag, "candidate-tag", "", "candidate tag, e.g. v0.10.0-rc.9 (required)")
	fs.StringVar(&cfg.CandidateSHA, "candidate-sha", "", "candidate commit, 40 lowercase hex characters (required)")
	fs.StringVar(&cfg.ReleaseIdentity, "release-identity", defaultReleaseIdentity, "RELEASE_IDENTITY file of the deployed worker release")
	fs.StringVar(&cfg.FixturesFile, "fixtures", "", "fixtures JSON from `go run ./scripts/seed/staging --iso004 --json`; STAGING_CERT_ISO004_* env vars are the fallback")
	fs.StringVar(&scenarios, "scenarios", "", "comma-separated scenario IDs (default: all Tier 1 scenarios, including email scenarios)")
	fs.IntVar(&cfg.MaxRetry, "max-retry", defaultMaxRetry, "asynq MaxRetry for every enqueued task")
	fs.DurationVar(&cfg.Timeout, "timeout", defaultTimeout, "overall convergence wait")
	fs.DurationVar(&cfg.Poll, "poll", defaultPoll, "task state poll interval")
	fs.StringVar(&cfg.WorkerUnit, "worker-unit", defaultWorkerUnit, "systemd unit of the staging worker (Environment/EnvironmentFile are inspected for SMTP_HOST/SMTP_PORT)")
	fs.Var(&workerEnv, "worker-env", "environment file to use instead of the worker unit's EnvironmentFile list (repeatable; local rehearsal)")
	fs.StringVar(&cfg.MailAPI, "mail-api", "", "Mailpit/MailHog HTTP API base URL, e.g. http://127.0.0.1:8025 (required with --allow-email)")
	fs.BoolVar(&cfg.AllowEmail, "allow-email", false, "enable email scenarios (still gated by the SMTP sink preflight)")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "print the scenario/enqueue plan without contacting Redis or Postgres")

	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("%w: %w", errUsage, err)
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("%w: unexpected arguments: %s", errUsage, strings.Join(fs.Args(), " "))
	}
	cfg.WorkerEnv = workerEnv
	cfg.Scenarios = splitCSV(scenarios)
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", errUsage, err)
	}
	return cfg, nil
}

func (c *Config) validate() error {
	var errs []error
	if !runIDRe.MatchString(c.RunID) {
		errs = append(errs, fmt.Errorf("--run-id %q must match %s", c.RunID, runIDRe))
	}
	if strings.TrimSpace(c.OutDir) == "" {
		errs = append(errs, errors.New("--out is required"))
	}
	if err := validateCandidateTag(c.CandidateTag); err != nil {
		errs = append(errs, err)
	}
	if !candidateSHARe.MatchString(c.CandidateSHA) {
		errs = append(errs, fmt.Errorf("--candidate-sha %q must be 40 lowercase hex characters", c.CandidateSHA))
	}
	if strings.TrimSpace(c.ReleaseIdentity) == "" {
		errs = append(errs, errors.New("--release-identity must not be empty"))
	}
	if c.MaxRetry < 1 || c.MaxRetry > 25 {
		errs = append(errs, fmt.Errorf("--max-retry %d must be between 1 and 25", c.MaxRetry))
	}
	if c.Timeout <= 0 {
		errs = append(errs, errors.New("--timeout must be positive"))
	}
	if c.Poll <= 0 || c.Poll > c.Timeout {
		errs = append(errs, errors.New("--poll must be positive and not longer than --timeout"))
	}
	if strings.TrimSpace(c.WorkerUnit) == "" && len(c.WorkerEnv) == 0 {
		errs = append(errs, errors.New("--worker-unit or --worker-env is required"))
	}
	if c.AllowEmail && c.MailAPI == "" {
		errs = append(errs, errors.New("--mail-api is required with --allow-email"))
	}
	if c.MailAPI != "" {
		if err := validateMailAPI(c.MailAPI); err != nil {
			errs = append(errs, err)
		}
	}
	if err := validateScenarioSelection(c.Scenarios); err != nil {
		errs = append(errs, err)
	}
	if !c.DryRun {
		if c.RedisAddr == "" {
			errs = append(errs, errors.New("--redis (or REDIS_ADDR) is required"))
		}
		if c.DSN == "" {
			errs = append(errs, errors.New("--dsn (or STAGING_CERT_PG_DSN / PG_DSN) is required"))
		}
	}
	return errors.Join(errs...)
}

func validateCandidateTag(tag string) error {
	if !candidateTagRe.MatchString(tag) {
		return fmt.Errorf("--candidate-tag %q must match %s", tag, candidateTagRe)
	}
	return nil
}

func validateMailAPI(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("--mail-api: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("--mail-api %q must be an http(s) URL with a host", raw)
	}
	if u.User != nil {
		return errors.New("--mail-api must not carry credentials")
	}
	return nil
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// redactedConfig is the credential-free view of Config written to JSON
// output. DSN and Redis address are reduced to host information.
type redactedConfig struct {
	Redis           string        `json:"redis"`
	DSN             string        `json:"dsn"`
	RunID           string        `json:"run_id"`
	OutDir          string        `json:"out"`
	CandidateTag    string        `json:"candidate_tag"`
	CandidateSHA    string        `json:"candidate_sha"`
	ReleaseIdentity string        `json:"release_identity"`
	FixturesFile    string        `json:"fixtures"`
	Scenarios       []string      `json:"scenarios"`
	MaxRetry        int           `json:"max_retry"`
	Timeout         time.Duration `json:"timeout_ns"`
	Poll            time.Duration `json:"poll_ns"`
	WorkerUnit      string        `json:"worker_unit"`
	WorkerEnv       []string      `json:"worker_env"`
	MailAPI         string        `json:"mail_api"`
	AllowEmail      bool          `json:"allow_email"`
	DryRun          bool          `json:"dry_run"`
}

func (c *Config) redacted() redactedConfig {
	return redactedConfig{
		Redis:           redactAddr(c.RedisAddr),
		DSN:             redactAddr(c.DSN),
		RunID:           c.RunID,
		OutDir:          c.OutDir,
		CandidateTag:    c.CandidateTag,
		CandidateSHA:    c.CandidateSHA,
		ReleaseIdentity: c.ReleaseIdentity,
		FixturesFile:    c.FixturesFile,
		Scenarios:       c.Scenarios,
		MaxRetry:        c.MaxRetry,
		Timeout:         c.Timeout,
		Poll:            c.Poll,
		WorkerUnit:      c.WorkerUnit,
		WorkerEnv:       c.WorkerEnv,
		MailAPI:         c.MailAPI,
		AllowEmail:      c.AllowEmail,
		DryRun:          c.DryRun,
	}
}

// redactAddr reduces a DSN or Redis address to scheme://host[:port]/path
// without user info or query parameters. Key/value DSNs ("host=... ") are
// reduced to their host and dbname fields. Anything unparseable is hidden.
func redactAddr(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return "<redacted>"
		}
		return u.Scheme + "://" + u.Host + u.EscapedPath()
	}
	if strings.Contains(raw, "=") {
		var keep []string
		for _, f := range strings.Fields(raw) {
			k, _, _ := strings.Cut(f, "=")
			switch k {
			case "host", "port", "dbname":
				keep = append(keep, f)
			}
		}
		if len(keep) == 0 {
			return "<redacted>"
		}
		return strings.Join(keep, " ")
	}
	if strings.ContainsAny(raw, "@ ") {
		return "<redacted>"
	}
	return raw // plain host:port
}
