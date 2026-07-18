package adminusers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts the admin user & role management endpoints under /admin.
// adminOnly is expected to be RequireAdmin + RequirePermission("user_management")
// (+ audit recording) so only permitted admins reach these writes.
func RegisterRoutes(r chi.Router, h *Handler, adminOnly func(http.Handler) http.Handler) {
	r.Group(func(r chi.Router) {
		r.Use(adminOnly)

		// The grantable-screen catalog that drives the role editor.
		r.Get("/admin/permissions", h.Permissions)

		// Roles.
		r.Get("/admin/admin-roles", h.ListRoles)
		r.Post("/admin/admin-roles", h.CreateRole)
		r.Get("/admin/admin-roles/{id}", h.GetRole)
		r.Put("/admin/admin-roles/{id}", h.UpdateRole)
		r.Delete("/admin/admin-roles/{id}", h.DeleteRole)

		// Admin employees.
		r.Get("/admin/users", h.ListUsers)
		r.Post("/admin/users", h.CreateUser)
		r.Get("/admin/users/{id}", h.GetUser)
		r.Put("/admin/users/{id}", h.UpdateUser)
	})
}
