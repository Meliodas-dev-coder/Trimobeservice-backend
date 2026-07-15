package mobility

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/trimo/backend/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ===================== public (client) reads =====================

func (h *Handler) ListCategoriesPublic(w http.ResponseWriter, r *http.Request) {
	cats, err := h.svc.ListCarCategories(r.Context(), true)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"car_categories": cats})
}

func (h *Handler) ListCarsPublic(w http.ResponseWriter, r *http.Request) {
	h.listCars(w, r, "", true, true)
}

func (h *Handler) GetCarPublic(w http.ResponseWriter, r *http.Request) {
	detail, err := h.svc.GetCarBySlug(r.Context(), chi.URLParam(r, "slug"), true)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"car": detail})
}

// ===================== admin: car categories =====================

func (h *Handler) ListCategoriesAdmin(w http.ResponseWriter, r *http.Request) {
	cats, err := h.svc.ListCarCategories(r.Context(), false)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"car_categories": cats})
}

func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req CarCategoryRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateCarCategory(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	c, err := h.svc.CreateCarCategory(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"car_category": c})
}

func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req CarCategoryRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateCarCategory(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	c, err := h.svc.UpdateCarCategory(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"car_category": c})
}

func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteCarCategory(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ===================== admin: cars =====================

func (h *Handler) ListCarsAdmin(w http.ResponseWriter, r *http.Request) {
	h.listCars(w, r, r.URL.Query().Get("status"), false, false)
}

func (h *Handler) GetCarAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	detail, err := h.svc.GetCarByID(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"car": detail})
}

func (h *Handler) GetCarOverviewAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	overview, err := h.svc.GetCarOverview(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"car":      overview.Car,
		"stats":    overview.Stats,
		"bookings": overview.Bookings,
	})
}

func (h *Handler) CreateCar(w http.ResponseWriter, r *http.Request) {
	var req CarRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateCar(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	c, err := h.svc.CreateCar(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"car": c})
}

func (h *Handler) UpdateCar(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req CarRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateCar(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	c, err := h.svc.UpdateCar(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"car": c})
}

func (h *Handler) DeleteCar(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteCar(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ===================== admin: car images =====================

func (h *Handler) CreateCarImage(w http.ResponseWriter, r *http.Request) {
	carID, ok := idParam(w, r)
	if !ok {
		return
	}
	var req CarImageRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateCarImage(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	im, err := h.svc.CreateCarImage(r.Context(), carID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"image": im})
}

func (h *Handler) DeleteCarImage(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteCarImage(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ===================== admin: drivers =====================

func (h *Handler) ListDrivers(w http.ResponseWriter, r *http.Request) {
	drivers, err := h.svc.ListDrivers(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"drivers": drivers})
}

func (h *Handler) GetDriver(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	d, err := h.svc.GetDriver(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"driver": d})
}

func (h *Handler) CreateDriver(w http.ResponseWriter, r *http.Request) {
	var req DriverRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateDriver(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	d, err := h.svc.CreateDriver(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"driver": d})
}

func (h *Handler) UpdateDriver(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req DriverRequest
	if !decode(w, r, &req) {
		return
	}
	if p := validateDriver(req); len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	d, err := h.svc.UpdateDriver(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"driver": d})
}

func (h *Handler) DeleteDriver(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteDriver(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ===================== shared helpers =====================

func (h *Handler) listCars(w http.ResponseWriter, r *http.Request, status string, includeBooked, excludeInactive bool) {
	q := r.URL.Query()
	limit, page := parsePage(r)
	f := CarFilter{
		Search:          q.Get("q"),
		Status:          status,
		IncludeBooked:   includeBooked,
		ExcludeInactive: excludeInactive,
		Limit:           limit,
		Offset:          (page - 1) * limit,
	}
	if startRaw, endRaw := q.Get("start"), q.Get("end"); startRaw != "" || endRaw != "" {
		if startRaw == "" || endRaw == "" {
			httpx.Error(w, http.StatusBadRequest, "start and end must be provided together")
			return
		}
		start, startErr := time.Parse(time.RFC3339, startRaw)
		end, endErr := time.Parse(time.RFC3339, endRaw)
		if startErr != nil || endErr != nil || !end.After(start) {
			httpx.Error(w, http.StatusBadRequest, "start and end must be valid RFC3339 timestamps")
			return
		}
		f.AvailableStart = &start
		f.AvailableEnd = &end
	}
	if v := q.Get("category_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.CategoryID = &id
		}
	}
	items, total, err := h.svc.ListCars(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"cars": items,
		"meta": httpx.Envelope{"total": total, "page": page, "limit": limit},
	})
}

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
	case errors.Is(err, ErrCarCategoryNotFound),
		errors.Is(err, ErrCarNotFound),
		errors.Is(err, ErrCarImageNotFound),
		errors.Is(err, ErrDriverNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrConflict),
		errors.Is(err, ErrInUse),
		errors.Is(err, ErrPlateTaken),
		errors.Is(err, ErrPhoneTaken),
		errors.Is(err, ErrLicenseTaken):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidCategory), errors.Is(err, ErrOutsideCategoryRateRequired), errors.Is(err, ErrOutsideRateRequired):
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
