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

// PlannedTask is one rendered asynq submission of a scenario. Payload is the
// exact JSON; dynamic values not yet resolved (dry run, preflight plan) appear
// as "<placeholder>" strings so the plan is still complete.
type PlannedTask struct {
	N              int           `json:"n"`
	Phase          int           `json:"phase"`
	Concurrent     bool          `json:"concurrent,omitempty"`
	TaskID         string        `json:"task_id"`
	Type           string        `json:"type"`
	Payload        string        `json:"payload"`
	Expect         string        `json:"expect"`
	Expectation    Expectation   `json:"expectation"`
	Requires       []Requirement `json:"requires,omitempty"`
	Note           string        `json:"note,omitempty"`
	ReusesTaskIDOf int           `json:"reuses_task_id_of,omitempty"`
	RenderError    string        `json:"render_error,omitempty"`
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
	// Rendered SQL snapshot queries, assertions and mail recipients for
	// Step 4 (scenarios.go).
	Queries        []RenderedQuery         `json:"sql_queries,omitempty"`
	Assertions     []RenderedAssertion     `json:"assertions,omitempty"`
	MailRecipients []string                `json:"mail_recipients,omitempty"`
	Observations   []string                `json:"observations,omitempty"`
	Dynamic        map[string]DynamicValue `json:"dynamic_values,omitempty"`
	RenderError    string                  `json:"render_error,omitempty"`
}

// buildPlan applies requirement outcomes (unmet: requirement -> reason) to
// the selected scenarios. With unmet == nil every requirement is treated as
// met (dry-run shows the full plan).
func buildPlan(cfg *Config, fx *Fixtures, unmet map[Requirement]string) []ScenarioPlan {
	env := renderEnv{Cfg: cfg, Fixtures: fx}
	var out []ScenarioPlan
	for _, s := range selectedScenarios(cfg) {
		out = append(out, planScenario(env, s, unmet))
	}
	return out
}

// planScenario renders one scenario and applies requirement outcomes.
func planScenario(env renderEnv, s Scenario, unmet map[Requirement]string) ScenarioPlan {
	sp, err := env.renderScenario(s)
	if err != nil {
		sp.RenderError = err.Error()
	}
	sp.Dynamic = env.Dynamic
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
	return sp
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
	p("queue: %s   options: MaxRetry(%d) Timeout(%s) Retention(%s)\n", workerQueue, cfg.MaxRetry, taskTimeout, taskRetention)
	p("convergence: poll %s, timeout %s\n", cfg.Poll, cfg.Timeout)
	p("email scenarios: %s\n", map[bool]string{true: "requested (--allow-email; still gated by the SMTP sink preflight)", false: "disabled (no --allow-email)"}[cfg.AllowEmail])
	p("scenarios: %d, task submissions: %d\n", len(plan), total)
	for _, sp := range plan {
		state := "enabled"
		if !sp.Enabled {
			state = "EXCLUDED: " + sp.Excluded
		}
		p("\n[%s] %s (%s)\n", sp.ID, sp.Title, state)
		if sp.RenderError != "" {
			p("  RENDER ERROR: %s\n", sp.RenderError)
		}
		for _, t := range sp.Tasks {
			mode := "sequential"
			if t.Concurrent {
				mode = "concurrent"
			}
			p("  %d. %-26s task_id=%s\n", t.N, t.Type, t.TaskID)
			p("     phase:   %d (%s)\n", t.Phase, mode)
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
		for _, q := range sp.Queries {
			p("  sql %s: args %v\n", q.Name, q.Args)
		}
		for _, a := range sp.Assertions {
			p("  assert %s (%s): %s\n", a.ID, a.Kind, a.Description)
		}
		if len(sp.MailRecipients) > 0 {
			p("  mail recipients: %s\n", strings.Join(sp.MailRecipients, ", "))
		}
		for _, o := range sp.Observations {
			p("  observation: %s\n", o)
		}
	}
	p("\ndeferred (listed in evidence details, never N/A): %s\n", strings.Join(deferredScenarios, "; "))
	return errors.Join(errs...)
}
