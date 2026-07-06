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
	ErrAddressNotFound = errors.New("address not found")
)

// Repository is the data-access layer for users and refresh tokens.
type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) InTx(ctx context.Context, fn func(tx *sqlx.Tx) error) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
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

const addressCols = `id, user_id, label, recipient_name, phone, line1, line2, city, region, country, postal_code, is_default, created_at, updated_at`

func (r *Repository) ListAddresses(ctx context.Context, userID int64) ([]Address, error) {
	out := []Address{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+addressCols+`
		   FROM addresses
		  WHERE user_id = ?
		  ORDER BY is_default DESC, updated_at DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetAddress(ctx context.Context, q sqlx.QueryerContext, userID, id int64) (*Address, error) {
	var a Address
	err := sqlx.GetContext(ctx, q, &a,
		`SELECT `+addressCols+` FROM addresses WHERE id = ? AND user_id = ?`, id, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAddressNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repository) CountAddresses(ctx context.Context, q sqlx.QueryerContext, userID int64) (int, error) {
	var total int
	err := sqlx.GetContext(ctx, q, &total, `SELECT COUNT(*) FROM addresses WHERE user_id = ?`, userID)
	return total, err
}

func (r *Repository) ClearDefaultAddresses(ctx context.Context, tx *sqlx.Tx, userID int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE addresses SET is_default = FALSE WHERE user_id = ?`, userID)
	return err
}

func (r *Repository) CreateAddressTx(ctx context.Context, tx *sqlx.Tx, a *Address) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO addresses (user_id, label, recipient_name, phone, line1, line2, city, region, country, postal_code, is_default)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.UserID, a.Label, a.RecipientName, a.Phone, a.Line1, a.Line2, a.City, a.Region, a.Country, a.PostalCode, a.IsDefault)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateAddressTx(ctx context.Context, tx *sqlx.Tx, a *Address) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE addresses
		    SET label = ?, recipient_name = ?, phone = ?, line1 = ?, line2 = ?, city = ?, region = ?, country = ?, postal_code = ?, is_default = ?
		  WHERE id = ? AND user_id = ?`,
		a.Label, a.RecipientName, a.Phone, a.Line1, a.Line2, a.City, a.Region, a.Country, a.PostalCode, a.IsDefault, a.ID, a.UserID)
	return err
}

func (r *Repository) DeleteAddress(ctx context.Context, userID, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM addresses WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrAddressNotFound
	}
	return nil
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
