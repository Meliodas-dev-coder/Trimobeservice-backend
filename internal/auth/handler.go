package auth

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/trimo/backend/internal/httpx"
)

// Handler adapts the auth Service to HTTP.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if problems := validateRegister(req); len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}

	u, pair, err := h.svc.Register(r.Context(), req, metaFrom(r))
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			httpx.Error(w, http.StatusConflict, err.Error())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "could not complete registration")
		return
	}
	user, err := h.svc.UserResponse(r.Context(), u)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not complete registration")
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"user": user, "tokens": pair})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if problems := validateLogin(req); len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}

	u, pair, err := h.svc.Login(r.Context(), req, metaFrom(r))
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			httpx.Error(w, http.StatusUnauthorized, err.Error())
		case errors.Is(err, ErrInactiveUser):
			httpx.Error(w, http.StatusForbidden, err.Error())
		default:
			httpx.Error(w, http.StatusInternalServerError, "could not complete login")
		}
		return
	}
	user, err := h.svc.UserResponse(r.Context(), u)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not complete login")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"user": user, "tokens": pair})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	pair, err := h.svc.Refresh(r.Context(), req, metaFrom(r))
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidRefresh):
			httpx.Error(w, http.StatusUnauthorized, err.Error())
		case errors.Is(err, ErrInactiveUser):
			httpx.Error(w, http.StatusForbidden, err.Error())
		default:
			httpx.Error(w, http.StatusInternalServerError, "could not refresh session")
		}
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"tokens": pair})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.Logout(r.Context(), req.RefreshToken); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not complete logout")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"message": "logged out"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	u, err := h.svc.Me(r.Context(), userID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not load profile")
		return
	}
	user, err := h.svc.UserResponse(r.Context(), u)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not load profile")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"user": user})
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromRequest(w, r)
	if !ok {
		return
	}
	var req ChangePasswordRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if problems := validateChangePassword(req); len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}
	if err := h.svc.ChangePassword(r.Context(), userID, req); err != nil {
		switch {
		case errors.Is(err, ErrWrongPassword):
			httpx.Error(w, http.StatusForbidden, err.Error())
		case errors.Is(err, ErrSamePassword):
			httpx.ValidationError(w, map[string]string{"new_password": "must be different from the current password"})
		default:
			httpx.Error(w, http.StatusInternalServerError, "could not change password")
		}
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"message": "password changed"})
}

func (h *Handler) ListAddresses(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromRequest(w, r)
	if !ok {
		return
	}
	addresses, err := h.svc.ListAddresses(r.Context(), userID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not load addresses")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"addresses": addresses})
}

func (h *Handler) CreateAddress(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromRequest(w, r)
	if !ok {
		return
	}
	var req AddressRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if problems := validateAddress(req); len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}
	address, err := h.svc.CreateAddress(r.Context(), userID, req)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not save address")
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"address": address})
}

func (h *Handler) UpdateAddress(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromRequest(w, r)
	if !ok {
		return
	}
	addressID, ok := addressIDParam(w, r)
	if !ok {
		return
	}
	var req AddressRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if problems := validateAddress(req); len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}
	address, err := h.svc.UpdateAddress(r.Context(), userID, addressID, req)
	if err != nil {
		if errors.Is(err, ErrAddressNotFound) {
			httpx.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "could not save address")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"address": address})
}

func (h *Handler) DeleteAddress(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromRequest(w, r)
	if !ok {
		return
	}
	addressID, ok := addressIDParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteAddress(r.Context(), userID, addressID); err != nil {
		if errors.Is(err, ErrAddressNotFound) {
			httpx.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "could not delete address")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func userIDFromRequest(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return 0, false
	}
	return userID, true
}

func addressIDParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}
