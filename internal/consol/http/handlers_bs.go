package http

import (
	"context"
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

type bsPDFExporter interface {
	Ready() bool
	Serve(http.ResponseWriter, *http.Request, *BalanceSheetHandler)
}

// BalanceSheetHandler wires the HTTP layer for consolidated balance sheet endpoints.
type BalanceSheetHandler struct {
	logger      *slog.Logger
	service     *consol.BalanceSheetService
	templates   *view.Engine
	csrf        *shared.CSRFManager
	sessions    *shared.SessionManager
	rbac        rbac.Middleware
	rateLimit   func(http.Handler) http.Handler
	pdfExporter bsPDFExporter
}

// NewBalanceSheetHandler constructs the handler instance.
func NewBalanceSheetHandler(logger *slog.Logger, service *consol.BalanceSheetService, templates *view.Engine, csrf *shared.CSRFManager, sessions *shared.SessionManager, rbac rbac.Middleware, pdfClient PDFRenderClient) (*BalanceSheetHandler, error) {
	if templates == nil {
		return nil, fmt.Errorf("consol bs handler: template engine required")
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
	exporter, err := newBSPDFExporter(logger, pdfClient)
	if err != nil {
		return nil, err
	}
	return &BalanceSheetHandler{
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

// MountRoutes registers consolidated balance sheet endpoints.
func (h *BalanceSheetHandler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermFinanceConsolView))
		r.Get("/finance/consol/bs", h.HandleGet)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermFinanceConsolExport))
		r.Use(h.rateLimit)
		r.Get("/finance/consol/bs/export.csv", h.HandleExportCSV)
		r.Get("/finance/consol/bs/pdf", h.HandleExportPDF)
	})
}

// HandleGet renders the consolidated balance sheet page.
func (h *BalanceSheetHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	filters, errors := h.parseFilters(r)
	vm := ConsolBSViewModel{Errors: errors}
	if len(errors) == 0 {
		cacheKey := buildCacheKey("bs", filters.GroupID, filters.Period, filters.Entities, filters.FxOn)
		cachedHit := false
		if cached, ok := viewModelCache.Get(cacheKey); ok {
			if cachedVM, ok := cached.(ConsolBSViewModel); ok {
				vm = cloneBSViewModel(cachedVM)
				vm.Errors = errors
				cachedHit = true
				recordCacheHit("bs", filters.GroupID, filters.Period)
			}
		}
		if !cachedHit {
			result, err, _ := singleflightBuild(r.Context(), cacheKey, func(ctx context.Context) (interface{}, error) {
				start := time.Now()
				recordCacheMiss("bs", filters.GroupID, filters.Period)
				defer func(start time.Time) {
					observeVMBuildDuration("bs", filters.GroupID, filters.Period, time.Since(start))
				}(start)
				report, warnings, err := h.service.Build(ctx, filters)
				if err != nil {
					return nil, err
				}
				extraWarnings := append([]string(nil), warnings...)
				if !report.Totals.Balanced {
					extraWarnings = append(extraWarnings, "Consolidated BS not balanced")
				}
				vm := NewConsolBSViewModel(report, extraWarnings)
				viewModelCache.Set(cacheKey, cloneBSViewModel(vm))
				return vm, nil
			})
			if err != nil {
				errors["general"] = err.Error()
			} else if result != nil {
				if builtVM, ok := result.(ConsolBSViewModel); ok {
					vm = builtVM
					vm.Errors = errors
				}
			}
		}
	}
	data := view.TemplateData{
		Title:       "Consolidated Balance Sheet",
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        vm,
	}
	if err := h.templates.Render(w, "pages/finance/consol_bs.html", data); err != nil {
		h.logger.Error("render consol bs", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// HandleExportCSV handles CSV exports of the balance sheet.
func (h *BalanceSheetHandler) HandleExportCSV(w http.ResponseWriter, r *http.Request) {
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
	exportWarnings := append([]string(nil), warnings...)
	if !report.Totals.Balanced {
		exportWarnings = append(exportWarnings, "Consolidated BS not balanced")
	}
	vm := NewConsolBSViewModel(report, exportWarnings)
	if len(vm.Warnings) > 0 {
		w.Header().Set("X-Consol-Warning", strings.Join(vm.Warnings, "; "))
	}
	filename := fmt.Sprintf("bs-%d-%s.csv", report.Filters.GroupID, report.Filters.Period)
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	if err := writeBSCsv(w, report, vm.Warnings); err != nil {
		h.logger.Error("stream consol bs csv", slog.Any("error", err))
	}
}

// HandleExportPDF handles PDF exports of the balance sheet.
func (h *BalanceSheetHandler) HandleExportPDF(w http.ResponseWriter, r *http.Request) {
	if h.pdfExporter == nil || !h.pdfExporter.Ready() {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	h.pdfExporter.Serve(w, r, h)
}

func (h *BalanceSheetHandler) parseFilters(r *http.Request) (consol.BalanceSheetFilters, map[string]string) {
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
		return consol.BalanceSheetFilters{}, errors
	}
	return consol.BalanceSheetFilters{GroupID: groupID, Period: period, Entities: entityIDs, FxOn: fxOn}, errors
}
