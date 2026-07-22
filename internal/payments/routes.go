package payments

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type AdminGuards struct {
	Read   func(http.Handler) http.Handler
	Manage func(http.Handler) http.Handler
}

// RegisterRoutes mounts the payment ledger under /admin (admin only).
func RegisterRoutes(r chi.Router, h *Handler, guards AdminGuards) {
	r.Group(func(r chi.Router) {
		r.Use(guards.Read)
		r.Get("/admin/payments", h.List)
		r.Get("/admin/payments/{id}", h.Get)
	})
	r.Group(func(r chi.Router) {
		r.Use(guards.Manage)
		r.Post("/admin/payments", h.Record)
		r.Post("/admin/payments/{id}/refund", h.Refund)
	})
}
