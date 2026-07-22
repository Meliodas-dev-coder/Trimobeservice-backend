package authz

import (
	"context"
	"net/http"

	"github.com/trimo/backend/internal/auth"
	"github.com/trimo/backend/internal/httpx"
)

type accessContextKey string

const currentAccessKey accessContextKey = "authz.current_access"

// CurrentAccess is attached by RequirePermission after it resolves a user. It
// lets downstream handlers (notably the SSE stream) filter records without a
// second authorization query.
type CurrentAccess struct {
	IsSuper     bool
	Permissions PermissionSet
}

func FromContext(ctx context.Context) (CurrentAccess, bool) {
	access, ok := ctx.Value(currentAccessKey).(CurrentAccess)
	return access, ok
}

func WithCurrentAccess(ctx context.Context, access CurrentAccess) context.Context {
	return context.WithValue(ctx, currentAccessKey, access)
}

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
			ctx := WithCurrentAccess(r.Context(), CurrentAccess{IsSuper: isSuper, Permissions: perms})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireCapability is the submenu/action variant of RequirePermission.
func RequireCapability(loader PermissionLoader, capability string, manage bool) func(http.Handler) http.Handler {
	return RequirePermission(loader, RequiredCapabilityKeys(capability, manage)...)
}

// RequireSuperAdmin protects authorization administration. Unlike a broad
// permission, this bypass cannot be delegated through a department, position,
// legacy role, or temporary assignment.
func RequireSuperAdmin(loader PermissionLoader) func(http.Handler) http.Handler {
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
			if !isSuper {
				httpx.Error(w, http.StatusForbidden, "super-admin access required")
				return
			}
			ctx := WithCurrentAccess(r.Context(), CurrentAccess{IsSuper: true, Permissions: perms})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
