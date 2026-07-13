package audit

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/trimo/backend/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts the read-only audit views under /admin (admin only).
// The log is written by the Record middleware, not by these endpoints.
func RegisterRoutes(r chi.Router, h *Handler, adminOnly func(http.Handler) http.Handler) {
	r.Group(func(r chi.Router) {
		r.Use(adminOnly)
		r.Get("/admin/audit-logs", h.List)
	})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	limit, page := parsePage(r)
	f := Filter{
		TargetType: r.URL.Query().Get("target_type"),
		Method:     r.URL.Query().Get("method"),
		Limit:      limit,
		Offset:     (page - 1) * limit,
	}
	if v := r.URL.Query().Get("actor_user_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.ActorUserID = &id
		}
	}

	items, total, err := h.svc.List(r.Context(), f)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"logs": items,
		"meta": httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

// parsePage clamps pagination to sane bounds (mirrors the other admin modules).
func parsePage(r *http.Request) (limit, page int) {
	limit, page = 25, 1
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 100 {
		limit = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		page = v
	}
	return limit, page
}
