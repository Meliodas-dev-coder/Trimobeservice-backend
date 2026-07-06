package auth

import "github.com/go-chi/chi/v5"

// RegisterRoutes mounts the auth endpoints under the given router (expected to
// be the /api/v1 subrouter), producing paths like /api/v1/auth/login.
func RegisterRoutes(r chi.Router, h *Handler, mw *Middleware) {
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", h.Register)
		r.Post("/login", h.Login)
		r.Post("/refresh", h.Refresh)
		r.Post("/logout", h.Logout)
		r.With(mw.RequireAuth).Get("/me", h.Me)
	})

	r.Group(func(r chi.Router) {
		r.Use(mw.RequireAuth)
		r.Get("/account/addresses", h.ListAddresses)
		r.Post("/account/addresses", h.CreateAddress)
		r.Put("/account/addresses/{id}", h.UpdateAddress)
		r.Delete("/account/addresses/{id}", h.DeleteAddress)
	})
}
