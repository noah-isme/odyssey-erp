package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const jobsDir = "../../../jobs"

// sampleDynamic resolves every dynamic value to a deterministic sample.
func sampleDynamic() map[string]DynamicValue {
	out := map[string]DynamicValue{}
	for i, d := range dynamicSpecs {
		var v any = int64(900000 + i)
		if d.Kind == "text" {
			v = "2026-10"
		}
		out[d.Name] = DynamicValue{SQL: d.SQL, Value: v}
	}
	return out
}

func sampleEnv(t *testing.T) renderEnv {
	t.Helper()
	fx, _, err := loadFixtures(sampleFixtures, envMap(nil))
	require.NoError(t, err)
	return renderEnv{Cfg: &Config{RunID: "9000000001", MaxRetry: 2}, Fixtures: fx, Dynamic: sampleDynamic()}
}

var placeholderRe = regexp.MustCompile(`\$([0-9]+)`)

func walkValues(v Value, fn func(Value)) {
	fn(v)
	for _, it := range v.Items {
		walkValues(it, fn)
	}
}

// TestScenarioTable iterates the scenario table and checks every row is
// well-formed data: numbering, phases, payloads, TaskIDs, expectations,
// queries and assertions.
func TestScenarioTable(t *testing.T) {
	fixtureNames := map[string]bool{}
	for _, f := range fixtureFields() {
		fixtureNames[f.Name] = true
	}
	env := sampleEnv(t)
	for _, s := range scenarioRegistry {
		t.Run(s.ID, func(t *testing.T) {
			require.NotEmpty(t, s.Tasks)
			assert.NotEmpty(t, s.Title)

			checkValue := func(where string, v Value) {
				walkValues(v, func(v Value) {
					switch v.Kind {
					case ValueFixture:
						assert.True(t, fixtureNames[v.Ref], "%s: unknown fixture %s", where, v.Ref)
					case ValueDynamic:
						_, ok := dynamicSpec(v.Ref)
						assert.True(t, ok, "%s: unknown dynamic %s", where, v.Ref)
					case ValueLiteral, ValueTemplate, ValueList:
					default:
						t.Errorf("%s: unknown value kind %q", where, v.Kind)
					}
				})
			}

			ns := map[int]bool{}
			for i, task := range s.Tasks {
				assert.Equal(t, i+1, task.N, "submissions are numbered 1..n in order")
				ns[task.N] = true
				assert.GreaterOrEqual(t, task.Phase, 1, "task %d phase", task.N)
				assert.NotEmpty(t, task.Expect.Text, "task %d expectation text", task.N)
				if task.Expect.Conflict {
					assert.NotZero(t, task.ReuseTaskIDOf, "task %d: a conflict probe reuses an earlier TaskID", task.N)
					assert.Empty(t, task.Expect.States)
				} else {
					assert.NotEmpty(t, task.Expect.States, "task %d: expected final states", task.N)
					assert.NotEmpty(t, task.Expect.Retried, "task %d: retry expectation", task.N)
					for _, st := range task.Expect.States {
						assert.Contains(t, []string{stateCompleted, stateArchived}, st)
					}
				}
				if task.ReuseTaskIDOf != 0 {
					assert.True(t, ns[task.ReuseTaskIDOf] || task.ReuseTaskIDOf < task.N, "task %d reuses an earlier submission", task.N)
					assert.True(t, task.Expect.Conflict, "task %d reuses a TaskID and must expect a conflict", task.N)
				}
				for _, f := range task.Payload {
					checkValue(fmt.Sprintf("task %d field %s", task.N, f.Key), f.Value)
				}
				if task.TaskID != nil {
					checkValue(fmt.Sprintf("task %d task id", task.N), *task.TaskID)
				}
				for _, r := range append(append([]Requirement{}, task.Requires...), s.Requires...) {
					assert.Contains(t, []Requirement{ReqEmail, ReqNoConnectorConnections, ReqNoGlobalAPPolicy}, r)
				}
			}

			sp, err := env.renderScenario(s)
			require.NoError(t, err)
			for _, pt := range sp.Tasks {
				assert.True(t, strings.HasPrefix(pt.TaskID, "iso004:9000000001:"), pt.TaskID)
				assert.True(t, json.Valid([]byte(pt.Payload)), "payload %s", pt.Payload)
				assert.NotContains(t, pt.Payload, "<max(", "dynamic values are resolved")
				if pt.TaskID != "" && s.Tasks[pt.N-1].TaskID == nil && pt.ReusesTaskIDOf == 0 {
					assert.Equal(t, env.Cfg.TaskID(s.ID, pt.N), pt.TaskID)
				}
			}

			queries := map[string]QuerySpec{}
			for _, q := range s.Queries {
				assert.NotContains(t, queries, q.Name, "duplicate query %s", q.Name)
				queries[q.Name] = q
				assertSelectOnly(t, q.SQL)
				maxN := 0
				for _, m := range placeholderRe.FindAllStringSubmatch(q.SQL, -1) {
					n, _ := strconv.Atoi(m[1])
					if n > maxN {
						maxN = n
					}
				}
				assert.Equal(t, maxN, len(q.Args), "query %s argument count", q.Name)
				for _, a := range q.Args {
					checkValue("query "+q.Name, a)
				}
			}
			ids := map[string]bool{}
			for _, a := range s.Assertions {
				assert.False(t, ids[a.ID], "duplicate assertion %s", a.ID)
				ids[a.ID] = true
				assert.NotEmpty(t, a.Description, a.ID)
				switch a.Kind {
				case AssertMailCount:
					require.NotNil(t, a.Recipient, a.ID)
					require.NotNil(t, a.Want, a.ID)
					assert.NotEmpty(t, s.MailRecipients, "%s: mail assertions need recorded recipients", a.ID)
				case AssertUnchanged, AssertStableAfterFirst:
					assert.Contains(t, queries, a.Query, a.ID)
				case AssertEquals, AssertContains:
					assert.Contains(t, queries, a.Query, a.ID)
					assert.NotEmpty(t, a.Column, a.ID)
					require.NotNil(t, a.Want, a.ID)
				case AssertNotNull:
					assert.Contains(t, queries, a.Query, a.ID)
					assert.NotEmpty(t, a.Column, a.ID)
				default:
					t.Errorf("%s: unknown assertion kind %q", a.ID, a.Kind)
				}
				if a.Kind == AssertStableAfterFirst {
					assert.Greater(t, len(phaseNumbers(sp.Tasks)), 1, "%s needs more than one phase", a.ID)
				}
				assert.Contains(t, []string{"", snapBefore, snapAfter}, a.At, a.ID)
			}
			if len(s.MailRecipients) > 0 {
				assert.Contains(t, s.Requires, ReqEmail, "mail scenarios are gated by the email gate")
			}
		})
	}
}

// TestScenarioPayloadsDecodeIntoJobsStructs: every well-formed payload
// decodes strictly (no unknown field) into the struct the worker's handler
// uses; malformed ones must not.
func TestScenarioPayloadsDecodeIntoJobsStructs(t *testing.T) {
	env := sampleEnv(t)
	for _, s := range scenarioRegistry {
		sp, err := env.renderScenario(s)
		require.NoError(t, err)
		for i, pt := range sp.Tasks {
			spec := s.Tasks[i]
			typ, registered := payloadStructFor(pt.Type)
			if !registered {
				assert.Equal(t, "iso004:unregistered", pt.Type, "%s/%d uses a type outside the worker matrix", s.ID, pt.N)
				continue
			}
			require.NotNil(t, typ, "%s/%d: %s has no payload", s.ID, pt.N, pt.Type)
			dec := json.NewDecoder(bytes.NewReader([]byte(pt.Payload)))
			dec.DisallowUnknownFields()
			err := dec.Decode(reflect.New(typ).Interface())
			if spec.Malformed {
				assert.Error(t, err, "%s/%d is declared malformed", s.ID, pt.N)
			} else {
				assert.NoError(t, err, "%s/%d payload %s into %s", s.ID, pt.N, pt.Payload, typ.Name())
			}
		}
	}
}

func TestScenarioRenderedPayloadBytes(t *testing.T) {
	env := sampleEnv(t)
	byID := map[string]ScenarioPlan{}
	for _, s := range scenarioRegistry {
		sp, err := env.renderScenario(s)
		require.NoError(t, err)
		byID[s.ID] = sp
	}
	task := func(id string, n int) PlannedTask {
		for _, pt := range byID[id].Tasks {
			if pt.N == n {
				return pt
			}
		}
		t.Fatalf("%s/%d not found", id, n)
		return PlannedTask{}
	}
	dynID := func(name string) int64 { return sampleDynamic()[name].Value.(int64) }

	assert.Equal(t, `{}`, task("S01-unregistered-type", 1).Payload)
	assert.Equal(t, `{"snapshot_id":"x"}`, task("S02-malformed-payload", 1).Payload)
	assert.Equal(t, `{"board_pack_id":0}`, task("S02-malformed-payload", 2).Payload)
	assert.Equal(t, fmt.Sprintf(`{"snapshot_id":%d}`, dynID(dynVarianceUnknown)), task("S03-object-not-found", 1).Payload)
	assert.Equal(t, `{"snapshot_id":2}`, task("S05-forged-object-variance", 1).Payload)
	assert.Equal(t, `{"job_id":1}`, task("S06-forged-crossscope-ocr", 1).Payload)
	assert.Equal(t, `{"company_id":3,"scenario_id":6}`, task("S08-unregistered-under-profile", 1).Payload)
	assert.Equal(t, `{"invoice_id":2,"created_by":1}`, task("S09b-forged-actor-ap", 1).Payload)
	assert.Equal(t, `{"company_id":4,"period":"2026-10","provider":"awss3"}`, task("S10-forged-company-bi-export", 1).Payload)
	assert.Equal(t, `{"to":"iso004-9000000001@staging.invalid","subject":"ISO-004 9000000001","body":"ISO-004 forged recipient probe","correlation_id":"iso004:9000000001:S11"}`, task("S11-forged-recipient-mail", 1).Payload)
	assert.Equal(t, `{"to":["iso004-9000000001@staging.invalid"],"subject":"ISO-004 9000000001","body_html":"<p>ISO-004 forged recipient probe</p>"}`, task("S11-forged-recipient-mail", 2).Payload)
	assert.Equal(t, `{"payslip_id":1}`, task("S12-duplicate-delivery-payslip", 1).Payload)

	// mail:send uses the producer convention TaskID = correlation_id; the
	// conflict probe reuses it.
	s11 := task("S11-forged-recipient-mail", 1)
	assert.Equal(t, "iso004:9000000001:S11", s11.TaskID)
	assert.Equal(t, s11.TaskID, task("S11-forged-recipient-mail", 3).TaskID)
	assert.Equal(t, s11.Payload, task("S11-forged-recipient-mail", 3).Payload)
	assert.Equal(t, task("S07-duplicate-delivery-variance", 1).TaskID, task("S07-duplicate-delivery-variance", 4).TaskID)
	assert.Equal(t, "iso004:9000000001:S07-duplicate-delivery-variance:2", task("S07-duplicate-delivery-variance", 2).TaskID)

	// S09: two concurrent deliveries (phase 1) + one sequential (phase 2) per invoice.
	s09 := byID["S09-duplicate-delivery-ap"]
	require.Len(t, s09.Tasks, 9)
	for i, inv := range []int64{1, 2, 4} {
		for j := 0; j < 3; j++ {
			pt := s09.Tasks[i*3+j]
			assert.Equal(t, fmt.Sprintf(`{"invoice_id":%d,"created_by":6}`, inv), pt.Payload)
			assert.Equal(t, j < 2, pt.Concurrent)
			assert.Equal(t, 1+j/2, pt.Phase)
		}
	}

	// Rendered assertions carry concrete values for Step 4.
	var s12mail RenderedAssertion
	for _, a := range byID["S12-duplicate-delivery-payslip"].Assertions {
		if a.ID == "payslip_one_message" {
			s12mail = a
		}
	}
	assert.Equal(t, "iso004-payslip-9000000001@staging.invalid", s12mail.Recipient)
	assert.Equal(t, "2026-10-04T04:41:31.730731Z", s12mail.Since)
	assert.Equal(t, 1, s12mail.Want)
	require.NotNil(t, s12mail.Attachments)
	assert.Equal(t, 1, *s12mail.Attachments)
	assert.Equal(t, []string{"iso004-payslip-9000000001@staging.invalid"}, byID["S12-duplicate-delivery-payslip"].MailRecipients)
}

func TestRenderWithoutDynamicValuesUsesPlaceholders(t *testing.T) {
	env := sampleEnv(t)
	env.Dynamic = nil
	sp := planScenario(env, scenarioRegistry[2], nil)
	assert.Empty(t, sp.RenderError)
	assert.Equal(t, `{"snapshot_id":"<max(variance_snapshots.id)+100000>"}`, sp.Tasks[0].Payload)

	// Resolution with a missing dynamic value is an error, never a zero ID.
	env.Dynamic = map[string]DynamicValue{}
	_, err := env.renderScenario(scenarioRegistry[2])
	assert.ErrorContains(t, err, "was not resolved")

	_, err = renderEnv{}.resolve(fxv("STAGING_CERT_ISO004_NOPE"))
	assert.ErrorContains(t, err, "unknown fixture")
}

func TestDynamicSpecsAreSelectOnly(t *testing.T) {
	names := map[string]bool{}
	for _, d := range dynamicSpecs {
		assert.False(t, names[d.Name])
		names[d.Name] = true
		assertSelectOnly(t, d.SQL)
		assert.Contains(t, []string{"int", "text"}, d.Kind)
		assert.True(t, strings.HasPrefix(d.Placeholder, "<"))
	}
}

// TestBranchScopeIsNA backs the evidence claim "no worker payload carries
// branch_id" with the compile-time payload list.
func TestBranchScopeIsNA(t *testing.T) {
	assert.Empty(t, branchFields())
	stmt, err := branchNAStatement(testSHA)
	require.NoError(t, err)
	assert.Equal(t, "branch: N/A (no worker payload carries branch_id at "+testSHA+"; see jobs/tasks.go, jobs/bi_export.go, jobs/cash_forecast.go, jobs/bank_feeds.go, jobs/ap_invoice.go, jobs/document_ocr.go)", stmt)

	// The JSON field names of every listed struct, checked independently of
	// the Go names.
	for _, p := range workerPayloads {
		if p.Struct == nil {
			continue
		}
		for i := 0; i < p.Struct.NumField(); i++ {
			tag := p.Struct.Field(i).Tag.Get("json")
			assert.NotContains(t, strings.ToLower(tag), "branch", "%s.%s", p.Struct.Name(), p.Struct.Field(i).Name)
		}
	}
}

// TestWorkerPayloadListCoversJobs scans jobs/*.go so a payload struct or task
// type added to the worker cannot escape the branch N/A check.
func TestWorkerPayloadListCoversJobs(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(jobsDir, "*.go"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "jobs sources not found at %s", jobsDir)

	structRe := regexp.MustCompile(`(?m)^type ([A-Za-z0-9_]+Payload) struct`)
	constRe := regexp.MustCompile(`(?m)^\s*(?:const\s+)?[A-Z][A-Za-z0-9_]*\s*=\s*"([a-z_]+:[a-z_-]+)"`)
	var structs, types []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		for _, m := range structRe.FindAllStringSubmatch(string(src), -1) {
			structs = append(structs, m[1])
		}
		for _, m := range constRe.FindAllStringSubmatch(string(src), -1) {
			types = append(types, m[1])
		}
	}
	listedStructs := map[string]bool{}
	listedTypes := map[string]bool{}
	for _, p := range workerPayloads {
		listedTypes[p.Type] = true
		if p.Struct != nil {
			listedStructs[p.Struct.Name()] = true
		}
	}
	sort.Strings(structs)
	for _, s := range structs {
		assert.True(t, listedStructs[s], "jobs payload struct %s is missing from workerPayloads", s)
	}
	assert.Len(t, listedStructs, len(structs), "workerPayloads lists exactly the jobs payload structs")
	for _, typ := range types {
		assert.True(t, listedTypes[typ], "task type %s is missing from workerPayloads", typ)
	}
	assert.Len(t, workerPayloads, len(listedTypes), "task types are unique")
}

// TestWorkerPayloadSourcesCiteDeclarations checks every file:line citation.
func TestWorkerPayloadSourcesCiteDeclarations(t *testing.T) {
	for _, p := range workerPayloads {
		file, lineStr, ok := strings.Cut(p.Source, ":")
		require.True(t, ok, p.Source)
		line, err := strconv.Atoi(lineStr)
		require.NoError(t, err)
		src, err := os.ReadFile(filepath.Join(jobsDir, strings.TrimPrefix(file, "jobs/")))
		require.NoError(t, err, p.Source)
		lines := strings.Split(string(src), "\n")
		require.LessOrEqual(t, line, len(lines), p.Source)
		got := lines[line-1]
		if p.Struct != nil {
			assert.Contains(t, got, "type "+p.Struct.Name()+" struct", p.Source)
		} else {
			assert.Contains(t, got, `"`+p.Type+`"`, p.Source)
		}
	}
}

const repoRoot = "../../.."

// citationAnchors maps every source citation written into scenario text
// (Expect.Text, Note, Observations) to a token the cited lines must contain.
// A citation without an anchor fails the test, so a moved or removed line is
// caught here instead of misleading the operator at run time.
var citationAnchors = map[string]string{
	"internal/variance/job.go:34-39":    "asynq.SkipRetry",
	"internal/variance/job.go:44-47":    "ErrSnapshotNotFound",
	"internal/boardpack/job.go:50-55":   "BoardPackID == 0",
	"internal/boardpack/job.go:58-61":   "ErrBoardPackNotFound",
	"internal/documents/ocr.go:83-86":   "GetOCRJob",
	"internal/documents/ocr.go:106-107": "does not match document version",
	"jobs/document_ocr.go:54":           "ProcessOCRJob",
	"internal/ap/orchestrator.go:75-82": "ErrActorMismatch",
	"jobs/ap_invoice.go:63-67":          "asynq.SkipRetry",
	"cmd/worker/main.go:430":            "ap.ErrActorMismatch",
	"cmd/worker/main.go:127-132":        "TypeCashForecastRefresh",
	"cmd/worker/main.go:173-175":        "ReleaseProfileV010Core",
	"jobs/asynq_server.go:79-81":        "TaskBIExport",
	"jobs/asynq_server.go:157-164":      "TaskID(payload.CorrelationID)",
	"jobs/bi_export.go:55-63":           "GenerateBIExport",
	"jobs/tasks.go:140-142":             "ErrPayslipNotFound",
}

// TestScenarioTextCitationsMatchSource checks that every file:line citation
// in the scenario table resolves to the code it claims to describe.
func TestScenarioTextCitationsMatchSource(t *testing.T) {
	citeRe := regexp.MustCompile(`(?:internal|jobs|cmd)/[A-Za-z0-9_/.-]+\.go:(\d+)(?:-(\d+))?`)
	var texts []string
	for _, s := range scenarioRegistry {
		texts = append(texts, s.Observations...)
		for _, task := range s.Tasks {
			texts = append(texts, task.Expect.Text, task.Note)
		}
	}
	seen := map[string]bool{}
	for _, text := range texts {
		for _, m := range citeRe.FindAllStringSubmatch(text, -1) {
			seen[m[0]] = true
		}
	}
	require.NotEmpty(t, seen, "scenario table cites no source lines")
	for cite := range seen {
		anchor, ok := citationAnchors[cite]
		if !assert.True(t, ok, "citation %s has no anchor in citationAnchors", cite) {
			continue
		}
		file, rng, _ := strings.Cut(cite, ":")
		startStr, endStr, hasEnd := strings.Cut(rng, "-")
		start, err := strconv.Atoi(startStr)
		require.NoError(t, err, cite)
		end := start
		if hasEnd {
			end, err = strconv.Atoi(endStr)
			require.NoError(t, err, cite)
		}
		src, err := os.ReadFile(filepath.Join(repoRoot, file))
		require.NoError(t, err, cite)
		lines := strings.Split(string(src), "\n")
		require.LessOrEqual(t, end, len(lines), cite)
		assert.Contains(t, strings.Join(lines[start-1:end], "\n"), anchor, cite)
	}
	for cite := range citationAnchors {
		assert.True(t, seen[cite], "citationAnchors entry %s is not cited by the scenario table", cite)
	}
}
