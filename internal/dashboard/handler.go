package dashboard

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/trimo/backend/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts the admin dashboard aggregate under /admin (admin only).
func RegisterRoutes(r chi.Router, h *Handler, adminOnly func(http.Handler) http.Handler) {
	r.Group(func(r chi.Router) {
		r.Use(adminOnly)
		r.Get("/admin/dashboard", h.Get)
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
