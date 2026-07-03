package catalog

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts catalog routes onto the /api/v1 subrouter. Public reads
// are open; everything under /admin is wrapped with adminOnly.
func RegisterRoutes(r chi.Router, h *Handler, adminOnly func(http.Handler) http.Handler) {
	// public (client)
	r.Get("/categories", h.ListCategoriesPublic)
	r.Get("/brands", h.ListBrandsPublic)
	r.Get("/products", h.ListProductsPublic)
	r.Get("/products/{slug}", h.GetProductPublic)

	// admin
	r.Group(func(r chi.Router) {
		r.Use(adminOnly)

		r.Get("/admin/categories", h.ListCategoriesAdmin)
		r.Post("/admin/categories", h.CreateCategory)
		r.Put("/admin/categories/{id}", h.UpdateCategory)
		r.Delete("/admin/categories/{id}", h.DeleteCategory)

		r.Get("/admin/brands", h.ListBrandsAdmin)
		r.Post("/admin/brands", h.CreateBrand)
		r.Put("/admin/brands/{id}", h.UpdateBrand)
		r.Delete("/admin/brands/{id}", h.DeleteBrand)

		r.Get("/admin/products", h.ListProductsAdmin)
		r.Get("/admin/products/{id}", h.GetProductAdmin)
		r.Post("/admin/products", h.CreateProduct)
		r.Put("/admin/products/{id}", h.UpdateProduct)
		r.Delete("/admin/products/{id}", h.DeleteProduct)

		r.Post("/admin/products/{id}/variants", h.CreateVariant)
		r.Put("/admin/variants/{id}", h.UpdateVariant)
		r.Delete("/admin/variants/{id}", h.DeleteVariant)

		r.Post("/admin/products/{id}/images", h.CreateImage)
		r.Delete("/admin/images/{id}", h.DeleteImage)
	})
}
