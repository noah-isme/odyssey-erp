package http

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"html/template"
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
	"github.com/odyssey-erp/odyssey-erp/web"
)

// Handler wires consolidation TB endpoints.
type Handler struct {
	logger       *slog.Logger
	service      *consol.Service
	templates    *view.Engine
	pdfTemplates *template.Template
	pdfClient    PDFRenderClient
	csrf         *shared.CSRFManager
	sessions     *shared.SessionManager
	rbac         rbac.Middleware
	rateLimit    func(http.Handler) http.Handler
}

// PDFRenderClient defines the minimal subset of the report client we use.
type PDFRenderClient interface {
	RenderHTML(ctx context.Context, html string) ([]byte, error)
}

// NewHandler constructs the consolidation handler.
func NewHandler(logger *slog.Logger, service *consol.Service, templates *view.Engine, csrf *shared.CSRFManager, sessions *shared.SessionManager, rbac rbac.Middleware, pdfClient PDFRenderClient) (*Handler, error) {
	if templates == nil {
		return nil, fmt.Errorf("consol handler: template engine required")
	}
	funcMap := template.FuncMap{
		"formatDecimal": func(v float64) string {
			return fmt.Sprintf("%.2f", v)
		},
	}
	pdfTpl, err := template.New("consol_tb_pdf.html").Funcs(funcMap).ParseFS(web.Templates, "templates/reports/finance/consol_tb_pdf.html")
	if err != nil {
		return nil, err
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
	return &Handler{
		logger:       logger,
		service:      service,
		templates:    templates,
		pdfTemplates: pdfTpl,
		pdfClient:    pdfClient,
		csrf:         csrf,
		sessions:     sessions,
		rbac:         rbac,
		rateLimit:    limiter,
	}, nil
}

// MountRoutes registers consolidation trial balance routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermFinanceConsolView))
		r.Get("/finance/consol/tb", h.handleGetTB)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermFinanceConsolExport))
		r.Use(h.rateLimit)
		r.Get("/finance/consol/tb/export.csv", h.handleExportCSV)
		r.Get("/finance/consol/tb/pdf", h.handleExportPDF)
	})
}

func (h *Handler) handleGetTB(w http.ResponseWriter, r *http.Request) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	filter, errors := h.parseFilters(r)
	vm := ConsolTBVM{Errors: errors}
	if len(errors) == 0 {
		tb, err := h.service.GetConsolidatedTB(r.Context(), filter)
		if err != nil {
			errors["general"] = err.Error()
		} else {
			vm = FromDomain(tb)
			vm.Errors = errors
		}
	}
	data := view.TemplateData{
		Title:       "Consolidated Trial Balance",
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        vm,
	}
	if err := h.templates.Render(w, "pages/finance/consol_tb.html", data); err != nil {
		h.logger.Error("render consol tb", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (h *Handler) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	filter, errors := h.parseFilters(r)
	if len(errors) > 0 {
		http.Error(w, strings.Join(mapValues(errors), "; "), http.StatusBadRequest)
		return
	}
	tb, err := h.service.GetConsolidatedTB(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	buf := &bytes.Buffer{}
	writer := csv.NewWriter(buf)
	_ = writer.Write([]string{"Group Account", "Name", "Local Amount", "Group Amount"})
	for _, line := range tb.Lines {
		writer.Write([]string{
			line.GroupAccountCode,
			line.GroupAccountName,
			fmt.Sprintf("%.2f", line.LocalAmount),
			fmt.Sprintf("%.2f", line.GroupAmount),
		})
	}
	writer.Flush()
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=consolidated_tb.csv")
	_, _ = w.Write(buf.Bytes())
}

func (h *Handler) handleExportPDF(w http.ResponseWriter, r *http.Request) {
	if h.pdfTemplates == nil || h.pdfClient == nil {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	filter, errors := h.parseFilters(r)
	if len(errors) > 0 {
		http.Error(w, strings.Join(mapValues(errors), "; "), http.StatusBadRequest)
		return
	}
	tb, err := h.service.GetConsolidatedTB(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	vm := FromDomain(tb)
	buf := &bytes.Buffer{}
	if err := h.pdfTemplates.ExecuteTemplate(buf, "consol_tb_pdf.html", vm); err != nil {
		h.logger.Error("render consol tb pdf", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	pdf, err := h.pdfClient.RenderHTML(r.Context(), buf.String())
	if err != nil {
		h.logger.Error("generate consol tb pdf", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=consolidated_tb.pdf")
	_, _ = w.Write(pdf)
}

func (h *Handler) parseFilters(r *http.Request) (consol.Filters, map[string]string) {
	q := r.URL.Query()
	errors := make(map[string]string)
	groupID, err := strconv.ParseInt(q.Get("group"), 10, 64)
	if err != nil || groupID <= 0 {
		errors["group"] = "Group tidak valid"
	}
	period := q.Get("period")
	if period == "" {
		errors["period"] = "Periode wajib diisi"
	}
	entitiesParam := strings.TrimSpace(q.Get("entities"))
	var entityIDs []int64
	switch {
	case entitiesParam == "" || strings.EqualFold(entitiesParam, "all"):
	default:
		parts := strings.Split(entitiesParam, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			id, parseErr := strconv.ParseInt(p, 10, 64)
			if parseErr != nil {
				errors["entities"] = "Daftar entitas tidak valid"
				break
			}
			entityIDs = append(entityIDs, id)
		}
	}
	if len(errors) > 0 {
		return consol.Filters{}, errors
	}
	return consol.Filters{GroupID: groupID, Period: period, Entities: entityIDs}, errors
}

func mapValues(m map[string]string) []string {
	values := make([]string, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}
