package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/trimo/backend/internal/hraccess"
)

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInactiveUser       = errors.New("account is inactive")
	ErrInvalidRefresh     = errors.New("invalid or expired refresh token")
	ErrWrongPassword      = errors.New("current password is incorrect")
	ErrSamePassword       = errors.New("new password must be different from the current one")
)

// sessionMeta captures per-request context stored alongside a refresh token.
type sessionMeta struct {
	UserAgent string
	IP        string
}

// Service holds the auth business logic.
type Service struct {
	repo   *Repository
	tokens *TokenManager
	access *hraccess.Resolver
}

func NewService(repo *Repository, tokens *TokenManager) *Service {
	return &Service{repo: repo, tokens: tokens, access: hraccess.NewResolver(repo.db)}
}

func (s *Service) Register(ctx context.Context, req RegisterRequest, meta sessionMeta) (*User, *TokenPair, error) {
	email := normalizeEmail(req.Email)

	exists, err := s.repo.EmailExists(ctx, email)
	if err != nil {
		return nil, nil, err
	}
	if exists {
		return nil, nil, ErrEmailTaken
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		return nil, nil, err
	}

	u := &User{
		Role:         RoleCustomer, // self-registration is always a customer
		Email:        email,
		PasswordHash: hash,
		FullName:     strings.TrimSpace(req.FullName),
	}
	if phone := strings.TrimSpace(req.Phone); phone != "" {
		u.Phone = &phone
	}

	id, err := s.repo.CreateUser(ctx, u)
	if err != nil {
		return nil, nil, err
	}
	u.ID = id
	u.IsActive = true

	pair, err := s.issueTokens(ctx, u, meta)
	if err != nil {
		return nil, nil, err
	}
	return u, pair, nil
}

func (s *Service) Login(ctx context.Context, req LoginRequest, meta sessionMeta) (*User, *TokenPair, error) {
	u, err := s.repo.GetByEmail(ctx, normalizeEmail(req.Email))
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Perform a dummy bcrypt comparison so that a missing account costs
			// roughly the same time as a wrong password — this prevents an
			// attacker from enumerating registered emails via response timing.
			checkPassword(dummyHash, req.Password)
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, err
	}
	if !checkPassword(u.PasswordHash, req.Password) {
		return nil, nil, ErrInvalidCredentials
	}
	if !u.IsActive {
		return nil, nil, ErrInactiveUser
	}

	pair, err := s.issueTokens(ctx, u, meta)
	if err != nil {
		return nil, nil, err
	}
	return u, pair, nil
}

// Refresh rotates a refresh token: the presented token is revoked and a fresh
// pair is issued.
func (s *Service) Refresh(ctx context.Context, req RefreshRequest, meta sessionMeta) (*TokenPair, error) {
	if req.RefreshToken == "" {
		return nil, ErrInvalidRefresh
	}
	hash := hashToken(req.RefreshToken)

	row, err := s.repo.GetRefreshToken(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrRefreshNotFound) {
			return nil, ErrInvalidRefresh
		}
		return nil, err
	}
	if row.RevokedAt.Valid || time.Now().After(row.ExpiresAt) {
		return nil, ErrInvalidRefresh
	}

	u, err := s.repo.GetByID(ctx, row.UserID)
	if err != nil {
		return nil, err
	}
	if !u.IsActive {
		return nil, ErrInactiveUser
	}

	if err := s.repo.RevokeRefreshToken(ctx, hash); err != nil {
		return nil, err
	}
	return s.issueTokens(ctx, u, meta)
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	return s.repo.RevokeRefreshToken(ctx, hashToken(refreshToken))
}

func (s *Service) Me(ctx context.Context, userID int64) (*User, error) {
	return s.repo.GetByID(ctx, userID)
}

// ChangePassword verifies the caller's current password before setting a new
// one. It does not touch existing sessions — the current access/refresh tokens
// stay valid, so the user is not logged out of their own session.
func (s *Service) ChangePassword(ctx context.Context, userID int64, req ChangePasswordRequest) error {
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if !checkPassword(u.PasswordHash, req.CurrentPassword) {
		return ErrWrongPassword
	}
	if checkPassword(u.PasswordHash, req.NewPassword) {
		return ErrSamePassword
	}
	hash, err := hashPassword(req.NewPassword)
	if err != nil {
		return err
	}
	return s.repo.UpdatePassword(ctx, userID, hash)
}

// UserResponse builds the API user object, enriching it with the effective
// admin section permissions from the assigned role (empty for customers and
// unassigned admins). Super-admins carry is_super_admin=true and bypass checks.
func (s *Service) UserResponse(ctx context.Context, u *User) (UserResponse, error) {
	resp := toUserResponse(u)
	perms, err := s.repo.LoadPermissions(ctx, u.ID)
	if err != nil {
		return resp, err
	}
	seen := make(map[string]bool, len(perms)+16)
	for _, permission := range perms {
		if !seen[permission] {
			seen[permission] = true
			resp.Permissions = append(resp.Permissions, permission)
		}
	}
	resolved, err := s.access.Resolve(ctx, u.ID)
	if err != nil {
		return resp, err
	}
	if resolved.Employee != nil {
		resp.Employee = resolved.Employee
		resp.AccessContext = &AccessContextResponse{
			BusinessCapabilities: resolved.BusinessCapabilities,
			HRPolicies:           resolved.HRPolicies,
			IsManager:            resolved.IsManager,
			IsDepartmentHead:     resolved.IsDepartmentHead,
		}
		if !seen["hr_employee"] && resolved.IsActive && resolved.Employee.EmploymentStatus != "offboarded" && resolved.Employee.EmploymentStatus != "suspended" {
			seen["hr_employee"] = true
			resp.Permissions = append(resp.Permissions, "hr_employee")
		}
		for _, grant := range resolved.BusinessCapabilities {
			if !seen[grant.Key] {
				seen[grant.Key] = true
				resp.Permissions = append(resp.Permissions, grant.Key)
			}
			if grant.AccessLevel == hraccess.LevelManage && !seen[grant.Key+".manage"] {
				seen[grant.Key+".manage"] = true
				resp.Permissions = append(resp.Permissions, grant.Key+".manage")
			}
		}
	}
	return resp, nil
}

func (s *Service) ListAddresses(ctx context.Context, userID int64) ([]Address, error) {
	return s.repo.ListAddresses(ctx, userID)
}

func (s *Service) CreateAddress(ctx context.Context, userID int64, req AddressRequest) (*Address, error) {
	a := addressFromRequest(userID, 0, req)
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		total, err := s.repo.CountAddresses(ctx, tx, userID)
		if err != nil {
			return err
		}
		if total == 0 || a.IsDefault {
			a.IsDefault = true
			if err := s.repo.ClearDefaultAddresses(ctx, tx, userID); err != nil {
				return err
			}
		}
		id, err := s.repo.CreateAddressTx(ctx, tx, a)
		if err != nil {
			return err
		}
		a.ID = id
		return nil
	})
	if err != nil {
		return nil, err
	}
	address, err := s.repo.GetAddress(ctx, s.repo.db, userID, a.ID)
	if err != nil {
		return nil, err
	}
	return address, nil
}

func (s *Service) UpdateAddress(ctx context.Context, userID, id int64, req AddressRequest) (*Address, error) {
	a := addressFromRequest(userID, id, req)
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		existing, err := s.repo.GetAddress(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		if req.IsDefault == nil {
			a.IsDefault = existing.IsDefault
		}
		if a.IsDefault {
			if err := s.repo.ClearDefaultAddresses(ctx, tx, userID); err != nil {
				return err
			}
		}
		return s.repo.UpdateAddressTx(ctx, tx, a)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetAddress(ctx, s.repo.db, userID, id)
}

func (s *Service) DeleteAddress(ctx context.Context, userID, id int64) error {
	return s.repo.DeleteAddress(ctx, userID, id)
}

func (s *Service) issueTokens(ctx context.Context, u *User, meta sessionMeta) (*TokenPair, error) {
	access, accessExp, err := s.tokens.GenerateAccess(u.ID, u.Role)
	if err != nil {
		return nil, err
	}
	refresh, refreshHash, err := newRefreshToken()
	if err != nil {
		return nil, err
	}
	refreshExp := time.Now().Add(s.tokens.refreshTTL)
	if err := s.repo.StoreRefreshToken(ctx, u.ID, refreshHash, refreshExp, meta.UserAgent, meta.IP); err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(time.Until(accessExp).Seconds()),
	}, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func addressFromRequest(userID, id int64, req AddressRequest) *Address {
	defaultCountry := strings.TrimSpace(req.Country)
	if defaultCountry == "" {
		defaultCountry = "Madagascar"
	}
	return &Address{
		ID:            id,
		UserID:        userID,
		Label:         cleanOptional(req.Label),
		RecipientName: strings.TrimSpace(req.RecipientName),
		Phone:         strings.TrimSpace(req.Phone),
		Line1:         strings.TrimSpace(req.Line1),
		Line2:         cleanOptional(req.Line2),
		City:          strings.TrimSpace(req.City),
		Region:        cleanOptional(req.Region),
		Country:       defaultCountry,
		PostalCode:    cleanOptional(req.PostalCode),
		IsDefault:     req.IsDefault != nil && *req.IsDefault,
	}
}

func cleanOptional(value *string) *string {
	if value == nil {
		return nil
	}
	cleaned := strings.TrimSpace(*value)
	if cleaned == "" {
		return nil
	}
	return &cleaned
}
