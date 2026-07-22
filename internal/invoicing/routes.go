package invoicing

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type AdminGuards struct {
	DocumentsRead   func(http.Handler) http.Handler
	DocumentsManage func(http.Handler) http.Handler
	SettingsRead    func(http.Handler) http.Handler
	SettingsManage  func(http.Handler) http.Handler
}

// RegisterRoutes mounts invoicing routes onto the /api/v1 subrouter. Everything
// is admin-only (billing lives entirely on the back office).
func RegisterRoutes(r chi.Router, h *Handler, guards AdminGuards) {
	r.Group(func(r chi.Router) {
		r.Use(guards.SettingsRead)
		r.Get("/admin/org-settings", h.GetOrgSettings)
	})
	r.Group(func(r chi.Router) {
		r.Use(guards.SettingsManage)
		r.Put("/admin/org-settings", h.UpdateOrgSettings)
	})

	r.Group(func(r chi.Router) {
		r.Use(guards.DocumentsRead)
		r.Get("/admin/invoices", h.List)
		r.Get("/admin/invoices/{id}", h.Get)
	})
	r.Group(func(r chi.Router) {
		r.Use(guards.DocumentsManage)
		r.Post("/admin/invoices", h.Create)
		r.Put("/admin/invoices/{id}", h.Update)
		r.Delete("/admin/invoices/{id}", h.Delete)
		r.Post("/admin/invoices/{id}/issue", h.Issue)
		r.Post("/admin/invoices/{id}/void", h.Void)
		r.Post("/admin/invoices/{id}/credit-note", h.CreditNote)
	})
}
