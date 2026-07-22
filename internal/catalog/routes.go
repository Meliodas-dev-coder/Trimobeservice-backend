package catalog

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// AdminGuards separates catalog visibility from mutation rights. The guards
// perform the broad route-level check and attach authz.CurrentAccess; handlers
// then verify the department of the requested row before reading or changing
// it. This second check is required because tech, fashion, and coffee share the
// same database tables and HTTP endpoints.
type AdminGuards struct {
	TemplatesRead    func(http.Handler) http.Handler
	CategoriesRead   func(http.Handler) http.Handler
	CategoriesManage func(http.Handler) http.Handler
	BrandsRead       func(http.Handler) http.Handler
	BrandsManage     func(http.Handler) http.Handler
	ProductsRead     func(http.Handler) http.Handler
	ProductsManage   func(http.Handler) http.Handler
}

// RegisterRoutes mounts catalog routes onto the /api/v1 subrouter. Public reads
// are open. Admin routes use resource/action-specific guards and repeat the
// authorization check against each resource's actual department in the handler.
func RegisterRoutes(r chi.Router, h *Handler, guards AdminGuards) {
	// public (client)
	r.Get("/categories", h.ListCategoriesPublic)
	r.Get("/brands", h.ListBrandsPublic)
	r.Get("/products", h.ListProductsPublic)
	r.Get("/products/{slug}", h.GetProductPublic)

	// admin
	r.With(guards.TemplatesRead).Get("/admin/product-templates", h.ListTemplates)

	r.With(guards.CategoriesRead).Get("/admin/categories", h.ListCategoriesAdmin)
	r.With(guards.CategoriesManage).Post("/admin/categories", h.CreateCategory)
	r.With(guards.CategoriesManage).Put("/admin/categories/{id}", h.UpdateCategory)
	r.With(guards.CategoriesManage).Delete("/admin/categories/{id}", h.DeleteCategory)

	r.With(guards.BrandsRead).Get("/admin/brands", h.ListBrandsAdmin)
	r.With(guards.BrandsManage).Post("/admin/brands", h.CreateBrand)
	r.With(guards.BrandsManage).Put("/admin/brands/{id}", h.UpdateBrand)
	r.With(guards.BrandsManage).Delete("/admin/brands/{id}", h.DeleteBrand)

	r.With(guards.ProductsRead).Get("/admin/products", h.ListProductsAdmin)
	r.With(guards.ProductsRead).Get("/admin/products/{id}", h.GetProductAdmin)
	r.With(guards.ProductsManage).Post("/admin/products", h.CreateProduct)
	r.With(guards.ProductsManage).Put("/admin/products/{id}", h.UpdateProduct)
	r.With(guards.ProductsManage).Delete("/admin/products/{id}", h.DeleteProduct)

	r.With(guards.ProductsManage).Post("/admin/products/{id}/variants", h.CreateVariant)
	r.With(guards.ProductsManage).Put("/admin/variants/{id}", h.UpdateVariant)
	r.With(guards.ProductsManage).Delete("/admin/variants/{id}", h.DeleteVariant)

	r.With(guards.ProductsManage).Post("/admin/products/{id}/images", h.CreateImage)
	r.With(guards.ProductsManage).Put("/admin/images/{id}", h.UpdateImage)
	r.With(guards.ProductsManage).Delete("/admin/images/{id}", h.DeleteImage)
}
