package insightshhtp

import (
	"log/slog"
	"net/http"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler menangani permintaan halaman finance insights.
type Handler struct {
	logger    *slog.Logger
	templates *view.Engine
}

// NewHandler membuat instance handler insights baru.
func NewHandler(logger *slog.Logger, templates *view.Engine) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{logger: logger, templates: templates}
}

func (h *Handler) handleInsights(w http.ResponseWriter, r *http.Request) {
	if h.templates == nil {
		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
		return
	}
	sess := shared.SessionFromContext(r.Context())
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	data := view.TemplateData{
		Title:       "Finance Insights",
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data: map[string]any{
			"Ready": false,
		},
	}
	if err := h.templates.Render(w, "pages/finance/insights.html", data); err != nil {
		h.logger.Error("render insights scaffold", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
