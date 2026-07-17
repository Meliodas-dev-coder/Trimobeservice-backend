package invoicing

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts invoicing routes onto the /api/v1 subrouter. Everything
// is admin-only (billing lives entirely on the back office).
func RegisterRoutes(r chi.Router, h *Handler, adminOnly func(http.Handler) http.Handler) {
	r.Group(func(r chi.Router) {
		r.Use(adminOnly)

		// Seller identity + invoice defaults (single record).
		r.Get("/admin/org-settings", h.GetOrgSettings)
		r.Put("/admin/org-settings", h.UpdateOrgSettings)

		// Invoices.
		r.Get("/admin/invoices", h.List)
		r.Post("/admin/invoices", h.Create)
		r.Get("/admin/invoices/{id}", h.Get)
		r.Put("/admin/invoices/{id}", h.Update)
		r.Delete("/admin/invoices/{id}", h.Delete)
		r.Post("/admin/invoices/{id}/issue", h.Issue)
		r.Post("/admin/invoices/{id}/void", h.Void)
		r.Post("/admin/invoices/{id}/credit-note", h.CreditNote)
	})
}
