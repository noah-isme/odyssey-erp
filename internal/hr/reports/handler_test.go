package reports

import (
	"context"
	"encoding/csv"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/payroll"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

const injectedDept = `=HYPERLINK("http://evil.example","click")`

type handlerPerms struct{ permissions []string }

func (p handlerPerms) EffectivePermissions(context.Context, int64) ([]string, error) {
	return p.permissions, nil
}

// fakeReports is an in-memory HandlerService that records which reports ran.
type fakeReports struct {
	calls     map[string]int
	companies []int64
	err       error
}

func newFakeReports() *fakeReports { return &fakeReports{calls: map[string]int{}} }

func (f *fakeReports) hit(name string, company int64) {
	f.calls[name]++
	f.companies = append(f.companies, company)
}

func (f *fakeReports) Headcount(_ context.Context, c int64, asOf time.Time) (HeadcountReport, error) {
	f.hit(ReportHeadcount, c)
	return HeadcountReport{AsOf: asOf, Total: 7, Rows: []HeadcountRow{
		{DepartmentID: 1, Department: injectedDept, EmploymentType: "PERMANENT", Count: 4},
		{DepartmentID: 2, Department: "Finance", EmploymentType: "CONTRACT", Count: 3},
	}}, f.err
}

func (f *fakeReports) HiresAndLeaversByMonth(_ context.Context, c int64, _, _ time.Time) ([]HiresLeaversRow, error) {
	f.hit(ReportHiresLeavers, c)
	return []HiresLeaversRow{
		{Month: "2026-09", DepartmentID: 1, Department: injectedDept, Hires: 2},
		{Month: "2026-10", DepartmentID: 2, Department: "Finance", Hires: 1, Leavers: 1},
	}, f.err
}

func (f *fakeReports) Turnover(_ context.Context, c int64, from, to time.Time) (TurnoverReport, error) {
	f.hit(ReportTurnover, c)
	return TurnoverReport{From: from, To: to, Leavers: 1, OpeningHeadcount: 6, ClosingHeadcount: 7, AverageHeadcount: 6.5, Rate: 1 / 6.5}, f.err
}

func (f *fakeReports) Absenteeism(_ context.Context, c int64, from, to time.Time) (AbsenteeismReport, error) {
	f.hit(ReportAbsenteeism, c)
	return AbsenteeismReport{From: from, To: to, Employees: 7, AbsentDays: 3, ScheduledDays: 300, Rate: 0.01,
		ByDepartment: []AbsenteeismRow{{DepartmentID: 1, Department: injectedDept, Employees: 4, AbsentDays: 3, ScheduledDays: 170, Rate: 3.0 / 170}}}, f.err
}

func (f *fakeReports) LeaveLiability(_ context.Context, c int64, year int) (LeaveLiabilityReport, error) {
	f.hit(ReportLeaveLiability, c)
	return LeaveLiabilityReport{Year: year, ValuationDate: time.Date(year, 10, 12, 0, 0, 0, 0, time.UTC), AvailableDays: "24.00", Amount: "9876543.21",
		Rows: []LeaveLiabilityRow{{DepartmentID: 1, Department: injectedDept, Employees: 4, AvailableDays: "24.00", Amount: "9876543.21"}}}, f.err
}

func (f *fakeReports) OvertimeCost(_ context.Context, c int64, from, to time.Time) (OvertimeCostReport, error) {
	f.hit(ReportOvertimeCost, c)
	return OvertimeCostReport{From: from, To: to, Posted: 500000, Unrun: 250000, Total: 750000, UnrunMinutes: 90,
		ByDepartment: []OvertimeCostRow{{DepartmentID: 1, Department: injectedDept, Posted: payroll.Money(500000), Unrun: 250000, Total: 750000, UnrunMinutes: 90}}}, f.err
}

func (f *fakeReports) UpcomingExpiries(_ context.Context, c int64, _ int) ([]Expiry, error) {
	f.hit(ReportExpiries, c)
	return []Expiry{{EmployeeID: 9, EmployeeNumber: "E-009", Name: "Ada Lovelace", DepartmentID: 1, Department: injectedDept,
		Kind: ExpiryContractEnd, ExpiresOn: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)}}, f.err
}

type fakeAudit struct {
	logs []shared.AuditLog
	err  error
}

func (a *fakeAudit) Record(_ context.Context, l shared.AuditLog) error {
	a.logs = append(a.logs, l)
	return a.err
}

var (
	reportsOnly  = []string{shared.PermHRReportsView}
	reportsComp  = []string{shared.PermHRReportsView, shared.PermHRCompensationView}
	compOnly     = []string{shared.PermHRCompensationView, shared.PermHREmployeeView}
	sensitiveHdr = []string{"national_id", "tax_id", "npwp", "nik", "date_of_birth", "dob", "birth", "address"}
)

func reportsRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	session, err := manager.Load(req.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	session.SetUser("3")
	session.Set("company_id", "1")
	return req.WithContext(shared.ContextWithSession(req.Context(), session))
}

func newReportsRouter(t *testing.T, perms []string, svc *fakeReports, audit *fakeAudit, withTemplates bool) http.Handler {
	t.Helper()
	var engine *view.Engine
	if withTemplates {
		var err error
		if engine, err = view.NewEngine(); err != nil {
			t.Fatal(err)
		}
	}
	var a Auditor
	if audit != nil {
		a = audit
	}
	h := NewHandler(nil, svc, a, engine, shared.NewCSRFManager("secret"), rbac.Middleware{Service: handlerPerms{perms}})
	h.now = func() time.Time { return time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC) }
	router := chi.NewRouter()
	h.MountRoutes(router)
	return router
}

func serve(t *testing.T, router http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, reportsRequest(t, path))
	return rec
}

func csvPath(name string) string { return "/hr/reports/" + name + ".csv" }

func TestReportsRequireReportsView(t *testing.T) {
	paths := []string{"/hr/dashboard"}
	for _, name := range ReportNames() {
		paths = append(paths, csvPath(name))
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			svc, audit := newFakeReports(), &fakeAudit{}
			rec := serve(t, newReportsRouter(t, compOnly, svc, audit, false), path)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d want 403", rec.Code)
			}
			if len(svc.calls) != 0 || len(audit.logs) != 0 {
				t.Fatalf("forbidden request reached the service: calls=%v audit=%d", svc.calls, len(audit.logs))
			}
		})
	}
}

func TestCompensationCSVsRequireCompensationView(t *testing.T) {
	for _, name := range []string{ReportLeaveLiability, ReportOvertimeCost} {
		t.Run(name, func(t *testing.T) {
			svc, audit := newFakeReports(), &fakeAudit{}
			rec := serve(t, newReportsRouter(t, reportsOnly, svc, audit, false), csvPath(name))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d want 403", rec.Code)
			}
			if svc.calls[name] != 0 || len(audit.logs) != 0 {
				t.Fatalf("compensation report computed without permission: calls=%v", svc.calls)
			}
		})
	}
}

func TestCSVExports(t *testing.T) {
	wantHeader := map[string][]string{
		ReportHeadcount:      {"as_of", "department", "employment_type", "headcount"},
		ReportHiresLeavers:   {"month", "department", "hires", "leavers"},
		ReportTurnover:       {"from", "to", "leavers", "opening_headcount", "closing_headcount", "average_headcount", "turnover_rate"},
		ReportAbsenteeism:    {"from", "to", "department", "employees", "absent_days", "scheduled_days", "absenteeism_rate"},
		ReportLeaveLiability: {"year", "valuation_date", "department", "employees", "employees_without_salary", "available_days", "liability_amount"},
		ReportOvertimeCost:   {"from", "to", "department", "posted_amount", "unrun_amount", "total_amount", "unrun_minutes"},
		ReportExpiries:       {"employee_number", "employee_name", "department", "expiry_kind", "expires_on"},
	}
	if len(wantHeader) != len(ReportNames()) {
		t.Fatalf("header table covers %d of %d reports", len(wantHeader), len(ReportNames()))
	}
	for _, name := range ReportNames() {
		t.Run(name, func(t *testing.T) {
			svc, audit := newFakeReports(), &fakeAudit{}
			rec := serve(t, newReportsRouter(t, reportsComp, svc, audit, false), csvPath(name))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "text/csv" {
				t.Fatalf("content-type=%q", ct)
			}
			wantDisp := `attachment; filename="hr_` + strings.ReplaceAll(name, "-", "_") + `_20261012.csv"`
			if cd := rec.Header().Get("Content-Disposition"); cd != wantDisp {
				t.Fatalf("content-disposition=%q want %q", cd, wantDisp)
			}
			records, err := csv.NewReader(strings.NewReader(rec.Body.String())).ReadAll()
			if err != nil {
				t.Fatalf("parse csv: %v", err)
			}
			if len(records) < 2 {
				t.Fatalf("want header and data rows, got %v", records)
			}
			if got := strings.Join(records[0], ","); got != strings.Join(wantHeader[name], ",") {
				t.Fatalf("header=%q want %q", got, strings.Join(wantHeader[name], ","))
			}
			for _, col := range records[0] {
				for _, bad := range sensitiveHdr {
					if strings.Contains(strings.ToLower(col), bad) {
						t.Fatalf("header %q contains sensitive name %q", col, bad)
					}
				}
			}
			if name != ReportTurnover {
				dept := -1
				for i, col := range records[0] {
					if col == "department" {
						dept = i
					}
				}
				if got := records[1][dept]; got != "'"+injectedDept {
					t.Fatalf("department cell=%q want %q", got, "'"+injectedDept)
				}
			}
			for _, rec := range records {
				for _, cell := range rec {
					if c := strings.TrimLeft(cell, " "); c != "" && strings.ContainsRune("=+-@\t\r", rune(c[0])) {
						t.Fatalf("unescaped formula cell %q", cell)
					}
				}
			}
			if svc.calls[name] != 1 || len(svc.calls) != 1 {
				t.Fatalf("calls=%v", svc.calls)
			}
			if svc.companies[0] != 1 {
				t.Fatalf("company=%d want 1", svc.companies[0])
			}
			if len(audit.logs) != 1 {
				t.Fatalf("audit logs=%d want 1", len(audit.logs))
			}
			l := audit.logs[0]
			if l.Action != ActionReportExport || l.EntityID != name || l.ActorID != 3 || l.Meta["report"] != name {
				t.Fatalf("audit=%+v", l)
			}
			if f, ok := l.Meta["filter"].(map[string]string); !ok || len(f) == 0 {
				t.Fatalf("audit filter=%#v", l.Meta["filter"])
			}
		})
	}
}

func TestCSVExportFilters(t *testing.T) {
	svc, audit := newFakeReports(), &fakeAudit{}
	router := newReportsRouter(t, reportsOnly, svc, audit, false)
	rec := serve(t, router, csvPath(ReportHiresLeavers)+"?from=2025-01-01&to=2025-06-30")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	f := audit.logs[0].Meta["filter"].(map[string]string)
	if f["from"] != "2025-01-01" || f["to"] != "2025-06-30" {
		t.Fatalf("filter=%v", f)
	}
	for _, path := range []string{csvPath(ReportHeadcount) + "?as_of=yesterday", csvPath(ReportExpiries) + "?days=x", csvPath(ReportTurnover) + "?from=2026-13-01"} {
		before := len(audit.logs)
		if rec := serve(t, router, path); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d want 400", path, rec.Code)
		}
		if len(audit.logs) != before {
			t.Fatalf("%s: failed export was audited", path)
		}
	}
}

func TestCSVExportServiceErrors(t *testing.T) {
	svc, audit := newFakeReports(), &fakeAudit{}
	svc.err = ErrInvalidRange
	if rec := serve(t, newReportsRouter(t, reportsOnly, svc, audit, false), csvPath(ReportTurnover)); rec.Code != http.StatusBadRequest {
		t.Fatalf("validation error status=%d want 400", rec.Code)
	}
	svc.err = errors.New("db down")
	rec := serve(t, newReportsRouter(t, reportsOnly, svc, audit, false), csvPath(ReportHeadcount))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "db down") {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if len(audit.logs) != 0 {
		t.Fatal("failed export was audited")
	}
}

func TestCSVExportAuditIsBestEffort(t *testing.T) {
	svc, audit := newFakeReports(), &fakeAudit{err: errors.New("audit down")}
	rec := serve(t, newReportsRouter(t, reportsOnly, svc, audit, false), csvPath(ReportHeadcount))
	if rec.Code != http.StatusOK || len(audit.logs) != 1 {
		t.Fatalf("status=%d audits=%d", rec.Code, len(audit.logs))
	}
	rec = serve(t, newReportsRouter(t, reportsOnly, newFakeReports(), nil, false), csvPath(ReportHeadcount))
	if rec.Code != http.StatusOK {
		t.Fatalf("nil auditor status=%d", rec.Code)
	}
}

func TestDashboardRendersWithoutCompensation(t *testing.T) {
	svc := newFakeReports()
	rec := serve(t, newReportsRouter(t, reportsOnly, svc, nil, true), "/hr/dashboard")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"HR Dashboard", `data-kpi="headcount"`, `data-kpi="hires-leavers"`, `data-kpi="turnover"`, `data-kpi="absenteeism"`, `data-kpi="expiries"`,
		"Ada Lovelace", "/hr/reports/headcount.csv", "/hr/reports/expiries.csv", `href="/hr/dashboard"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard missing %q", want)
		}
	}
	for _, absent := range []string{`data-kpi="leave-liability"`, `data-kpi="overtime-cost"`, "Leave Liability", "Overtime Cost", "9876543.21", "/hr/reports/leave-liability.csv", "/hr/reports/overtime-cost.csv"} {
		if strings.Contains(body, absent) {
			t.Fatalf("dashboard without hr.compensation.view contains %q", absent)
		}
	}
	if svc.calls[ReportLeaveLiability] != 0 || svc.calls[ReportOvertimeCost] != 0 {
		t.Fatalf("compensation reports computed without permission: %v", svc.calls)
	}
	// Hires and leavers this month come from the 2026-10 row only.
	if !strings.Contains(body, "1 / 1") {
		t.Fatal("dashboard hires/leavers KPI not computed from the current month")
	}
}

func TestDashboardRendersCompensationWithPermission(t *testing.T) {
	svc := newFakeReports()
	rec := serve(t, newReportsRouter(t, reportsComp, svc, nil, true), "/hr/dashboard")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`data-kpi="leave-liability"`, `data-kpi="overtime-cost"`, "9876543.21", "750000", "/hr/reports/leave-liability.csv", "/hr/reports/overtime-cost.csv"} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard missing %q", want)
		}
	}
	if svc.calls[ReportLeaveLiability] != 1 || svc.calls[ReportOvertimeCost] != 1 {
		t.Fatalf("calls=%v", svc.calls)
	}
}

func TestDashboardShowsLoadFailures(t *testing.T) {
	svc := newFakeReports()
	svc.err = errors.New("db down")
	rec := serve(t, newReportsRouter(t, reportsOnly, svc, nil, true), "/hr/dashboard")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Some figures could not be loaded") || strings.Contains(body, "db down") {
		t.Fatal("dashboard should report failures without leaking error text")
	}
}
