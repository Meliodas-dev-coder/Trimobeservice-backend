package mobility

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts mobility routes onto the /api/v1 subrouter. Public reads
// are open; drivers and everything under /admin require adminOnly.
func RegisterRoutes(r chi.Router, h *Handler, adminOnly func(http.Handler) http.Handler) {
	// public (client)
	r.Get("/car-categories", h.ListCategoriesPublic)
	r.Get("/cars", h.ListCarsPublic)
	r.Get("/cars/{slug}", h.GetCarPublic)

	// admin
	r.Group(func(r chi.Router) {
		r.Use(adminOnly)

		r.Get("/admin/car-categories", h.ListCategoriesAdmin)
		r.Post("/admin/car-categories", h.CreateCategory)
		r.Put("/admin/car-categories/{id}", h.UpdateCategory)
		r.Delete("/admin/car-categories/{id}", h.DeleteCategory)

		r.Get("/admin/cars", h.ListCarsAdmin)
		r.Get("/admin/cars/{id}", h.GetCarAdmin)
		r.Post("/admin/cars", h.CreateCar)
		r.Put("/admin/cars/{id}", h.UpdateCar)
		r.Delete("/admin/cars/{id}", h.DeleteCar)

		r.Post("/admin/cars/{id}/images", h.CreateCarImage)
		r.Delete("/admin/car-images/{id}", h.DeleteCarImage)

		r.Get("/admin/drivers", h.ListDrivers)
		r.Get("/admin/drivers/{id}", h.GetDriver)
		r.Post("/admin/drivers", h.CreateDriver)
		r.Put("/admin/drivers/{id}", h.UpdateDriver)
		r.Delete("/admin/drivers/{id}", h.DeleteDriver)
	})
}
