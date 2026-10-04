package main

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Requirement is a preflight condition a scenario (or one of its tasks)
// depends on. Unmet requirements exclude the scenario/task and are recorded
// in preflight.json; they never abort the whole run.
type Requirement string

const (
	// ReqEmail: the --allow-email switch is on and the SMTP sink gate passed.
	ReqEmail Requirement = "email-gate"
	// ReqNoConnectorConnections: connector_connections has no row for A or B.
	ReqNoConnectorConnections Requirement = "no-connector-connections"
	// ReqNoGlobalAPPolicy: no active ap_matching_policies row with
	// company_id IS NULL AND supplier_id IS NULL.
	ReqNoGlobalAPPolicy Requirement = "no-global-ap-policy"
)

// PlannedTask is one asynq submission of a scenario. Payload is the exact
// JSON when every value is known before contacting Postgres; values that are
// resolved at run time appear as "<placeholder>" strings so the dry-run plan
// is still complete.
type PlannedTask struct {
	N        int           `json:"n"`
	TaskID   string        `json:"task_id"`
	Type     string        `json:"type"`
	Payload  string        `json:"payload"`
	Expect   string        `json:"expect"`
	Requires []Requirement `json:"requires,omitempty"`
	Note     string        `json:"note,omitempty"`
}

// Scenario is one ISO-004 scenario. Build returns the planned submissions for
// a run; it must not perform I/O.
type Scenario struct {
	ID       string
	Title    string
	Requires []Requirement
	Build    func(pc PlanContext) []PlannedTask
}

// PlanContext is everything a scenario may use to build its plan.
type PlanContext struct {
	Cfg      *Config
	Fixtures *Fixtures
}

// task is a helper that numbers submissions and derives the TaskID.
func (pc PlanContext) task(scenario string, n int, typ, payload, expect string, reqs ...Requirement) PlannedTask {
	return PlannedTask{N: n, TaskID: pc.Cfg.TaskID(scenario, n), Type: typ, Payload: payload, Expect: expect, Requires: reqs}
}

// scenarioRegistry is the ordered list of Tier 1 scenarios. Step 3
// (scenarios.go) replaces the Build functions below with the authoritative
// payload builders, expectations and SQL assertions; the IDs, order and
// requirements here define the selection contract that preflight evaluates.
var scenarioRegistry = defaultScenarioPlan()

func defaultScenarioPlan() []Scenario {
	return []Scenario{
		{ID: "S01-unregistered-type", Title: "unregistered task type converges via retry/dead-letter", Build: func(pc PlanContext) []PlannedTask {
			return []PlannedTask{pc.task("S01-unregistered-type", 1, "iso004:unregistered", `{}`, "archived, Retried == MaxRetry, LastErr contains \"handler not found\"")}
		}},
		{ID: "S02-malformed-payload", Title: "malformed payloads are SkipRetry", Build: func(pc PlanContext) []PlannedTask {
			id := "S02-malformed-payload"
			return []PlannedTask{
				pc.task(id, 1, "variance:snapshot_process", `{"snapshot_id":"x"}`, "archived, Retried == 0"),
				pc.task(id, 2, "boardpack:generate", `{"board_pack_id":0}`, "archived, Retried == 0"),
			}
		}},
		{ID: "S03-object-not-found", Title: "unknown object IDs converge without effects", Build: func(pc PlanContext) []PlannedTask {
			id := "S03-object-not-found"
			return []PlannedTask{
				pc.task(id, 1, "variance:snapshot_process", `{"snapshot_id":"<max(variance_snapshots.id)+100000>"}`, "archived, Retried == 0 (rc.9 SkipRetry)"),
				pc.task(id, 2, "boardpack:generate", `{"board_pack_id":"<max(board_packs.id)+100000>"}`, "archived, Retried == 0"),
				pc.task(id, 3, "documents:ocr", `{"job_id":"<max(doc_ocr_jobs.id)+100000>"}`, "archived, Retried == MaxRetry"),
			}
		}},
		{ID: "S05-forged-object-variance", Title: "company-B variance snapshot injected by an unrelated operator", Build: func(pc PlanContext) []PlannedTask {
			return []PlannedTask{pc.task("S05-forged-object-variance", 1, "variance:snapshot_process", fmt.Sprintf(`{"snapshot_id":%d}`, pc.Fixtures.VarianceSnapshotB), "completed or archived; only snapshot B changed")}
		}},
		{ID: "S06-forged-crossscope-ocr", Title: "OCR job whose company disagrees with its document version", Build: func(pc PlanContext) []PlannedTask {
			return []PlannedTask{pc.task("S06-forged-crossscope-ocr", 1, "documents:ocr", fmt.Sprintf(`{"job_id":%d}`, pc.Fixtures.OCRJobForged), "archived; job FAILED, no extracted text, company-A index unchanged")}
		}},
		{ID: "S07-duplicate-delivery-variance", Title: "duplicate delivery of the same variance snapshot", Build: func(pc PlanContext) []PlannedTask {
			id := "S07-duplicate-delivery-variance"
			p := fmt.Sprintf(`{"snapshot_id":%d}`, pc.Fixtures.VarianceSnapshotB)
			reuse := pc.task(id, 1, "variance:snapshot_process", p, "ErrTaskIDConflict recorded")
			reuse.N, reuse.Note = 4, "re-enqueue reusing TaskID of n=1 while retained"
			return []PlannedTask{
				pc.task(id, 1, "variance:snapshot_process", p, "converges; one snapshot row"),
				pc.task(id, 2, "variance:snapshot_process", p, "converges; one snapshot row (concurrent with n=1)"),
				pc.task(id, 3, "variance:snapshot_process", p, "converges after n=1,2; payload identical"),
				reuse,
			}
		}},
		{ID: "S08-unregistered-under-profile", Title: "v0.11-only handlers are not registered under v0.10-core", Build: func(pc PlanContext) []PlannedTask {
			id := "S08-unregistered-under-profile"
			return []PlannedTask{
				pc.task(id, 1, "cashforecast:refresh", fmt.Sprintf(`{"company_id":%d,"scenario_id":%d}`, pc.Fixtures.CompanyA, pc.Fixtures.ForecastScenarioB), "archived, Retried == MaxRetry, \"handler not found\"; no forecast_runs for (A, scen_B)"),
				pc.task(id, 2, "bankfeeds:event", `{"event_id":"<max(bank_feed_events.id)+100000>"}`, "archived, Retried == MaxRetry, \"handler not found\""),
			}
		}},
		{ID: "S09-duplicate-delivery-ap", Title: "duplicate delivery of AP invoice processing (three paths)", Build: func(pc PlanContext) []PlannedTask {
			id := "S09-duplicate-delivery-ap"
			fx := pc.Fixtures
			var out []PlannedTask
			n := 0
			for _, inv := range []struct {
				id     int64
				expect string
				reqs   []Requirement
			}{
				{fx.APInvoiceBMissing, "MISSING_MAPPING: 0 runs, exactly 1 exception", []Requirement{ReqNoGlobalAPPolicy}},
				{fx.APInvoiceBException, "EXCEPTION: exactly 1 run, 1 exception", nil},
				{fx.APInvoiceBMatched, "MATCHED: exactly 1 run, 0 exceptions, POSTED by created_by", nil},
			} {
				p := fmt.Sprintf(`{"invoice_id":%d,"created_by":%d}`, inv.id, fx.APInvoiceBCreatedBy)
				for _, when := range []string{"concurrent", "concurrent", "sequential"} {
					n++
					t := pc.task(id, n, "ap:invoice_process", p, inv.expect, inv.reqs...)
					t.Note = when
					out = append(out, t)
				}
			}
			return out
		}},
		{ID: "S09b-forged-actor-ap", Title: "AP invoice processing with a spoofed created_by", Build: func(pc PlanContext) []PlannedTask {
			return []PlannedTask{pc.task("S09b-forged-actor-ap", 1, "ap:invoice_process", fmt.Sprintf(`{"invoice_id":%d,"created_by":%d}`, pc.Fixtures.APInvoiceBException, pc.Fixtures.AdminUserA), "archived, Retried == 0, actor mismatch; no new runs/exceptions")}
		}},
		{ID: "S10-forged-company-bi-export", Title: "BI export for a forged company", Requires: []Requirement{ReqNoConnectorConnections}, Build: func(pc PlanContext) []PlannedTask {
			return []PlannedTask{pc.task("S10-forged-company-bi-export", 1, "analytics:bi_export", fmt.Sprintf(`{"company_id":%d,"period":"<current YYYY-MM>","provider":"awss3"}`, pc.Fixtures.CompanyB), "archived, \"handler not found\"; connector_outbox_commands unchanged")}
		}},
		{ID: "S11-forged-recipient-mail", Title: "recipient-addressed mail tasks with a forged recipient", Requires: []Requirement{ReqEmail}, Build: func(pc PlanContext) []PlannedTask {
			id := "S11-forged-recipient-mail"
			rcpt := fmt.Sprintf("iso004-%s@staging.invalid", pc.Cfg.RunID)
			corr := fmt.Sprintf("%s:%s:S11", taskIDRoot, pc.Cfg.RunID)
			mail := fmt.Sprintf(`{"to":%q,"subject":"ISO-004 %s","body":"ISO-004 forged recipient probe","correlation_id":%q}`, rcpt, pc.Cfg.RunID, corr)
			dup := PlannedTask{N: 3, TaskID: corr, Type: "mail:send", Payload: mail, Expect: "ErrTaskIDConflict recorded", Requires: []Requirement{ReqEmail}, Note: "TaskID = correlation_id while n=1 is retained"}
			first := dup
			first.N, first.Expect, first.Note = 1, "completed; exactly one Mailpit message", "TaskID = correlation_id (producer convention)"
			return []PlannedTask{
				first,
				pc.task(id, 2, "email:deliver", fmt.Sprintf(`{"to":[%q],"subject":"ISO-004 %s","body_html":"<p>ISO-004 forged recipient probe</p>"}`, rcpt, pc.Cfg.RunID), "completed; exactly one Mailpit message", ReqEmail),
				dup,
			}
		}},
		{ID: "S12-duplicate-delivery-payslip", Title: "duplicate delivery of a company-B payslip email", Requires: []Requirement{ReqEmail}, Build: func(pc PlanContext) []PlannedTask {
			id := "S12-duplicate-delivery-payslip"
			p := fmt.Sprintf(`{"payslip_id":%d}`, pc.Fixtures.PayslipB)
			return []PlannedTask{
				pc.task(id, 1, "payroll:payslip_email", p, "completed; one message total since seed", ReqEmail),
				pc.task(id, 2, "payroll:payslip_email", p, "completed; one message total since seed (concurrent)", ReqEmail),
				pc.task(id, 3, "payroll:payslip_email", p, "completed; delivered_at unchanged (sequential)", ReqEmail),
				pc.task(id, 4, "payroll:payslip_email", `{"payslip_id":"<max(payroll_payslips.id)+100000>"}`, "archived, Retried == 0", ReqEmail),
			}
		}},
	}
}

// deferredScenarios are listed in the evidence details as deferred (never as
// N/A) by user decision.
var deferredScenarios = []string{
	"boardpack:generate full generation",
	"documents:ocr legitimate duplicate",
	"bankfeeds:event legitimate delivery",
}

func scenarioIDs() []string {
	ids := make([]string, len(scenarioRegistry))
	for i, s := range scenarioRegistry {
		ids[i] = s.ID
	}
	return ids
}

func validateScenarioSelection(sel []string) error {
	known := map[string]bool{}
	for _, id := range scenarioIDs() {
		known[id] = true
	}
	seen := map[string]bool{}
	var bad []string
	for _, id := range sel {
		if !known[id] {
			bad = append(bad, id)
		}
		if seen[id] {
			return fmt.Errorf("--scenarios lists %s twice", id)
		}
		seen[id] = true
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return fmt.Errorf("--scenarios has unknown IDs %s (known: %s)", strings.Join(bad, ", "), strings.Join(scenarioIDs(), ", "))
	}
	return nil
}

// selectedScenarios returns the scenarios in registry order, filtered by
// --scenarios when set.
func selectedScenarios(cfg *Config) []Scenario {
	if len(cfg.Scenarios) == 0 {
		return append([]Scenario(nil), scenarioRegistry...)
	}
	want := map[string]bool{}
	for _, id := range cfg.Scenarios {
		want[id] = true
	}
	var out []Scenario
	for _, s := range scenarioRegistry {
		if want[s.ID] {
			out = append(out, s)
		}
	}
	return out
}

// ScenarioPlan is the plan for one scenario after requirements are applied.
type ScenarioPlan struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	Enabled  bool          `json:"enabled"`
	Excluded string        `json:"excluded_reason,omitempty"`
	Tasks    []PlannedTask `json:"tasks"`
	// Skipped lists TaskIDs skipped because a task-level requirement is
	// unmet; the scenario counts them as FAIL-by-precondition.
	Skipped map[string]string `json:"skipped_tasks,omitempty"`
}

// buildPlan applies requirement outcomes (unmet: requirement -> reason) to
// the selected scenarios. With unmet == nil every requirement is treated as
// met (dry-run shows the full plan).
func buildPlan(cfg *Config, fx *Fixtures, unmet map[Requirement]string) []ScenarioPlan {
	if fx == nil {
		fx = &Fixtures{}
	}
	pc := PlanContext{Cfg: cfg, Fixtures: fx}
	var out []ScenarioPlan
	for _, s := range selectedScenarios(cfg) {
		sp := ScenarioPlan{ID: s.ID, Title: s.Title, Enabled: true, Tasks: s.Build(pc)}
		for _, r := range s.Requires {
			if reason, bad := unmet[r]; bad {
				sp.Enabled = false
				sp.Excluded = fmt.Sprintf("%s unmet: %s", r, reason)
				break
			}
		}
		if sp.Enabled {
			for _, t := range sp.Tasks {
				for _, r := range t.Requires {
					if reason, bad := unmet[r]; bad {
						if sp.Skipped == nil {
							sp.Skipped = map[string]string{}
						}
						sp.Skipped[t.TaskID] = fmt.Sprintf("FAIL-by-precondition: %s unmet: %s", r, reason)
						break
					}
				}
			}
		}
		out = append(out, sp)
	}
	return out
}

func printPlan(w io.Writer, cfg *Config, plan []ScenarioPlan) error {
	var errs []error
	p := func(format string, args ...any) {
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			errs = append(errs, err)
		}
	}
	total := 0
	for _, sp := range plan {
		total += len(sp.Tasks)
	}
	p("ISO-004 enqueue plan (dry run: Redis and Postgres were not contacted)\n")
	p("candidate: %s %s\n", cfg.CandidateTag, cfg.CandidateSHA)
	p("run id: %s   task ID prefix: %s\n", cfg.RunID, cfg.TaskIDPrefix())
	p("queue: %s   options: MaxRetry(%d) Timeout(2m) Retention(72h)\n", workerQueue, cfg.MaxRetry)
	p("convergence: poll %s, timeout %s\n", cfg.Poll, cfg.Timeout)
	p("email scenarios: %s\n", map[bool]string{true: "requested (--allow-email; still gated by the SMTP sink preflight)", false: "disabled (no --allow-email)"}[cfg.AllowEmail])
	p("scenarios: %d, task submissions: %d\n", len(plan), total)
	for _, sp := range plan {
		state := "enabled"
		if !sp.Enabled {
			state = "EXCLUDED: " + sp.Excluded
		}
		p("\n[%s] %s (%s)\n", sp.ID, sp.Title, state)
		for _, t := range sp.Tasks {
			p("  %d. %-26s task_id=%s\n", t.N, t.Type, t.TaskID)
			p("     payload: %s\n", t.Payload)
			p("     expect:  %s\n", t.Expect)
			if t.Note != "" {
				p("     note:    %s\n", t.Note)
			}
			if len(t.Requires) > 0 {
				p("     requires: %v\n", t.Requires)
			}
			if reason, ok := sp.Skipped[t.TaskID]; ok {
				p("     SKIPPED: %s\n", reason)
			}
		}
	}
	p("\ndeferred (listed in evidence details, never N/A): %s\n", strings.Join(deferredScenarios, "; "))
	return errors.Join(errs...)
}
