package accounts

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
	accounts, err := h.service.List(r.Context())
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
