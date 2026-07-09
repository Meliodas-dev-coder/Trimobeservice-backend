package healthcare

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts healthcare routes. Catalog + emergency reads are public;
// client request routes require a logged-in customer; management requires an
// admin. Practitioners are an admin-only roster (like drivers).
func RegisterRoutes(r chi.Router, h *Handler, requireAuth, requireAdmin func(http.Handler) http.Handler) {
	// public (client)
	r.Get("/healthcare/emergency", h.GetEmergencyPublic)
	r.Get("/healthcare/categories", h.ListCategoriesPublic)
	r.Get("/healthcare/services", h.ListServicesPublic)
	r.Get("/healthcare/services/{slug}", h.GetServicePublic)

	// client (authenticated customer)
	r.Group(func(r chi.Router) {
		r.Use(requireAuth)

		r.Post("/healthcare/requests", h.CreateRequest)
		r.Get("/healthcare/requests", h.ListMine)
		r.Get("/healthcare/requests/{id}", h.GetMine)
		r.Post("/healthcare/requests/{id}/cancel", h.Cancel)
	})

	// admin
	r.Group(func(r chi.Router) {
		r.Use(requireAdmin)

		r.Get("/admin/practitioners", h.ListPractitioners)
		r.Get("/admin/practitioners/{id}", h.GetPractitioner)
		r.Post("/admin/practitioners", h.CreatePractitioner)
		r.Put("/admin/practitioners/{id}", h.UpdatePractitioner)
		r.Delete("/admin/practitioners/{id}", h.DeletePractitioner)

		r.Get("/admin/healthcare/categories", h.ListCategoriesAdmin)
		r.Post("/admin/healthcare/categories", h.CreateCategory)
		r.Put("/admin/healthcare/categories/{id}", h.UpdateCategory)
		r.Delete("/admin/healthcare/categories/{id}", h.DeleteCategory)

		r.Get("/admin/healthcare/services", h.ListServicesAdmin)
		r.Get("/admin/healthcare/services/{id}", h.GetServiceAdmin)
		r.Post("/admin/healthcare/services", h.CreateService)
		r.Put("/admin/healthcare/services/{id}", h.UpdateService)
		r.Delete("/admin/healthcare/services/{id}", h.DeleteService)

		r.Get("/admin/healthcare/requests", h.ListRequestsAdmin)
		r.Post("/admin/healthcare/requests", h.CreateRequestAdmin)
		r.Get("/admin/healthcare/requests/{id}", h.GetRequestAdmin)
		r.Patch("/admin/healthcare/requests/{id}/status", h.UpdateStatus)
		r.Patch("/admin/healthcare/requests/{id}/quote", h.SetQuote)
		r.Post("/admin/healthcare/requests/{id}/assignments", h.Assign)
		r.Delete("/admin/healthcare/assignments/{assignmentId}", h.Unassign)
		// Payment is recorded via the payments module (POST /admin/payments,
		// payable_type 'healthcare'), which flips payment_status in one audited tx.

		r.Get("/admin/healthcare/settings", h.GetSettingsAdmin)
		r.Put("/admin/healthcare/settings", h.UpdateSettingsAdmin)
	})
}
