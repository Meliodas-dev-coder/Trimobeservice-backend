package orders

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type AdminGuards struct {
	Read   func(http.Handler) http.Handler
	Manage func(http.Handler) http.Handler
}

// RegisterRoutes mounts orders routes. Cart + customer order routes require a
// logged-in user; order management requires an admin.
func RegisterRoutes(r chi.Router, h *Handler, requireAuth func(http.Handler) http.Handler, guards AdminGuards) {
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

	// Admin reads and writes are intentionally mounted separately so a
	// position with read-only order access cannot mutate order state.
	r.Group(func(r chi.Router) {
		r.Use(guards.Read)
		r.Get("/admin/orders", h.ListOrdersAdmin)
		r.Get("/admin/orders/{id}", h.GetOrderAdmin)
	})
	r.Group(func(r chi.Router) {
		r.Use(guards.Manage)
		r.Post("/admin/orders", h.CreateOrderAdmin)
		r.Patch("/admin/orders/{id}/status", h.UpdateOrderStatus)
		// Payment confirmation is handled by the payments module
		// (POST /admin/payments), which records an audited ledger entry.
	})
}
