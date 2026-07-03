package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrRefreshNotFound = errors.New("refresh token not found")
)

// Repository is the data-access layer for users and refresh tokens.
type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(ctx context.Context, u *User) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users (role, email, password_hash, full_name, phone)
		 VALUES (?, ?, ?, ?, ?)`,
		u.Role, u.Email, u.PasswordHash, u.FullName, u.Phone,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) EmailExists(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists,
		`SELECT EXISTS(SELECT 1 FROM users WHERE email = ?)`, email)
	return exists, err
}

func (r *Repository) GetByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := r.db.GetContext(ctx, &u,
		`SELECT id, role, email, password_hash, full_name, phone,
		        email_verified_at, is_active, created_at, updated_at
		 FROM users WHERE email = ?`, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*User, error) {
	var u User
	err := r.db.GetContext(ctx, &u,
		`SELECT id, role, email, password_hash, full_name, phone,
		        email_verified_at, is_active, created_at, updated_at
		 FROM users WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// refreshRow is the subset of refresh_tokens we read back for validation.
type refreshRow struct {
	ID        int64        `db:"id"`
	UserID    int64        `db:"user_id"`
	ExpiresAt time.Time    `db:"expires_at"`
	RevokedAt sql.NullTime `db:"revoked_at"`
}

func (r *Repository) StoreRefreshToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time, userAgent, ip string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, user_agent, ip_address, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		userID, tokenHash, nullIfEmpty(userAgent), nullIfEmpty(ip), expiresAt,
	)
	return err
}

func (r *Repository) GetRefreshToken(ctx context.Context, tokenHash string) (*refreshRow, error) {
	var row refreshRow
	err := r.db.GetContext(ctx, &row,
		`SELECT id, user_id, expires_at, revoked_at
		 FROM refresh_tokens WHERE token_hash = ?`, tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRefreshNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked_at = NOW()
		 WHERE token_hash = ? AND revoked_at IS NULL`, tokenHash)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
