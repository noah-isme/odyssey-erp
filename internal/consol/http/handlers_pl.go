package http

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	"github.com/odyssey-erp/odyssey-erp/internal/consol"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type plPDFExporter interface {
	Ready() bool
	Serve(http.ResponseWriter, *http.Request, *ProfitLossHandler)
}

// ProfitLossHandler wires HTTP interactions for the consolidated P&L feature.
type ProfitLossHandler struct {
	logger      *slog.Logger
	service     *consol.ProfitLossService
	templates   *view.Engine
	csrf        *shared.CSRFManager
	sessions    *shared.SessionManager
	rbac        rbac.Middleware
	rateLimit   func(http.Handler) http.Handler
	pdfExporter plPDFExporter
}

// NewProfitLossHandler constructs a new P&L handler.
func NewProfitLossHandler(logger *slog.Logger, service *consol.ProfitLossService, templates *view.Engine, csrf *shared.CSRFManager, sessions *shared.SessionManager, rbac rbac.Middleware, pdfClient PDFRenderClient) (*ProfitLossHandler, error) {
	if templates == nil {
		return nil, fmt.Errorf("consol pl handler: template engine required")
	}
	limiter := httprate.Limit(10, time.Minute, httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
		sess := shared.SessionFromContext(r.Context())
		if sess != nil {
			if user := strings.TrimSpace(sess.User()); user != "" {
				return "user:" + user, nil
			}
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return "ip:" + r.RemoteAddr, nil
		}
		return "ip:" + host, nil
	}))
	exporter, err := newPLPDFExporter(logger, pdfClient)
	if err != nil {
		return nil, err
	}
	return &ProfitLossHandler{
		logger:      logger,
		service:     service,
		templates:   templates,
		csrf:        csrf,
		sessions:    sessions,
		rbac:        rbac,
		rateLimit:   limiter,
		pdfExporter: exporter,
	}, nil
}

// MountRoutes registers the consolidated profit & loss endpoints.
func (h *ProfitLossHandler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermFinanceConsolView))
		r.Get("/finance/consol/pl", h.HandleGet)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermFinanceConsolExport))
		r.Use(h.rateLimit)
		r.Get("/finance/consol/pl/export.csv", h.HandleExportCSV)
		r.Get("/finance/consol/pl/pdf", h.HandleExportPDF)
	})
}

// HandleGet renders the server side P&L page.
func (h *ProfitLossHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	filters, errors := h.parseFilters(r)
	vm := ConsolPLViewModel{Errors: errors}
	if len(errors) == 0 {
		cacheKey := buildCacheKey("pl", filters.GroupID, filters.Period, filters.Entities, filters.FxOn)
		cachedHit := false
		if cached, ok := viewModelCache.Get(cacheKey); ok {
			if cachedVM, ok := cached.(ConsolPLViewModel); ok {
				vm = clonePLViewModel(cachedVM)
				vm.Errors = errors
				cachedHit = true
				recordCacheHit("pl", filters.GroupID, filters.Period)
			}
		}
		if !cachedHit {
			result, err, _ := singleflightBuild(r.Context(), cacheKey, func(ctx context.Context) (interface{}, error) {
				start := time.Now()
				recordCacheMiss("pl", filters.GroupID, filters.Period)
				defer func(start time.Time) {
					observeVMBuildDuration("pl", filters.GroupID, filters.Period, time.Since(start))
				}(start)
				report, warnings, err := h.service.Build(ctx, filters)
				if err != nil {
					return nil, err
				}
				vm := NewConsolPLViewModel(report, warnings)
				viewModelCache.Set(cacheKey, clonePLViewModel(vm))
				return vm, nil
			})
			if err != nil {
				errors["general"] = err.Error()
			} else if result != nil {
				if builtVM, ok := result.(ConsolPLViewModel); ok {
					vm = builtVM
					vm.Errors = errors
				}
			}
		}
	}
	data := view.TemplateData{
		Title:       "Consolidated Profit & Loss",
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        vm,
	}
	if err := h.templates.Render(w, "pages/finance/consol_pl.html", data); err != nil {
		h.logger.Error("render consol pl", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// HandleExportCSV serves the CSV export of the consolidated P&L statement.
func (h *ProfitLossHandler) HandleExportCSV(w http.ResponseWriter, r *http.Request) {
	filters, errors := h.parseFilters(r)
	if len(errors) > 0 {
		http.Error(w, strings.Join(mapValues(errors), "; "), http.StatusBadRequest)
		return
	}
	report, warnings, err := h.service.Build(r.Context(), filters)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	buf := &bytes.Buffer{}
	writer := csv.NewWriter(buf)
	if err := writer.Write([]string{"Section", "Account Code", "Account Name", "Local Amount", "Group Amount"}); err != nil {
		h.logger.Error("write consol pl csv header", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	for _, line := range report.Lines {
		if err := writer.Write([]string{
			line.Section,
			line.AccountCode,
			line.AccountName,
			fmt.Sprintf("%.2f", line.LocalAmount),
			fmt.Sprintf("%.2f", line.GroupAmount),
		}); err != nil {
			h.logger.Error("write consol pl csv line", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}
	if err := writer.Write([]string{"", "", "", "", ""}); err != nil {
		h.logger.Error("write consol pl csv spacer", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	totalsRows := [][]string{
		{"Totals", "", "Revenue", "", fmt.Sprintf("%.2f", report.Totals.Revenue)},
		{"Totals", "", "COGS", "", fmt.Sprintf("%.2f", report.Totals.COGS)},
		{"Totals", "", "Gross Profit", "", fmt.Sprintf("%.2f", report.Totals.GrossProfit)},
		{"Totals", "", "Opex", "", fmt.Sprintf("%.2f", report.Totals.Opex)},
		{"Totals", "", "Net Income", "", fmt.Sprintf("%.2f", report.Totals.NetIncome)},
		{"Totals", "", "Delta FX", "", fmt.Sprintf("%.2f", report.Totals.DeltaFX)},
	}
	for _, row := range totalsRows {
		if err := writer.Write(row); err != nil {
			h.logger.Error("write consol pl csv totals", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		h.logger.Error("flush consol pl csv", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if len(warnings) > 0 {
		w.Header().Set("X-Consol-Warning", strings.Join(warnings, "; "))
	}
	filename := fmt.Sprintf("pl-%d-%s.csv", report.Filters.GroupID, report.Filters.Period)
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	if _, err := w.Write(buf.Bytes()); err != nil {
		h.logger.Error("write consol pl csv", slog.Any("error", err))
	}
}

// HandleExportPDF serves the PDF export of the consolidated P&L statement.
func (h *ProfitLossHandler) HandleExportPDF(w http.ResponseWriter, r *http.Request) {
	if h.pdfExporter == nil || !h.pdfExporter.Ready() {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	h.pdfExporter.Serve(w, r, h)
}

func (h *ProfitLossHandler) parseFilters(r *http.Request) (consol.ProfitLossFilters, map[string]string) {
	q := r.URL.Query()
	errors := make(map[string]string)
	groupID, err := strconv.ParseInt(q.Get("group"), 10, 64)
	if err != nil || groupID <= 0 {
		errors["group"] = "Group tidak valid"
	}
	period := strings.TrimSpace(q.Get("period"))
	if period == "" {
		errors["period"] = "Periode wajib diisi"
	} else if _, err := time.Parse("2006-01", period); err != nil {
		errors["period"] = "Format periode tidak valid"
	}
	entitiesParam := strings.TrimSpace(q.Get("entities"))
	var entityIDs []int64
	if entitiesParam != "" && !strings.EqualFold(entitiesParam, "all") {
		seen := make(map[int64]struct{})
		parts := strings.Split(entitiesParam, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, parseErr := strconv.ParseInt(part, 10, 64)
			if parseErr != nil || id <= 0 {
				errors["entities"] = "Daftar entitas tidak valid"
				entityIDs = nil
				break
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			entityIDs = append(entityIDs, id)
		}
	}
	fxParam := strings.ToLower(strings.TrimSpace(q.Get("fx")))
	fxOn := false
	switch fxParam {
	case "", "off":
	case "on":
		fxOn = true
	default:
		errors["fx"] = "Pilihan FX tidak valid"
	}
	if len(errors) > 0 {
		return consol.ProfitLossFilters{}, errors
	}
	return consol.ProfitLossFilters{GroupID: groupID, Period: period, Entities: entityIDs, FxOn: fxOn}, errors
}
