// Package bookings is the car-rental transaction module: availability checks,
// booking a car over a date range (no double-booking), driver assignment, and
// the rental lifecycle. Payment is tracked separately (manual/admin-confirmed).
package bookings

import "time"

// Rental lifecycle (payment is a separate concern; see PaymentStatus).
const (
	StatusRequested      = "requested"
	StatusConfirmed      = "confirmed"
	StatusDriverAssigned = "driver_assigned"
	StatusActive         = "active"
	StatusCompleted      = "completed"
	StatusCancelled      = "cancelled"
)

const (
	PaymentUnpaid   = "unpaid"
	PaymentPaid     = "paid"
	PaymentRefunded = "refunded"
)

// Occupying statuses (confirmed, driver_assigned, active) hold the car for a
// time range; the SQL below and the DB triggers must agree on this set.

// --- domain model ---

type Booking struct {
	ID                int64      `db:"id" json:"id"`
	UserID            int64      `db:"user_id" json:"user_id"`
	CarID             int64      `db:"car_id" json:"car_id"`
	DriverID          *int64     `db:"driver_id" json:"driver_id,omitempty"`
	BookingNumber     string     `db:"booking_number" json:"booking_number"`
	Status            string     `db:"status" json:"status"`
	PaymentStatus     string     `db:"payment_status" json:"payment_status"`
	StartAt           time.Time  `db:"start_at" json:"start_at"`
	EndAt             time.Time  `db:"end_at" json:"end_at"`
	Days              int        `db:"days" json:"days"`
	DailyRateSnapshot string     `db:"daily_rate_snapshot" json:"daily_rate_snapshot"`
	Fees              string     `db:"fees" json:"fees"`
	TotalPrice        string     `db:"total_price" json:"total_price"`
	CarName           string     `db:"car_name" json:"car_name"`
	CarCategory       *string    `db:"car_category" json:"car_category,omitempty"`
	PickupLocation    string     `db:"pickup_location" json:"pickup_location"`
	DropoffLocation   *string    `db:"dropoff_location" json:"dropoff_location,omitempty"`
	ContactPhone      string     `db:"contact_phone" json:"contact_phone"`
	Note              *string    `db:"note" json:"note,omitempty"`
	PaidAt            *time.Time `db:"paid_at" json:"paid_at,omitempty"`
	CreatedAt         time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time  `db:"updated_at" json:"updated_at"`
}

type DriverInfo struct {
	ID       int64  `db:"id" json:"id"`
	FullName string `db:"full_name" json:"full_name"`
	Phone    string `db:"phone" json:"phone"`
}

type BookingDetail struct {
	Booking
	Driver *DriverInfo `json:"driver,omitempty"`
}

// BookingFilter drives list queries (admin and per-user).
type BookingFilter struct {
	UserID        *int64
	Status        string
	PaymentStatus string
	CarID         *int64
	Limit         int
	Offset        int
}

// carRow is the locked/read subset of a car used during booking.
type carRow struct {
	ID         int64  `db:"id"`
	Name       string `db:"name"`
	DailyRate  string `db:"daily_rate"`
	Status     string `db:"status"`
	CategoryID int64  `db:"category_id"`
}

type driverRow struct {
	ID       int64  `db:"id"`
	FullName string `db:"full_name"`
	Phone    string `db:"phone"`
	Status   string `db:"status"`
}

// --- request DTOs ---

type CreateBookingRequest struct {
	CarID           int64     `json:"car_id"`
	StartAt         time.Time `json:"start_at"` // RFC3339
	EndAt           time.Time `json:"end_at"`   // RFC3339
	PickupLocation  string    `json:"pickup_location"`
	DropoffLocation *string   `json:"dropoff_location"`
	ContactPhone    string    `json:"contact_phone"`
	Note            *string   `json:"note"`
}

type AssignDriverRequest struct {
	DriverID int64 `json:"driver_id"`
}

type UpdateStatusRequest struct {
	Status string `json:"status"`
}

// AvailabilityResult is returned by the public availability check.
type AvailabilityResult struct {
	CarID     int64     `json:"car_id"`
	StartAt   time.Time `json:"start_at"`
	EndAt     time.Time `json:"end_at"`
	Available bool      `json:"available"`
}
