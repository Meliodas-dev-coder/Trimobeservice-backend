// Package mobility is the cars-for-hire module: car categories, cars (with a
// per-car daily rate seeded from the category default), car images, and drivers.
package mobility

import (
	"time"

	"github.com/jmoiron/sqlx/types"
)

// Car operational status (distinct from booking availability, which the
// bookings module computes from date ranges).
const (
	CarStatusAvailable   = "available"
	CarStatusMaintenance = "maintenance"
	CarStatusInactive    = "inactive"
)

const (
	DriverStatusAvailable = "available"
	DriverStatusAssigned  = "assigned"
	DriverStatusInactive  = "inactive"
)

// --- domain models ---

type CarCategory struct {
	ID               int64     `db:"id" json:"id"`
	Name             string    `db:"name" json:"name"`
	Slug             string    `db:"slug" json:"slug"`
	Description      *string   `db:"description" json:"description,omitempty"`
	DefaultDailyRate string    `db:"default_daily_rate" json:"default_daily_rate"` // DECIMAL(12,2) as string
	ImageURL         *string   `db:"image_url" json:"image_url,omitempty"`
	SortOrder        int       `db:"sort_order" json:"sort_order"`
	IsActive         bool      `db:"is_active" json:"is_active"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time `db:"updated_at" json:"updated_at"`
}

type Car struct {
	ID                int64          `db:"id" json:"id"`
	CategoryID        int64          `db:"category_id" json:"category_id"`
	Name              string         `db:"name" json:"name"`
	Slug              string         `db:"slug" json:"slug"`
	Make              *string        `db:"make" json:"make,omitempty"`
	Model             *string        `db:"model" json:"model,omitempty"`
	Year              *int           `db:"year" json:"year,omitempty"`
	RegistrationPlate *string        `db:"registration_plate" json:"registration_plate,omitempty"`
	Color             *string        `db:"color" json:"color,omitempty"`
	Seats             *int           `db:"seats" json:"seats,omitempty"`
	Transmission      *string        `db:"transmission" json:"transmission,omitempty"`
	FuelType          *string        `db:"fuel_type" json:"fuel_type,omitempty"`
	DailyRate         string         `db:"daily_rate" json:"daily_rate"` // DECIMAL(12,2) as string
	Attributes        types.JSONText `db:"attributes" json:"attributes,omitempty"`
	Description       *string        `db:"description" json:"description,omitempty"`
	Status            string         `db:"status" json:"status"`
	CreatedAt         time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time      `db:"updated_at" json:"updated_at"`
}

type CarImage struct {
	ID        int64     `db:"id" json:"id"`
	CarID     int64     `db:"car_id" json:"car_id"`
	URL       string    `db:"url" json:"url"`
	AltText   *string   `db:"alt_text" json:"alt_text,omitempty"`
	IsPrimary bool      `db:"is_primary" json:"is_primary"`
	SortOrder int       `db:"sort_order" json:"sort_order"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type Driver struct {
	ID            int64     `db:"id" json:"id"`
	FullName      string    `db:"full_name" json:"full_name"`
	Phone         string    `db:"phone" json:"phone"`
	LicenseNumber string    `db:"license_number" json:"license_number"`
	Status        string    `db:"status" json:"status"`
	Notes         *string   `db:"notes" json:"notes,omitempty"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

// CarDetail is a car plus its related data.
type CarDetail struct {
	Car
	Category *CarCategory `json:"category,omitempty"`
	Images   []CarImage   `json:"images"`
}

// CarFilter drives the car list query.
type CarFilter struct {
	CategoryID *int64
	Search     string
	Status     string // "" = any (admin); public forces "available"
	Limit      int
	Offset     int
}

// --- request DTOs ---

type CarCategoryRequest struct {
	Name             string  `json:"name"`
	Description      *string `json:"description"`
	DefaultDailyRate string  `json:"default_daily_rate"`
	ImageURL         *string `json:"image_url"`
	SortOrder        int     `json:"sort_order"`
	IsActive         *bool   `json:"is_active"`
}

type CarRequest struct {
	CategoryID        int64          `json:"category_id"`
	Name              string         `json:"name"`
	Make              *string        `json:"make"`
	Model             *string        `json:"model"`
	Year              *int           `json:"year"`
	RegistrationPlate *string        `json:"registration_plate"`
	Color             *string        `json:"color"`
	Seats             *int           `json:"seats"`
	Transmission      *string        `json:"transmission"`
	FuelType          *string        `json:"fuel_type"`
	DailyRate         string         `json:"daily_rate"` // blank = seed from category default (create) / keep (update)
	Attributes        types.JSONText `json:"attributes"`
	Description       *string        `json:"description"`
	Status            *string        `json:"status"`
}

type CarImageRequest struct {
	URL       string  `json:"url"`
	AltText   *string `json:"alt_text"`
	IsPrimary bool    `json:"is_primary"`
	SortOrder int     `json:"sort_order"`
}

type DriverRequest struct {
	FullName      string  `json:"full_name"`
	Phone         string  `json:"phone"`
	LicenseNumber string  `json:"license_number"`
	Status        *string `json:"status"`
	Notes         *string `json:"notes"`
}

func derefBool(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}
