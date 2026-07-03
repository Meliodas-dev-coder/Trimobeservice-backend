package payments

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts the payment ledger under /admin (admin only).
func RegisterRoutes(r chi.Router, h *Handler, adminOnly func(http.Handler) http.Handler) {
	r.Group(func(r chi.Router) {
		r.Use(adminOnly)

		r.Post("/admin/payments", h.Record)
		r.Get("/admin/payments", h.List)
		r.Get("/admin/payments/{id}", h.Get)
		r.Post("/admin/payments/{id}/refund", h.Refund)
	})
}
