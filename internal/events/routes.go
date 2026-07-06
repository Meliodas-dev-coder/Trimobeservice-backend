package events

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts event-planning routes. Catalog reads are public; client
// request routes require a logged-in customer; management requires an admin.
func RegisterRoutes(r chi.Router, h *Handler, requireAuth, requireAdmin func(http.Handler) http.Handler) {
	// public (client) catalog
	r.Get("/event-service-categories", h.ListCategoriesPublic)
	r.Get("/event-services", h.ListServicesPublic)
	r.Get("/event-services/{slug}", h.GetServicePublic)
	r.Get("/artists", h.ListArtistsPublic)
	r.Get("/artists/{slug}", h.GetArtistPublic)

	// client (authenticated customer)
	r.Group(func(r chi.Router) {
		r.Use(requireAuth)

		r.Post("/event-requests", h.CreateRequest)
		r.Get("/event-requests", h.ListMine)
		r.Get("/event-requests/{id}", h.GetMine)
		r.Post("/event-requests/{id}/cancel", h.Cancel)
	})

	// admin
	r.Group(func(r chi.Router) {
		r.Use(requireAdmin)

		r.Get("/admin/event-service-categories", h.ListCategoriesAdmin)
		r.Post("/admin/event-service-categories", h.CreateCategory)
		r.Put("/admin/event-service-categories/{id}", h.UpdateCategory)
		r.Delete("/admin/event-service-categories/{id}", h.DeleteCategory)

		r.Get("/admin/event-services", h.ListServicesAdmin)
		r.Get("/admin/event-services/{id}", h.GetServiceAdmin)
		r.Post("/admin/event-services", h.CreateService)
		r.Put("/admin/event-services/{id}", h.UpdateService)
		r.Delete("/admin/event-services/{id}", h.DeleteService)

		r.Get("/admin/artists", h.ListArtistsAdmin)
		r.Get("/admin/artists/{id}", h.GetArtistAdmin)
		r.Post("/admin/artists", h.CreateArtist)
		r.Put("/admin/artists/{id}", h.UpdateArtist)
		r.Delete("/admin/artists/{id}", h.DeleteArtist)

		r.Get("/admin/event-requests", h.ListRequestsAdmin)
		r.Post("/admin/event-requests", h.CreateRequestAdmin)
		r.Get("/admin/event-requests/{id}", h.GetRequestAdmin)
		r.Patch("/admin/event-requests/{id}/status", h.UpdateStatus)
		r.Patch("/admin/event-requests/{id}/quote", h.SetQuote)
		// Payment is recorded via the payments module (POST /admin/payments,
		// payable_type 'event'), which flips payment_status in one audited tx.
	})
}
