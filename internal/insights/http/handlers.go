package insightshhtp

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/insights"
	insightssvg "github.com/odyssey-erp/odyssey-erp/internal/insights/svg"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

var periodRegex = regexp.MustCompile(`^\d{4}-\d{2}$`)

const (
	chartWidth     = 720
	chartHeight    = 240
	requestTimeout = 2 * time.Second
)

// Service exposes the business logic required by the handler.
type Service interface {
	Load(ctx context.Context, filters insights.CompareFilters) (insights.Result, error)
}

// RBACService resolves effective permissions for the logged-in user.
type RBACService interface {
	EffectivePermissions(ctx context.Context, userID int64) ([]string, error)
}

type chartFunc func(width, height int, seriesA, seriesB []float64, labels []string) (template.HTML, error)

// Handler menangani permintaan halaman finance insights.
type Handler struct {
	logger    *slog.Logger
	service   Service
	templates *view.Engine
	rbac      RBACService
	chart     chartFunc
	now       func() time.Time
}

// NewHandler membuat instance handler insights baru.
func NewHandler(logger *slog.Logger, service Service, templates *view.Engine, rbac RBACService) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{
		logger:    logger,
		service:   service,
		templates: templates,
		rbac:      rbac,
		chart: func(width, height int, seriesA, seriesB []float64, labels []string) (template.HTML, error) {
			html, err := insightssvg.LineMulti(width, height, seriesA, seriesB, labels)
			return html, err
		},
		now: time.Now,
	}
	return h
}

func (h *Handler) handleInsights(w http.ResponseWriter, r *http.Request) {
	if h.templates == nil || h.service == nil || h.chart == nil {
		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
		return
	}

	sess := shared.SessionFromContext(r.Context())
	if err := h.authorize(r.Context(), sess, shared.PermFinanceInsightsView); err != nil {
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

	result, err := h.service.Load(ctx, filters)
	if err != nil {
		h.handleServerError(w, "load insights", err)
		return
	}

	vm, err := h.buildViewModel(filters, result)
	if err != nil {
		h.handleServerError(w, "build view model", err)
		return
	}

	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}

	data := view.TemplateData{
		Title:       "Finance Insights",
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        vm,
	}
	if err := h.templates.Render(w, "pages/finance/insights.html", data); err != nil {
		h.handleServerError(w, "render template", err)
	}
}

func (h *Handler) parseFilters(r *http.Request) (insights.CompareFilters, error) {
	now := h.now().UTC()
	toStr := strings.TrimSpace(r.URL.Query().Get("to"))
	if toStr == "" {
		toStr = now.Format("2006-01")
	}
	toTime, err := parseMonthParam(toStr)
	if err != nil {
		return insights.CompareFilters{}, err
	}

	fromStr := strings.TrimSpace(r.URL.Query().Get("from"))
	if fromStr == "" {
		fromStr = toTime.AddDate(0, -11, 0).Format("2006-01")
	}
	fromTime, err := parseMonthParam(fromStr)
	if err != nil {
		return insights.CompareFilters{}, err
	}
	if fromTime.After(toTime) {
		return insights.CompareFilters{}, validationError{field: "from"}
	}
	if monthSpan(fromTime, toTime) > 18 {
		return insights.CompareFilters{}, validationError{field: "range"}
	}

	companyStr := strings.TrimSpace(r.URL.Query().Get("company_id"))
	if companyStr == "" {
		companyStr = "1"
	}
	companyID, err := strconv.ParseInt(companyStr, 10, 64)
	if err != nil || companyID <= 0 {
		return insights.CompareFilters{}, validationError{field: "company_id"}
	}

	var branchID *int64
	branchStr := strings.TrimSpace(r.URL.Query().Get("branch_id"))
	if branchStr != "" {
		value, err := strconv.ParseInt(branchStr, 10, 64)
		if err != nil || value <= 0 {
			return insights.CompareFilters{}, validationError{field: "branch_id"}
		}
		branchID = &value
	}

	return insights.CompareFilters{
		From:      fromTime.Format("2006-01"),
		To:        toTime.Format("2006-01"),
		CompanyID: &companyID,
		BranchID:  branchID,
	}, nil
}

func (h *Handler) buildViewModel(filters insights.CompareFilters, result insights.Result) (insights.ViewModel, error) {
	labels := make([]string, len(result.Series))
	netSeries := make([]float64, len(result.Series))
	revenueSeries := make([]float64, len(result.Series))
	points := make([]insights.PointViewModel, 0, len(result.Series))
	for i, point := range result.Series {
		labels[i] = point.Month
		netSeries[i] = point.Net
		revenueSeries[i] = point.Revenue
		points = append(points, insights.PointViewModel(point))
	}

	var chart template.HTML
	var err error
	if len(points) > 0 {
		chart, err = h.chart(chartWidth, chartHeight, netSeries, revenueSeries, labels)
		if err != nil {
			return insights.ViewModel{}, err
		}
	}

	variances := make([]insights.VarianceViewModel, 0, len(result.Variance))
	for _, item := range result.Variance {
		variances = append(variances, insights.VarianceViewModel(item))
	}

	contrib := make([]insights.ContributionViewModel, 0, len(result.Contribution))
	for _, item := range result.Contribution {
		contrib = append(contrib, insights.ContributionViewModel(item))
	}

	return insights.ViewModel{
		Filters:      insights.FiltersViewModel(filters),
		Series:       points,
		Variances:    variances,
		Contribution: contrib,
		Chart:        chart,
		Ready:        len(points) > 0,
	}, nil
}

func (h *Handler) authorize(ctx context.Context, sess *shared.Session, perm string) error {
	if h.rbac == nil {
		return fmt.Errorf("insights: rbac not configured")
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
		return errPermissionDenied
	}
	perms, err := h.rbac.EffectivePermissions(ctx, userID)
	if err != nil {
		return err
	}
	required := strings.ToLower(strings.TrimSpace(perm))
	for _, p := range perms {
		if strings.EqualFold(p, required) {
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
	h.handleServerError(w, "authorize", err)
}

func (h *Handler) handleFilterError(w http.ResponseWriter, err error) {
	var v validationError
	if errors.As(err, &v) {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	h.handleServerError(w, "validate filters", err)
}

func (h *Handler) handleServerError(w http.ResponseWriter, message string, err error) {
	if h.logger != nil {
		h.logger.Error(message, slog.Any("error", err))
	}
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

type validationError struct {
	field string
}

func (validationError) Error() string {
	return "validation failed"
}

var errPermissionDenied = errors.New("insights: permission denied")

func parseMonthParam(value string) (time.Time, error) {
	if !periodRegex.MatchString(value) {
		return time.Time{}, validationError{field: "period"}
	}
	t, err := time.Parse("2006-01", value)
	if err != nil {
		return time.Time{}, validationError{field: "period"}
	}
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC), nil
}

func monthSpan(from, to time.Time) int {
	months := (to.Year()-from.Year())*12 + int(to.Month()-from.Month()) + 1
	if months < 0 {
		return 0
	}
	return months
}
