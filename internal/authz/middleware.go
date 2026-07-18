package authz

import (
	"context"
	"net/http"

	"github.com/trimo/backend/internal/auth"
	"github.com/trimo/backend/internal/httpx"
)

// PermissionLoader resolves an authenticated user's effective access. It is
// implemented by the adminusers service (a single indexed query per request;
// admin traffic is low and this keeps role changes effective immediately).
type PermissionLoader interface {
	// Load returns whether the user bypasses all checks (super-admin) and, if
	// not, the set of section keys granted by their assigned role.
	Load(ctx context.Context, userID int64) (isSuper bool, perms PermissionSet, err error)
}

// RequirePermission gates a route: the request passes when the authenticated
// user is a super-admin OR their role grants at least one of keys. It must be
// composed AFTER auth.RequireAuth/RequireAdmin, which put the user id in the
// request context.
func RequirePermission(loader PermissionLoader, keys ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := auth.UserIDFromContext(r.Context())
			if !ok {
				httpx.Error(w, http.StatusUnauthorized, "authentication required")
				return
			}
			isSuper, perms, err := loader.Load(r.Context(), userID)
			if err != nil {
				httpx.Error(w, http.StatusInternalServerError, "internal server error")
				return
			}
			if !isSuper && !perms.HasAny(keys...) {
				httpx.Error(w, http.StatusForbidden, "you do not have access to this section")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
