package bookings

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts booking routes. Availability is public; customer booking
// routes require a logged-in user; management requires an admin.
func RegisterRoutes(r chi.Router, h *Handler, requireAuth, requireAdmin func(http.Handler) http.Handler) {
	// public
	r.Get("/availability", h.CheckAvailability)

	// client (authenticated customer)
	r.Group(func(r chi.Router) {
		r.Use(requireAuth)

		r.Post("/bookings", h.Create)
		r.Get("/bookings", h.ListMine)
		r.Get("/bookings/{id}", h.GetMine)
		r.Post("/bookings/{id}/cancel", h.Cancel)
	})

	// admin
	r.Group(func(r chi.Router) {
		r.Use(requireAdmin)

		r.Get("/admin/bookings", h.ListAdmin)
		r.Get("/admin/bookings/{id}", h.GetAdmin)
		r.Post("/admin/bookings/{id}/assign-driver", h.AssignDriver)
		r.Patch("/admin/bookings/{id}/status", h.UpdateStatus)
		// Payment confirmation is handled by the payments module
		// (POST /admin/payments), which records an audited ledger entry.
	})
}
