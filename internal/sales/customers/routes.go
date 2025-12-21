package customers

import (
	"github.com/go-chi/chi/v5"
)

func (h *Handler) MountRoutes(r chi.Router) {
	// Customer routes
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("sales.customer.view"))
		r.Get("/customers", h.List)
		r.Get("/customers/{id}", h.Show)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.customer.create"))
		r.Get("/customers/new", h.ShowForm)
		r.Post("/customers", h.Create)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.customer.edit"))
		r.Get("/customers/{id}/edit", h.ShowEditForm)
		r.Post("/customers/{id}/edit", h.Update)
	})
}
