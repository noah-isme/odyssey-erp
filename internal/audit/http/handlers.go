package audithttp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/audit"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

const (
	defaultPageSize   = 20
	maxPageSize       = 50
	defaultDateRange  = 7 * 24 * time.Hour
	maxDateRangeHours = 24 * 90
)

// TimelineService defines the business contract for timeline data.
type TimelineService interface {
	Timeline(ctx context.Context, filters audit.TimelineFilters) (audit.Result, error)
	Export(ctx context.Context, filters audit.TimelineFilters) ([]audit.TimelineRow, error)
}

// Exporter writes audit timeline exports.
type Exporter interface {
	WriteCSV(rows []audit.TimelineRow) ([]byte, error)
	RenderPDF(ctx context.Context, vm audit.ViewModel) ([]byte, error)
}

// RBACService resolves permissions for the current user.
type RBACService interface {
	EffectivePermissions(ctx context.Context, userID int64) ([]string, error)
}

// Handler menangani permintaan audit timeline.
type Handler struct {
	logger    *slog.Logger
	service   TimelineService
	exporter  Exporter
	templates *view.Engine
	rbac      RBACService
	now       func() time.Time
}

// NewHandler membuat handler audit baru.
func NewHandler(logger *slog.Logger, service TimelineService, templates *view.Engine, exporter Exporter, rbac RBACService) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		logger:    logger,
		service:   service,
		exporter:  exporter,
		templates: templates,
		rbac:      rbac,
		now:       time.Now,
	}
}

func (h *Handler) handleTimeline(w http.ResponseWriter, r *http.Request) {
	if h.templates == nil || h.service == nil {
		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
		return
	}

	sess := shared.SessionFromContext(r.Context())
	if err := h.authorize(r.Context(), sess, shared.PermFinanceAuditView); err != nil {
		h.respondAuthError(w, err)
		return
	}

	filters, err := h.parseFilters(r)
	if err != nil {
		h.handleFilterError(w, err)
		return
	}

	result, err := h.service.Timeline(r.Context(), filters)
	if err != nil {
		h.handleServerError(w, "load audit timeline", err)
		return
	}

	vm := h.buildViewModel(filters, result)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	data := view.TemplateData{
		Title:       "Audit Timeline",
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        vm,
	}
	if err := h.templates.Render(w, "pages/finance/audit_timeline.html", data); err != nil {
		h.handleServerError(w, "render audit timeline", err)
	}
}

func (h *Handler) handleExport(w http.ResponseWriter, r *http.Request) {
	if h.exporter == nil || h.service == nil {
		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
		return
	}
	sess := shared.SessionFromContext(r.Context())
	if err := h.authorize(r.Context(), sess, shared.PermFinanceInsightsExport); err != nil {
		h.respondAuthError(w, err)
		return
	}
	filters, err := h.parseFilters(r)
	if err != nil {
		h.handleFilterError(w, err)
		return
	}
	rows, err := h.service.Export(r.Context(), filters)
	if err != nil {
		h.handleServerError(w, "export audit timeline", err)
		return
	}
	csvBytes, err := h.exporter.WriteCSV(rows)
	if err != nil {
		h.handleServerError(w, "encode csv", err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"audit-timeline.csv\"")
	if _, err := w.Write(csvBytes); err != nil {
		h.logger.Warn("write csv", slog.Any("error", err))
	}
}

func (h *Handler) handlePDF(w http.ResponseWriter, r *http.Request) {
	if h.exporter == nil || h.service == nil {
		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
		return
	}
	sess := shared.SessionFromContext(r.Context())
	if err := h.authorize(r.Context(), sess, shared.PermFinanceInsightsExport); err != nil {
		h.respondAuthError(w, err)
		return
	}
	filters, err := h.parseFilters(r)
	if err != nil {
		h.handleFilterError(w, err)
		return
	}
	rows, err := h.service.Export(r.Context(), filters)
	if err != nil {
		h.handleServerError(w, "export pdf data", err)
		return
	}
	vm := h.buildViewModel(filters, audit.Result{Rows: rows, Paging: audit.PagingInfo{Page: 1, PageSize: len(rows)}})
	pdfBytes, err := h.exporter.RenderPDF(r.Context(), vm)
	if err != nil {
		if errors.Is(err, audit.ErrPDFUnavailable) {
			http.Error(w, "PDF export belum tersedia", http.StatusNotImplemented)
			return
		}
		h.handleServerError(w, "render pdf", err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=\"audit-timeline.pdf\"")
	if _, err := w.Write(pdfBytes); err != nil {
		h.logger.Warn("write pdf", slog.Any("error", err))
	}
}

func (h *Handler) parseFilters(r *http.Request) (audit.TimelineFilters, error) {
	now := h.now().UTC()
	toStr := strings.TrimSpace(r.URL.Query().Get("to"))
	if toStr == "" {
		toStr = now.Format("2006-01-02")
	}
	toTime, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		return audit.TimelineFilters{}, validationError{field: "to"}
	}
	fromStr := strings.TrimSpace(r.URL.Query().Get("from"))
	if fromStr == "" {
		fromStr = toTime.Add(-defaultDateRange).Format("2006-01-02")
	}
	fromTime, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		return audit.TimelineFilters{}, validationError{field: "from"}
	}
	if fromTime.After(toTime) {
		return audit.TimelineFilters{}, validationError{field: "range"}
	}
	if toTime.Sub(fromTime) > maxDateRangeHours*time.Hour {
		return audit.TimelineFilters{}, validationError{field: "range"}
	}

	page := 1
	if v := strings.TrimSpace(r.URL.Query().Get("page")); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			return audit.TimelineFilters{}, validationError{field: "page"}
		}
		page = parsed
	}
	pageSize := defaultPageSize
	if v := strings.TrimSpace(r.URL.Query().Get("page_size")); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			return audit.TimelineFilters{}, validationError{field: "page_size"}
		}
		if parsed > maxPageSize {
			parsed = maxPageSize
		}
		pageSize = parsed
	}

	return audit.TimelineFilters{
		From:     fromTime,
		To:       toTime,
		Actor:    strings.TrimSpace(r.URL.Query().Get("actor")),
		Entity:   strings.TrimSpace(r.URL.Query().Get("entity")),
		Action:   strings.TrimSpace(r.URL.Query().Get("action")),
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (h *Handler) buildViewModel(filters audit.TimelineFilters, result audit.Result) audit.ViewModel {
	rows := make([]audit.TimelineRow, len(result.Rows))
	copy(rows, result.Rows)
	return audit.ViewModel{
		Filters: audit.FiltersViewModel{
			From:   filters.From,
			To:     filters.To,
			Actor:  filters.Actor,
			Entity: filters.Entity,
			Action: filters.Action,
		},
		Rows:   rows,
		Paging: result.Paging,
	}
}

func (h *Handler) authorize(ctx context.Context, sess *shared.Session, perm string) error {
	if h.rbac == nil {
		return fmt.Errorf("audit: rbac not configured")
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

var errPermissionDenied = errors.New("audit: permission denied")

// no additional helpers
