package orders

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

// ===================== client: cart =====================

func (h *Handler) GetCart(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	cart, err := h.svc.GetCart(r.Context(), uid)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"cart": cart})
}

func (h *Handler) AddCartItem(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var req AddCartItemRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateAddCartItem(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	cart, err := h.svc.AddCartItem(r.Context(), uid, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"cart": cart})
}

func (h *Handler) UpdateCartItem(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	itemID, ok := idParam(w, r)
	if !ok {
		return
	}
	var req UpdateCartItemRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateUpdateCartItem(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	cart, err := h.svc.UpdateCartItem(r.Context(), uid, itemID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"cart": cart})
}

func (h *Handler) DeleteCartItem(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	itemID, ok := idParam(w, r)
	if !ok {
		return
	}
	cart, err := h.svc.RemoveCartItem(r.Context(), uid, itemID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"cart": cart})
}

func (h *Handler) ClearCart(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	cart, err := h.svc.ClearCart(r.Context(), uid)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"cart": cart})
}

// ===================== client: orders =====================

func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var req CheckoutRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateCheckout(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	order, err := h.svc.Checkout(r.Context(), uid, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"order": order})
}

func (h *Handler) ListMyOrders(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	limit, page := parsePage(r)
	items, total, err := h.svc.ListMyOrders(r.Context(), uid, limit, (page-1)*limit)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"orders": items,
		"meta":   httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

func (h *Handler) GetMyOrder(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	order, err := h.svc.GetMyOrder(r.Context(), uid, id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"order": order})
}

func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	order, err := h.svc.CancelMyOrder(r.Context(), uid, id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"order": order})
}

// ===================== admin: orders =====================

func (h *Handler) ListOrdersAdmin(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, page := parsePage(r)
	f := OrderFilter{
		Status:          q.Get("status"),
		PaymentStatus:   q.Get("payment_status"),
		FulfillmentType: q.Get("fulfillment_type"),
		Limit:           limit,
		Offset:          (page - 1) * limit,
	}
	items, total, err := h.svc.ListOrders(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"orders": items,
		"meta":   httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

func (h *Handler) CreateOrderAdmin(w http.ResponseWriter, r *http.Request) {
	var req AdminCreateOrderRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateAdminCreateOrder(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	order, err := h.svc.AdminCreateOrder(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"order": order})
}

func (h *Handler) GetOrderAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	order, err := h.svc.GetOrder(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"order": order})
}

func (h *Handler) UpdateOrderStatus(w http.ResponseWriter, r *http.Request) {
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
	order, err := h.svc.UpdateStatus(r.Context(), id, req.Status)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"order": order})
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
	case errors.Is(err, ErrOrderNotFound),
		errors.Is(err, ErrCartItemNotFound),
		errors.Is(err, ErrVariantMissing):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrInsufficientStock),
		errors.Is(err, ErrVariantInactive),
		errors.Is(err, ErrNotCancellable),
		errors.Is(err, ErrInvalidTransition):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrCartEmpty), errors.Is(err, ErrInvalidFulfillment), errors.Is(err, ErrNoItems):
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
