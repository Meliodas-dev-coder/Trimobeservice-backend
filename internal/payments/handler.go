package payments

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

// SetPublisher wires the realtime hub so recorded/refunded payments stream to
// admins. Nil-safe.
func (h *Handler) SetPublisher(p realtime.Publisher) { h.pub = p }

func (h *Handler) publishPayment(evtType string, p *Payment) {
	if h.pub == nil || p == nil {
		return
	}
	h.pub.Publish(realtime.Event{
		Type: evtType,
		Payload: map[string]any{
			"id":           p.ID,
			"payable_type": p.PayableType,
			"payable_id":   p.PayableID,
			"amount":       p.Amount,
			"method":       p.Method,
			"status":       p.Status,
		},
	})
}

func (h *Handler) Record(w http.ResponseWriter, r *http.Request) {
	adminID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req RecordPaymentRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateRecord(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	payment, err := h.svc.Record(r.Context(), adminID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	h.publishPayment("payment.recorded", payment)
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"payment": payment})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, page := parsePage(r)
	f := PaymentFilter{
		PayableType: q.Get("payable_type"),
		Status:      q.Get("status"),
		Method:      q.Get("method"),
		Limit:       limit,
		Offset:      (page - 1) * limit,
	}
	if v := q.Get("payable_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.PayableID = &id
		}
	}
	items, total, err := h.svc.List(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"payments": items,
		"meta":     httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	payment, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"payment": payment})
}

func (h *Handler) Refund(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	payment, err := h.svc.Refund(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	h.publishPayment("payment.refunded", payment)
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"payment": payment})
}

// --- helpers ---

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
	case errors.Is(err, ErrTargetNotFound), errors.Is(err, ErrPaymentNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrAlreadyPaid),
		errors.Is(err, ErrTargetClosed),
		errors.Is(err, ErrOrderNotFulfilled),
		errors.Is(err, ErrEventNotQuoted),
		errors.Is(err, ErrHealthcareNotQuoted),
		errors.Is(err, ErrNotRefundable):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidPayable):
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
