package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/jobs"
)

// This file is the ISO-004 scenario table. Scenarios are data: every task
// submission, payload field, expectation, SQL snapshot query and assertion is
// declared below and rendered by the generic code at the end of the file.
// Tests iterate scenarioRegistry; no scenario has its own code path.

// Enqueue options shared by every submission (plan Step 3).
const (
	taskTimeout   = 2 * time.Minute
	taskRetention = 72 * time.Hour
)

// Asynq task states used in expectations (asynq.TaskState.String()).
const (
	stateCompleted = "completed"
	stateArchived  = "archived"
)

// RetryExpect constrains TaskInfo.Retried once the task converged.
type RetryExpect string

const (
	RetriedAny  RetryExpect = "any"
	RetriedZero RetryExpect = "zero" // SkipRetry path: archived on the first delivery
	RetriedMax  RetryExpect = "max"  // Retried == MaxRetry (--max-retry)
)

// Expectation is the expected convergence of one submission. Step 4 compares
// the final TaskInfo against it.
type Expectation struct {
	// States are the acceptable final asynq states; empty for conflict probes
	// (nothing is enqueued).
	States  []string    `json:"states,omitempty"`
	Retried RetryExpect `json:"retried,omitempty"`
	// LastErrAnyOf: LastErr must contain at least one of these substrings
	// (case-insensitive). Empty means no constraint.
	LastErrAnyOf []string `json:"last_err_any_of,omitempty"`
	// Conflict: the enqueue itself must fail with asynq.ErrTaskIDConflict.
	Conflict bool   `json:"enqueue_conflict,omitempty"`
	Text     string `json:"text"`
}

// ValueKind selects how a Value is resolved.
type ValueKind string

const (
	ValueLiteral  ValueKind = "literal"  // Lit, encoded as JSON
	ValueFixture  ValueKind = "fixture"  // Ref is a fixtures JSON key (STAGING_CERT_ISO004_*)
	ValueDynamic  ValueKind = "dynamic"  // Ref is a dynamicSpecs name, resolved at run time by a read-only SELECT
	ValueTemplate ValueKind = "template" // Ref with {run} and {taskroot} expanded
	ValueList     ValueKind = "list"     // Items, encoded as a JSON array
)

// Value is a payload field value, SQL argument or expected value.
type Value struct {
	Kind  ValueKind `json:"kind"`
	Lit   any       `json:"literal,omitempty"`
	Ref   string    `json:"ref,omitempty"`
	Items []Value   `json:"items,omitempty"`
}

func lit(v any) Value           { return Value{Kind: ValueLiteral, Lit: v} }
func fxv(name string) Value     { return Value{Kind: ValueFixture, Ref: name} }
func dyn(name string) Value     { return Value{Kind: ValueDynamic, Ref: name} }
func tmpl(s string) Value       { return Value{Kind: ValueTemplate, Ref: s} }
func list(items ...Value) Value { return Value{Kind: ValueList, Items: items} }
func vp(v Value) *Value         { return &v }
func intp(n int) *int           { return &n }

// Field is one payload member. Payloads are rendered in field order so the
// bytes are deterministic and match the jobs payload struct field order.
type Field struct {
	Key   string `json:"key"`
	Value Value  `json:"value"`
}

// TaskSpec is one submission of a scenario.
type TaskSpec struct {
	N int
	// Phase orders submissions: phase k+1 is submitted only after every task
	// of phase k converged (ConvergenceWaiter, Step 4).
	Phase int
	// Concurrent tasks of a phase are released together from a barrier
	// (back-to-back delivery); the others follow sequentially in N order.
	Concurrent bool
	Type       string
	Payload    []Field
	// Malformed marks payloads that intentionally do not decode into the
	// type's payload struct (S02).
	Malformed bool
	// TaskID overrides the default iso004:<run>:<scenario>:<n>.
	TaskID *Value
	// ReuseTaskIDOf reuses the TaskID of submission N (conflict probes).
	ReuseTaskIDOf int
	Requires      []Requirement
	Expect        Expectation
	Note          string
}

// QuerySpec is one read-only snapshot query. Step 4 runs every query of the
// scenario before enqueue, after each phase and after convergence.
type QuerySpec struct {
	Name string
	SQL  string
	Args []Value
}

// AssertKind is the assertion vocabulary Step 4 evaluates over snapshots.
type AssertKind string

const (
	// AssertUnchanged: the query's rows are identical in sql-before and sql-after.
	AssertUnchanged AssertKind = "unchanged"
	// AssertEquals: Column of the query's single row at At equals Want.
	AssertEquals AssertKind = "equals"
	// AssertNotNull: Column of the query's single row at At is not NULL.
	AssertNotNull AssertKind = "not_null"
	// AssertContains: Column (text) at At contains Want.
	AssertContains AssertKind = "contains"
	// AssertStableAfterFirst: Column is identical in every snapshot taken
	// after phase 1 (after-phase-1 ... after).
	AssertStableAfterFirst AssertKind = "stable_after_first"
	// AssertMailCount: the mail snapshot at At holds exactly Want messages
	// to Recipient (created at or after Since when set); when Attachments is
	// set every counted message has that many attachments; when
	// SubjectContains is set every counted subject contains it.
	AssertMailCount AssertKind = "mail_count"
)

// Snapshot labels.
const (
	snapBefore = "before"
	snapAfter  = "after"
)

// AssertionSpec is one assertion of a scenario.
type AssertionSpec struct {
	ID              string
	Kind            AssertKind
	Query           string
	Column          string
	At              string // snapBefore or snapAfter (default after)
	Want            *Value
	Recipient       *Value
	Since           *Value
	Attachments     *int
	SubjectContains *Value
	// Requires: an unmet requirement turns the assertion into
	// FAIL-by-precondition (S09 MISSING_MAPPING).
	Requires    []Requirement
	Description string
}

// Scenario is one ISO-004 scenario.
type Scenario struct {
	ID       string
	Title    string
	Requires []Requirement
	Tasks    []TaskSpec
	Queries  []QuerySpec
	// Assertions over SQL snapshots and mail snapshots.
	Assertions []AssertionSpec
	// MailRecipients are queried in mail-before.json/mail-after.json.
	MailRecipients []Value
	Observations   []string
}

// DynamicSpec is a value resolved at run time by a read-only SELECT that
// returns exactly one column of one row.
type DynamicSpec struct {
	Name        string
	SQL         string
	Kind        string // "int" or "text"
	Placeholder string // shown in the dry-run plan
}

// Dynamic value names.
const (
	dynVarianceUnknown  = "variance_snapshot_unknown_id"
	dynBoardPackUnknown = "board_pack_unknown_id"
	dynOCRJobUnknown    = "ocr_job_unknown_id"
	dynBankEventUnknown = "bank_feed_event_unknown_id"
	dynPayslipUnknown   = "payslip_unknown_id"
	dynCurrentPeriod    = "current_period"
)

var dynamicSpecs = []DynamicSpec{
	{Name: dynVarianceUnknown, SQL: `SELECT COALESCE(MAX(id), 0) + 100000 FROM variance_snapshots`, Kind: "int", Placeholder: "<max(variance_snapshots.id)+100000>"},
	{Name: dynBoardPackUnknown, SQL: `SELECT COALESCE(MAX(id), 0) + 100000 FROM board_packs`, Kind: "int", Placeholder: "<max(board_packs.id)+100000>"},
	{Name: dynOCRJobUnknown, SQL: `SELECT COALESCE(MAX(id), 0) + 100000 FROM doc_ocr_jobs`, Kind: "int", Placeholder: "<max(doc_ocr_jobs.id)+100000>"},
	{Name: dynBankEventUnknown, SQL: `SELECT COALESCE(MAX(id), 0) + 100000 FROM bank_feed_events`, Kind: "int", Placeholder: "<max(bank_feed_events.id)+100000>"},
	{Name: dynPayslipUnknown, SQL: `SELECT COALESCE(MAX(id), 0) + 100000 FROM payroll_payslips`, Kind: "int", Placeholder: "<max(payroll_payslips.id)+100000>"},
	{Name: dynCurrentPeriod, SQL: `SELECT to_char(CURRENT_DATE, 'YYYY-MM')`, Kind: "text", Placeholder: "<current YYYY-MM>"},
}

func dynamicSpec(name string) (DynamicSpec, bool) {
	for _, d := range dynamicSpecs {
		if d.Name == name {
			return d, true
		}
	}
	return DynamicSpec{}, false
}

// Fixture keys referenced by the scenarios (see fixtures.go).
const (
	fxCompanyA        = "STAGING_CERT_ISO004_COMPANY_A_ID"
	fxCompanyB        = "STAGING_CERT_ISO004_COMPANY_B_ID"
	fxAdminUserA      = "STAGING_CERT_ISO004_ADMIN_USER_ID"
	fxScenarioA       = "STAGING_CERT_ISO004_FORECAST_SCENARIO_A_ID"
	fxScenarioB       = "STAGING_CERT_ISO004_FORECAST_SCENARIO_B_ID"
	fxVarianceRuleB   = "STAGING_CERT_ISO004_VARIANCE_RULE_B_ID"
	fxVarianceSnapB   = "STAGING_CERT_ISO004_VARIANCE_SNAPSHOT_B_ID"
	fxAPMissing       = "STAGING_CERT_ISO004_AP_INVOICE_B_MISSING_ID"
	fxAPException     = "STAGING_CERT_ISO004_AP_INVOICE_B_EXCEPTION_ID"
	fxAPMatched       = "STAGING_CERT_ISO004_AP_INVOICE_B_MATCHED_ID"
	fxAPCreatedBy     = "STAGING_CERT_ISO004_AP_INVOICE_B_CREATED_BY"
	fxDocVersionA     = "STAGING_CERT_ISO004_DOCUMENT_VERSION_A_ID"
	fxOCRJobForged    = "STAGING_CERT_ISO004_OCR_JOB_FORGED_ID"
	fxPayslipB        = "STAGING_CERT_ISO004_PAYSLIP_B_ID"
	fxPayslipEmail    = "STAGING_CERT_ISO004_PAYSLIP_B_EMAIL"
	fxPayslipCreated  = "STAGING_CERT_ISO004_PAYSLIP_B_CREATED_AT"
	handlerNotFound   = "handler not found"
	s11CorrelationID  = "{taskroot}:{run}:S11"
	s11Recipient      = "iso004-{run}@staging.invalid"
	s11Subject        = "ISO-004 {run}"
	ocrMismatchErr    = "does not match document version"
	apMissingMapping  = "MISSING_MAPPING"
	apStatusPosted    = "POSTED"
	ocrStatusFailed   = "FAILED"
	maxRetryNote      = "Retried == MaxRetry (--max-retry)"
	skipRetryNote     = "Retried == 0 (SkipRetry)"
	convergedAnyState = "completed or archived"
)

var converged = []string{stateCompleted, stateArchived}

// Shared snapshot queries. Every statement is a single parameterized SELECT
// (asserted by tests).
var (
	qVarianceSnapB = QuerySpec{Name: "variance_snapshot_b", Args: []Value{fxv(fxVarianceSnapB)},
		SQL: `SELECT vs.id, vs.rule_id, vs.status::text AS status, md5(COALESCE(vs.payload::text, '')) AS payload_md5, vs.error_message, vs.updated_at FROM variance_snapshots vs WHERE vs.id = $1`}
	qVarianceSnapBCount = QuerySpec{Name: "variance_snapshot_b_count", Args: []Value{fxv(fxVarianceSnapB)},
		SQL: `SELECT COUNT(*) AS n FROM variance_snapshots WHERE id = $1`}
	qVarianceCountsAB = QuerySpec{Name: "variance_counts_ab", Args: []Value{fxv(fxCompanyA), fxv(fxCompanyB)},
		SQL: `SELECT vr.company_id, COUNT(*) AS n FROM variance_snapshots vs JOIN variance_rules vr ON vr.id = vs.rule_id WHERE vr.company_id IN ($1, $2) GROUP BY vr.company_id ORDER BY vr.company_id`}
	qVarianceRowsA = QuerySpec{Name: "variance_rows_a", Args: []Value{fxv(fxCompanyA)},
		SQL: `SELECT vs.id, vs.status::text AS status, vs.updated_at FROM variance_snapshots vs JOIN variance_rules vr ON vr.id = vs.rule_id WHERE vr.company_id = $1 ORDER BY vs.id`}
	qVarianceRowsBOthers = QuerySpec{Name: "variance_rows_b_others", Args: []Value{fxv(fxCompanyB), fxv(fxVarianceSnapB)},
		SQL: `SELECT vs.id, vs.status::text AS status, vs.updated_at FROM variance_snapshots vs JOIN variance_rules vr ON vr.id = vs.rule_id WHERE vr.company_id = $1 AND vs.id <> $2 ORDER BY vs.id`}
	qVarianceTotal = QuerySpec{Name: "variance_snapshots_total",
		SQL: `SELECT COUNT(*) AS n FROM variance_snapshots`}
	qBoardPackTotal = QuerySpec{Name: "board_packs_total",
		SQL: `SELECT COUNT(*) AS n FROM board_packs`}
	qOCRJobTotal = QuerySpec{Name: "doc_ocr_jobs_total",
		SQL: `SELECT COUNT(*) AS n FROM doc_ocr_jobs`}
)

// apQueries returns the per-invoice AP snapshot queries for one path.
func apQueries(path, invoice string) []QuerySpec {
	inv := []Value{fxv(invoice)}
	return []QuerySpec{
		{Name: "ap_runs_" + path, Args: inv, SQL: `SELECT COUNT(*) AS n FROM ap_matching_runs WHERE ap_invoice_id = $1`},
		{Name: "ap_exceptions_" + path, Args: inv, SQL: `SELECT COUNT(*) AS n FROM ap_exceptions WHERE ap_invoice_id = $1`},
		{Name: "ap_exceptions_by_run_" + path, Args: inv, SQL: `SELECT COUNT(*) AS n FROM ap_exceptions e WHERE e.ap_invoice_id = $1 AND e.ap_matching_run_id IN (SELECT r.id FROM ap_matching_runs r WHERE r.ap_invoice_id = $1)`},
		{Name: "ap_invoice_" + path, Args: inv, SQL: `SELECT i.id, i.status, i.posted_by, i.created_by FROM ap_invoices i WHERE i.id = $1`},
	}
}

func eq(id, query, column string, want Value, desc string, reqs ...Requirement) AssertionSpec {
	return AssertionSpec{ID: id, Kind: AssertEquals, Query: query, Column: column, At: snapAfter, Want: vp(want), Description: desc, Requires: reqs}
}

func unchanged(id, query, desc string) AssertionSpec {
	return AssertionSpec{ID: id, Kind: AssertUnchanged, Query: query, Description: desc}
}

// scenarioRegistry is the ordered Tier 1 scenario table. preflight evaluates
// Requires; the executor renders and submits Tasks; Step 4 runs Queries and
// evaluates Assertions.
var scenarioRegistry = []Scenario{
	{
		ID:    "S01-unregistered-type",
		Title: "unregistered task type converges via retry/dead-letter",
		Tasks: []TaskSpec{{
			N: 1, Phase: 1, Type: "iso004:unregistered",
			Expect: Expectation{States: []string{stateArchived}, Retried: RetriedMax, LastErrAnyOf: []string{handlerNotFound},
				Text: "archived, " + maxRetryNote + `, LastErr contains "handler not found"`},
		}},
		Observations: []string{"proves the worker's retry/dead-letter path end to end"},
	},
	{
		ID:    "S02-malformed-payload",
		Title: "malformed payloads are SkipRetry",
		Tasks: []TaskSpec{
			{N: 1, Phase: 1, Type: jobs.TaskVarianceSnapshotProcess, Malformed: true,
				Payload: []Field{{"snapshot_id", lit("x")}},
				Expect:  Expectation{States: []string{stateArchived}, Retried: RetriedZero, Text: "archived, " + skipRetryNote + " (internal/variance/job.go:34-39)"}},
			{N: 2, Phase: 1, Type: jobs.TaskBoardPackGenerate,
				Payload: []Field{{"board_pack_id", lit(0)}},
				Expect:  Expectation{States: []string{stateArchived}, Retried: RetriedZero, Text: "archived, " + skipRetryNote + " (internal/boardpack/job.go:50-55)"}},
		},
		Queries: []QuerySpec{qVarianceTotal, qBoardPackTotal},
		Assertions: []AssertionSpec{
			unchanged("variance_total_unchanged", qVarianceTotal.Name, "no variance snapshot created"),
			unchanged("board_packs_total_unchanged", qBoardPackTotal.Name, "no board pack created"),
		},
	},
	{
		ID:    "S03-object-not-found",
		Title: "unknown object IDs converge without effects",
		Tasks: []TaskSpec{
			{N: 1, Phase: 1, Type: jobs.TaskVarianceSnapshotProcess,
				Payload: []Field{{"snapshot_id", dyn(dynVarianceUnknown)}},
				Expect:  Expectation{States: []string{stateArchived}, Retried: RetriedZero, Text: "archived, " + skipRetryNote + " under rc.9 (ErrSnapshotNotFound is wrapped with SkipRetry, internal/variance/job.go:44-47; rc.8 retried to MaxRetry)"}},
			{N: 2, Phase: 1, Type: jobs.TaskBoardPackGenerate,
				Payload: []Field{{"board_pack_id", dyn(dynBoardPackUnknown)}},
				Expect:  Expectation{States: []string{stateArchived}, Retried: RetriedZero, Text: "archived, " + skipRetryNote + " (internal/boardpack/job.go:58-61)"}},
			{N: 3, Phase: 1, Type: jobs.TaskDocumentOCR,
				Payload: []Field{{"job_id", dyn(dynOCRJobUnknown)}},
				Expect:  Expectation{States: []string{stateArchived}, Retried: RetriedMax, Text: "archived, " + maxRetryNote + " (unchanged in rc.9: a missing OCR job is a plain error, internal/documents/ocr.go:83-86, jobs/document_ocr.go:54)"}},
		},
		Queries: []QuerySpec{
			qVarianceTotal, qBoardPackTotal, qOCRJobTotal,
			{Name: "variance_unknown_id", Args: []Value{dyn(dynVarianceUnknown)}, SQL: `SELECT COUNT(*) AS n FROM variance_snapshots WHERE id = $1`},
			{Name: "board_pack_unknown_id", Args: []Value{dyn(dynBoardPackUnknown)}, SQL: `SELECT COUNT(*) AS n FROM board_packs WHERE id = $1`},
			{Name: "ocr_job_unknown_id", Args: []Value{dyn(dynOCRJobUnknown)}, SQL: `SELECT COUNT(*) AS n FROM doc_ocr_jobs WHERE id = $1`},
		},
		Assertions: []AssertionSpec{
			unchanged("variance_total_unchanged", qVarianceTotal.Name, "variance_snapshots row count unchanged"),
			unchanged("board_packs_total_unchanged", qBoardPackTotal.Name, "board_packs row count unchanged"),
			unchanged("doc_ocr_jobs_total_unchanged", qOCRJobTotal.Name, "doc_ocr_jobs row count unchanged"),
			eq("variance_unknown_absent", "variance_unknown_id", "n", lit(0), "the forged snapshot ID was not created"),
			eq("board_pack_unknown_absent", "board_pack_unknown_id", "n", lit(0), "the forged board pack ID was not created"),
			eq("ocr_job_unknown_absent", "ocr_job_unknown_id", "n", lit(0), "the forged OCR job ID was not created"),
		},
	},
	{
		ID:    "S05-forged-object-variance",
		Title: "company-B variance snapshot injected by an unrelated operator",
		Tasks: []TaskSpec{{
			N: 1, Phase: 1, Type: jobs.TaskVarianceSnapshotProcess,
			Payload: []Field{{"snapshot_id", fxv(fxVarianceSnapB)}},
			Expect:  Expectation{States: converged, Retried: RetriedAny, Text: convergedAnyState + "; only snapshot B changed"},
		}},
		Queries: []QuerySpec{qVarianceSnapB, qVarianceSnapBCount, qVarianceCountsAB, qVarianceRowsA, qVarianceRowsBOthers},
		Assertions: []AssertionSpec{
			eq("snapshot_b_single_row", qVarianceSnapBCount.Name, "n", lit(1), "exactly one variance_snapshots row for snapB"),
			eq("snapshot_b_rule_unchanged", qVarianceSnapB.Name, "rule_id", fxv(fxVarianceRuleB), "snapB still references company B's rule"),
			unchanged("variance_counts_ab_unchanged", qVarianceCountsAB.Name, "no snapshot added or removed for A or B"),
			unchanged("variance_rows_a_unchanged", qVarianceRowsA.Name, "no company-A snapshot changed (status, updated_at)"),
			unchanged("variance_rows_b_others_unchanged", qVarianceRowsBOthers.Name, "no other company-B snapshot changed"),
		},
		Observations: []string{"object-ID tasks carry no actor binding: any operator able to enqueue can trigger company B's own pending object (not cross-scope by the plan's definition; the producer boundary is ISO-002)"},
	},
	{
		ID:    "S06-forged-crossscope-ocr",
		Title: "OCR job whose company disagrees with its document version",
		Tasks: []TaskSpec{{
			N: 1, Phase: 1, Type: jobs.TaskDocumentOCR,
			Payload: []Field{{"job_id", fxv(fxOCRJobForged)}},
			Expect: Expectation{States: []string{stateArchived}, Retried: RetriedAny, LastErrAnyOf: []string{ocrMismatchErr},
				Text: `archived; job FAILED with "does not match document version" (internal/documents/ocr.go:106-107)`},
		}},
		Queries: []QuerySpec{
			{Name: "ocr_job_forged", Args: []Value{fxv(fxOCRJobForged)},
				SQL: `SELECT j.id, j.company_id, j.status, j.extracted_text IS NULL AS extracted_text_null, j.error_message FROM doc_ocr_jobs j WHERE j.id = $1`},
			{Name: "doc_index_a", Args: []Value{fxv(fxDocVersionA)},
				SQL: `SELECT COUNT(*) AS n, MAX(x.indexed_at) AS max_indexed_at FROM doc_search_indices x WHERE x.document_id = (SELECT v.document_id FROM document_versions v WHERE v.id = $1)`},
		},
		Assertions: []AssertionSpec{
			eq("ocr_job_failed", "ocr_job_forged", "status", lit(ocrStatusFailed), "the forged job is marked FAILED"),
			eq("ocr_job_no_text", "ocr_job_forged", "extracted_text_null", lit(true), "no text was extracted from company A's document"),
			eq("ocr_job_company_b", "ocr_job_forged", "company_id", fxv(fxCompanyB), "the job still belongs to company B"),
			{ID: "ocr_job_mismatch_reason", Kind: AssertContains, Query: "ocr_job_forged", Column: "error_message", At: snapAfter, Want: vp(lit(ocrMismatchErr)), Description: "the failure names the company/version mismatch"},
			unchanged("doc_index_a_unchanged", "doc_index_a", "company A's search index rows unchanged (count and indexed_at)"),
		},
	},
	{
		ID:    "S07-duplicate-delivery-variance",
		Title: "duplicate delivery of the same variance snapshot",
		Tasks: []TaskSpec{
			{N: 1, Phase: 1, Concurrent: true, Type: jobs.TaskVarianceSnapshotProcess,
				Payload: []Field{{"snapshot_id", fxv(fxVarianceSnapB)}},
				Expect:  Expectation{States: converged, Retried: RetriedAny, Text: "converges; one snapshot row"}, Note: "concurrent with n=2"},
			{N: 2, Phase: 1, Concurrent: true, Type: jobs.TaskVarianceSnapshotProcess,
				Payload: []Field{{"snapshot_id", fxv(fxVarianceSnapB)}},
				Expect:  Expectation{States: converged, Retried: RetriedAny, Text: "converges; one snapshot row"}, Note: "concurrent with n=1"},
			{N: 3, Phase: 2, Type: jobs.TaskVarianceSnapshotProcess,
				Payload: []Field{{"snapshot_id", fxv(fxVarianceSnapB)}},
				Expect:  Expectation{States: converged, Retried: RetriedAny, Text: "converges after n=1,2; payload identical"}, Note: "sequential, after n=1,2 converged"},
			{N: 4, Phase: 1, Type: jobs.TaskVarianceSnapshotProcess, ReuseTaskIDOf: 1,
				Payload: []Field{{"snapshot_id", fxv(fxVarianceSnapB)}},
				Expect:  Expectation{Conflict: true, Text: "ErrTaskIDConflict recorded"}, Note: "re-enqueue reusing the TaskID of n=1 while it is pending/retained"},
		},
		Queries: []QuerySpec{qVarianceSnapB, qVarianceSnapBCount, qVarianceCountsAB, qVarianceRowsA},
		Assertions: []AssertionSpec{
			eq("snapshot_b_single_row", qVarianceSnapBCount.Name, "n", lit(1), "still exactly one variance_snapshots row for snapB"),
			{ID: "snapshot_b_payload_stable", Kind: AssertStableAfterFirst, Query: qVarianceSnapB.Name, Column: "payload_md5", Description: "payload identical across deliveries"},
			unchanged("variance_counts_ab_unchanged", qVarianceCountsAB.Name, "no additional snapshot rows for A or B"),
			unchanged("variance_rows_a_unchanged", qVarianceRowsA.Name, "no company-A snapshot changed"),
		},
	},
	{
		ID:    "S08-unregistered-under-profile",
		Title: "v0.11-only handlers are not registered under v0.10-core",
		Tasks: []TaskSpec{
			{N: 1, Phase: 1, Type: jobs.TypeCashForecastRefresh,
				Payload: []Field{{"company_id", fxv(fxCompanyA)}, {"scenario_id", fxv(fxScenarioB)}},
				Expect: Expectation{States: []string{stateArchived}, Retried: RetriedMax, LastErrAnyOf: []string{handlerNotFound},
					Text: `archived, ` + maxRetryNote + `, "handler not found" (cmd/worker/main.go:127-132); no forecast_runs for (A, scen_B)`}},
			{N: 2, Phase: 1, Type: jobs.TypeBankFeedsEvent,
				Payload: []Field{{"event_id", dyn(dynBankEventUnknown)}},
				Expect: Expectation{States: []string{stateArchived}, Retried: RetriedMax, LastErrAnyOf: []string{handlerNotFound},
					Text: `archived, ` + maxRetryNote + `, "handler not found" (cmd/worker/main.go:127-132)`}},
		},
		Queries: []QuerySpec{
			{Name: "forecast_runs_forged_pair", Args: []Value{fxv(fxCompanyA), fxv(fxScenarioB)}, SQL: `SELECT COUNT(*) AS n FROM forecast_runs WHERE company_id = $1 AND scenario_id = $2`},
			{Name: "forecast_runs_scenarios_ab", Args: []Value{fxv(fxScenarioA), fxv(fxScenarioB)}, SQL: `SELECT COUNT(*) AS n FROM forecast_runs WHERE scenario_id IN ($1, $2)`},
			{Name: "bank_feed_events_total", SQL: `SELECT COUNT(*) AS n FROM bank_feed_events`},
			{Name: "bank_feed_event_unknown_id", Args: []Value{dyn(dynBankEventUnknown)}, SQL: `SELECT COUNT(*) AS n FROM bank_feed_events WHERE id = $1`},
		},
		Assertions: []AssertionSpec{
			eq("forecast_forged_pair_absent", "forecast_runs_forged_pair", "n", lit(0), "absolute: no forecast run for the forged (A, scen_B) pair"),
			unchanged("forecast_runs_scenarios_unchanged", "forecast_runs_scenarios_ab", "no forecast run for either fixture scenario"),
			unchanged("bank_feed_events_unchanged", "bank_feed_events_total", "bank_feed_events row count unchanged"),
			eq("bank_feed_event_unknown_absent", "bank_feed_event_unknown_id", "n", lit(0), "the forged event ID was not created"),
		},
		Observations: []string{`a registered handler (LastErr other than "handler not found") means release-profile gating is missing: FAIL`},
	},
	{
		ID:    "S09-duplicate-delivery-ap",
		Title: "duplicate delivery of AP invoice processing (three paths)",
		Tasks: apDuplicateTasks(),
		Queries: concatQueries(apQueries("missing", fxAPMissing), apQueries("exception", fxAPException), apQueries("matched", fxAPMatched), []QuerySpec{
			{Name: "ap_missing_mapping_exceptions", Args: []Value{fxv(fxAPMissing), lit(apMissingMapping)},
				SQL: `SELECT COUNT(*) AS n FROM ap_exceptions WHERE ap_invoice_id = $1 AND exception_type = $2`},
			{Name: "ap_runs_foreign_actor", Args: []Value{fxv(fxAPMissing), fxv(fxAPException), fxv(fxAPMatched), fxv(fxAPCreatedBy)},
				SQL: `SELECT COUNT(*) AS n FROM ap_matching_runs WHERE ap_invoice_id IN ($1, $2, $3) AND run_by IS DISTINCT FROM $4`},
		}),
		Assertions: []AssertionSpec{
			eq("missing_runs_zero", "ap_runs_missing", "n", lit(0), "MISSING_MAPPING: 0 ap_matching_runs", ReqNoGlobalAPPolicy),
			eq("missing_exceptions_one", "ap_exceptions_missing", "n", lit(1), "MISSING_MAPPING: exactly 1 ap_exceptions row", ReqNoGlobalAPPolicy),
			{ID: "missing_exception_type", Kind: AssertEquals, Query: "ap_missing_mapping_exceptions", Column: "n", At: snapAfter, Want: vp(lit(1)), Requires: []Requirement{ReqNoGlobalAPPolicy}, Description: "the one exception is MISSING_MAPPING"},
			eq("missing_invoice_draft", "ap_invoice_missing", "status", lit("DRAFT"), "MISSING_MAPPING invoice stays DRAFT", ReqNoGlobalAPPolicy),
			eq("exception_runs_one", "ap_runs_exception", "n", lit(1), "EXCEPTION: exactly 1 ap_matching_runs row"),
			eq("exception_exceptions_one", "ap_exceptions_exception", "n", lit(1), "EXCEPTION: exactly 1 ap_exceptions row"),
			eq("exception_keyed_by_run", "ap_exceptions_by_run_exception", "n", lit(1), "EXCEPTION: the exception is keyed by that run"),
			eq("matched_runs_one", "ap_runs_matched", "n", lit(1), "MATCHED: exactly 1 ap_matching_runs row"),
			eq("matched_exceptions_zero", "ap_exceptions_matched", "n", lit(0), "MATCHED: 0 ap_exceptions rows"),
			eq("matched_posted", "ap_invoice_matched", "status", lit(apStatusPosted), "MATCHED: invoice POSTED"),
			eq("matched_posted_by_creator", "ap_invoice_matched", "posted_by", fxv(fxAPCreatedBy), "MATCHED: posted_by = created_by"),
			eq("runs_by_creator_only", "ap_runs_foreign_actor", "n", lit(0), "every run is attributed to the invoice's created_by"),
		},
		Observations: []string{"the sequential delivery after an operator-simulated resolution is covered by the rc.9 integration test (the tool is read-only)"},
	},
	{
		ID:    "S09b-forged-actor-ap",
		Title: "AP invoice processing with a spoofed created_by",
		Tasks: []TaskSpec{{
			N: 1, Phase: 1, Type: jobs.TaskProcessAPInvoice,
			Payload: []Field{{"invoice_id", fxv(fxAPException)}, {"created_by", fxv(fxAdminUserA)}},
			Expect: Expectation{States: []string{stateArchived}, Retried: RetriedZero, LastErrAnyOf: []string{"actor", "created_by"},
				Text: "archived, " + skipRetryNote + ", LastErr names the actor mismatch; no new runs/exceptions (internal/ap/orchestrator.go:75-82, jobs/ap_invoice.go:63-67, cmd/worker/main.go:430)"},
		}},
		Queries: apQueries("exception", fxAPException),
		Assertions: []AssertionSpec{
			unchanged("runs_unchanged", "ap_runs_exception", "zero new ap_matching_runs rows"),
			unchanged("exceptions_unchanged", "ap_exceptions_exception", "zero new ap_exceptions rows"),
			unchanged("invoice_unchanged", "ap_invoice_exception", "ap_invoices status/posted_by unchanged"),
		},
	},
	{
		ID:       "S10-forged-company-bi-export",
		Title:    "BI export for a forged company",
		Requires: []Requirement{ReqNoConnectorConnections},
		Tasks: []TaskSpec{{
			N: 1, Phase: 1, Type: jobs.TaskBIExport,
			Payload: []Field{{"company_id", fxv(fxCompanyB)}, {"period", dyn(dynCurrentPeriod)}, {"provider", lit("awss3")}},
			Expect: Expectation{States: []string{stateArchived}, Retried: RetriedMax, LastErrAnyOf: []string{handlerNotFound},
				Text: `archived, "handler not found" (rc.9 profile gating, jobs/asynq_server.go:79-81); rc.8 "no connection found" = FAIL`},
		}},
		Queries: []QuerySpec{
			{Name: "connector_outbox_ab", Args: []Value{fxv(fxCompanyA), fxv(fxCompanyB)}, SQL: `SELECT company_id, COUNT(*) AS n FROM connector_outbox_commands WHERE company_id IN ($1, $2) GROUP BY company_id ORDER BY company_id`},
			{Name: "connector_connections_ab", Args: []Value{fxv(fxCompanyA), fxv(fxCompanyB)}, SQL: `SELECT COUNT(*) AS n FROM connector_connections WHERE company_id IN ($1, $2)`},
		},
		Assertions: []AssertionSpec{
			unchanged("connector_outbox_unchanged", "connector_outbox_ab", "connector_outbox_commands for A and B unchanged"),
			eq("connector_connections_none", "connector_connections_ab", "n", lit(0), "defense in depth: no connector connection for A or B"),
		},
		Observations: []string{"payload company_id is trusted by the handler (jobs/bi_export.go:55-63, unchanged in rc.9); rc.9 does not register it under v0.10-core (jobs/asynq_server.go:79-81, cmd/worker/main.go:173-175)"},
	},
	{
		ID:       "S11-forged-recipient-mail",
		Title:    "recipient-addressed mail tasks with a forged recipient",
		Requires: []Requirement{ReqEmail},
		Tasks: []TaskSpec{
			{N: 1, Phase: 1, Type: jobs.TaskTypeSendEmail, TaskID: vp(tmpl(s11CorrelationID)), Requires: []Requirement{ReqEmail},
				Payload: []Field{{"to", tmpl(s11Recipient)}, {"subject", tmpl(s11Subject)}, {"body", lit("ISO-004 forged recipient probe")}, {"correlation_id", tmpl(s11CorrelationID)}},
				Expect:  Expectation{States: []string{stateCompleted}, Retried: RetriedAny, Text: "completed; exactly one Mailpit message"},
				Note:    "TaskID = correlation_id (producer convention, jobs/asynq_server.go:157-164)"},
			{N: 2, Phase: 1, Type: jobs.TypeEmailDelivery, Requires: []Requirement{ReqEmail},
				Payload: []Field{{"to", list(tmpl(s11Recipient))}, {"subject", tmpl(s11Subject)}, {"body_html", lit("<p>ISO-004 forged recipient probe</p>")}},
				Expect:  Expectation{States: []string{stateCompleted}, Retried: RetriedAny, Text: "completed; exactly one Mailpit message"}},
			{N: 3, Phase: 2, Type: jobs.TaskTypeSendEmail, ReuseTaskIDOf: 1, Requires: []Requirement{ReqEmail},
				Payload: []Field{{"to", tmpl(s11Recipient)}, {"subject", tmpl(s11Subject)}, {"body", lit("ISO-004 forged recipient probe")}, {"correlation_id", tmpl(s11CorrelationID)}},
				Expect:  Expectation{Conflict: true, Text: "ErrTaskIDConflict recorded"},
				Note:    "same correlation_id TaskID while n=1 is retained"},
		},
		MailRecipients: []Value{tmpl(s11Recipient)},
		Assertions: []AssertionSpec{
			{ID: "mail_none_before", Kind: AssertMailCount, At: snapBefore, Recipient: vp(tmpl(s11Recipient)), Want: vp(lit(0)), Description: "the run-unique recipient had no mail before"},
			{ID: "mail_one_per_task", Kind: AssertMailCount, At: snapAfter, Recipient: vp(tmpl(s11Recipient)), Want: vp(lit(2)), SubjectContains: vp(tmpl(s11Subject)), Description: "exactly one message per task (mail:send + email:deliver)"},
		},
		Observations: []string{"the worker has no tenant binding for recipient-addressed payloads (finding, not FAIL: no company data involved)"},
	},
	{
		ID:       "S12-duplicate-delivery-payslip",
		Title:    "duplicate delivery of a company-B payslip email",
		Requires: []Requirement{ReqEmail},
		Tasks: []TaskSpec{
			{N: 1, Phase: 1, Concurrent: true, Type: jobs.TaskPayrollPayslipEmail, Requires: []Requirement{ReqEmail},
				Payload: []Field{{"payslip_id", fxv(fxPayslipB)}},
				Expect:  Expectation{States: []string{stateCompleted}, Retried: RetriedAny, Text: "completed; one message total since seed"}, Note: "concurrent with n=2"},
			{N: 2, Phase: 1, Concurrent: true, Type: jobs.TaskPayrollPayslipEmail, Requires: []Requirement{ReqEmail},
				Payload: []Field{{"payslip_id", fxv(fxPayslipB)}},
				Expect:  Expectation{States: []string{stateCompleted}, Retried: RetriedAny, Text: "completed; one message total since seed"}, Note: "concurrent with n=1"},
			{N: 3, Phase: 2, Type: jobs.TaskPayrollPayslipEmail, Requires: []Requirement{ReqEmail},
				Payload: []Field{{"payslip_id", fxv(fxPayslipB)}},
				Expect:  Expectation{States: []string{stateCompleted}, Retried: RetriedAny, Text: "completed; delivered_at unchanged"}, Note: "sequential, after n=1,2 converged"},
			{N: 4, Phase: 1, Type: jobs.TaskPayrollPayslipEmail, Requires: []Requirement{ReqEmail},
				Payload: []Field{{"payslip_id", dyn(dynPayslipUnknown)}},
				Expect:  Expectation{States: []string{stateArchived}, Retried: RetriedZero, Text: "archived, " + skipRetryNote + " under rc.9 (payroll.ErrPayslipNotFound maps to SkipRetry, jobs/tasks.go:140-142)"}},
		},
		Queries: []QuerySpec{
			{Name: "payslip_b", Args: []Value{fxv(fxPayslipB)}, SQL: `SELECT ps.id, ps.delivered_at FROM payroll_payslips ps WHERE ps.id = $1`},
			{Name: "payslip_unknown_id", Args: []Value{dyn(dynPayslipUnknown)}, SQL: `SELECT COUNT(*) AS n FROM payroll_payslips WHERE id = $1`},
		},
		MailRecipients: []Value{fxv(fxPayslipEmail)},
		Assertions: []AssertionSpec{
			{ID: "payslip_delivered", Kind: AssertNotNull, Query: "payslip_b", Column: "delivered_at", At: snapAfter, Description: "delivered_at is set"},
			{ID: "payslip_delivered_once", Kind: AssertStableAfterFirst, Query: "payslip_b", Column: "delivered_at", Description: "delivered_at identical in every snapshot after the first delivery"},
			eq("payslip_unknown_absent", "payslip_unknown_id", "n", lit(0), "the forged payslip ID was not created"),
			{ID: "payslip_one_message", Kind: AssertMailCount, At: snapAfter, Recipient: vp(fxv(fxPayslipEmail)), Since: vp(fxv(fxPayslipCreated)), Want: vp(lit(1)), Attachments: intp(1),
				Description: "absolute: exactly one message to the run-unique recipient since the seed, with one PDF attachment (sweep or tool delivery)"},
		},
		Observations: []string{"the payroll:payslip_dispatch sweep (every 5 minutes) may deliver the fixture before the tool runs; mail-before.json records that state"},
	},
}

// apDuplicateTasks declares S09: per fixture invoice two concurrent
// deliveries (phase 1) and one sequential delivery (phase 2).
func apDuplicateTasks() []TaskSpec {
	paths := []struct {
		invoice, expect string
		reqs            []Requirement
	}{
		{fxAPMissing, "MISSING_MAPPING: 0 runs, exactly 1 exception", []Requirement{ReqNoGlobalAPPolicy}},
		{fxAPException, "EXCEPTION: exactly 1 run, 1 exception", nil},
		{fxAPMatched, "MATCHED: exactly 1 run, 0 exceptions, POSTED by created_by", nil},
	}
	var out []TaskSpec
	n := 0
	for _, p := range paths {
		for i, when := range []string{"concurrent", "concurrent", "sequential"} {
			n++
			out = append(out, TaskSpec{
				N: n, Phase: 1 + i/2, Concurrent: i < 2, Type: jobs.TaskProcessAPInvoice, Requires: p.reqs,
				Payload: []Field{{"invoice_id", fxv(p.invoice)}, {"created_by", fxv(fxAPCreatedBy)}},
				Expect:  Expectation{States: converged, Retried: RetriedAny, Text: p.expect},
				Note:    when,
			})
		}
	}
	return out
}

func concatQueries(groups ...[]QuerySpec) []QuerySpec {
	var out []QuerySpec
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// deferredScenarios are listed in the evidence details as deferred (never as
// N/A) by user decision.
var deferredScenarios = []string{
	"boardpack:generate full generation",
	"documents:ocr legitimate duplicate",
	"bankfeeds:event legitimate delivery",
}

// workerPayload is one task type defined in the jobs package at rc.9 with the
// Go struct its handler decodes. Struct is nil for sweeps/scans without a
// payload. The list is compile-time: a renamed or removed jobs struct breaks
// the build, and scenarios_test.go checks it against jobs/*.go.
type workerPayload struct {
	Type   string
	Struct reflect.Type
	Source string // file:line of the struct (or of the type constant)
}

var workerPayloads = []workerPayload{
	{jobs.TaskTypeSendEmail, reflect.TypeOf(jobs.SendEmailPayload{}), "jobs/tasks.go:148"},
	{jobs.TypeEmailDelivery, reflect.TypeOf(jobs.EmailDeliveryPayload{}), "jobs/email_task.go:18"},
	{jobs.TaskInventoryRevaluation, reflect.TypeOf(jobs.InventoryRevaluationPayload{}), "jobs/inventory_reval.go:18"},
	{jobs.TaskProcurementReindex, reflect.TypeOf(jobs.ProcurementReindexPayload{}), "jobs/procure_reindex.go:17"},
	{jobs.TaskFXDailyRates, reflect.TypeOf(jobs.FXDailyRatesPayload{}), "jobs/fx_daily_rates.go:23"},
	{jobs.TaskBIExport, reflect.TypeOf(jobs.BIExportPayload{}), "jobs/bi_export.go:18"},
	{jobs.TaskAnalyticsInsightsWarmup, reflect.TypeOf(jobs.InsightsWarmupPayload{}), "jobs/tasks.go:190"},
	{jobs.TaskAnalyticsAnomalyScan, reflect.TypeOf(jobs.AnomalyScanPayload{}), "jobs/tasks.go:207"},
	{jobs.TaskConsolidateRefresh, reflect.TypeOf(jobs.ConsolidateRefreshPayload{}), "jobs/consolidate_refresh.go:24"},
	{jobs.TaskVarianceSnapshotProcess, reflect.TypeOf(jobs.VarianceSnapshotPayload{}), "jobs/tasks.go:228"},
	{jobs.TaskBoardPackGenerate, reflect.TypeOf(jobs.BoardPackPayload{}), "jobs/tasks.go:233"},
	{jobs.TaskPayrollPayslipEmail, reflect.TypeOf(jobs.PayrollPayslipPayload{}), "jobs/tasks.go:111"},
	{jobs.TypeBankFeedsSync, reflect.TypeOf(jobs.BankFeedsSyncPayload{}), "jobs/bank_feeds.go:18"},
	{jobs.TypeBankFeedsEvent, reflect.TypeOf(jobs.BankFeedsEventPayload{}), "jobs/bank_feeds.go:22"},
	{jobs.TypeCashForecastRefresh, reflect.TypeOf(jobs.CashForecastRefreshPayload{}), "jobs/cash_forecast.go:17"},
	{jobs.TaskDocumentOCR, reflect.TypeOf(jobs.DocumentOCRPayload{}), "jobs/document_ocr.go:15"},
	{jobs.TaskProcessAPInvoice, reflect.TypeOf(jobs.ProcessAPInvoicePayload{}), "jobs/ap_invoice.go:14"},
	{jobs.TypeOverdueInvoicesScan, nil, "jobs/scheduler.go:15"},
	{jobs.TypeReportScheduleScan, nil, "jobs/report_schedule.go:14"},
	{jobs.TaskFixedAssetDepreciation, nil, "jobs/fixed_assets.go:10"},
	{jobs.TaskPayrollPayslipDispatch, nil, "jobs/tasks.go:30"},
	{jobs.TaskTaxCaptureDispatch, nil, "jobs/tasks.go:32"},
	{jobs.TaskCRMReminderDispatch, nil, "jobs/tasks.go:39"},
	{jobs.TaskWebhookDeliveryDispatch, nil, "jobs/tasks.go:40"},
	{jobs.TaskOutboxSweep, nil, "jobs/tasks.go:35"},
	{jobs.TaskFinanceAutomationDispatch, nil, "jobs/tasks.go:43"},
	{jobs.TypeCMMSPMGeneratorScan, nil, "jobs/cmms_pm_generator.go:11"},
	{jobs.TaskDocumentDisposition, nil, "jobs/document_disposition.go:11"},
	{jobs.TaskConnectorOutboxSweep, nil, "jobs/tasks.go:37"},
}

func payloadStructFor(taskType string) (reflect.Type, bool) {
	for _, p := range workerPayloads {
		if p.Type == taskType {
			return p.Struct, true
		}
	}
	return nil, false
}

// branchFields returns every field path of the worker payload structs whose
// Go name or JSON name mentions a branch. It must stay empty for the branch
// N/A claim to hold.
func branchFields() []string {
	var out []string
	for _, p := range workerPayloads {
		if p.Struct == nil {
			continue
		}
		for i := 0; i < p.Struct.NumField(); i++ {
			f := p.Struct.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")[0]
			if strings.Contains(strings.ToLower(f.Name), "branch") || strings.Contains(strings.ToLower(tag), "branch") {
				out = append(out, p.Struct.Name()+"."+f.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// branchNAStatement is the evidence details text for the contract's branch
// forgery (plan Step 3, "Branch scope"). It returns an error when any worker
// payload struct carries a branch field, so the claim is never written
// without the code supporting it.
func branchNAStatement(candidateSHA string) (string, error) {
	if bf := branchFields(); len(bf) > 0 {
		return "", fmt.Errorf("branch N/A claim does not hold: worker payload fields %s carry a branch", strings.Join(bf, ", "))
	}
	return fmt.Sprintf("branch: N/A (no worker payload carries branch_id at %s; see jobs/tasks.go, jobs/bi_export.go, jobs/cash_forecast.go, jobs/bank_feeds.go, jobs/ap_invoice.go, jobs/document_ocr.go)", candidateSHA), nil
}

// ---- rendering ----------------------------------------------------------

// DynamicValue is a resolved dynamic value with the SELECT that produced it.
type DynamicValue struct {
	SQL   string `json:"sql"`
	Value any    `json:"value"`
}

// renderEnv resolves Values. With Dynamic == nil dynamic values render as
// their placeholder (dry run and preflight plan).
type renderEnv struct {
	Cfg      *Config
	Fixtures *Fixtures
	Dynamic  map[string]DynamicValue
}

func (env renderEnv) expand(s string) string {
	run := ""
	if env.Cfg != nil {
		run = env.Cfg.RunID
	}
	return strings.NewReplacer("{run}", run, "{taskroot}", taskIDRoot).Replace(s)
}

func (env renderEnv) resolve(v Value) (any, error) {
	switch v.Kind {
	case ValueLiteral:
		return v.Lit, nil
	case ValueTemplate:
		return env.expand(v.Ref), nil
	case ValueFixture:
		fx := env.Fixtures
		if fx == nil {
			fx = &Fixtures{}
		}
		for _, f := range fixtureFields() {
			if f.Name == v.Ref {
				if f.Numeric {
					return *f.int(fx), nil
				}
				return *f.str(fx), nil
			}
		}
		return nil, fmt.Errorf("unknown fixture %q", v.Ref)
	case ValueDynamic:
		spec, ok := dynamicSpec(v.Ref)
		if !ok {
			return nil, fmt.Errorf("unknown dynamic value %q", v.Ref)
		}
		if env.Dynamic == nil {
			return spec.Placeholder, nil
		}
		dv, ok := env.Dynamic[v.Ref]
		if !ok {
			return nil, fmt.Errorf("dynamic value %q was not resolved", v.Ref)
		}
		return dv.Value, nil
	case ValueList:
		out := make([]any, 0, len(v.Items))
		for _, it := range v.Items {
			r, err := env.resolve(it)
			if err != nil {
				return nil, err
			}
			out = append(out, r)
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown value kind %q", v.Kind)
}

// encodeJSON encodes without HTML escaping so the payload bytes stay
// readable ("<p>" rather than "<p>"); both decode identically.
func encodeJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// renderPayload produces the exact payload bytes in field order.
func (env renderEnv) renderPayload(fields []Field) ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range fields {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := encodeJSON(f.Key)
		if err != nil {
			return nil, err
		}
		val, err := env.resolve(f.Value)
		if err != nil {
			return nil, fmt.Errorf("payload field %s: %w", f.Key, err)
		}
		vb, err := encodeJSON(val)
		if err != nil {
			return nil, fmt.Errorf("payload field %s: %w", f.Key, err)
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// dynamicRefs lists the dynamic values a scenario uses, in dynamicSpecs order.
func (s Scenario) dynamicRefs() []string {
	used := map[string]bool{}
	var walk func(v Value)
	walk = func(v Value) {
		if v.Kind == ValueDynamic {
			used[v.Ref] = true
		}
		for _, it := range v.Items {
			walk(it)
		}
	}
	for _, t := range s.Tasks {
		for _, f := range t.Payload {
			walk(f.Value)
		}
	}
	for _, q := range s.Queries {
		for _, a := range q.Args {
			walk(a)
		}
	}
	for _, a := range s.Assertions {
		for _, v := range []*Value{a.Want, a.Recipient, a.Since, a.SubjectContains} {
			if v != nil {
				walk(*v)
			}
		}
	}
	var out []string
	for _, d := range dynamicSpecs {
		if used[d.Name] {
			out = append(out, d.Name)
		}
	}
	return out
}

// RenderedQuery is a QuerySpec with concrete arguments (Step 4 executes it).
type RenderedQuery struct {
	Name string `json:"name"`
	SQL  string `json:"sql"`
	Args []any  `json:"args"`
}

// RenderedAssertion is an AssertionSpec with concrete values.
type RenderedAssertion struct {
	ID              string        `json:"id"`
	Kind            AssertKind    `json:"kind"`
	Query           string        `json:"query,omitempty"`
	Column          string        `json:"column,omitempty"`
	At              string        `json:"at,omitempty"`
	Want            any           `json:"want,omitempty"`
	Recipient       string        `json:"recipient,omitempty"`
	Since           string        `json:"since,omitempty"`
	Attachments     *int          `json:"attachments,omitempty"`
	SubjectContains string        `json:"subject_contains,omitempty"`
	Requires        []Requirement `json:"requires,omitempty"`
	Description     string        `json:"description"`
}

// renderTasks renders the submissions of a scenario. Render errors are
// recorded on the task (RenderError) rather than aborting, so the plan stays
// printable; the executor refuses to submit a scenario with render errors.
func (env renderEnv) renderTasks(s Scenario) []PlannedTask {
	out := make([]PlannedTask, 0, len(s.Tasks))
	byN := map[int]string{}
	for _, t := range s.Tasks {
		pt := PlannedTask{
			N: t.N, Phase: t.Phase, Concurrent: t.Concurrent, Type: t.Type, Expect: t.Expect.Text,
			Expectation: t.Expect, Requires: t.Requires, Note: t.Note, ReusesTaskIDOf: t.ReuseTaskIDOf,
		}
		var errs []string
		payload, err := env.renderPayload(t.Payload)
		if err != nil {
			errs = append(errs, err.Error())
		}
		pt.Payload = string(payload)
		switch {
		case t.TaskID != nil:
			id, err := env.resolve(*t.TaskID)
			if err != nil {
				errs = append(errs, "task id: "+err.Error())
			}
			pt.TaskID = fmt.Sprint(id)
		case env.Cfg != nil:
			pt.TaskID = env.Cfg.TaskID(s.ID, t.N)
		}
		if t.TaskID != nil || t.ReuseTaskIDOf == 0 {
			byN[t.N] = pt.TaskID
		}
		if len(errs) > 0 {
			pt.RenderError = strings.Join(errs, "; ")
		}
		out = append(out, pt)
	}
	// Conflict probes reuse the TaskID of an earlier submission.
	for i := range out {
		if ref := out[i].ReusesTaskIDOf; ref != 0 {
			id, ok := byN[ref]
			if !ok {
				out[i].RenderError = strings.TrimPrefix(out[i].RenderError+fmt.Sprintf("; reuses TaskID of unknown submission %d", ref), "; ")
				continue
			}
			out[i].TaskID = id
		}
	}
	return out
}

func (env renderEnv) renderQueries(s Scenario) ([]RenderedQuery, error) {
	out := make([]RenderedQuery, 0, len(s.Queries))
	for _, q := range s.Queries {
		rq := RenderedQuery{Name: q.Name, SQL: q.SQL, Args: []any{}}
		for _, a := range q.Args {
			v, err := env.resolve(a)
			if err != nil {
				return nil, fmt.Errorf("query %s: %w", q.Name, err)
			}
			rq.Args = append(rq.Args, v)
		}
		out = append(out, rq)
	}
	return out, nil
}

func (env renderEnv) renderAssertions(s Scenario) ([]RenderedAssertion, error) {
	str := func(v *Value) (string, error) {
		if v == nil {
			return "", nil
		}
		r, err := env.resolve(*v)
		if err != nil {
			return "", err
		}
		return fmt.Sprint(r), nil
	}
	out := make([]RenderedAssertion, 0, len(s.Assertions))
	for _, a := range s.Assertions {
		ra := RenderedAssertion{ID: a.ID, Kind: a.Kind, Query: a.Query, Column: a.Column, At: a.At, Attachments: a.Attachments, Requires: a.Requires, Description: a.Description}
		if a.Want != nil {
			w, err := env.resolve(*a.Want)
			if err != nil {
				return nil, fmt.Errorf("assertion %s: %w", a.ID, err)
			}
			ra.Want = w
		}
		var err error
		if ra.Recipient, err = str(a.Recipient); err != nil {
			return nil, fmt.Errorf("assertion %s: %w", a.ID, err)
		}
		if ra.Since, err = str(a.Since); err != nil {
			return nil, fmt.Errorf("assertion %s: %w", a.ID, err)
		}
		if ra.SubjectContains, err = str(a.SubjectContains); err != nil {
			return nil, fmt.Errorf("assertion %s: %w", a.ID, err)
		}
		out = append(out, ra)
	}
	return out, nil
}

func (env renderEnv) renderRecipients(s Scenario) ([]string, error) {
	var out []string
	for _, r := range s.MailRecipients {
		v, err := env.resolve(r)
		if err != nil {
			return nil, fmt.Errorf("mail recipient: %w", err)
		}
		out = append(out, fmt.Sprint(v))
	}
	return out, nil
}

// renderScenario renders everything the executor and Step 4 need.
func (env renderEnv) renderScenario(s Scenario) (ScenarioPlan, error) {
	sp := ScenarioPlan{ID: s.ID, Title: s.Title, Enabled: true, Tasks: env.renderTasks(s), Observations: s.Observations}
	var errs []string
	for _, t := range sp.Tasks {
		if t.RenderError != "" {
			errs = append(errs, fmt.Sprintf("task %d: %s", t.N, t.RenderError))
		}
	}
	var err error
	if sp.Queries, err = env.renderQueries(s); err != nil {
		errs = append(errs, err.Error())
	}
	if sp.Assertions, err = env.renderAssertions(s); err != nil {
		errs = append(errs, err.Error())
	}
	if sp.MailRecipients, err = env.renderRecipients(s); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return sp, fmt.Errorf("render %s: %s", s.ID, strings.Join(errs, "; "))
	}
	return sp, nil
}
