package customers

import (
	"errors"
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

// RegisterRoutes mounts the read-only customer views under /admin (admin only).
func RegisterRoutes(r chi.Router, h *Handler, adminOnly func(http.Handler) http.Handler) {
	r.Group(func(r chi.Router) {
		r.Use(adminOnly)
		r.Get("/admin/customers", h.List)
		r.Get("/admin/customers/{id}", h.Get)
	})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	limit, page := parsePage(r)
	items, total, err := h.svc.List(r.Context(), Filter{
		Search: r.URL.Query().Get("q"),
		Limit:  limit,
		Offset: (page - 1) * limit,
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"customers": items,
		"meta":      httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	customer, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrCustomerNotFound) {
			httpx.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"customer": customer})
}

func parsePage(r *http.Request) (limit, page int) {
	limit, page = 20, 1
	q := r.URL.Query()
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 1 {
			page = n
		}
	}
	return limit, page
}
