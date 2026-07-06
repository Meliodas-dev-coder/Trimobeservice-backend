// Package auth handles registration, login, JWT access tokens, and rotating
// refresh tokens for both customers and the admin.
package auth

import "time"

// Role mirrors the users.role enum.
type Role string

const (
	RoleCustomer Role = "customer"
	RoleAdmin    Role = "admin"
)

// User maps to the users table.
type User struct {
	ID              int64      `db:"id"`
	Role            Role       `db:"role"`
	Email           string     `db:"email"`
	PasswordHash    string     `db:"password_hash"`
	FullName        string     `db:"full_name"`
	Phone           *string    `db:"phone"`
	EmailVerifiedAt *time.Time `db:"email_verified_at"`
	IsActive        bool       `db:"is_active"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
}

type Address struct {
	ID            int64     `db:"id" json:"id"`
	UserID        int64     `db:"user_id" json:"user_id"`
	Label         *string   `db:"label" json:"label,omitempty"`
	RecipientName string    `db:"recipient_name" json:"recipient_name"`
	Phone         string    `db:"phone" json:"phone"`
	Line1         string    `db:"line1" json:"line1"`
	Line2         *string   `db:"line2" json:"line2,omitempty"`
	City          string    `db:"city" json:"city"`
	Region        *string   `db:"region" json:"region,omitempty"`
	Country       string    `db:"country" json:"country"`
	PostalCode    *string   `db:"postal_code" json:"postal_code,omitempty"`
	IsDefault     bool      `db:"is_default" json:"is_default"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

// --- request DTOs ---

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type AddressRequest struct {
	Label         *string `json:"label"`
	RecipientName string  `json:"recipient_name"`
	Phone         string  `json:"phone"`
	Line1         string  `json:"line1"`
	Line2         *string `json:"line2"`
	City          string  `json:"city"`
	Region        *string `json:"region"`
	Country       string  `json:"country"`
	PostalCode    *string `json:"postal_code"`
	IsDefault     *bool   `json:"is_default"`
}

// --- response DTOs ---

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"` // access token lifetime in seconds
}

type UserResponse struct {
	ID       int64  `json:"id"`
	Role     Role   `json:"role"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Phone    string `json:"phone,omitempty"`
}

func toUserResponse(u *User) UserResponse {
	resp := UserResponse{
		ID:       u.ID,
		Role:     u.Role,
		Email:    u.Email,
		FullName: u.FullName,
	}
	if u.Phone != nil {
		resp.Phone = *u.Phone
	}
	return resp
}
