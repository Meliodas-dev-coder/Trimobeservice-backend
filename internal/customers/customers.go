// Package customers is a read-only, admin-only view over customer accounts.
// It exposes no mutations — the back office can list customers and open a
// single customer, nothing more.
package customers

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

var ErrCustomerNotFound = errors.New("customer not found")

// Customer is a customer account enriched with order/booking counts.
type Customer struct {
	ID              int64      `db:"id" json:"id"`
	FullName        string     `db:"full_name" json:"full_name"`
	Email           string     `db:"email" json:"email"`
	Phone           *string    `db:"phone" json:"phone,omitempty"`
	Role            string     `db:"role" json:"role"`
	IsActive        bool       `db:"is_active" json:"is_active"`
	EmailVerifiedAt *time.Time `db:"email_verified_at" json:"email_verified_at,omitempty"`
	OrderCount      int        `db:"order_count" json:"order_count"`
	BookingCount    int        `db:"booking_count" json:"booking_count"`
	CreatedAt       time.Time  `db:"created_at" json:"created_at"`
}

type Filter struct {
	Search string
	Limit  int
	Offset int
}

const cols = `u.id, u.full_name, u.email, u.phone, u.role, u.is_active, u.email_verified_at, u.created_at,
	(SELECT COUNT(*) FROM orders o WHERE o.user_id = u.id) AS order_count,
	(SELECT COUNT(*) FROM bookings b
	  WHERE b.user_id = u.id AND (b.booking_group_id IS NULL OR b.id = b.booking_group_id)) AS booking_count`

// Repository reads customer accounts.
type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List(ctx context.Context, f Filter) ([]Customer, int, error) {
	where := []string{"u.role = 'customer'"}
	var args []any
	if f.Search != "" {
		where = append(where, "(u.full_name LIKE ? OR u.email LIKE ? OR u.phone LIKE ?)")
		like := "%" + f.Search + "%"
		args = append(args, like, like, like)
	}
	clause := " WHERE " + strings.Join(where, " AND ")

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM users u`+clause, args...); err != nil {
		return nil, 0, err
	}

	listArgs := append(append([]any{}, args...), f.Limit, f.Offset)
	out := []Customer{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+cols+` FROM users u`+clause+` ORDER BY u.created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*Customer, error) {
	var c Customer
	err := r.db.GetContext(ctx, &c,
		`SELECT `+cols+` FROM users u WHERE u.id = ? AND u.role = 'customer'`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCustomerNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Service is a thin read-only pass-through over the repository.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, f Filter) ([]Customer, int, error) {
	return s.repo.List(ctx, f)
}

func (s *Service) Get(ctx context.Context, id int64) (*Customer, error) {
	return s.repo.GetByID(ctx, id)
}
