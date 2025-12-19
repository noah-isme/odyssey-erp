package accounts

import "github.com/go-chi/chi/v5"

func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/", h.List)
}
