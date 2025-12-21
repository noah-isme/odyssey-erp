package quotations

import (
	"github.com/go-chi/chi/v5"
)

func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("sales.quotation.view"))
		r.Get("/quotations", h.List)
		r.Get("/quotations/{id}", h.Show)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.quotation.create"))
		r.Get("/quotations/new", h.ShowForm)
		r.Post("/quotations", h.Create)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.quotation.edit"))
		r.Get("/quotations/{id}/edit", h.ShowEditForm)
		r.Post("/quotations/{id}/edit", h.Update)
		r.Post("/quotations/{id}/submit", h.Submit)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.quotation.approve"))
		r.Post("/quotations/{id}/approve", h.Approve)
		r.Post("/quotations/{id}/reject", h.Reject)
	})
}
