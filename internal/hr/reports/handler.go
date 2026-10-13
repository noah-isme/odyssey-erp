package reports

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// ActionReportExport is the audit action recorded for every CSV export.
const ActionReportExport = "HR_REPORT_EXPORT"

// Report names served under /hr/reports/{name}.csv.
const (
	ReportHeadcount      = "headcount"
	ReportHiresLeavers   = "hires-leavers"
	ReportTurnover       = "turnover"
	ReportAbsenteeism    = "absenteeism"
	ReportLeaveLiability = "leave-liability"
	ReportOvertimeCost   = "overtime-cost"
	ReportExpiries       = "expiries"
)

// DashboardExpiryDays is the expiry window shown on the dashboard and the
// default for the expiries export.
const DashboardExpiryDays = 30

// compensationReports need hr.compensation.view in addition to hr.reports.view.
var compensationReports = map[string]bool{ReportLeaveLiability: true, ReportOvertimeCost: true}

// ReportNames lists every exportable report in display order.
func ReportNames() []string {
	return []string{ReportHeadcount, ReportHiresLeavers, ReportTurnover, ReportAbsenteeism, ReportLeaveLiability, ReportOvertimeCost, ReportExpiries}
}

// HandlerService is the subset of *Service used by the HTTP handler.
type HandlerService interface {
	Headcount(ctx context.Context, companyID int64, asOf time.Time) (HeadcountReport, error)
	HiresAndLeaversByMonth(ctx context.Context, companyID int64, from, to time.Time) ([]HiresLeaversRow, error)
	Turnover(ctx context.Context, companyID int64, from, to time.Time) (TurnoverReport, error)
	Absenteeism(ctx context.Context, companyID int64, from, to time.Time) (AbsenteeismReport, error)
	LeaveLiability(ctx context.Context, companyID int64, year int) (LeaveLiabilityReport, error)
	OvertimeCost(ctx context.Context, companyID int64, from, to time.Time) (OvertimeCostReport, error)
	UpcomingExpiries(ctx context.Context, companyID int64, withinDays int) ([]Expiry, error)
}

var _ HandlerService = (*Service)(nil)

// Auditor is satisfied by *shared.AuditLogger. Export auditing is best
// effort: a failed audit write is logged and the export still succeeds.
type Auditor interface {
	Record(ctx context.Context, log shared.AuditLog) error
}

// Handler serves /hr/dashboard and /hr/reports/*.csv.
type Handler struct {
	logger    *slog.Logger
	service   HandlerService
	audit     Auditor
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
	now       func() time.Time
}

// NewHandler builds a Handler.
func NewHandler(l *slog.Logger, s HandlerService, a Auditor, t *view.Engine, c *shared.CSRFManager, r rbac.Middleware) *Handler {
	if l == nil {
		l = slog.Default()
	}
	return &Handler{logger: l, service: s, audit: a, templates: t, csrf: c, rbac: r, now: time.Now}
}

// MountRoutes mounts /hr/dashboard and /hr/reports/{name}.csv on the root
// router. Every route requires hr.reports.view; compensation reports are
// additionally checked against hr.compensation.view inside the handler.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHRReportsView))
		r.Get("/hr/dashboard", h.dashboard)
		for _, name := range ReportNames() {
			r.Get("/hr/reports/"+name+".csv", h.export(name))
		}
	})
}

// canSeeCompensation reports whether the user may see compensation figures
// (leave liability and overtime cost).
func (h *Handler) canSeeCompensation(r *http.Request, userID int64) bool {
	if h.rbac.Service == nil {
		return false
	}
	perms, err := h.rbac.Service.EffectivePermissions(r.Context(), userID)
	if err != nil {
		h.logger.Error("hr reports permissions", slog.Any("error", err))
		return false
	}
	for _, p := range perms {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "admin", "*", shared.PermHRCompensationView, shared.PermHRCompensationAdmin:
			return true
		}
	}
	return false
}

func (h *Handler) today() time.Time { return dateOnly(h.now()) }

// ---- Dashboard ----------------------------------------------------------------

// dashboardData is the template model of pages/hr/dashboard.html.
type dashboardData struct {
	Today            time.Time
	YearStart        time.Time
	MonthLabel       string
	Headcount        HeadcountReport
	HiresLeavers     []HiresLeaversRow
	HiresThisMonth   int64
	LeaversThisMonth int64
	Turnover         TurnoverReport
	Absenteeism      AbsenteeismReport
	Expiries         []Expiry
	ExpiryDays       int
	CanSeeComp       bool
	LeaveLiability   *LeaveLiabilityReport
	OvertimeCost     *OvertimeCostReport
	Failed           []string
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	ctx, company := r.Context(), ident.CompanyID
	today := h.today()
	yearStart := time.Date(today.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	data := dashboardData{
		Today:      today,
		YearStart:  yearStart,
		MonthLabel: today.Format("January 2006"),
		ExpiryDays: DashboardExpiryDays,
	}
	fail := func(what string, err error) {
		h.logger.Error("hr dashboard "+what, slog.Any("error", err))
		data.Failed = append(data.Failed, what)
	}

	var err error
	if data.Headcount, err = h.service.Headcount(ctx, company, today); err != nil {
		fail("headcount", err)
	}
	if data.HiresLeavers, err = h.service.HiresAndLeaversByMonth(ctx, company, yearStart, today); err != nil {
		fail("hires and leavers", err)
	}
	month := today.Format("2006-01")
	for _, row := range data.HiresLeavers {
		if row.Month == month {
			data.HiresThisMonth += row.Hires
			data.LeaversThisMonth += row.Leavers
		}
	}
	if data.Turnover, err = h.service.Turnover(ctx, company, yearStart, today); err != nil {
		fail("turnover", err)
	}
	if data.Absenteeism, err = h.service.Absenteeism(ctx, company, yearStart, today); err != nil {
		fail("absenteeism", err)
	}
	if data.Expiries, err = h.service.UpcomingExpiries(ctx, company, DashboardExpiryDays); err != nil {
		fail("upcoming expiries", err)
	}

	// Compensation figures are fetched only after the permission check, so a
	// user without hr.compensation.view never causes them to be computed.
	data.CanSeeComp = h.canSeeCompensation(r, ident.UserID)
	if data.CanSeeComp {
		if ll, err := h.service.LeaveLiability(ctx, company, today.Year()); err != nil {
			fail("leave liability", err)
		} else {
			data.LeaveLiability = &ll
		}
		if oc, err := h.service.OvertimeCost(ctx, company, yearStart, today); err != nil {
			fail("overtime cost", err)
		} else {
			data.OvertimeCost = &oc
		}
	}

	s := shared.SessionFromContext(r.Context())
	token, _ := h.csrf.EnsureToken(r.Context(), s)
	if rerr := h.templates.Render(w, "pages/hr/dashboard.html", view.TemplateData{
		Title:       "HR Dashboard",
		CurrentPath: r.URL.Path,
		CSRFToken:   token,
		Flash:       s.PopFlash(),
		Data:        data,
	}); rerr != nil {
		h.logger.Error("render hr dashboard", slog.Any("error", rerr))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

// ---- CSV exports --------------------------------------------------------------

// errBadFilter marks unparsable query parameters.
var errBadFilter = errors.New("hr: invalid report filter")

// reportFilter is the parsed query string of an export. Only the fields the
// report uses are set and audited.
type reportFilter struct {
	asOf, from, to time.Time
	year, days     int
	meta           map[string]string
}

func parseDateParam(r *http.Request, key string, def time.Time) (time.Time, error) {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return def, nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, errBadFilter
	}
	return t, nil
}

func parseIntParam(r *http.Request, key string, def int) (int, error) {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, errBadFilter
	}
	return n, nil
}

func (h *Handler) parseFilter(r *http.Request, name string) (reportFilter, error) {
	today := h.today()
	f := reportFilter{meta: map[string]string{}}
	var err error
	switch name {
	case ReportHeadcount:
		if f.asOf, err = parseDateParam(r, "as_of", today); err != nil {
			return f, err
		}
		f.meta["as_of"] = f.asOf.Format("2006-01-02")
	case ReportLeaveLiability:
		if f.year, err = parseIntParam(r, "year", today.Year()); err != nil {
			return f, err
		}
		f.meta["year"] = strconv.Itoa(f.year)
	case ReportExpiries:
		if f.days, err = parseIntParam(r, "days", DashboardExpiryDays); err != nil {
			return f, err
		}
		f.meta["days"] = strconv.Itoa(f.days)
	default:
		if f.from, err = parseDateParam(r, "from", time.Date(today.Year(), 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
			return f, err
		}
		if f.to, err = parseDateParam(r, "to", today); err != nil {
			return f, err
		}
		f.meta["from"] = f.from.Format("2006-01-02")
		f.meta["to"] = f.to.Format("2006-01-02")
	}
	return f, nil
}

// isInputError reports whether err is a validation error the user can fix.
func isInputError(err error) bool {
	for _, target := range []error{errBadFilter, ErrDateRequired, ErrInvalidRange, ErrRangeTooLong, ErrInvalidYear, ErrInvalidWindow} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func (h *Handler) export(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ident, ok := shared.IdentityFromContext(r.Context())
		if !ok {
			shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
			return
		}
		if compensationReports[name] && !h.canSeeCompensation(r, ident.UserID) {
			shared.WriteHTTPError(w, http.StatusForbidden, "")
			return
		}
		f, err := h.parseFilter(r, name)
		if err == nil {
			var body []byte
			body, err = h.buildCSV(r.Context(), ident.CompanyID, name, f)
			if err == nil {
				h.recordExport(r.Context(), ident, name, f)
				w.Header().Set("Content-Type", "text/csv")
				w.Header().Set("Content-Disposition", `attachment; filename="hr_`+strings.ReplaceAll(name, "-", "_")+`_`+h.today().Format("20060102")+`.csv"`)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(body)
				return
			}
		}
		if isInputError(err) {
			shared.WriteHTTPError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "hr: "))
			return
		}
		h.logger.Error("hr report export", slog.String("report", name), slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

// recordExport audits the export. It is best effort: failures are logged.
func (h *Handler) recordExport(ctx context.Context, ident shared.RequestIdentity, name string, f reportFilter) {
	if h.audit == nil {
		h.logger.Warn("hr report export not audited: audit logger not configured", slog.String("report", name))
		return
	}
	err := h.audit.Record(ctx, shared.AuditLog{
		ActorID:  ident.UserID,
		Action:   ActionReportExport,
		Entity:   "hr_report",
		EntityID: name,
		Meta:     map[string]any{"report": name, "company_id": ident.CompanyID, "filter": f.meta},
	})
	if err != nil {
		h.logger.Error("audit hr report export", slog.String("report", name), slog.Any("error", err))
	}
}

func fmtDate(t time.Time) string { return t.Format("2006-01-02") }

func fmtInt(n int64) string { return strconv.FormatInt(n, 10) }

func fmtRate(f float64) string { return strconv.FormatFloat(f, 'f', 4, 64) }

func fmtDays(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }

func departmentLabel(name string) string {
	if name == "" {
		return "(no department)"
	}
	return name
}

// buildCSV renders the report into memory so a query failure yields a clean
// error response instead of a truncated download. Every cell goes through
// shared.WriteCSVRow. No report selects or writes national_id, tax_id, date
// of birth or address.
func (h *Handler) buildCSV(ctx context.Context, company int64, name string, f reportFilter) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	write := func(cells ...string) error { return shared.WriteCSVRow(w, cells...) }
	var err error
	switch name {
	case ReportHeadcount:
		var rep HeadcountReport
		if rep, err = h.service.Headcount(ctx, company, f.asOf); err != nil {
			return nil, err
		}
		if err = write("as_of", "department", "employment_type", "headcount"); err != nil {
			return nil, err
		}
		for _, row := range rep.Rows {
			if err = write(fmtDate(rep.AsOf), departmentLabel(row.Department), row.EmploymentType, fmtInt(row.Count)); err != nil {
				return nil, err
			}
		}
	case ReportHiresLeavers:
		var rows []HiresLeaversRow
		if rows, err = h.service.HiresAndLeaversByMonth(ctx, company, f.from, f.to); err != nil {
			return nil, err
		}
		if err = write("month", "department", "hires", "leavers"); err != nil {
			return nil, err
		}
		for _, row := range rows {
			if err = write(row.Month, departmentLabel(row.Department), fmtInt(row.Hires), fmtInt(row.Leavers)); err != nil {
				return nil, err
			}
		}
	case ReportTurnover:
		var rep TurnoverReport
		if rep, err = h.service.Turnover(ctx, company, f.from, f.to); err != nil {
			return nil, err
		}
		if err = write("from", "to", "leavers", "opening_headcount", "closing_headcount", "average_headcount", "turnover_rate"); err != nil {
			return nil, err
		}
		if err = write(fmtDate(rep.From), fmtDate(rep.To), fmtInt(rep.Leavers), fmtInt(rep.OpeningHeadcount), fmtInt(rep.ClosingHeadcount), fmtDays(rep.AverageHeadcount), fmtRate(rep.Rate)); err != nil {
			return nil, err
		}
	case ReportAbsenteeism:
		var rep AbsenteeismReport
		if rep, err = h.service.Absenteeism(ctx, company, f.from, f.to); err != nil {
			return nil, err
		}
		if err = write("from", "to", "department", "employees", "absent_days", "scheduled_days", "absenteeism_rate"); err != nil {
			return nil, err
		}
		for _, row := range rep.ByDepartment {
			if err = write(fmtDate(rep.From), fmtDate(rep.To), departmentLabel(row.Department), fmtInt(row.Employees), fmtInt(row.AbsentDays), fmtDays(row.ScheduledDays), fmtRate(row.Rate)); err != nil {
				return nil, err
			}
		}
	case ReportLeaveLiability:
		var rep LeaveLiabilityReport
		if rep, err = h.service.LeaveLiability(ctx, company, f.year); err != nil {
			return nil, err
		}
		if err = write("year", "valuation_date", "department", "employees", "employees_without_salary", "available_days", "liability_amount"); err != nil {
			return nil, err
		}
		for _, row := range rep.Rows {
			if err = write(strconv.Itoa(rep.Year), fmtDate(rep.ValuationDate), departmentLabel(row.Department), fmtInt(row.Employees), fmtInt(row.EmployeesWithoutSalary), row.AvailableDays, row.Amount); err != nil {
				return nil, err
			}
		}
	case ReportOvertimeCost:
		var rep OvertimeCostReport
		if rep, err = h.service.OvertimeCost(ctx, company, f.from, f.to); err != nil {
			return nil, err
		}
		if err = write("from", "to", "department", "posted_amount", "unrun_amount", "total_amount", "unrun_minutes"); err != nil {
			return nil, err
		}
		for _, row := range rep.ByDepartment {
			if err = write(fmtDate(rep.From), fmtDate(rep.To), departmentLabel(row.Department), fmtInt(int64(row.Posted)), fmtInt(int64(row.Unrun)), fmtInt(int64(row.Total)), fmtInt(row.UnrunMinutes)); err != nil {
				return nil, err
			}
		}
	case ReportExpiries:
		var rows []Expiry
		if rows, err = h.service.UpcomingExpiries(ctx, company, f.days); err != nil {
			return nil, err
		}
		if err = write("employee_number", "employee_name", "department", "expiry_kind", "expires_on"); err != nil {
			return nil, err
		}
		for _, x := range rows {
			if err = write(x.EmployeeNumber, x.Name, departmentLabel(x.Department), x.Kind, fmtDate(x.ExpiresOn)); err != nil {
				return nil, err
			}
		}
	default:
		return nil, errBadFilter
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
