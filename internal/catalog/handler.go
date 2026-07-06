package catalog

import (
	"errors"
	"net/http"
	"strconv"

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
	cats, err := h.svc.ListCategories(r.Context(), true)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"categories": cats})
}

func (h *Handler) ListBrandsPublic(w http.ResponseWriter, r *http.Request) {
	brands, err := h.svc.ListBrands(r.Context(), true)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"brands": brands})
}

func (h *Handler) ListProductsPublic(w http.ResponseWriter, r *http.Request) {
	h.listProducts(w, r, true)
}

func (h *Handler) GetProductPublic(w http.ResponseWriter, r *http.Request) {
	detail, err := h.svc.GetProductBySlug(r.Context(), chi.URLParam(r, "slug"), true)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"product": detail})
}

// ===================== admin: product templates =====================

// ListTemplates returns the product-type registry the admin form renders from.
func (h *Handler) ListTemplates(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"templates": Templates()})
}

// ===================== admin: categories =====================

func (h *Handler) ListCategoriesAdmin(w http.ResponseWriter, r *http.Request) {
	cats, err := h.svc.ListCategories(r.Context(), false)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"categories": cats})
}

func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req CategoryRequest
	if !decode(w, r, &req) {
		return
	}
	if problems := validateCategory(req); len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}
	c, err := h.svc.CreateCategory(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"category": c})
}

func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req CategoryRequest
	if !decode(w, r, &req) {
		return
	}
	if problems := validateCategory(req); len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}
	c, err := h.svc.UpdateCategory(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"category": c})
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

// ===================== admin: brands =====================

func (h *Handler) ListBrandsAdmin(w http.ResponseWriter, r *http.Request) {
	brands, err := h.svc.ListBrands(r.Context(), false)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"brands": brands})
}

func (h *Handler) CreateBrand(w http.ResponseWriter, r *http.Request) {
	var req BrandRequest
	if !decode(w, r, &req) {
		return
	}
	if problems := validateBrand(req); len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}
	b, err := h.svc.CreateBrand(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"brand": b})
}

func (h *Handler) UpdateBrand(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req BrandRequest
	if !decode(w, r, &req) {
		return
	}
	if problems := validateBrand(req); len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}
	b, err := h.svc.UpdateBrand(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"brand": b})
}

func (h *Handler) DeleteBrand(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteBrand(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ===================== admin: products =====================

func (h *Handler) ListProductsAdmin(w http.ResponseWriter, r *http.Request) {
	h.listProducts(w, r, false)
}

func (h *Handler) GetProductAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	detail, err := h.svc.GetProductByID(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"product": detail})
}

func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var req ProductRequest
	if !decode(w, r, &req) {
		return
	}
	if problems := validateProduct(req); len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}
	p, err := h.svc.CreateProduct(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"product": p})
}

func (h *Handler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req ProductRequest
	if !decode(w, r, &req) {
		return
	}
	if problems := validateProduct(req); len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}
	p, err := h.svc.UpdateProduct(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"product": p})
}

func (h *Handler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteProduct(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ===================== admin: variants =====================

func (h *Handler) CreateVariant(w http.ResponseWriter, r *http.Request) {
	productID, ok := idParam(w, r)
	if !ok {
		return
	}
	var req VariantRequest
	if !decode(w, r, &req) {
		return
	}
	if problems := validateVariant(req); len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}
	v, err := h.svc.CreateVariant(r.Context(), productID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"variant": v})
}

func (h *Handler) UpdateVariant(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req VariantRequest
	if !decode(w, r, &req) {
		return
	}
	if problems := validateVariant(req); len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}
	v, err := h.svc.UpdateVariant(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"variant": v})
}

func (h *Handler) DeleteVariant(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteVariant(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ===================== admin: images =====================

func (h *Handler) CreateImage(w http.ResponseWriter, r *http.Request) {
	productID, ok := idParam(w, r)
	if !ok {
		return
	}
	var req ImageRequest
	if !decode(w, r, &req) {
		return
	}
	if problems := validateImage(req); len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}
	im, err := h.svc.CreateImage(r.Context(), productID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"image": im})
}

func (h *Handler) UpdateImage(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req ImageUpdateRequest
	if !decode(w, r, &req) {
		return
	}
	im, err := h.svc.UpdateImage(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"image": im})
}

func (h *Handler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteImage(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ===================== shared helpers =====================

func (h *Handler) listProducts(w http.ResponseWriter, r *http.Request, publicOnly bool) {
	q := r.URL.Query()
	limit, page := parsePage(r)
	f := ProductFilter{
		Search:     q.Get("q"),
		ActiveOnly: publicOnly,
		Limit:      limit,
		Offset:     (page - 1) * limit,
	}
	if v := q.Get("category_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.CategoryID = &id
		}
	}
	if v := q.Get("brand_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.BrandID = &id
		}
	}
	f.TemplateKey = q.Get("template_key")
	f.ExcludeTemplateKey = q.Get("exclude_template_key")

	items, total, err := h.svc.ListProducts(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{
		"products": items,
		"meta":     httpx.Envelope{"total": total, "page": page, "limit": limit},
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
	var pe *ProblemError
	if errors.As(err, &pe) {
		httpx.ValidationError(w, pe.Problems)
		return
	}
	switch {
	case errors.Is(err, ErrCategoryNotFound),
		errors.Is(err, ErrBrandNotFound),
		errors.Is(err, ErrProductNotFound),
		errors.Is(err, ErrVariantNotFound),
		errors.Is(err, ErrImageNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrSKUTaken), errors.Is(err, ErrConflict), errors.Is(err, ErrInUse):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidCategory), errors.Is(err, ErrInvalidBrand):
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
