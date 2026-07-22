package stock

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// AdminGuards separate reading the shelf from moving it, so a department can be
// granted stock visibility without the right to restock or correct.
type AdminGuards struct {
	Read   func(http.Handler) http.Handler
	Manage func(http.Handler) http.Handler
}

// RegisterRoutes mounts the admin-only stock routes. There is no public side:
// customers see availability through the catalog, never the ledger.
func RegisterRoutes(r chi.Router, h *Handler, guards AdminGuards) {
	r.Group(func(r chi.Router) {
		r.Use(guards.Read)
		r.Get("/admin/stock", h.List)
		r.Get("/admin/stock/summary", h.Summary)
		r.Get("/admin/stock/{id}", h.Get)
		r.Get("/admin/stock/{id}/movements", h.Movements)
	})
	r.Group(func(r chi.Router) {
		r.Use(guards.Manage)
		r.Post("/admin/stock/{id}/adjust", h.Adjust)
		r.Patch("/admin/stock/{id}/threshold", h.SetThreshold)
	})
}
