package auth

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInactiveUser       = errors.New("account is inactive")
	ErrInvalidRefresh     = errors.New("invalid or expired refresh token")
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
}

func NewService(repo *Repository, tokens *TokenManager) *Service {
	return &Service{repo: repo, tokens: tokens}
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
