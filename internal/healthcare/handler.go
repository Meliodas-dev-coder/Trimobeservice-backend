package healthcare

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/trimo/backend/internal/auth"
	"github.com/trimo/backend/internal/httpx"
	"github.com/trimo/backend/internal/realtime"
)

type Handler struct {
	svc *Service
	pub realtime.Publisher
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// SetPublisher wires the realtime hub so new care requests stream to admins. Nil-safe.
func (h *Handler) SetPublisher(p realtime.Publisher) { h.pub = p }

func (h *Handler) publishCreated(e *RequestDetail) { h.publish("healthcare_request.created", e) }
func (h *Handler) publishStatus(e *RequestDetail)  { h.publish("healthcare_request.status_changed", e) }

func (h *Handler) publish(evtType string, e *RequestDetail) {
	if h.pub == nil || e == nil {
		return
	}
	h.pub.Publish(realtime.Event{
		Type: evtType,
		Payload: map[string]any{
			"id":             e.ID,
			"number":         e.RequestNumber,
			"customer_name":  e.CustomerName,
			"patient_name":   e.PatientName,
			"request_type":   e.RequestType,
			"status":         e.Status,
			"payment_status": e.PaymentStatus,
		},
	})
}

// ===================== public (client) reads =====================

func (h *Handler) GetEmergencyPublic(w http.ResponseWriter, r *http.Request) {
	set, err := h.svc.GetSettings(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"emergency": set})
}

func (h *Handler) ListCategoriesPublic(w http.ResponseWriter, r *http.Request) {
	cats, err := h.svc.ListCategories(r.Context(), true)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"healthcare_service_categories": cats})
}

func (h *Handler) ListServicesPublic(w http.ResponseWriter, r *http.Request) {
	f := ServiceFilter{ActiveOnly: true, ServiceType: r.URL.Query().Get("type")}
	if v := r.URL.Query().Get("category_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.CategoryID = &id
		}
	}
	items, _, err := h.svc.ListServices(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"healthcare_services": items})
}

func (h *Handler) GetServicePublic(w http.ResponseWriter, r *http.Request) {
	svc, err := h.svc.GetServiceBySlug(r.Context(), chi.URLParam(r, "slug"), true)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"healthcare_service": svc})
}

// ===================== client: requests =====================

func (h *Handler) CreateRequest(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var req CreateRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateCreateRequest(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	e, err := h.svc.Create(r.Context(), uid, req)
	if err != nil {
		writeError(w, err)
		return
	}
	h.publishCreated(e)
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"healthcare_request": e})
}

func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	limit, page := parsePage(r)
	items, total, err := h.svc.ListMyRequests(r.Context(), uid, limit, (page-1)*limit)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"healthcare_requests": items,
		"meta":                httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

func (h *Handler) GetMine(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	e, err := h.svc.GetMyRequest(r.Context(), uid, id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"healthcare_request": e})
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	e, err := h.svc.CancelMyRequest(r.Context(), uid, id)
	if err != nil {
		writeError(w, err)
		return
	}
	h.publishStatus(e)
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"healthcare_request": e})
}

// ===================== admin: practitioners =====================

func (h *Handler) ListPractitioners(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, page := parsePage(r)
	f := PractitionerFilter{
		Type:   q.Get("type"),
		Search: q.Get("q"),
		Limit:  limit,
		Offset: (page - 1) * limit,
	}
	items, total, err := h.svc.ListPractitioners(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"practitioners": items,
		"meta":          httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

func (h *Handler) GetPractitioner(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	p, err := h.svc.GetPractitioner(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"practitioner": p})
}

func (h *Handler) CreatePractitioner(w http.ResponseWriter, r *http.Request) {
	var req PractitionerRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validatePractitioner(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	pr, err := h.svc.CreatePractitioner(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"practitioner": pr})
}

func (h *Handler) UpdatePractitioner(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req PractitionerRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validatePractitioner(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	pr, err := h.svc.UpdatePractitioner(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"practitioner": pr})
}

func (h *Handler) DeletePractitioner(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeletePractitioner(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ===================== admin: service categories =====================

func (h *Handler) ListCategoriesAdmin(w http.ResponseWriter, r *http.Request) {
	cats, err := h.svc.ListCategories(r.Context(), false)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"healthcare_service_categories": cats})
}

func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req ServiceCategoryRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateCategory(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	c, err := h.svc.CreateCategory(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"healthcare_service_category": c})
}

func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req ServiceCategoryRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateCategory(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	c, err := h.svc.UpdateCategory(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"healthcare_service_category": c})
}

func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteCategory(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ===================== admin: services =====================

func (h *Handler) ListServicesAdmin(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, page := parsePage(r)
	f := ServiceFilter{
		ServiceType: q.Get("type"),
		Search:      q.Get("q"),
		Limit:       limit,
		Offset:      (page - 1) * limit,
	}
	if v := q.Get("category_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.CategoryID = &id
		}
	}
	items, total, err := h.svc.ListServices(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"healthcare_services": items,
		"meta":                httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

func (h *Handler) GetServiceAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	svc, err := h.svc.GetServiceByID(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"healthcare_service": svc})
}

func (h *Handler) CreateService(w http.ResponseWriter, r *http.Request) {
	var req ServiceRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateService(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	svc, err := h.svc.CreateService(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"healthcare_service": svc})
}

func (h *Handler) UpdateService(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req ServiceRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateService(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	svc, err := h.svc.UpdateService(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"healthcare_service": svc})
}

func (h *Handler) DeleteService(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteService(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ===================== admin: requests =====================

func (h *Handler) ListRequestsAdmin(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, page := parsePage(r)
	f := RequestFilter{
		Status:        q.Get("status"),
		PaymentStatus: q.Get("payment_status"),
		RequestType:   q.Get("type"),
		Limit:         limit,
		Offset:        (page - 1) * limit,
	}
	items, total, err := h.svc.List(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"healthcare_requests": items,
		"meta":                httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

func (h *Handler) CreateRequestAdmin(w http.ResponseWriter, r *http.Request) {
	var req AdminCreateRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateCreateRequest(req.CreateRequest); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	e, err := h.svc.CreateAdmin(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	h.publishCreated(e)
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"healthcare_request": e})
}

func (h *Handler) GetRequestAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	e, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"healthcare_request": e})
}

func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req UpdateStatusRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Status == "" {
		httpx.ValidationError(w, map[string]string{"status": "is required"})
		return
	}
	e, err := h.svc.UpdateStatus(r.Context(), id, req.Status)
	if err != nil {
		writeError(w, err)
		return
	}
	h.publishStatus(e)
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"healthcare_request": e})
}

func (h *Handler) SetQuote(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req QuoteRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateQuote(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	e, err := h.svc.SetQuote(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"healthcare_request": e})
}

func (h *Handler) Assign(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req AssignRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateAssign(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	e, warnings, err := h.svc.Assign(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"healthcare_request": e, "warnings": warnings})
}

func (h *Handler) Unassign(w http.ResponseWriter, r *http.Request) {
	assignmentID, err := strconv.ParseInt(chi.URLParam(r, "assignmentId"), 10, 64)
	if err != nil || assignmentID <= 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid assignment id")
		return
	}
	if err := h.svc.Unassign(r.Context(), assignmentID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ===================== admin: settings =====================

func (h *Handler) GetSettingsAdmin(w http.ResponseWriter, r *http.Request) {
	set, err := h.svc.GetSettings(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"emergency": set})
}

func (h *Handler) UpdateSettingsAdmin(w http.ResponseWriter, r *http.Request) {
	var req SettingsRequest
	if !decode(w, r, &req) {
		return
	}
	set, err := h.svc.UpdateSettings(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"emergency": set})
}

// ===================== shared helpers =====================

func userID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return 0, false
	}
	return id, true
}

func idParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := httpx.DecodeJSON(w, r, dst); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
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
	case errors.Is(err, ErrPractitionerNotFound),
		errors.Is(err, ErrCategoryNotFound),
		errors.Is(err, ErrServiceNotFound),
		errors.Is(err, ErrRequestNotFound),
		errors.Is(err, ErrAssignmentNotFound),
		errors.Is(err, ErrCustomerMissing):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrConflict),
		errors.Is(err, ErrInUse),
		errors.Is(err, ErrInvalidTransition),
		errors.Is(err, ErrNotCancellable),
		errors.Is(err, ErrNotQuotable),
		errors.Is(err, ErrRequestClosed),
		errors.Is(err, ErrPhoneTaken),
		errors.Is(err, ErrLicenseTaken):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidCategory),
		errors.Is(err, ErrServiceUnavailable),
		errors.Is(err, ErrServiceTypeMismatch),
		errors.Is(err, ErrMissingWindow),
		errors.Is(err, ErrPractitionerInactive):
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
