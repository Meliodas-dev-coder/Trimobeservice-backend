package bookings

import (
	"errors"
	"net/http"
	"strconv"
	"time"

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

// SetPublisher wires the realtime hub so new bookings stream to admins. Nil-safe.
func (h *Handler) SetPublisher(p realtime.Publisher) { h.pub = p }

func (h *Handler) BookingCarOptions(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.BookingCarOptions(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"cars": items})
}

func (h *Handler) BookingDriverOptions(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.BookingDriverOptions(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"drivers": items})
}

func (h *Handler) publishCreated(b *BookingDetail) { h.publish("booking.created", b) }
func (h *Handler) publishStatus(b *BookingDetail)  { h.publish("booking.status_changed", b) }

func (h *Handler) publish(evtType string, b *BookingDetail) {
	if h.pub == nil || b == nil {
		return
	}
	h.pub.Publish(realtime.Event{
		Type: evtType,
		Payload: map[string]any{
			"id":             b.ID,
			"number":         b.BookingNumber,
			"customer_name":  b.CustomerName,
			"car_name":       b.CarName,
			"amount":         b.TotalPrice,
			"status":         b.Status,
			"payment_status": b.PaymentStatus,
			"start_at":       b.StartAt,
		},
	})
}

// ===================== public: availability =====================

// CheckAvailability: GET /availability?car_id=&start=&end=  (RFC3339 times)
func (h *Handler) CheckAvailability(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	carID, err := strconv.ParseInt(q.Get("car_id"), 10, 64)
	if err != nil || carID <= 0 {
		httpx.Error(w, http.StatusBadRequest, "car_id is required")
		return
	}
	start, err := time.Parse(time.RFC3339, q.Get("start"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "start must be an RFC3339 timestamp")
		return
	}
	end, err := time.Parse(time.RFC3339, q.Get("end"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "end must be an RFC3339 timestamp")
		return
	}
	res, err := h.svc.CheckAvailability(r.Context(), carID, start, end)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"availability": res})
}

// BookedRanges: GET /availability/ranges?car_id=
func (h *Handler) BookedRanges(w http.ResponseWriter, r *http.Request) {
	carID, err := strconv.ParseInt(r.URL.Query().Get("car_id"), 10, 64)
	if err != nil || carID <= 0 {
		httpx.Error(w, http.StatusBadRequest, "car_id is required")
		return
	}
	ranges, err := h.svc.BookedRanges(r.Context(), carID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"car_id": carID, "booked_ranges": ranges})
}

// ===================== client: bookings =====================

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var req CreateBookingRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateCreateBooking(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	b, err := h.svc.Create(r.Context(), uid, req)
	if err != nil {
		writeError(w, err)
		return
	}
	h.publishCreated(b)
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"booking": b})
}

func (h *Handler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var req CreateBookingBatchRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateCreateBookingBatch(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	b, err := h.svc.CreateBatch(r.Context(), uid, req)
	if err != nil {
		writeError(w, err)
		return
	}
	h.publishCreated(b)
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"booking": b})
}

func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	limit, page := parsePage(r)
	items, total, err := h.svc.ListMyBookings(r.Context(), uid, limit, (page-1)*limit)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"bookings": items,
		"meta":     httpx.Envelope{"total": total, "page": page, "limit": limit},
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
	b, err := h.svc.GetMyBooking(r.Context(), uid, id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"booking": b})
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
	b, err := h.svc.CancelMyBooking(r.Context(), uid, id)
	if err != nil {
		writeError(w, err)
		return
	}
	h.publishStatus(b)
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"booking": b})
}

// ===================== admin: bookings =====================

func (h *Handler) CreateAdmin(w http.ResponseWriter, r *http.Request) {
	var req AdminCreateBookingRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateAdminCreateBooking(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	b, err := h.svc.CreateAdmin(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	h.publishCreated(b)
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"booking": b})
}

func (h *Handler) ListAdmin(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, page := parsePage(r)
	f := BookingFilter{
		Status:        q.Get("status"),
		PaymentStatus: q.Get("payment_status"),
		Limit:         limit,
		Offset:        (page - 1) * limit,
	}
	if v := q.Get("car_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.CarID = &id
		}
	}
	items, total, err := h.svc.List(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"bookings": items,
		"meta":     httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

func (h *Handler) GetAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	b, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"booking": b})
}

func (h *Handler) DeleteAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) AssignDriver(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req AssignDriverRequest
	if !decode(w, r, &req) {
		return
	}
	if req.DriverID <= 0 {
		httpx.ValidationError(w, map[string]string{"driver_id": "is required"})
		return
	}
	itemID := int64(0)
	if req.BookingItemID != nil {
		itemID = *req.BookingItemID
		if itemID <= 0 {
			httpx.ValidationError(w, map[string]string{"booking_item_id": "must be greater than zero"})
			return
		}
	}
	b, err := h.svc.AssignDriverForBooking(r.Context(), id, itemID, req.DriverID)
	if err != nil {
		writeError(w, err)
		return
	}
	h.publishStatus(b)
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"booking": b})
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
	b, err := h.svc.UpdateStatus(r.Context(), id, req.Status)
	if err != nil {
		writeError(w, err)
		return
	}
	h.publishStatus(b)
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"booking": b})
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
	case errors.Is(err, ErrBookingNotFound),
		errors.Is(err, ErrCarMissing),
		errors.Is(err, ErrDriverMissing),
		errors.Is(err, ErrCustomerMissing):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrCarNotFree),
		errors.Is(err, ErrCarUnavailable),
		errors.Is(err, ErrDriverBusy),
		errors.Is(err, ErrDriverInactive),
		errors.Is(err, ErrNotAssignable),
		errors.Is(err, ErrInvalidTransition),
		errors.Is(err, ErrNotCancellable),
		errors.Is(err, ErrBookingHasPayments):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidDates),
		errors.Is(err, ErrPastStart),
		errors.Is(err, ErrCarSelectionRequired),
		errors.Is(err, ErrDuplicateCar),
		errors.Is(err, ErrDistanceRequired),
		errors.Is(err, ErrInvalidDistance),
		errors.Is(err, ErrRegionChoiceRequired):
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
