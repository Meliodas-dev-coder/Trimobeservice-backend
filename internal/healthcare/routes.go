package healthcare

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts healthcare routes. Catalog + emergency reads are public;
// client request routes require a logged-in customer; management requires an
// admin. Practitioners are an admin-only roster (like drivers).
type AdminGuards struct {
	PractitionersRead, PractitionersManage func(http.Handler) http.Handler
	CategoriesRead, CategoriesManage       func(http.Handler) http.Handler
	ServicesRead, ServicesManage           func(http.Handler) http.Handler
	RequestsRead, RequestsManage           func(http.Handler) http.Handler
	SettingsRead, SettingsManage           func(http.Handler) http.Handler
}

func RegisterRoutes(r chi.Router, h *Handler, requireAuth func(http.Handler) http.Handler, g AdminGuards) {
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
		r.With(g.PractitionersRead).Get("/admin/practitioners", h.ListPractitioners)
		r.With(g.PractitionersRead).Get("/admin/practitioners/{id}", h.GetPractitioner)
		r.With(g.PractitionersManage).Post("/admin/practitioners", h.CreatePractitioner)
		r.With(g.PractitionersManage).Put("/admin/practitioners/{id}", h.UpdatePractitioner)
		r.With(g.PractitionersManage).Delete("/admin/practitioners/{id}", h.DeletePractitioner)

		r.With(g.CategoriesRead).Get("/admin/healthcare/categories", h.ListCategoriesAdmin)
		r.With(g.CategoriesManage).Post("/admin/healthcare/categories", h.CreateCategory)
		r.With(g.CategoriesManage).Put("/admin/healthcare/categories/{id}", h.UpdateCategory)
		r.With(g.CategoriesManage).Delete("/admin/healthcare/categories/{id}", h.DeleteCategory)

		r.With(g.ServicesRead).Get("/admin/healthcare/services", h.ListServicesAdmin)
		r.With(g.ServicesRead).Get("/admin/healthcare/services/{id}", h.GetServiceAdmin)
		r.With(g.ServicesManage).Post("/admin/healthcare/services", h.CreateService)
		r.With(g.ServicesManage).Put("/admin/healthcare/services/{id}", h.UpdateService)
		r.With(g.ServicesManage).Delete("/admin/healthcare/services/{id}", h.DeleteService)

		r.With(g.RequestsRead).Get("/admin/healthcare/requests", h.ListRequestsAdmin)
		r.With(g.RequestsManage).Post("/admin/healthcare/requests", h.CreateRequestAdmin)
		r.With(g.RequestsRead).Get("/admin/healthcare/requests/{id}", h.GetRequestAdmin)
		r.With(g.RequestsManage).Patch("/admin/healthcare/requests/{id}/status", h.UpdateStatus)
		r.With(g.RequestsManage).Patch("/admin/healthcare/requests/{id}/quote", h.SetQuote)
		r.With(g.RequestsManage).Post("/admin/healthcare/requests/{id}/assignments", h.Assign)
		r.With(g.RequestsManage).Delete("/admin/healthcare/assignments/{assignmentId}", h.Unassign)
		// Payment is recorded via the payments module (POST /admin/payments,
		// payable_type 'healthcare'), which flips payment_status in one audited tx.

		r.With(g.SettingsRead).Get("/admin/healthcare/settings", h.GetSettingsAdmin)
		r.With(g.SettingsManage).Put("/admin/healthcare/settings", h.UpdateSettingsAdmin)
	})
}
