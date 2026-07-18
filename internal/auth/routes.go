package auth

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts the auth endpoints under the given router (expected to
// be the /api/v1 subrouter), producing paths like /api/v1/auth/login. throttle
// is a strict per-IP rate-limit middleware applied to the credential endpoints
// to blunt brute-force/credential-stuffing; pass nil to skip it.
func RegisterRoutes(r chi.Router, h *Handler, mw *Middleware, throttle func(http.Handler) http.Handler) {
	r.Route("/auth", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			if throttle != nil {
				r.Use(throttle)
			}
			r.Post("/register", h.Register)
			r.Post("/login", h.Login)
			r.Post("/refresh", h.Refresh)
		})
		r.Post("/logout", h.Logout)
		r.With(mw.RequireAuth).Get("/me", h.Me)
	})

	r.Group(func(r chi.Router) {
		r.Use(mw.RequireAuth)
		r.Post("/account/password", h.ChangePassword)
		r.Get("/account/addresses", h.ListAddresses)
		r.Post("/account/addresses", h.CreateAddress)
		r.Put("/account/addresses/{id}", h.UpdateAddress)
		r.Delete("/account/addresses/{id}", h.DeleteAddress)
	})
}
