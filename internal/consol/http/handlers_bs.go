package http

import (
	"bytes"
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
			}
		}
		if !cachedHit {
			report, warnings, err := h.service.Build(r.Context(), filters)
			if err != nil {
				errors["general"] = err.Error()
			} else {
				extraWarnings := append([]string(nil), warnings...)
				if !report.Totals.Balanced {
					extraWarnings = append(extraWarnings, "Consolidated BS not balanced")
				}
				vm = NewConsolBSViewModel(report, extraWarnings)
				vm.Errors = errors
				viewModelCache.Set(cacheKey, cloneBSViewModel(vm))
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
	buf := &bytes.Buffer{}
	writer := csv.NewWriter(buf)
	if err := writer.Write([]string{"Section", "Account Code", "Account Name", "Local Amount", "Group Amount"}); err != nil {
		h.logger.Error("write consol bs csv header", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	for _, line := range report.Assets {
		if err := writer.Write([]string{
			"ASSET",
			line.AccountCode,
			line.AccountName,
			fmt.Sprintf("%.2f", line.LocalAmount),
			fmt.Sprintf("%.2f", line.GroupAmount),
		}); err != nil {
			h.logger.Error("write consol bs csv asset", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}
	if err := writer.Write([]string{"", "", "", "", ""}); err != nil {
		h.logger.Error("write consol bs csv spacer", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	for _, line := range report.LiabilitiesEq {
		if err := writer.Write([]string{
			line.Section,
			line.AccountCode,
			line.AccountName,
			fmt.Sprintf("%.2f", line.LocalAmount),
			fmt.Sprintf("%.2f", line.GroupAmount),
		}); err != nil {
			h.logger.Error("write consol bs csv liab", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}
	if err := writer.Write([]string{"", "", "", "", ""}); err != nil {
		h.logger.Error("write consol bs totals spacer", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	totalsRows := [][]string{
		{"Totals", "", "Assets", "", fmt.Sprintf("%.2f", report.Totals.Assets)},
		{"Totals", "", "Liabilities + Equity", "", fmt.Sprintf("%.2f", report.Totals.LiabEquity)},
		{"Totals", "", "Balanced", "", fmt.Sprintf("%t", report.Totals.Balanced)},
		{"Totals", "", "Delta FX", "", fmt.Sprintf("%.2f", report.Totals.DeltaFX)},
	}
	for _, row := range totalsRows {
		if err := writer.Write(row); err != nil {
			h.logger.Error("write consol bs totals", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		h.logger.Error("flush consol bs csv", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	warningsHeader := append([]string(nil), warnings...)
	if !report.Totals.Balanced {
		warningsHeader = append(warningsHeader, "Consolidated BS not balanced")
	}
	if len(warningsHeader) > 0 {
		w.Header().Set("X-Consol-Warning", strings.Join(warningsHeader, "; "))
	}
	filename := fmt.Sprintf("bs-%d-%s.csv", report.Filters.GroupID, report.Filters.Period)
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	if _, err := w.Write(buf.Bytes()); err != nil {
		h.logger.Error("write consol bs csv", slog.Any("error", err))
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
