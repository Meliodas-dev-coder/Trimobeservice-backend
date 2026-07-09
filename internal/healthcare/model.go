// Package healthcare is Trimobe's fourth domain: home healthcare. It is a hybrid
// of event planning (a home consultation is a form the team quotes manually) and
// bookings (a care package is a coverage window with practitioners assigned from
// a roster, like drivers on cars). It follows the platform's
// Category -> Item -> Transaction shape:
//
//	ServiceCategory -> Service (+ package staff) -> Request (+ assignments)
//
// Practitioners (doctors & nurses) are a shared roster. Consultations are
// quote-priced; packages carry a fixed price. Payment flows through the shared
// manual-payments ledger (payable_type 'healthcare').
package healthcare

import (
	"time"

	"github.com/jmoiron/sqlx/types"
)

// Practitioner types (also used for package staff and assignment snapshots).
const (
	TypeDoctor = "doctor"
	TypeNurse  = "nurse"
)

// Practitioner roster status.
const (
	PractitionerActive   = "active"
	PractitionerInactive = "inactive"
)

// Service / request kinds.
const (
	ServiceConsultation = "consultation"
	ServicePackage      = "package"
)

// Request lifecycle (payment is a separate concern; see PaymentStatus).
const (
	StatusRequested  = "requested"
	StatusReviewing  = "reviewing"
	StatusQuoted     = "quoted"
	StatusConfirmed  = "confirmed"
	StatusAssigned   = "assigned"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusCancelled  = "cancelled"
)

const (
	PaymentUnpaid   = "unpaid"
	PaymentPaid     = "paid"
	PaymentRefunded = "refunded"
)

// --- roster: practitioners ---

type Practitioner struct {
	ID            int64     `db:"id" json:"id"`
	Type          string    `db:"type" json:"type"`
	FullName      string    `db:"full_name" json:"full_name"`
	Specialty     *string   `db:"specialty" json:"specialty,omitempty"`
	Phone         string    `db:"phone" json:"phone"`
	Email         *string   `db:"email" json:"email,omitempty"`
	LicenseNumber *string   `db:"license_number" json:"license_number,omitempty"`
	Bio           *string   `db:"bio" json:"bio,omitempty"`
	PhotoURL      *string   `db:"photo_url" json:"photo_url,omitempty"`
	Status        string    `db:"status" json:"status"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

// PractitionerFilter drives the roster list query.
type PractitionerFilter struct {
	Type       string
	Search     string
	ActiveOnly bool
	Limit      int
	Offset     int
}

// --- catalog: categories ---

type ServiceCategory struct {
	ID          int64     `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	Slug        string    `db:"slug" json:"slug"`
	Description *string   `db:"description" json:"description,omitempty"`
	Icon        *string   `db:"icon" json:"icon,omitempty"`
	ImageURL    *string   `db:"image_url" json:"image_url,omitempty"`
	SortOrder   int       `db:"sort_order" json:"sort_order"`
	IsActive    bool      `db:"is_active" json:"is_active"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`

	// List-only aggregate.
	ServiceCount int `db:"service_count" json:"service_count"`
}

// --- catalog: services (consultation | package) ---

type HealthcareService struct {
	ID           int64          `db:"id" json:"id"`
	CategoryID   int64          `db:"category_id" json:"category_id"`
	Name         string         `db:"name" json:"name"`
	Slug         string         `db:"slug" json:"slug"`
	Description  *string        `db:"description" json:"description,omitempty"`
	ServiceType  string         `db:"service_type" json:"service_type"`
	FromPrice    *string        `db:"from_price" json:"from_price,omitempty"` // indicative (consultations)
	Price        *string        `db:"price" json:"price,omitempty"`           // fixed (packages)
	PriceUnit    *string        `db:"price_unit" json:"price_unit,omitempty"`
	DurationDays *int           `db:"duration_days" json:"duration_days,omitempty"`
	ImageURL     *string        `db:"image_url" json:"image_url,omitempty"`
	Attributes   types.JSONText `db:"attributes" json:"attributes,omitempty"`
	SortOrder    int            `db:"sort_order" json:"sort_order"`
	IsActive     bool           `db:"is_active" json:"is_active"`
	CreatedAt    time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time      `db:"updated_at" json:"updated_at"`

	// List-only join.
	CategoryName *string `db:"category_name" json:"category_name,omitempty"`

	// List-only aggregates: a package's staff makeup, for admin display/prefill.
	StaffDoctors int `db:"staff_doctors" json:"staff_doctors"`
	StaffNurses  int `db:"staff_nurses" json:"staff_nurses"`
}

// PackageStaff is one line of a package's staff makeup (e.g. nurse x2).
type PackageStaff struct {
	ID               int64  `db:"id" json:"id"`
	ServiceID        int64  `db:"service_id" json:"service_id"`
	PractitionerType string `db:"practitioner_type" json:"practitioner_type"`
	Quantity         int    `db:"quantity" json:"quantity"`
}

// ServiceDetail is a service with its package staff (empty for consultations).
type ServiceDetail struct {
	HealthcareService
	Staff []PackageStaff `json:"staff"`
}

// serviceRow is the read subset of a service used to snapshot it onto a request.
type serviceRow struct {
	ID           int64   `db:"id"`
	Name         string  `db:"name"`
	ServiceType  string  `db:"service_type"`
	FromPrice    *string `db:"from_price"`
	Price        *string `db:"price"`
	DurationDays *int    `db:"duration_days"`
	IsActive     bool    `db:"is_active"`
	CategoryName string  `db:"category_name"`
}

// ServiceFilter drives the service list query.
type ServiceFilter struct {
	CategoryID  *int64
	ServiceType string
	Search      string
	ActiveOnly  bool
	Limit       int // 0 = no limit (public grouped catalog)
	Offset      int
}

// --- transaction: healthcare requests ---

type Request struct {
	ID            int64      `db:"id" json:"id"`
	UserID        *int64     `db:"user_id" json:"user_id,omitempty"`
	CustomerName  *string    `db:"customer_name" json:"customer_name,omitempty"`
	RequestNumber string     `db:"request_number" json:"request_number"`
	RequestType   string     `db:"request_type" json:"request_type"`
	ServiceID     *int64     `db:"service_id" json:"service_id,omitempty"`
	ServiceName   *string    `db:"service_name" json:"service_name,omitempty"`
	CategoryName  *string    `db:"category_name" json:"category_name,omitempty"`
	PriceSnapshot *string    `db:"price_snapshot" json:"price_snapshot,omitempty"`
	PatientName   string     `db:"patient_name" json:"patient_name"`
	PatientAge    *int       `db:"patient_age" json:"patient_age,omitempty"`
	PatientGender *string    `db:"patient_gender" json:"patient_gender,omitempty"`
	PreferredAt   *time.Time `db:"preferred_at" json:"preferred_at,omitempty"`
	StartAt       *time.Time `db:"start_at" json:"start_at,omitempty"`
	EndAt         *time.Time `db:"end_at" json:"end_at,omitempty"`
	Address       string     `db:"address" json:"address"`
	Symptoms      *string    `db:"symptoms" json:"symptoms,omitempty"`
	Status        string     `db:"status" json:"status"`
	PaymentStatus string     `db:"payment_status" json:"payment_status"`
	PaidAt        *time.Time `db:"paid_at" json:"paid_at,omitempty"`
	QuotedPrice   *string    `db:"quoted_price" json:"quoted_price,omitempty"`
	ContactPhone  string     `db:"contact_phone" json:"contact_phone"`
	ContactEmail  *string    `db:"contact_email" json:"contact_email,omitempty"`
	Note          *string    `db:"note" json:"note,omitempty"`
	AdminNote     *string    `db:"admin_note" json:"admin_note,omitempty"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at" json:"updated_at"`

	// List-only aggregate.
	AssignmentCount int `db:"assignment_count" json:"assignment_count"`
}

// Assignment is a practitioner attached to a request (snapshotted).
type Assignment struct {
	ID               int64     `db:"id" json:"id"`
	RequestID        int64     `db:"request_id" json:"request_id"`
	PractitionerID   *int64    `db:"practitioner_id" json:"practitioner_id,omitempty"`
	PractitionerName string    `db:"practitioner_name" json:"practitioner_name"`
	PractitionerType string    `db:"practitioner_type" json:"practitioner_type"`
	Note             *string   `db:"note" json:"note,omitempty"`
	AssignedAt       time.Time `db:"assigned_at" json:"assigned_at"`
}

// RequestDetail is a request with its catalog staff need and assigned people.
type RequestDetail struct {
	Request
	Staff       []PackageStaff `json:"staff"`       // the package's required makeup (empty for consultations)
	Assignments []Assignment   `json:"assignments"` // practitioners assigned so far
}

// RequestFilter drives list queries (admin and per-user).
type RequestFilter struct {
	UserID        *int64
	Status        string
	PaymentStatus string
	RequestType   string
	Limit         int
	Offset        int
}

// --- settings ---

type Settings struct {
	EmergencyPhone *string   `db:"emergency_phone" json:"emergency_phone,omitempty"`
	EmergencyHours *string   `db:"emergency_hours" json:"emergency_hours,omitempty"`
	EmergencyNote  *string   `db:"emergency_note" json:"emergency_note,omitempty"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
}

// --- request DTOs ---

type PractitionerRequest struct {
	Type          string  `json:"type"`
	FullName      string  `json:"full_name"`
	Specialty     *string `json:"specialty"`
	Phone         string  `json:"phone"`
	Email         *string `json:"email"`
	LicenseNumber *string `json:"license_number"`
	Bio           *string `json:"bio"`
	PhotoURL      *string `json:"photo_url"`
	Status        *string `json:"status"`
}

type ServiceCategoryRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Icon        *string `json:"icon"`
	ImageURL    *string `json:"image_url"`
	SortOrder   int     `json:"sort_order"`
	IsActive    *bool   `json:"is_active"`
}

// StaffLine is one entry of a package's staff makeup in a service payload.
type StaffLine struct {
	PractitionerType string `json:"practitioner_type"`
	Quantity         int    `json:"quantity"`
}

type ServiceRequest struct {
	CategoryID   int64          `json:"category_id"`
	Name         string         `json:"name"`
	Description  *string        `json:"description"`
	ServiceType  string         `json:"service_type"`
	FromPrice    *string        `json:"from_price"`
	Price        *string        `json:"price"`
	PriceUnit    *string        `json:"price_unit"`
	DurationDays *int           `json:"duration_days"`
	ImageURL     *string        `json:"image_url"`
	Attributes   types.JSONText `json:"attributes"`
	SortOrder    int            `json:"sort_order"`
	IsActive     *bool          `json:"is_active"`
	Staff        []StaffLine    `json:"staff"` // package makeup; ignored for consultations
}

// CreateRequest is the client payload for a new healthcare request. request_type
// is derived from the chosen service (or defaults to consultation when none).
type CreateRequest struct {
	ServiceID     *int64     `json:"service_id"` // required for packages; optional for consultations
	PatientName   string     `json:"patient_name"`
	PatientAge    *int       `json:"patient_age"`
	PatientGender *string    `json:"patient_gender"`
	PreferredAt   *time.Time `json:"preferred_at"` // consultation visit time (RFC3339)
	StartAt       *time.Time `json:"start_at"`     // package window start (RFC3339)
	EndAt         *time.Time `json:"end_at"`       // package window end (RFC3339); derived if absent
	Address       string     `json:"address"`
	Symptoms      *string    `json:"symptoms"`
	ContactPhone  string     `json:"contact_phone"`
	ContactEmail  *string    `json:"contact_email"`
	Note          *string    `json:"note"`
}

// AdminCreateRequest lets an admin log a phone/walk-in request.
type AdminCreateRequest struct {
	UserID       *int64 `json:"user_id"`
	CustomerName string `json:"customer_name"`
	CreateRequest
}

type UpdateStatusRequest struct {
	Status string `json:"status"`
}

// QuoteRequest sets (or revises) the admin quote for a request.
type QuoteRequest struct {
	QuotedPrice string  `json:"quoted_price"`
	AdminNote   *string `json:"admin_note"`
}

// AssignRequest attaches a practitioner to a request.
type AssignRequest struct {
	PractitionerID int64   `json:"practitioner_id"`
	Note           *string `json:"note"`
}

type SettingsRequest struct {
	EmergencyPhone *string `json:"emergency_phone"`
	EmergencyHours *string `json:"emergency_hours"`
	EmergencyNote  *string `json:"emergency_note"`
}

func derefBool(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}
