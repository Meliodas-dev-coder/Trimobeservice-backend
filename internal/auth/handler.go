package auth

import (
	"errors"
	"net/http"

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
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"user": toUserResponse(u), "tokens": pair})
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
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"user": toUserResponse(u), "tokens": pair})
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
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"user": toUserResponse(u)})
}
