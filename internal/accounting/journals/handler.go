package journals

import (
	"log/slog"
	"net/http"

	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type Handler struct {
	service   *Service
	logger    *slog.Logger
	templates *view.Engine
}

func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine) *Handler {
	return &Handler{logger: logger, service: service, templates: templates}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	entries, err := h.service.List(r.Context())
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

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented yet", http.StatusNotImplemented)
}

func (h *Handler) Void(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented yet", http.StatusNotImplemented)
}

func (h *Handler) Reverse(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented yet", http.StatusNotImplemented)
}
