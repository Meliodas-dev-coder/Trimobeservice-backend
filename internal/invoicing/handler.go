package invoicing

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/trimo/backend/internal/auth"
	"github.com/trimo/backend/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ===================== org settings =====================

func (h *Handler) GetOrgSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.GetOrgSettings(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"org_settings": s})
}

func (h *Handler) UpdateOrgSettings(w http.ResponseWriter, r *http.Request) {
	var req OrgSettingsRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateOrgSettings(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	s, err := h.svc.UpdateOrgSettings(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"org_settings": s})
}

// ===================== invoices =====================

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, page := parsePage(r)
	f := InvoiceFilter{
		InvoiceableType: q.Get("invoiceable_type"),
		Kind:            q.Get("kind"),
		Status:          q.Get("status"),
		Limit:           limit,
		Offset:          (page - 1) * limit,
	}
	if v := q.Get("invoiceable_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.InvoiceableID = &id
		}
	}
	items, total, err := h.svc.List(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"invoices": items,
		"meta":     httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	inv, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"invoice": inv})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	adminID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req CreateInvoiceRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateCreateInvoice(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	inv, err := h.svc.CreateInvoice(r.Context(), adminID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"invoice": inv})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req UpdateInvoiceRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateUpdateInvoice(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	inv, err := h.svc.UpdateInvoice(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"invoice": inv})
}

func (h *Handler) Issue(w http.ResponseWriter, r *http.Request) {
	adminID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	inv, err := h.svc.IssueInvoice(r.Context(), adminID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"invoice": inv})
}

func (h *Handler) Void(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	inv, err := h.svc.VoidInvoice(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"invoice": inv})
}

func (h *Handler) CreditNote(w http.ResponseWriter, r *http.Request) {
	adminID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	inv, err := h.svc.CreateCreditNote(r.Context(), adminID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"invoice": inv})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteInvoice(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ===================== helpers =====================

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := httpx.DecodeJSON(w, r, dst); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
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

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvoiceNotFound), errors.Is(err, ErrSourceNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrNotDraft),
		errors.Is(err, ErrCannotVoid),
		errors.Is(err, ErrNotFinalSource),
		errors.Is(err, ErrAlreadyCredit),
		errors.Is(err, ErrConflict):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidType),
		errors.Is(err, ErrInvalidKind),
		errors.Is(err, ErrNoQuote),
		errors.Is(err, ErrBadAmount),
		errors.Is(err, ErrBadDate):
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
