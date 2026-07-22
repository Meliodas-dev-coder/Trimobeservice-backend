package mobility

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type AdminGuards struct {
	CategoriesRead, CategoriesManage func(http.Handler) http.Handler
	CarsRead, CarsManage             func(http.Handler) http.Handler
	DriversRead, DriversManage       func(http.Handler) http.Handler
}

// RegisterRoutes mounts mobility routes onto the /api/v1 subrouter. Admin reads
// and writes are independently guarded at submenu/action granularity.
func RegisterRoutes(r chi.Router, h *Handler, g AdminGuards) {
	// public (client)
	r.Get("/car-categories", h.ListCategoriesPublic)
	r.Get("/cars", h.ListCarsPublic)
	r.Get("/cars/{slug}", h.GetCarPublic)

	// admin
	r.With(g.CategoriesRead).Get("/admin/car-categories", h.ListCategoriesAdmin)
	r.With(g.CategoriesManage).Post("/admin/car-categories", h.CreateCategory)
	r.With(g.CategoriesManage).Put("/admin/car-categories/{id}", h.UpdateCategory)
	r.With(g.CategoriesManage).Delete("/admin/car-categories/{id}", h.DeleteCategory)

	r.With(g.CarsRead).Get("/admin/cars", h.ListCarsAdmin)
	r.With(g.CarsRead).Get("/admin/cars/{id}/overview", h.GetCarOverviewAdmin)
	r.With(g.CarsRead).Get("/admin/cars/{id}", h.GetCarAdmin)
	r.With(g.CarsManage).Post("/admin/cars", h.CreateCar)
	r.With(g.CarsManage).Put("/admin/cars/{id}", h.UpdateCar)
	r.With(g.CarsManage).Delete("/admin/cars/{id}", h.DeleteCar)
	r.With(g.CarsManage).Post("/admin/cars/{id}/images", h.CreateCarImage)
	r.With(g.CarsManage).Delete("/admin/car-images/{id}", h.DeleteCarImage)

	r.With(g.DriversRead).Get("/admin/drivers", h.ListDrivers)
	r.With(g.DriversRead).Get("/admin/drivers/{id}", h.GetDriver)
	r.With(g.DriversManage).Post("/admin/drivers", h.CreateDriver)
	r.With(g.DriversManage).Put("/admin/drivers/{id}", h.UpdateDriver)
	r.With(g.DriversManage).Delete("/admin/drivers/{id}", h.DeleteDriver)
}
