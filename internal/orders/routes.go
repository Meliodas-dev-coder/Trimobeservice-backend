package orders

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts orders routes. Cart + customer order routes require a
// logged-in user; order management requires an admin.
func RegisterRoutes(r chi.Router, h *Handler, requireAuth, requireAdmin func(http.Handler) http.Handler) {
	// client (authenticated customer)
	r.Group(func(r chi.Router) {
		r.Use(requireAuth)

		r.Get("/cart", h.GetCart)
		r.Post("/cart/items", h.AddCartItem)
		r.Patch("/cart/items/{id}", h.UpdateCartItem)
		r.Delete("/cart/items/{id}", h.DeleteCartItem)
		r.Delete("/cart", h.ClearCart)

		r.Post("/orders", h.Checkout)
		r.Get("/orders", h.ListMyOrders)
		r.Get("/orders/{id}", h.GetMyOrder)
		r.Post("/orders/{id}/cancel", h.CancelOrder)
	})

	// admin
	r.Group(func(r chi.Router) {
		r.Use(requireAdmin)

		r.Get("/admin/orders", h.ListOrdersAdmin)
		r.Get("/admin/orders/{id}", h.GetOrderAdmin)
		r.Patch("/admin/orders/{id}/status", h.UpdateOrderStatus)
		// Payment confirmation is handled by the payments module
		// (POST /admin/payments), which records an audited ledger entry.
	})
}
