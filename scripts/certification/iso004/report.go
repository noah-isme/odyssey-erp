package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/hibiken/asynq"
)

// Output bundle (plan Step 5). After the executor finished, writeBundle adds
// run.json, summary.json, summary.md, archived-tasks.json, candidate.txt and
// evidence-record.log to --out and seals everything with SHA256SUMS. The
// evidence record is the single line the operator appends to the operator
// candidate's staging-certification.log; scripts/staging-certification-
// evidence.sh parses it (see evidence_record_test.go).

const (
	evidenceID            = "ISO-004"
	evidenceBundleDir     = "iso-004-worker-injection"
	evidenceRecordPrefix  = "CERTIFICATION_EVIDENCE evidence_id=" + evidenceID + " "
	runFileName           = "run.json"
	summaryJSONFileName   = "summary.json"
	summaryMDFileName     = "summary.md"
	archivedTasksFileName = "archived-tasks.json"
	candidateFileName     = "candidate.txt"
	evidenceRecordName    = "evidence-record.log"
	sha256SumsFileName    = "SHA256SUMS"
	collectedUTCLayout    = "2006-01-02T15:04:05Z"
	archivedPageSize      = 500
)

// ArchivedLister lists archived tasks; *asynq.Inspector satisfies it.
type ArchivedLister interface {
	ListArchivedTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
}

// Bundle is everything writeBundle needs. Records come from Executor.Run and
// ExitCode is its exit code; the bundle never turns a failing run into PASS.
type Bundle struct {
	Cfg        *Config
	Preflight  *Preflight
	Records    []*EnqueueRecord
	ExitCode   int
	RunErr     error
	StartedUTC time.Time
	Archived   ArchivedLister
	// GitDescribe returns the tool's version; nil uses gitDescribe.
	GitDescribe func() (string, error)
	// ReadFile reads RELEASE_IDENTITY; nil uses os.ReadFile.
	ReadFile func(string) ([]byte, error)
	Now      func() time.Time
}

func (b *Bundle) now() time.Time {
	if b.Now != nil {
		return b.Now().UTC()
	}
	return time.Now().UTC()
}

// EvidenceRecord is the JSON payload of the CERTIFICATION_EVIDENCE line. The
// field order is the order the plan documents.
type EvidenceRecord struct {
	EvidenceID   string `json:"evidence_id"`
	Result       string `json:"result"`
	RunID        string `json:"run_id"`
	CollectedUTC string `json:"collected_utc"`
	Details      string `json:"details"`
}

// ToolInfo identifies the tool build that produced the bundle.
type ToolInfo struct {
	Name             string `json:"name"`
	GitDescribe      string `json:"git_describe"`
	GitDescribeError string `json:"git_describe_error,omitempty"`
	VCSRevision      string `json:"vcs_revision,omitempty"`
	VCSModified      string `json:"vcs_modified,omitempty"`
	GoVersion        string `json:"go_version"`
}

// ReleaseIdentityRecord is the worker host's RELEASE_IDENTITY as read at the
// end of the run (preflight already required tag/commit to match).
type ReleaseIdentityRecord struct {
	Path      string            `json:"path"`
	Contents  string            `json:"contents,omitempty"`
	Parsed    map[string]string `json:"parsed,omitempty"`
	ReadError string            `json:"read_error,omitempty"`
}

// MailTarget records the SMTP endpoint and mail API the run used.
type MailTarget struct {
	EmailRequested bool        `json:"email_requested"`
	EmailEnabled   bool        `json:"email_enabled"`
	GateReasons    []string    `json:"gate_reasons,omitempty"`
	EnvSource      string      `json:"env_source,omitempty"`
	SMTP           *SMTPTarget `json:"smtp,omitempty"`
	MailAPI        string      `json:"mail_api,omitempty"`
	MailAPISink    string      `json:"mail_api_sink,omitempty"`
	MailAPIProbe   string      `json:"mail_api_endpoint,omitempty"`
}

// RunFile is written to run.json.
type RunFile struct {
	Tool            string                `json:"tool"`
	SchemaVersion   int                   `json:"schema_version"`
	EvidenceID      string                `json:"evidence_id"`
	Build           ToolInfo              `json:"build"`
	Config          redactedConfig        `json:"config"`
	CandidateTag    string                `json:"candidate_tag"`
	CandidateSHA    string                `json:"candidate_sha"`
	ReleaseIdentity ReleaseIdentityRecord `json:"release_identity"`
	Fixtures        *Fixtures             `json:"fixtures,omitempty"`
	Servers         []ServerSummary       `json:"worker_servers"`
	Mail            MailTarget            `json:"mail"`
	TaskIDPrefix    string                `json:"task_id_prefix"`
	StartedUTC      time.Time             `json:"started_utc"`
	FinishedUTC     time.Time             `json:"finished_utc"`
	Result          string                `json:"result"`
	ExitCode        int                   `json:"exit_code"`
	Errors          []string              `json:"errors"`
}

// SummaryAssertion is one assertion outcome in a summary row.
type SummaryAssertion struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// SummaryRow is one scenario in summary.json.
type SummaryRow struct {
	Scenario     string             `json:"scenario"`
	Title        string             `json:"title"`
	Result       string             `json:"result"`
	Reasons      []string           `json:"reasons"`
	TaskIDs      []string           `json:"task_ids"`
	FinalStates  map[string]string  `json:"final_states"`
	Assertions   []SummaryAssertion `json:"assertions"`
	Observations []string           `json:"observations"`
	Timing       string             `json:"timing"`
}

// SummaryFile is written to summary.json.
type SummaryFile struct {
	EvidenceID   string       `json:"evidence_id"`
	RunID        string       `json:"run_id"`
	CandidateTag string       `json:"candidate_tag"`
	CandidateSHA string       `json:"candidate_sha"`
	TaskIDPrefix string       `json:"task_id_prefix"`
	Result       string       `json:"result"`
	Reasons      []string     `json:"reasons"`
	Deferred     []string     `json:"deferred"`
	BranchScope  string       `json:"branch_scope"`
	Timing       string       `json:"timing"`
	Scenarios    []SummaryRow `json:"scenarios"`
}

// ArchivedTasksFile is written to archived-tasks.json.
type ArchivedTasksFile struct {
	Queue        string            `json:"queue"`
	TaskIDPrefix string            `json:"task_id_prefix"`
	ListedUTC    time.Time         `json:"listed_utc"`
	Scanned      int               `json:"scanned"`
	Error        string            `json:"error,omitempty"`
	Tasks        []*TaskInfoRecord `json:"tasks"`
}

// writeBundle writes the run-level files and SHA256SUMS. It returns the
// evidence result (PASS/FAIL) and the run's final exit code: any FAIL
// becomes non-zero, and a bundle that could not be written fully is FAIL.
func writeBundle(ctx context.Context, b *Bundle) (string, int, error) {
	cfg := b.Cfg
	dir := cfg.OutDir
	var errs []error

	archived := listRunArchived(ctx, b.Archived, cfg.TaskIDPrefix(), b.now())
	if err := writeJSONFile(filepath.Join(dir, archivedTasksFileName), archived); err != nil {
		errs = append(errs, err)
	}

	rows := summaryRows(dir, b.Records, cfg.MaxRetry)
	branch, branchErr := branchNAStatement(cfg.CandidateSHA)
	if branchErr != nil {
		branch = "branch: claim does not hold: " + branchErr.Error()
	}
	reasons := overallReasons(b, rows, archived, branchErr)
	result := resultPass
	if len(reasons) > 0 {
		result = resultFail
	}

	summary := SummaryFile{
		EvidenceID: evidenceID, RunID: cfg.RunID, CandidateTag: cfg.CandidateTag, CandidateSHA: cfg.CandidateSHA,
		TaskIDPrefix: cfg.TaskIDPrefix(), Result: result, Reasons: reasons,
		Deferred: append([]string{}, deferredScenarios...), BranchScope: branch,
		Timing: timingFidelity(cfg.MaxRetry).Note, Scenarios: rows,
	}
	if err := writeJSONFile(filepath.Join(dir, summaryJSONFileName), summary); err != nil {
		errs = append(errs, err)
	}
	if err := os.WriteFile(filepath.Join(dir, summaryMDFileName), []byte(renderSummaryMD(summary)), 0o644); err != nil {
		errs = append(errs, fmt.Errorf("write %s: %w", summaryMDFileName, err))
	}
	if err := os.WriteFile(filepath.Join(dir, candidateFileName), []byte(candidateTxt(cfg)), 0o644); err != nil {
		errs = append(errs, fmt.Errorf("write %s: %w", candidateFileName, err))
	}

	// A bundle that is missing files is never PASS. The run file and the
	// record are written after this point and carry the final result.
	if len(errs) > 0 {
		result = resultFail
	}
	finished := b.now()
	runFile := b.runFile(result, finished)
	runFile.Errors = append(runFile.Errors, reasons...)
	for _, e := range errs {
		runFile.Errors = append(runFile.Errors, e.Error())
	}
	if err := writeJSONFile(filepath.Join(dir, runFileName), runFile); err != nil {
		errs = append(errs, err)
		result = resultFail
	}

	rec := EvidenceRecord{
		EvidenceID: evidenceID, Result: result, RunID: cfg.RunID,
		CollectedUTC: finished.Format(collectedUTCLayout),
		Details:      evidenceDetails(b, rows, branch, archived),
	}
	line, err := evidenceLine(rec)
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, evidenceRecordName), []byte(line+"\n"), 0o644)
	}
	if err != nil {
		errs = append(errs, fmt.Errorf("write %s: %w", evidenceRecordName, err))
		result = resultFail
	}
	if err := writeSHA256SUMS(dir); err != nil {
		errs = append(errs, err)
		result = resultFail
	}

	code := b.ExitCode
	if result != resultPass && code == exitOK {
		code = exitFail
	}
	return result, code, errors.Join(errs...)
}

// notRunScenarios returns the Tier 1 scenario IDs, in registry order, that
// have no row: the scenarios a --scenarios subset left out.
func notRunScenarios(rows []SummaryRow) []string {
	ran := make(map[string]bool, len(rows))
	for _, r := range rows {
		ran[r.Scenario] = true
	}
	var missing []string
	for _, id := range scenarioIDs() {
		if !ran[id] {
			missing = append(missing, id)
		}
	}
	return missing
}

// subsetReason says why a run that left Tier 1 scenarios out is never PASS.
// A subset is a rehearsal: ISO-004 is certified only by one run of every Tier
// 1 scenario, so the record is FAIL and names what was not run.
func subsetReason(missing []string) string {
	return fmt.Sprintf("rehearsal/subset run, not certification evidence: %d of %d Tier 1 scenarios not run: %s",
		len(missing), len(scenarioRegistry), strings.Join(missing, ", "))
}

// overallReasons lists why the ISO-004 record is not PASS; empty means PASS.
func overallReasons(b *Bundle, rows []SummaryRow, archived ArchivedTasksFile, branchErr error) []string {
	reasons := []string{}
	if len(rows) == 0 {
		reasons = append(reasons, "no scenario was executed")
	} else if missing := notRunScenarios(rows); len(missing) > 0 {
		reasons = append(reasons, subsetReason(missing))
	}
	for _, r := range rows {
		if r.Result != resultPass {
			reasons = append(reasons, fmt.Sprintf("scenario %s is %s", r.Scenario, r.Result))
		}
	}
	if b.ExitCode != exitOK {
		reasons = append(reasons, fmt.Sprintf("executor exit code %d", b.ExitCode))
	}
	if b.RunErr != nil {
		reasons = append(reasons, "executor error: "+b.RunErr.Error())
	}
	if archived.Error != "" {
		reasons = append(reasons, "archived task listing failed: "+archived.Error)
	}
	if branchErr != nil {
		reasons = append(reasons, branchErr.Error())
	}
	return reasons
}

func (b *Bundle) runFile(result string, finished time.Time) RunFile {
	cfg := b.Cfg
	rf := RunFile{
		Tool: "iso004", SchemaVersion: 1, EvidenceID: evidenceID,
		Build: toolInfo(b.GitDescribe), Config: cfg.redacted(),
		CandidateTag: cfg.CandidateTag, CandidateSHA: cfg.CandidateSHA,
		ReleaseIdentity: readReleaseIdentity(cfg.ReleaseIdentity, b.ReadFile),
		Servers:         []ServerSummary{}, TaskIDPrefix: cfg.TaskIDPrefix(),
		StartedUTC: b.StartedUTC.UTC(), FinishedUTC: finished, Result: result,
		Mail: MailTarget{EmailRequested: cfg.AllowEmail, MailAPI: cfg.MailAPI}, Errors: []string{},
	}
	if p := b.Preflight; p != nil {
		rf.Fixtures = p.Fixtures
		rf.Servers = append(rf.Servers, p.Servers...)
		g := p.EmailGate
		rf.Mail.EmailEnabled, rf.Mail.GateReasons, rf.Mail.EnvSource, rf.Mail.SMTP = g.Enabled, g.Reasons, g.EnvSource, g.SMTP
		if g.Probe != nil {
			rf.Mail.MailAPISink, rf.Mail.MailAPIProbe = g.Probe.APISink, g.Probe.APIEndpoint
		}
	}
	rf.ExitCode = b.ExitCode
	if result != resultPass && rf.ExitCode == exitOK {
		rf.ExitCode = exitFail
	}
	return rf
}

func readReleaseIdentity(path string, readFile func(string) ([]byte, error)) ReleaseIdentityRecord {
	if readFile == nil {
		readFile = os.ReadFile
	}
	rec := ReleaseIdentityRecord{Path: path}
	data, err := readFile(path)
	if err != nil {
		rec.ReadError = err.Error()
		return rec
	}
	rec.Contents = string(data)
	rec.Parsed = parseReleaseIdentity(data)
	return rec
}

// toolInfo records the tool's version. git is optional: a binary copied to
// the worker host has no worktree, so the build info is recorded as well.
func toolInfo(describe func() (string, error)) ToolInfo {
	if describe == nil {
		describe = gitDescribe
	}
	ti := ToolInfo{Name: "scripts/certification/iso004", GoVersion: runtime.Version()}
	if v, err := describe(); err != nil {
		ti.GitDescribe, ti.GitDescribeError = "unknown", err.Error()
	} else {
		ti.GitDescribe = v
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				ti.VCSRevision = s.Value
			case "vcs.modified":
				ti.VCSModified = s.Value
			}
		}
	}
	return ti
}

// gitDescribe runs `git describe` in the working directory.
func gitDescribe() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "describe", "--tags", "--always", "--dirty", "--abbrev=40").Output()
	if err != nil {
		return "", fmt.Errorf("git describe: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// listRunArchived lists the queue's archived tasks and keeps those with the
// run's TaskID prefix. A missing queue is an empty list, not an error.
func listRunArchived(ctx context.Context, l ArchivedLister, prefix string, now time.Time) ArchivedTasksFile {
	f := ArchivedTasksFile{Queue: workerQueue, TaskIDPrefix: prefix, ListedUTC: now, Tasks: []*TaskInfoRecord{}}
	if l == nil {
		f.Error = "no inspector available"
		return f
	}
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			f.Error = err.Error()
			return f
		}
		tasks, err := l.ListArchivedTasks(workerQueue, asynq.Page(page), asynq.PageSize(archivedPageSize))
		if errors.Is(err, asynq.ErrQueueNotFound) {
			break
		}
		if err != nil {
			f.Error = fmt.Sprintf("list archived tasks (page %d): %v", page, err)
			return f
		}
		f.Scanned += len(tasks)
		for _, t := range tasks {
			if strings.HasPrefix(t.ID, prefix) {
				f.Tasks = append(f.Tasks, taskInfoRecord(t))
			}
		}
		if len(tasks) < archivedPageSize {
			break
		}
	}
	sort.Slice(f.Tasks, func(i, j int) bool { return f.Tasks[i].ID < f.Tasks[j].ID })
	return f
}

// summaryRows builds one row per executed scenario from its record and the
// assertions.json the evaluator wrote.
func summaryRows(dir string, records []*EnqueueRecord, maxRetry int) []SummaryRow {
	rows := make([]SummaryRow, 0, len(records))
	timing := timingFidelity(maxRetry).Note
	for _, r := range records {
		row := SummaryRow{
			Scenario: r.Scenario, Title: r.Title, Result: "NOT EVALUATED", Reasons: []string{},
			TaskIDs: []string{}, FinalStates: map[string]string{}, Assertions: []SummaryAssertion{},
			Observations: append([]string{}, r.Plan.Observations...), Timing: timing,
		}
		if res := r.Result; res != nil {
			row.Result = res.Result
			row.Reasons = append(row.Reasons, res.Reasons...)
			row.TaskIDs = append(row.TaskIDs, res.TaskIDs...)
			for k, v := range res.FinalStates {
				row.FinalStates[k] = v
			}
			if res.Timing.Note != "" {
				row.Timing = res.Timing.Note
			}
		} else {
			row.Reasons = append(row.Reasons, "scenario was not evaluated")
			seen := map[string]bool{}
			for _, s := range r.Submissions {
				if !seen[s.TaskID] {
					seen[s.TaskID] = true
					row.TaskIDs = append(row.TaskIDs, s.TaskID)
				}
			}
		}
		var af AssertionsFile
		ok, err := readJSONFile(filepath.Join(dir, "scenarios", r.Scenario, assertionsFileName), &af)
		switch {
		case err != nil:
			row.Reasons = append(row.Reasons, err.Error())
			if row.Result == resultPass {
				row.Result = resultFail
			}
		case ok:
			for _, a := range af.Assertions {
				row.Assertions = append(row.Assertions, SummaryAssertion{ID: a.ID, Status: a.Status, Reason: a.Reason})
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func candidateTxt(cfg *Config) string {
	return fmt.Sprintf("candidate=%s\ntag=%s\nsha=%s\nexpected_sha=%s\n", cfg.CandidateTag, cfg.CandidateTag, cfg.CandidateSHA, cfg.CandidateSHA)
}

// evidenceDetails is the single-line details string of the record. It
// starts with the reviewed-by clause the plan prescribes.
func evidenceDetails(b *Bundle, rows []SummaryRow, branch string, archived ArchivedTasksFile) string {
	cfg := b.Cfg
	reviewer := cfg.Reviewer
	if strings.TrimSpace(reviewer) == "" {
		reviewer = defaultReviewer
	}
	parts := []string{
		fmt.Sprintf("%s/%s reviewed by %s", evidenceBundleDir, summaryJSONFileName, reviewer),
		fmt.Sprintf("candidate %s sha %s", cfg.CandidateTag, cfg.CandidateSHA),
	}
	scen := make([]string, 0, len(rows))
	for _, r := range rows {
		scen = append(scen, r.Scenario+"="+r.Result)
	}
	if len(scen) == 0 {
		scen = append(scen, "none")
	}
	selection := "all Tier 1"
	missing := notRunScenarios(rows)
	if len(missing) > 0 {
		selection = fmt.Sprintf("subset %d of %d Tier 1", len(rows), len(scenarioRegistry))
	}
	parts = append(parts, fmt.Sprintf("scenarios (%s): %s", selection, strings.Join(scen, ", ")))
	if len(rows) > 0 && len(missing) > 0 {
		parts = append(parts, "REHEARSAL/SUBSET RUN, NOT CERTIFICATION EVIDENCE (result is never PASS); scenarios not run: "+strings.Join(missing, ", "))
	}
	parts = append(parts,
		"task ID prefix "+cfg.TaskIDPrefix(),
		fmt.Sprintf("archived tasks with prefix: %d", len(archived.Tasks)),
		"fixtures "+fixtureSummary(b.Preflight),
		"worker "+workerSummary(b.Preflight),
		"deferred (not executed, recorded as deferred, not N/A): "+strings.Join(deferredScenarios, ", "),
		branch,
		fmt.Sprintf("timing: MaxRetry(%d) vs production %d-%d, same retry/dead-letter path", cfg.MaxRetry, productionMaxRetryMin, productionMaxRetryMax),
	)
	return oneLine(strings.Join(parts, "; "))
}

func fixtureSummary(p *Preflight) string {
	if p == nil || p.Fixtures == nil {
		return "unavailable"
	}
	var out []string
	for _, f := range fixtureFields() {
		name := strings.ToLower(strings.TrimPrefix(f.Name, "STAGING_CERT_ISO004_"))
		var v string
		switch {
		case f.int != nil:
			v = fmt.Sprint(*f.int(p.Fixtures))
		case f.str != nil:
			v = *f.str(p.Fixtures)
		}
		if strings.HasSuffix(name, "_email") {
			continue // a recipient address is not an ID; run.json keeps it
		}
		out = append(out, name+"="+v)
	}
	return strings.Join(out, " ")
}

func workerSummary(p *Preflight) string {
	if p == nil || len(p.Servers) == 0 {
		return "unknown"
	}
	var out []string
	for _, s := range p.Servers {
		if s.Queues[workerQueue] > 0 {
			out = append(out, fmt.Sprintf("%s pid %d concurrency %d", s.Host, s.PID, s.Concurrency))
		}
	}
	if len(out) == 0 {
		return "none consuming " + workerQueue
	}
	return strings.Join(out, ", ")
}

// oneLine replaces line breaks so the record stays one line.
func oneLine(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
}

// evidenceLine renders the CERTIFICATION_EVIDENCE line. HTML escaping is
// disabled so the details stay readable; the payload is still strict JSON.
func evidenceLine(rec EvidenceRecord) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(rec); err != nil {
		return "", fmt.Errorf("encode evidence record: %w", err)
	}
	return evidenceRecordPrefix + strings.TrimRight(buf.String(), "\n"), nil
}

func renderSummaryMD(s SummaryFile) string {
	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	p("# ISO-004 worker injection: %s\n\n", s.Result)
	p("- Run ID: `%s`\n- Candidate: `%s` (`%s`)\n- Task ID prefix: `%s`\n", s.RunID, s.CandidateTag, s.CandidateSHA, s.TaskIDPrefix)
	p("- Deferred (listed, never N/A): %s\n- %s\n- Timing: %s\n\n", strings.Join(s.Deferred, "; "), mdCell(s.BranchScope), mdCell(s.Timing))
	if len(s.Reasons) > 0 {
		p("## Why the result is not PASS\n\n")
		for _, r := range s.Reasons {
			p("- %s\n", mdCell(r))
		}
		p("\n")
	}
	p("## Scenarios\n\n| Scenario | Result | Task IDs | Final states | Assertions | Observations |\n|---|---|---|---|---|---|\n")
	for _, r := range s.Scenarios {
		states := make([]string, 0, len(r.FinalStates))
		for _, id := range sortedKeys(r.FinalStates) {
			states = append(states, id+"="+r.FinalStates[id])
		}
		asserts := make([]string, 0, len(r.Assertions))
		for _, a := range r.Assertions {
			asserts = append(asserts, a.ID+"="+a.Status)
		}
		p("| %s | %s | %s | %s | %s | %s |\n", mdCell(r.Scenario), r.Result, mdCell(strings.Join(r.TaskIDs, "<br>")),
			mdCell(strings.Join(states, "<br>")), mdCell(strings.Join(asserts, "<br>")), mdCell(strings.Join(r.Observations, "<br>")))
	}
	for _, r := range s.Scenarios {
		if len(r.Reasons) == 0 {
			continue
		}
		p("\n### %s reasons\n\n", r.Scenario)
		for _, reason := range r.Reasons {
			p("- %s\n", mdCell(reason))
		}
	}
	return b.String()
}

func mdCell(s string) string {
	return strings.ReplaceAll(oneLine(s), "|", `\|`)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ---- SHA256SUMS (shared with the Step 6 seal subcommand) -----------------

// SumEntry is one line of SHA256SUMS.
type SumEntry struct {
	Path   string // slash-separated, relative to the bundle root
	SHA256 string
}

// computeSums hashes every regular file under dir except SHA256SUMS, sorted
// by path. Symlinks and other non-regular files are refused so the manifest
// always describes the bytes in the bundle.
func computeSums(dir string) ([]SumEntry, error) {
	var out []SumEntry
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == sha256SumsFileName {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", rel)
		}
		if strings.ContainsAny(rel, "\n\r\\") {
			return fmt.Errorf("file name %q cannot be listed in SHA256SUMS", rel)
		}
		sum, err := fileSHA256(path)
		if err != nil {
			return err
		}
		out = append(out, SumEntry{Path: rel, SHA256: sum})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("hash bundle %s: %w", dir, err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// formatSums renders entries in `sha256sum` text format ("<hex>  <path>").
func formatSums(entries []SumEntry) []byte {
	var b bytes.Buffer
	for _, e := range entries {
		fmt.Fprintf(&b, "%s  %s\n", e.SHA256, e.Path)
	}
	return b.Bytes()
}

// parseSums reads a SHA256SUMS file written by formatSums (or sha256sum).
func parseSums(data []byte) ([]SumEntry, error) {
	var out []SumEntry
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" {
			continue
		}
		sum, path, ok := strings.Cut(line, "  ")
		if !ok {
			sum, path, ok = strings.Cut(line, " *")
		}
		if !ok || len(sum) != 64 || path == "" {
			return nil, fmt.Errorf("%s line %d is malformed", sha256SumsFileName, n)
		}
		if _, err := hex.DecodeString(sum); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", sha256SumsFileName, n, err)
		}
		out = append(out, SumEntry{Path: strings.TrimPrefix(path, "./"), SHA256: sum})
	}
	return out, sc.Err()
}

// writeSHA256SUMS (re)writes dir/SHA256SUMS over every other file in dir.
func writeSHA256SUMS(dir string) error {
	entries, err := computeSums(dir)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, sha256SumsFileName), formatSums(entries), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", sha256SumsFileName, err)
	}
	return nil
}
