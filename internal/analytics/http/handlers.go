package analytichttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics/export"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics/svg"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics/ui"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"golang.org/x/sync/errgroup"
)

var periodRegex = regexp.MustCompile(`^\d{4}-\d{2}$`)

var errPermissionDenied = errors.New("analytics: permission denied")

const trendWindowMonths = 12
const requestTimeout = 2 * time.Second

// AnalyticsService defines the dashboard data contract used by the handler.
type AnalyticsService interface {
	GetKPISummary(ctx context.Context, filter analytics.KPIFilter) (analytics.KPISummary, error)
	GetPLTrend(ctx context.Context, filter analytics.TrendFilter) ([]analytics.PLTrendPoint, error)
	GetCashflowTrend(ctx context.Context, filter analytics.TrendFilter) ([]analytics.CashflowTrendPoint, error)
	GetARAging(ctx context.Context, filter analytics.AgingFilter) ([]analytics.AgingBucket, error)
	GetAPAging(ctx context.Context, filter analytics.AgingFilter) ([]analytics.AgingBucket, error)
}

// RBACService exposes permission resolution for RBAC guards.
type RBACService interface {
	EffectivePermissions(ctx context.Context, userID int64) ([]string, error)
}

// PeriodValidator validates whether a fiscal period is accessible.
type PeriodValidator interface {
	ValidatePeriod(ctx context.Context, period string) error
}

// PDFService renders dashboard content to PDF bytes.
type PDFService interface {
	RenderDashboard(ctx context.Context, payload export.DashboardPayload) ([]byte, error)
}

// Handler coordinates HTTP requests for the finance analytics dashboard.
type Handler struct {
	logger    *slog.Logger
	service   AnalyticsService
	templates *view.Engine
	line      ui.LineRenderer
	bar       ui.BarRenderer
	pdf       PDFService
	rbac      RBACService
	periods   PeriodValidator
	csvPool   sync.Pool
	now       func() time.Time
}

// NewHandler constructs the analytics HTTP handler.
func NewHandler(logger *slog.Logger, service AnalyticsService, templates *view.Engine, line ui.LineRenderer, bar ui.BarRenderer, pdf PDFService, rbac RBACService, periods PeriodValidator) *Handler {
	h := &Handler{
		logger:    logger,
		service:   service,
		templates: templates,
		line:      line,
		bar:       bar,
		pdf:       pdf,
		rbac:      rbac,
		periods:   periods,
		now:       time.Now,
	}
	h.csvPool.New = func() interface{} { return new(bytes.Buffer) }
	return h
}

// WithNow overrides the handler clock for testing.
func (h *Handler) WithNow(fn func() time.Time) {
	if fn != nil {
		h.now = fn
	}
}

func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	sess := shared.SessionFromContext(r.Context())
	if err := h.authorize(r.Context(), sess, shared.PermFinanceAnalyticsView); err != nil {
		h.respondAuthError(w, err)
		return
	}

	filters, err := h.parseFilters(r)
	if err != nil {
		h.handleFilterError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	if h.periods != nil {
		if err := h.periods.ValidatePeriod(ctx, filters.Period); err != nil {
			h.handleValidationFailure(w, fmt.Errorf("period invalid: %w", err))
			return
		}
	}

	data, err := h.loadDashboardData(ctx, filters)
	if err != nil {
		h.handleServerError(w, "load dashboard", err)
		return
	}

	vm, err := h.buildViewModel(filters, data)
	if err != nil {
		h.handleServerError(w, "render charts", err)
		return
	}

	var flash *shared.FlashMessage
	csrfToken := ""
	if sess != nil {
		flash = sess.PopFlash()
	}

	viewData := view.TemplateData{
		Title:       "Finance Analytics",
		Flash:       flash,
		CSRFToken:   csrfToken,
		CurrentPath: r.URL.Path,
		Data:        vm,
	}
	if err := h.templates.Render(w, "pages/finance/dashboard.html", viewData); err != nil {
		h.handleServerError(w, "render template", err)
	}
}

func (h *Handler) handleKPI(w http.ResponseWriter, r *http.Request) {
	// For now, KPI tracking uses the same dashboard but could be specialized later
	h.handleDashboard(w, r)
}

func (h *Handler) handlePDF(w http.ResponseWriter, r *http.Request) {
	sess := shared.SessionFromContext(r.Context())
	if err := h.authorize(r.Context(), sess, shared.PermFinanceAnalyticsExport); err != nil {
		h.respondAuthError(w, err)
		return
	}
	if h.pdf == nil {
		h.handleServerError(w, "pdf exporter", errors.New("pdf exporter not configured"))
		return
	}

	filters, err := h.parseFilters(r)
	if err != nil {
		h.handleFilterError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	if h.periods != nil {
		if err := h.periods.ValidatePeriod(ctx, filters.Period); err != nil {
			h.handleValidationFailure(w, fmt.Errorf("period invalid: %w", err))
			return
		}
	}

	data, err := h.loadDashboardData(ctx, filters)
	if err != nil {
		h.handleServerError(w, "load dashboard", err)
		return
	}

	payload := export.DashboardPayload{
		Period:   filters.Period,
		Summary:  data.summary,
		PL:       data.pl,
		Cashflow: data.cashflow,
		ARAging:  data.ar,
		APAging:  data.ap,
	}
	pdfBytes, err := h.pdf.RenderDashboard(ctx, payload)
	if err != nil {
		h.handleServerError(w, "render pdf", err)
		return
	}

	filename := fmt.Sprintf("finance-analytics-%s.pdf", filters.Period)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	if _, err := w.Write(pdfBytes); err != nil {
		h.logError("stream pdf", err)
	}
}

func (h *Handler) handleCSV(w http.ResponseWriter, r *http.Request) {
	sess := shared.SessionFromContext(r.Context())
	if err := h.authorize(r.Context(), sess, shared.PermFinanceAnalyticsExport); err != nil {
		h.respondAuthError(w, err)
		return
	}

	filters, err := h.parseFilters(r)
	if err != nil {
		h.handleFilterError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	if h.periods != nil {
		if err := h.periods.ValidatePeriod(ctx, filters.Period); err != nil {
			h.handleValidationFailure(w, fmt.Errorf("period invalid: %w", err))
			return
		}
	}

	data, err := h.loadDashboardData(ctx, filters)
	if err != nil {
		h.handleServerError(w, "load dashboard", err)
		return
	}

	buf := h.csvPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		buf.Reset()
		h.csvPool.Put(buf)
	}()

	if err := export.WriteKPICSV(buf, data.summary, filters.Period); err != nil {
		h.handleServerError(w, "write kpi csv", err)
		return
	}
	buf.WriteString("\n")
	if err := export.WritePLTrendCSV(buf, data.pl); err != nil {
		h.handleServerError(w, "write pl csv", err)
		return
	}
	buf.WriteString("\n")
	if err := export.WriteCashflowTrendCSV(buf, data.cashflow); err != nil {
		h.handleServerError(w, "write cashflow csv", err)
		return
	}
	buf.WriteString("\n")
	if err := export.WriteAgingCSV(buf, data.ar); err != nil {
		h.handleServerError(w, "write ar csv", err)
		return
	}
	buf.WriteString("\n")
	if err := export.WriteAgingCSV(buf, data.ap); err != nil {
		h.handleServerError(w, "write ap csv", err)
		return
	}

	filename := fmt.Sprintf("finance-analytics-%s.csv", filters.Period)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	if _, err := w.Write(buf.Bytes()); err != nil {
		h.logError("stream csv", err)
	}
}

func (h *Handler) authorize(ctx context.Context, sess *shared.Session, perm string) error {
	if h.rbac == nil {
		return fmt.Errorf("rbac service missing")
	}
	if sess == nil {
		return errPermissionDenied
	}
	userIDStr := strings.TrimSpace(sess.User())
	if userIDStr == "" {
		return errPermissionDenied
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		h.logError("parse user id", err)
		return errPermissionDenied
	}
	perms, err := h.rbac.EffectivePermissions(ctx, userID)
	if err != nil {
		return err
	}
	required := strings.ToLower(strings.TrimSpace(perm))
	for _, granted := range perms {
		if strings.EqualFold(granted, required) {
			return nil
		}
	}
	return errPermissionDenied
}

func (h *Handler) respondAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, errPermissionDenied) {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	h.handleServerError(w, "authorization", err)
}

func (h *Handler) parseFilters(r *http.Request) (ui.DashboardFilters, error) {
	now := h.now().UTC()
	period := strings.TrimSpace(r.URL.Query().Get("period"))
	if period == "" {
		period = now.Format("2006-01")
	}
	if !periodRegex.MatchString(period) {
		return ui.DashboardFilters{}, validationError{field: "period"}
	}
	if _, err := time.Parse("2006-01", period); err != nil {
		return ui.DashboardFilters{}, validationError{field: "period"}
	}

	companyStr := strings.TrimSpace(r.URL.Query().Get("company_id"))
	if companyStr == "" {
		companyStr = "1"
	}
	companyID, err := strconv.ParseInt(companyStr, 10, 64)
	if err != nil || companyID <= 0 {
		return ui.DashboardFilters{}, validationError{field: "company_id"}
	}

	var branchID *int64
	branchStr := strings.TrimSpace(r.URL.Query().Get("branch_id"))
	if branchStr != "" {
		value, err := strconv.ParseInt(branchStr, 10, 64)
		if err != nil || value <= 0 {
			return ui.DashboardFilters{}, validationError{field: "branch_id"}
		}
		branchID = &value
	}

	return ui.DashboardFilters{Period: period, CompanyID: companyID, BranchID: branchID}, nil
}

type dashboardData struct {
	summary  analytics.KPISummary
	pl       []analytics.PLTrendPoint
	cashflow []analytics.CashflowTrendPoint
	ar       []analytics.AgingBucket
	ap       []analytics.AgingBucket
	asOf     time.Time
}

func (h *Handler) loadDashboardData(ctx context.Context, filters ui.DashboardFilters) (dashboardData, error) {
	from, to, asOf, err := computePeriodRange(filters.Period, trendWindowMonths)
	if err != nil {
		return dashboardData{}, err
	}

	var data dashboardData
	data.asOf = asOf

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		summary, err := h.service.GetKPISummary(ctx, analytics.KPIFilter{
			Period:    filters.Period,
			CompanyID: filters.CompanyID,
			BranchID:  filters.BranchID,
			AsOf:      asOf,
		})
		if err != nil {
			return err
		}
		data.summary = summary
		return nil
	})

	g.Go(func() error {
		points, err := h.service.GetPLTrend(ctx, analytics.TrendFilter{
			From:      from,
			To:        to,
			CompanyID: filters.CompanyID,
			BranchID:  filters.BranchID,
		})
		if err != nil {
			return err
		}
		data.pl = points
		return nil
	})

	g.Go(func() error {
		points, err := h.service.GetCashflowTrend(ctx, analytics.TrendFilter{
			From:      from,
			To:        to,
			CompanyID: filters.CompanyID,
			BranchID:  filters.BranchID,
		})
		if err != nil {
			return err
		}
		data.cashflow = points
		return nil
	})

	g.Go(func() error {
		buckets, err := h.service.GetARAging(ctx, analytics.AgingFilter{
			AsOf:      asOf,
			CompanyID: filters.CompanyID,
			BranchID:  filters.BranchID,
		})
		if err != nil {
			return err
		}
		data.ar = buckets
		return nil
	})

	g.Go(func() error {
		buckets, err := h.service.GetAPAging(ctx, analytics.AgingFilter{
			AsOf:      asOf,
			CompanyID: filters.CompanyID,
			BranchID:  filters.BranchID,
		})
		if err != nil {
			return err
		}
		data.ap = buckets
		return nil
	})

	if err := g.Wait(); err != nil {
		return dashboardData{}, err
	}
	return data, nil
}

func (h *Handler) buildViewModel(filters ui.DashboardFilters, data dashboardData) (ui.DashboardViewModel, error) {
	if h.line == nil || h.bar == nil {
		return ui.DashboardViewModel{}, fmt.Errorf("svg renderer missing")
	}
	vm := ui.DashboardViewModel{Filters: filters}
	vm.KPI = ui.DashboardKPI{
		Revenue:       data.summary.Revenue,
		Opex:          data.summary.Opex,
		NetProfit:     data.summary.NetProfit,
		CashIn:        data.summary.CashIn,
		CashOut:       data.summary.CashOut,
		COGS:          data.summary.COGS,
		AROutstanding: data.summary.AROutstanding,
		APOutstanding: data.summary.APOutstanding,
	}
	vm.PLTrend = ui.ToPLTrendPoints(data.pl)
	vm.CashflowTrend = ui.ToCashflowPoints(data.cashflow)
	vm.AgingAR = ui.ToAgingBuckets(data.ar)
	vm.AgingAP = ui.ToAgingBuckets(data.ap)

	labels := make([]string, 0, len(vm.PLTrend))
	netSeries := make([]float64, 0, len(vm.PLTrend))
	for _, point := range vm.PLTrend {
		labels = append(labels, point.Month)
		netSeries = append(netSeries, point.Net)
	}
	if len(labels) == 0 {
		labels = []string{filters.Period}
		netSeries = []float64{0}
	}

	svgLine, err := h.line.Line(svg.DefaultWidth, svg.DefaultHeight, netSeries, labels, svg.LineOpts{
		Title:       "Trend Laba Bersih",
		Description: "Pergerakan laba bersih per bulan",
		ShowDots:    true,
	})
	if err != nil {
		return ui.DashboardViewModel{}, err
	}
	vm.PLTrendSVG = svgLine

	cashLabels := make([]string, 0, len(vm.CashflowTrend))
	cashIn := make([]float64, 0, len(vm.CashflowTrend))
	cashOut := make([]float64, 0, len(vm.CashflowTrend))
	for _, point := range vm.CashflowTrend {
		cashLabels = append(cashLabels, point.Month)
		cashIn = append(cashIn, point.In)
		cashOut = append(cashOut, point.Out)
	}
	if len(cashLabels) == 0 {
		cashLabels = labels
		cashIn = make([]float64, len(labels))
		cashOut = make([]float64, len(labels))
	}

	svgBar, err := h.bar.Bars(svg.DefaultWidth, svg.DefaultHeight, cashIn, cashOut, cashLabels, svg.BarOpts{
		Title:        "Cashflow",
		Description:  "Perbandingan cash in dan cash out per bulan",
		SeriesALabel: "Cash In",
		SeriesBLabel: "Cash Out",
	})
	if err != nil {
		return ui.DashboardViewModel{}, err
	}
	vm.CashflowSVG = svgBar

	return vm, nil
}

func (h *Handler) handleFilterError(w http.ResponseWriter, err error) {
	var vErr validationError
	if errors.As(err, &vErr) {
		http.Error(w, "Parameter tidak valid", http.StatusBadRequest)
		return
	}
	h.handleServerError(w, "parse filters", err)
}

func (h *Handler) handleValidationFailure(w http.ResponseWriter, err error) {
	h.logError("validate period", err)
	http.Error(w, "Periode tidak ditemukan", http.StatusBadRequest)
}

func (h *Handler) handleServerError(w http.ResponseWriter, context string, err error) {
	h.logError(context, err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (h *Handler) logError(context string, err error) {
	if h.logger != nil {
		h.logger.Error(context, slog.Any("error", err))
	}
}

type validationError struct {
	field string
}

func (v validationError) Error() string {
	return fmt.Sprintf("invalid %s", v.field)
}

func computePeriodRange(period string, months int) (string, string, time.Time, error) {
	if months <= 0 {
		months = trendWindowMonths
	}
	base, err := time.Parse("2006-01", period)
	if err != nil {
		return "", "", time.Time{}, err
	}
	base = time.Date(base.Year(), base.Month(), 1, 0, 0, 0, 0, time.UTC)
	from := base.AddDate(0, -months+1, 0)
	asOf := time.Date(base.Year(), base.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	return from.Format("2006-01"), base.Format("2006-01"), asOf, nil
}

// HandleDashboardForTest exposes the dashboard handler for tests.
func (h *Handler) HandleDashboardForTest(w http.ResponseWriter, r *http.Request) {
	h.handleDashboard(w, r)
}

// HandlePDFForTest exposes the PDF handler for tests.
func (h *Handler) HandlePDFForTest(w http.ResponseWriter, r *http.Request) { h.handlePDF(w, r) }

// HandleCSVForTest exposes the CSV handler for tests.
func (h *Handler) HandleCSVForTest(w http.ResponseWriter, r *http.Request) { h.handleCSV(w, r) }
