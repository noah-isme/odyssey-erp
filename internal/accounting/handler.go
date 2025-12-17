package accounting

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler wires finance ledger endpoints.
type Handler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
}

// NewHandler builds a Handler instance.
func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine) *Handler {
	return &Handler{logger: logger, service: service, templates: templates}
}

// MountRoutes registers HTTP routes for the ledger module.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/coa", h.handleListAccounts)
	r.Get("/journals", h.handleListJournals)
	r.Post("/finance/journals", h.handleNotImplemented)
	r.Post("/finance/journals/{id}/void", h.handleNotImplemented)
	r.Post("/finance/journals/{id}/reverse", h.handleNotImplemented)
	r.Get("/finance/gl", h.handleNotImplemented)
	r.Get("/finance/reports/trial-balance", h.handleNotImplemented)
	r.Get("/finance/reports/pl", h.handleNotImplemented)
	r.Get("/finance/reports/bs", h.handleNotImplemented)
	r.Get("/finance/reports/trial-balance/pdf", h.handleNotImplemented)
	r.Get("/finance/reports/pl/pdf", h.handleNotImplemented)
	r.Get("/finance/reports/bs/pdf", h.handleNotImplemented)
}

func (h *Handler) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.service.ListAccounts(r.Context())
	if err != nil {
		h.logger.Error("list accounts", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	data := map[string]any{"Accounts": accounts}
	viewData := view.TemplateData{Title: "Chart of Accounts", Data: data}
	if err := h.templates.Render(w, "pages/accounting/coa_list.html", viewData); err != nil {
		h.logger.Error("render coa", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (h *Handler) handleListJournals(w http.ResponseWriter, r *http.Request) {
	entries, err := h.service.ListJournalEntries(r.Context())
	if err != nil {
		h.logger.Error("list journals", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	data := map[string]any{"JournalEntries": entries}
	viewData := view.TemplateData{Title: "Journal Entries", Data: data}
	if err := h.templates.Render(w, "pages/accounting/journals_list.html", viewData); err != nil {
		h.logger.Error("render journals", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (h *Handler) handleNotImplemented(w http.ResponseWriter, _ *http.Request) {
	h.logger.Info("ledger handler invoked", slog.String("path", "finance"))
	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}
