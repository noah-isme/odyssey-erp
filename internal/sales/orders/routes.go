package orders

import (
	"github.com/go-chi/chi/v5"
)

func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("sales.order.view"))
		r.Get("/orders", h.List)
		r.Get("/orders/{id}", h.Show)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.order.create"))
		r.Get("/orders/new", h.ShowForm)
		r.Post("/orders", h.Create)
		r.Post("/quotations/{id}/convert", h.ConvertFromQuotation)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.order.edit"))
		r.Get("/orders/{id}/edit", h.ShowEditForm)
		r.Post("/orders/{id}/edit", h.Update)
		r.Post("/orders/{id}/confirm", h.Confirm)
		r.Post("/orders/{id}/cancel", h.Cancel)
	})
}
