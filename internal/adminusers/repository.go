package adminusers

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jmoiron/sqlx"
)

var (
	ErrRoleNotFound = errors.New("role not found")
	ErrUserNotFound = errors.New("admin user not found")
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// --- roles ---

const roleCols = `r.id, r.name, r.description, r.permissions, r.is_system, r.created_at, r.updated_at,
	(SELECT COUNT(*) FROM users u WHERE u.admin_role_id = r.id) AS user_count`

func (r *Repository) ListRoles(ctx context.Context, f Filter) ([]Role, int, error) {
	where := []string{"1 = 1"}
	var args []any
	if f.Search != "" {
		where = append(where, "(r.name LIKE ? OR r.description LIKE ?)")
		like := "%" + f.Search + "%"
		args = append(args, like, like)
	}
	clause := " WHERE " + strings.Join(where, " AND ")

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM admin_roles r`+clause, args...); err != nil {
		return nil, 0, err
	}

	listArgs := append(append([]any{}, args...), f.Limit, f.Offset)
	out := []Role{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+roleCols+` FROM admin_roles r`+clause+` ORDER BY r.is_system DESC, r.name ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) GetRole(ctx context.Context, id int64) (*Role, error) {
	var role Role
	err := r.db.GetContext(ctx, &role, `SELECT `+roleCols+` FROM admin_roles r WHERE r.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// RoleNameExists reports whether another role already uses name (case-insensitive
// via the column collation). excludeID skips the row being updated.
func (r *Repository) RoleNameExists(ctx context.Context, name string, excludeID int64) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists,
		`SELECT EXISTS(SELECT 1 FROM admin_roles WHERE name = ? AND id <> ?)`, name, excludeID)
	return exists, err
}

func (r *Repository) CreateRole(ctx context.Context, name string, description *string, perms StringSlice) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO admin_roles (name, description, permissions) VALUES (?, ?, ?)`,
		name, description, perms)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateRole(ctx context.Context, id int64, name string, description *string, perms StringSlice) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE admin_roles SET name = ?, description = ?, permissions = ? WHERE id = ?`,
		name, description, perms, id)
	if err != nil {
		return err
	}
	return affectedOne(res, ErrRoleNotFound)
}

func (r *Repository) DeleteRole(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM admin_roles WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return affectedOne(res, ErrRoleNotFound)
}

// --- admin users ---

const userCols = `u.id, u.full_name, u.email, u.phone, u.is_active, u.is_super_admin,
	u.admin_role_id, r.name AS role_name, u.created_at`

func (r *Repository) ListUsers(ctx context.Context, f Filter) ([]AdminUser, int, error) {
	where := []string{"u.role = 'admin'"}
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
	out := []AdminUser{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+userCols+` FROM users u LEFT JOIN admin_roles r ON r.id = u.admin_role_id`+
			clause+` ORDER BY u.is_super_admin DESC, u.created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) GetUser(ctx context.Context, id int64) (*AdminUser, error) {
	var u AdminUser
	err := r.db.GetContext(ctx, &u,
		`SELECT `+userCols+` FROM users u LEFT JOIN admin_roles r ON r.id = u.admin_role_id
		 WHERE u.id = ? AND u.role = 'admin'`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repository) EmailExists(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM users WHERE email = ?)`, email)
	return exists, err
}

func (r *Repository) CreateUser(ctx context.Context, fullName, email, passwordHash string, phone *string, roleID *int64, isActive bool) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users (role, is_super_admin, admin_role_id, email, password_hash, full_name, phone, is_active)
		 VALUES ('admin', FALSE, ?, ?, ?, ?, ?, ?)`,
		roleID, email, passwordHash, fullName, phone, isActive)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateUser(ctx context.Context, id int64, fullName string, phone *string, roleID *int64, isActive bool) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET full_name = ?, phone = ?, admin_role_id = ?, is_active = ?
		 WHERE id = ? AND role = 'admin'`,
		fullName, phone, roleID, isActive, id)
	if err != nil {
		return err
	}
	return affectedOne(res, ErrUserNotFound)
}

// LoadAuthz returns the user's super-admin flag and the JSON permissions of
// their assigned role (an empty array when they have no role). Missing users
// resolve to no access rather than an error, so the middleware simply denies.
func (r *Repository) LoadAuthz(ctx context.Context, userID int64) (bool, StringSlice, error) {
	var row struct {
		IsSuper bool        `db:"is_super_admin"`
		Perms   StringSlice `db:"permissions"`
	}
	err := r.db.GetContext(ctx, &row,
		`SELECT u.is_super_admin, COALESCE(r.permissions, JSON_ARRAY()) AS permissions
		   FROM users u
		   LEFT JOIN admin_roles r ON r.id = u.admin_role_id
		  WHERE u.id = ?`, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, StringSlice{}, nil
	}
	if err != nil {
		return false, nil, err
	}
	return row.IsSuper, row.Perms, nil
}

func affectedOne(res sql.Result, notFound error) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return notFound
	}
	return nil
}
