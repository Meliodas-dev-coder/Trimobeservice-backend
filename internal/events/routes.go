package events

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts event-planning routes. Catalog reads are public; client
// request routes require a logged-in customer; management requires an admin.
type AdminGuards struct {
	CategoriesRead, CategoriesManage func(http.Handler) http.Handler
	ServicesRead, ServicesManage     func(http.Handler) http.Handler
	ArtistsRead, ArtistsManage       func(http.Handler) http.Handler
	RequestsRead, RequestsManage     func(http.Handler) http.Handler
}

func RegisterRoutes(r chi.Router, h *Handler, requireAuth func(http.Handler) http.Handler, g AdminGuards) {
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
		r.With(g.CategoriesRead).Get("/admin/event-service-categories", h.ListCategoriesAdmin)
		r.With(g.CategoriesManage).Post("/admin/event-service-categories", h.CreateCategory)
		r.With(g.CategoriesManage).Put("/admin/event-service-categories/{id}", h.UpdateCategory)
		r.With(g.CategoriesManage).Delete("/admin/event-service-categories/{id}", h.DeleteCategory)

		r.With(g.ServicesRead).Get("/admin/event-services", h.ListServicesAdmin)
		r.With(g.ServicesRead).Get("/admin/event-services/{id}", h.GetServiceAdmin)
		r.With(g.ServicesManage).Post("/admin/event-services", h.CreateService)
		r.With(g.ServicesManage).Put("/admin/event-services/{id}", h.UpdateService)
		r.With(g.ServicesManage).Delete("/admin/event-services/{id}", h.DeleteService)

		r.With(g.ArtistsRead).Get("/admin/artists", h.ListArtistsAdmin)
		r.With(g.ArtistsRead).Get("/admin/artists/{id}", h.GetArtistAdmin)
		r.With(g.ArtistsManage).Post("/admin/artists", h.CreateArtist)
		r.With(g.ArtistsManage).Put("/admin/artists/{id}", h.UpdateArtist)
		r.With(g.ArtistsManage).Delete("/admin/artists/{id}", h.DeleteArtist)

		r.With(g.RequestsRead).Get("/admin/event-requests", h.ListRequestsAdmin)
		r.With(g.RequestsManage).Post("/admin/event-requests", h.CreateRequestAdmin)
		r.With(g.RequestsRead).Get("/admin/event-requests/{id}", h.GetRequestAdmin)
		r.With(g.RequestsManage).Patch("/admin/event-requests/{id}/status", h.UpdateStatus)
		r.With(g.RequestsManage).Patch("/admin/event-requests/{id}/quote", h.SetQuote)
		// Payment is recorded via the payments module (POST /admin/payments,
		// payable_type 'event'), which flips payment_status in one audited tx.
	})
}
