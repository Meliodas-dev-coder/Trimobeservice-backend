package bookings

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts booking routes. Availability is public; customer booking
// routes require a logged-in user; management requires an admin.
type AdminGuards struct {
	Read, Manage func(http.Handler) http.Handler
}

func RegisterRoutes(r chi.Router, h *Handler, requireAuth func(http.Handler) http.Handler, admin AdminGuards) {
	// public
	r.Get("/availability", h.CheckAvailability)
	r.Get("/availability/ranges", h.BookedRanges)

	// client (authenticated customer)
	r.Group(func(r chi.Router) {
		r.Use(requireAuth)

		r.Post("/bookings", h.Create)
		r.Post("/bookings/batch", h.CreateBatch)
		r.Get("/bookings", h.ListMine)
		r.Get("/bookings/{id}", h.GetMine)
		r.Post("/bookings/{id}/cancel", h.Cancel)
	})

	// admin
	r.Group(func(r chi.Router) {
		r.With(admin.Read).Get("/admin/bookings/lookups/cars", h.BookingCarOptions)
		r.With(admin.Read).Get("/admin/bookings/lookups/drivers", h.BookingDriverOptions)
		r.With(admin.Read).Get("/admin/bookings", h.ListAdmin)
		r.With(admin.Manage).Post("/admin/bookings", h.CreateAdmin)
		r.With(admin.Read).Get("/admin/bookings/{id}", h.GetAdmin)
		r.With(admin.Manage).Delete("/admin/bookings/{id}", h.DeleteAdmin)
		r.With(admin.Manage).Post("/admin/bookings/{id}/assign-driver", h.AssignDriver)
		r.With(admin.Manage).Patch("/admin/bookings/{id}/status", h.UpdateStatus)
		// Payment confirmation is handled by the payments module
		// (POST /admin/payments), which records an audited ledger entry.
	})
}
