package journals

import "github.com/go-chi/chi/v5"

func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Post("/{id}/void", h.Void)
	r.Post("/{id}/reverse", h.Reverse)
}
