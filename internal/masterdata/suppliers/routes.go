package suppliers

import "github.com/go-chi/chi/v5"

func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("master.view"))
		r.Get("/", h.List)
		r.Get("/{id}", h.Show)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("master.edit"))
		r.Get("/new", h.Form)
		r.Post("/", h.Create)
		r.Get("/{id}/edit", h.EditForm)
		r.Post("/{id}/edit", h.Update)
		r.Post("/{id}/delete", h.Delete)
	})
}
