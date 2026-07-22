package adminusers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/trimo/backend/internal/authz"
	"github.com/trimo/backend/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// --- permission catalog ---

func (h *Handler) Permissions(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"permissions": authz.Catalog})
}

// --- roles ---

func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	limit, page := parsePage(r)
	items, total, err := h.svc.ListRoles(r.Context(), Filter{
		Search: r.URL.Query().Get("q"),
		Limit:  limit,
		Offset: (page - 1) * limit,
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"admin_roles": items,
		"meta":        httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

func (h *Handler) GetRole(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	role, err := h.svc.GetRole(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"admin_role": role})
}

func (h *Handler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req RoleRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	role, err := h.svc.CreateRole(r.Context(), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"admin_role": role})
}

func (h *Handler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req RoleRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	role, err := h.svc.UpdateRole(r.Context(), id, req)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"admin_role": role})
}

func (h *Handler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteRole(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- admin users ---

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	limit, page := parsePage(r)
	items, total, err := h.svc.ListUsers(r.Context(), Filter{
		Search: r.URL.Query().Get("q"),
		Limit:  limit,
		Offset: (page - 1) * limit,
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"users": items,
		"meta":  httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	user, err := h.svc.GetUser(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"user": user})
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := h.svc.CreateUser(r.Context(), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"user": user})
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req UpdateUserRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := h.svc.UpdateUser(r.Context(), id, req)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"user": user})
}

// --- shared ---

// writeErr maps service errors to HTTP status codes.
func writeErr(w http.ResponseWriter, err error) {
	var vp *ValidationProblems
	switch {
	case errors.As(err, &vp):
		httpx.ValidationError(w, vp.Fields)
	case errors.Is(err, ErrRoleNotFound), errors.Is(err, ErrUserNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrRoleNameTaken), errors.Is(err, ErrEmailTaken), errors.Is(err, ErrSystemRole), errors.Is(err, ErrEmployeeStatus):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrSuperAdminProtected):
		httpx.Error(w, http.StatusForbidden, err.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

func idParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
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
