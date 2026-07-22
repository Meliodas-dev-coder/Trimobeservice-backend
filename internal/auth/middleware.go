package auth

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/trimo/backend/internal/httpx"
)

type ctxKey string

const (
	ctxUserID ctxKey = "auth.user_id"
	ctxRole   ctxKey = "auth.role"
)

// Middleware guards routes using access tokens.
type Middleware struct {
	tokens       *TokenManager
	statusLoader AccountStatusLoader
}

type AccountStatusLoader interface {
	IsActive(ctx context.Context, userID int64) (bool, error)
}

func NewMiddleware(tokens *TokenManager) *Middleware {
	return &Middleware{tokens: tokens}
}

func (m *Middleware) SetAccountStatusLoader(loader AccountStatusLoader) {
	m.statusLoader = loader
}

// RequireAuth rejects requests without a valid Bearer access token and stores
// the user id + role in the request context.
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearerToken(r)
		if !ok {
			httpx.Error(w, http.StatusUnauthorized, "missing or malformed authorization header")
			return
		}
		claims, err := m.tokens.ParseAccess(raw)
		if err != nil {
			httpx.Error(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		userID, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil {
			httpx.Error(w, http.StatusUnauthorized, "invalid token subject")
			return
		}
		if m.statusLoader != nil {
			active, err := m.statusLoader.IsActive(r.Context(), userID)
			if err != nil {
				httpx.Error(w, http.StatusInternalServerError, "could not verify account status")
				return
			}
			if !active {
				httpx.Error(w, http.StatusForbidden, "account is inactive")
				return
			}
		}
		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		ctx = context.WithValue(ctx, ctxRole, claims.Role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin is RequireAuth plus a role check.
func (m *Middleware) RequireAdmin(next http.Handler) http.Handler {
	return m.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RoleFromContext(r.Context()) != RoleAdmin {
			httpx.Error(w, http.StatusForbidden, "admin access required")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// UserIDFromContext returns the authenticated user id, if any.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(ctxUserID).(int64)
	return id, ok
}

// RoleFromContext returns the authenticated user's role (empty if none).
func RoleFromContext(ctx context.Context) Role {
	role, _ := ctx.Value(ctxRole).(Role)
	return role
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func metaFrom(r *http.Request) sessionMeta {
	return sessionMeta{UserAgent: r.UserAgent(), IP: r.RemoteAddr}
}
