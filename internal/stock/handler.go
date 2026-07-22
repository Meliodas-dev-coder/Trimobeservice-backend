package stock

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/trimo/backend/internal/auth"
	"github.com/trimo/backend/internal/catalog"
	"github.com/trimo/backend/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// List returns the SKUs the caller may see, worst stock level first.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	department, ok := departmentParam(w, r)
	if !ok {
		return
	}
	level, ok := levelParam(w, r)
	if !ok {
		return
	}
	limit, page := parsePage(r)
	f := Filter{
		Department: department,
		Level:      level,
		Query:      r.URL.Query().Get("q"),
		ProductID:  optionalID(r, "product_id"),
		Limit:      limit,
		Offset:     (page - 1) * limit,
	}
	items, total, err := h.svc.List(r.Context(), h.allowed(r, false), f)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"stock": items,
		"meta":  httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

// Summary is the KPI header: counts, units, shelf value, and what is running out.
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	department, ok := departmentParam(w, r)
	if !ok {
		return
	}
	allowed := h.allowed(r, false)
	summary, err := h.svc.Summary(r.Context(), allowed, department)
	if err != nil {
		writeError(w, err)
		return
	}
	low, err := h.svc.LowStock(r.Context(), allowed, department, 8)
	if err != nil {
		writeError(w, err)
		return
	}
	movements, err := h.svc.RecentMovements(r.Context(), allowed, department, 8)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"summary":          summary,
		"low_stock":        low,
		"recent_movements": movements,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	item, err := h.svc.GetWithHistory(r.Context(), id, false)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"item": item})
}

func (h *Handler) Movements(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	limit, page := parsePage(r)
	movements, total, err := h.svc.Movements(r.Context(), id, limit, (page-1)*limit)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"movements": movements,
		"meta":      httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

func (h *Handler) Adjust(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req AdjustRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	var actor *int64
	if uid, ok := auth.UserIDFromContext(r.Context()); ok {
		actor = &uid
	}
	item, err := h.svc.Adjust(r.Context(), id, req, actor)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"item": item})
}

func (h *Handler) SetThreshold(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req ThresholdRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.svc.SetThreshold(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"item": item})
}

// --- helpers ---

// allowed is the caller's department scope. The route guard only decides that
// the caller may enter the stock routes at all; this is the precise check.
func (h *Handler) allowed(r *http.Request, manage bool) []string {
	return catalog.AllowedDepartments(r.Context(), catalog.ResourceStock, manage)
}

// departmentParam distinguishes an absent filter from an invalid one, so an
// unknown value can never widen a scoped request to the whole catalog.
func departmentParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	department := strings.TrimSpace(r.URL.Query().Get("department"))
	if department == "" {
		return "", true
	}
	if !catalog.IsValidDepartment(department) {
		httpx.ValidationError(w, map[string]string{"department": "unknown department"})
		return "", false
	}
	return department, true
}

func levelParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	level := strings.TrimSpace(r.URL.Query().Get("level"))
	switch level {
	case "", LevelOK, LevelLow, LevelOut:
		return level, true
	}
	httpx.ValidationError(w, map[string]string{"level": "must be ok, low, or out"})
	return "", false
}

func idParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func optionalID(r *http.Request, key string) *int64 {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return nil
	}
	return &id
}

func parsePage(r *http.Request) (limit, page int) {
	limit, page = 20, 1
	q := r.URL.Query()
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
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

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrVariantNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrForbiddenDepartment):
		httpx.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrNegativeStock), errors.Is(err, ErrInvalidAdjustment):
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
