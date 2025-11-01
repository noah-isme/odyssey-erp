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

// Handler wires consolidation TB endpoints.
type Handler struct {
	logger      *slog.Logger
	service     *consol.Service
	templates   *view.Engine
	pdfExporter pdfExporter
	csrf        *shared.CSRFManager
	sessions    *shared.SessionManager
	rbac        rbac.Middleware
	rateLimit   func(http.Handler) http.Handler
}

type pdfExporter interface {
	Ready() bool
	Serve(http.ResponseWriter, *http.Request, *Handler)
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
	pdfExporter, err := newPDFExporter(logger, pdfClient)
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
		logger:      logger,
		service:     service,
		templates:   templates,
		pdfExporter: pdfExporter,
		csrf:        csrf,
		sessions:    sessions,
		rbac:        rbac,
		rateLimit:   limiter,
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
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=consolidated_tb.csv")
	if err := writeTBCsv(w, tb, nil); err != nil {
		h.logger.Error("stream consol tb csv", slog.Any("error", err))
	}
}

func (h *Handler) handleExportPDF(w http.ResponseWriter, r *http.Request) {
	if h.pdfExporter == nil || !h.pdfExporter.Ready() {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	h.pdfExporter.Serve(w, r, h)
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
