// Package adminusers manages admin employees and the reusable roles that grant
// them screen-level access. It also implements authz.PermissionLoader, resolving
// a signed-in user's effective permissions for the enforcement middleware.
//
// Roles bundle a set of section keys (see internal/authz.Catalog); each admin is
// assigned one role. The seeded owner account is is_super_admin and bypasses all
// checks. This module never mints super-admins — that stays a cmd/seed operation.
package adminusers

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// StringSlice marshals a []string to/from a JSON column (admin_roles.permissions).
type StringSlice []string

// Scan implements sql.Scanner for a JSON text/blob column.
func (s *StringSlice) Scan(src any) error {
	if src == nil {
		*s = StringSlice{}
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("adminusers: cannot scan %T into StringSlice", src)
	}
	if len(b) == 0 {
		*s = StringSlice{}
		return nil
	}
	return json.Unmarshal(b, (*[]string)(s))
}

// Value implements driver.Valuer, always emitting a JSON array (never NULL).
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]string(s))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Role is a reusable bundle of section permissions.
type Role struct {
	ID          int64       `db:"id" json:"id"`
	Name        string      `db:"name" json:"name"`
	Description *string     `db:"description" json:"description,omitempty"`
	Permissions StringSlice `db:"permissions" json:"permissions"`
	IsSystem    bool        `db:"is_system" json:"is_system"`
	UserCount   int         `db:"user_count" json:"user_count"`
	CreatedAt   time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time   `db:"updated_at" json:"updated_at"`
}

// AdminUser is an admin employee row enriched with its role name.
type AdminUser struct {
	ID           int64     `db:"id" json:"id"`
	FullName     string    `db:"full_name" json:"full_name"`
	Email        string    `db:"email" json:"email"`
	Phone        *string   `db:"phone" json:"phone,omitempty"`
	IsActive     bool      `db:"is_active" json:"is_active"`
	IsSuperAdmin bool      `db:"is_super_admin" json:"is_super_admin"`
	AdminRoleID  *int64    `db:"admin_role_id" json:"admin_role_id,omitempty"`
	RoleName     *string   `db:"role_name" json:"role_name,omitempty"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

// CapabilityGrant is a position/employee business access row. Effect is empty
// for position defaults and allow/deny for employee exceptions.
type CapabilityGrant struct {
	Key         string `db:"capability_key"`
	AccessLevel string `db:"access_level"`
	Effect      string `db:"effect"`
}

// --- request DTOs ---

type RoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

type CreateUserRequest struct {
	FullName    string `json:"full_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	Phone       string `json:"phone"`
	AdminRoleID *int64 `json:"admin_role_id"`
	IsActive    *bool  `json:"is_active"`
}

type UpdateUserRequest struct {
	FullName    string `json:"full_name"`
	Phone       string `json:"phone"`
	AdminRoleID *int64 `json:"admin_role_id"`
	IsActive    *bool  `json:"is_active"`
}

// Filter is the shared list filter for users and roles.
type Filter struct {
	Search string
	Limit  int
	Offset int
}
