package dashboard

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/trimo/backend/internal/catalog"
	"github.com/trimo/backend/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts the admin dashboard aggregates under /admin.
//
// The cross-domain dashboard and a single department's overview are separate
// grants: a coffee manager gets their own home screen without being handed the
// company-wide revenue figures.
func RegisterRoutes(r chi.Router, h *Handler, adminOnly, departmentGate func(http.Handler) http.Handler) {
	r.Group(func(r chi.Router) {
		r.Use(adminOnly)
		r.Get("/admin/dashboard", h.Get)
	})
	r.Group(func(r chi.Router) {
		r.Use(departmentGate)
		r.Get("/admin/dashboard/departments/{department}", h.GetDepartment)
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.Load(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"dashboard": data})
}

// GetDepartment returns one catalog department's overview. The route guard
// admits any admin with department access; this re-checks the exact one, so a
// tech grant cannot read the coffee numbers.
func (h *Handler) GetDepartment(w http.ResponseWriter, r *http.Request) {
	department := chi.URLParam(r, "department")
	if !catalog.IsValidDepartment(department) {
		httpx.Error(w, http.StatusNotFound, "unknown department")
		return
	}
	if !catalog.CanDepartment(r.Context(), department, catalog.ResourceOverview, false) {
		httpx.Error(w, http.StatusForbidden, "you do not have access to this catalog department")
		return
	}
	data, err := h.svc.LoadDepartment(r.Context(), department)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"overview": data})
}
