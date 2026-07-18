package adminusers

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/trimo/backend/internal/auth"
	"github.com/trimo/backend/internal/authz"
)

var (
	ErrRoleNameTaken       = errors.New("a role with this name already exists")
	ErrEmailTaken          = errors.New("email already registered")
	ErrSystemRole          = errors.New("built-in roles cannot be modified")
	ErrSuperAdminProtected = errors.New("the super-admin account cannot be edited here")
)

// Service holds the admin users/roles business logic and doubles as the
// authz.PermissionLoader for the enforcement middleware.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// Load implements authz.PermissionLoader.
func (s *Service) Load(ctx context.Context, userID int64) (bool, authz.PermissionSet, error) {
	isSuper, perms, err := s.repo.LoadAuthz(ctx, userID)
	if err != nil {
		return false, nil, err
	}
	return isSuper, authz.NewPermissionSet(perms), nil
}

// --- roles ---

func (s *Service) ListRoles(ctx context.Context, f Filter) ([]Role, int, error) {
	return s.repo.ListRoles(ctx, f)
}

func (s *Service) GetRole(ctx context.Context, id int64) (*Role, error) {
	return s.repo.GetRole(ctx, id)
}

func (s *Service) CreateRole(ctx context.Context, req RoleRequest) (*Role, error) {
	name, perms, problems := normalizeRole(req)
	if len(problems) > 0 {
		return nil, validationError(problems)
	}
	taken, err := s.repo.RoleNameExists(ctx, name, 0)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, ErrRoleNameTaken
	}
	id, err := s.repo.CreateRole(ctx, name, cleanOptional(req.Description), perms)
	if err != nil {
		return nil, err
	}
	return s.repo.GetRole(ctx, id)
}

func (s *Service) UpdateRole(ctx context.Context, id int64, req RoleRequest) (*Role, error) {
	existing, err := s.repo.GetRole(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing.IsSystem {
		return nil, ErrSystemRole
	}
	name, perms, problems := normalizeRole(req)
	if len(problems) > 0 {
		return nil, validationError(problems)
	}
	taken, err := s.repo.RoleNameExists(ctx, name, id)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, ErrRoleNameTaken
	}
	if err := s.repo.UpdateRole(ctx, id, name, cleanOptional(req.Description), perms); err != nil {
		return nil, err
	}
	return s.repo.GetRole(ctx, id)
}

func (s *Service) DeleteRole(ctx context.Context, id int64) error {
	existing, err := s.repo.GetRole(ctx, id)
	if err != nil {
		return err
	}
	if existing.IsSystem {
		return ErrSystemRole
	}
	return s.repo.DeleteRole(ctx, id)
}

// --- admin users ---

func (s *Service) ListUsers(ctx context.Context, f Filter) ([]AdminUser, int, error) {
	return s.repo.ListUsers(ctx, f)
}

func (s *Service) GetUser(ctx context.Context, id int64) (*AdminUser, error) {
	return s.repo.GetUser(ctx, id)
}

func (s *Service) CreateUser(ctx context.Context, req CreateUserRequest) (*AdminUser, error) {
	problems := map[string]string{}
	email := normalizeEmail(req.Email)
	if _, err := mail.ParseAddress(email); err != nil {
		problems["email"] = "must be a valid email address"
	}
	if utf8.RuneCountInString(req.Password) < 8 {
		problems["password"] = "must be at least 8 characters"
	}
	if len(req.Password) > 72 {
		problems["password"] = "must be at most 72 bytes"
	}
	if strings.TrimSpace(req.FullName) == "" {
		problems["full_name"] = "is required"
	}
	if len(problems) > 0 {
		return nil, validationError(problems)
	}

	if err := s.ensureRoleExists(ctx, req.AdminRoleID); err != nil {
		return nil, err
	}
	taken, err := s.repo.EmailExists(ctx, email)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, ErrEmailTaken
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	isActive := req.IsActive == nil || *req.IsActive
	id, err := s.repo.CreateUser(ctx, strings.TrimSpace(req.FullName), email, hash, cleanOptional(req.Phone), req.AdminRoleID, isActive)
	if err != nil {
		return nil, err
	}
	return s.repo.GetUser(ctx, id)
}

func (s *Service) UpdateUser(ctx context.Context, id int64, req UpdateUserRequest) (*AdminUser, error) {
	existing, err := s.repo.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}
	// Super-admins are managed via cmd/seed; block edits here so they can never
	// be demoted, deactivated, or locked out through the console.
	if existing.IsSuperAdmin {
		return nil, ErrSuperAdminProtected
	}
	if strings.TrimSpace(req.FullName) == "" {
		return nil, validationError(map[string]string{"full_name": "is required"})
	}
	if err := s.ensureRoleExists(ctx, req.AdminRoleID); err != nil {
		return nil, err
	}
	isActive := existing.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	if err := s.repo.UpdateUser(ctx, id, strings.TrimSpace(req.FullName), cleanOptional(req.Phone), req.AdminRoleID, isActive); err != nil {
		return nil, err
	}
	return s.repo.GetUser(ctx, id)
}

// ensureRoleExists validates an optional role assignment against the roles table.
func (s *Service) ensureRoleExists(ctx context.Context, roleID *int64) error {
	if roleID == nil {
		return nil
	}
	if _, err := s.repo.GetRole(ctx, *roleID); err != nil {
		return err // ErrRoleNotFound → 404 at the handler
	}
	return nil
}

// --- helpers ---

// normalizeRole trims the name and validates the permission keys against the
// authz catalog. Unknown keys are reported rather than silently dropped.
func normalizeRole(req RoleRequest) (name string, perms StringSlice, problems map[string]string) {
	problems = map[string]string{}
	name = strings.TrimSpace(req.Name)
	if name == "" {
		problems["name"] = "is required"
	}
	seen := map[string]struct{}{}
	perms = StringSlice{}
	for _, key := range req.Permissions {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if !authz.IsValidKey(key) {
			problems["permissions"] = "contains an unknown screen: " + key
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		perms = append(perms, key)
	}
	return name, perms, problems
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func cleanOptional(value string) *string {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return nil
	}
	return &cleaned
}

// validationError wraps field problems so the handler can surface a 422.
type ValidationProblems struct {
	Fields map[string]string
}

func (e *ValidationProblems) Error() string { return "validation failed" }

func validationError(problems map[string]string) error {
	return &ValidationProblems{Fields: problems}
}
