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

const (
	PricingDaily         = "daily"
	PricingCargoDistance = "cargo_distance"
)

// Occupying statuses (confirmed, driver_assigned, active) hold the car for a
// time range; the SQL below and the DB triggers must agree on this set.

// --- domain model ---

type Booking struct {
	ID                  int64      `db:"id" json:"id"`
	BookingGroupID      *int64     `db:"booking_group_id" json:"-"`
	UserID              *int64     `db:"user_id" json:"user_id,omitempty"`
	CustomerName        *string    `db:"customer_name" json:"customer_name,omitempty"`
	CarID               int64      `db:"car_id" json:"car_id"`
	DriverID            *int64     `db:"driver_id" json:"driver_id,omitempty"`
	BookingNumber       string     `db:"booking_number" json:"booking_number"`
	Status              string     `db:"status" json:"status"`
	PaymentStatus       string     `db:"payment_status" json:"payment_status"`
	StartAt             time.Time  `db:"start_at" json:"start_at"`
	EndAt               time.Time  `db:"end_at" json:"end_at"`
	Days                int        `db:"days" json:"days"`
	DailyRateSnapshot   string     `db:"daily_rate_snapshot" json:"daily_rate_snapshot"`
	OutsideAntananarivo bool       `db:"outside_antananarivo" json:"outside_antananarivo"`
	Fees                string     `db:"fees" json:"fees"`
	TotalPrice          string     `db:"total_price" json:"total_price"`
	PricingModel        string     `db:"pricing_model" json:"pricing_model"`
	DistanceKm          *string    `db:"distance_km" json:"distance_km,omitempty"`
	CargoPerKmRate      *string    `db:"cargo_per_km_rate_snapshot" json:"cargo_per_km_rate_snapshot,omitempty"`
	CargoMinimumRate    *string    `db:"cargo_minimum_rate_snapshot" json:"cargo_minimum_rate_snapshot,omitempty"`
	CarName             string     `db:"car_name" json:"car_name"`
	CarCategory         *string    `db:"car_category" json:"car_category,omitempty"`
	PickupLocation      string     `db:"pickup_location" json:"pickup_location"`
	PickupLatitude      *float64   `db:"pickup_latitude" json:"pickup_latitude,omitempty"`
	PickupLongitude     *float64   `db:"pickup_longitude" json:"pickup_longitude,omitempty"`
	PickupReference     *string    `db:"pickup_reference" json:"pickup_reference,omitempty"`
	DropoffLocation     *string    `db:"dropoff_location" json:"dropoff_location,omitempty"`
	DropoffLatitude     *float64   `db:"dropoff_latitude" json:"dropoff_latitude,omitempty"`
	DropoffLongitude    *float64   `db:"dropoff_longitude" json:"dropoff_longitude,omitempty"`
	DropoffReference    *string    `db:"dropoff_reference" json:"dropoff_reference,omitempty"`
	ContactPhone        string     `db:"contact_phone" json:"contact_phone"`
	Note                *string    `db:"note" json:"note,omitempty"`
	PaidAt              *time.Time `db:"paid_at" json:"paid_at,omitempty"`
	CreatedAt           time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time  `db:"updated_at" json:"updated_at"`
	IsMultiCar          bool       `db:"is_multi_car" json:"is_multi_car"`
	CarCount            int        `db:"car_count" json:"car_count"`
}

type DriverInfo struct {
	ID       int64  `db:"id" json:"id"`
	FullName string `db:"full_name" json:"full_name"`
	Phone    string `db:"phone" json:"phone"`
}

// BookingCarOption/BookingDriverOption are deliberately least-data selector
// rows. Booking-only positions can create and assign bookings without receiving
// fleet notes, vehicle details, driver licences, or other submenu data.
type BookingCarOption struct {
	ID           int64  `db:"id" json:"id"`
	Name         string `db:"name" json:"name"`
	CategoryName string `db:"category_name" json:"category_name"`
	Status       string `db:"status" json:"status"`
}

type BookingDriverOption struct {
	ID       int64  `db:"id" json:"id"`
	FullName string `db:"full_name" json:"full_name"`
	Status   string `db:"status" json:"status"`
}

type BookingDetail struct {
	Booking
	Driver *DriverInfo        `json:"driver,omitempty"`
	Cars   []BookingCarDetail `json:"cars,omitempty"`
}

// BookingCarDetail is one physical-car item within a multi-car booking. The
// item ID is intentionally exposed to admins so each car can receive its own
// driver, while the customer-facing reference and payment stay at group level.
type BookingCarDetail struct {
	ID                int64       `db:"id" json:"id"`
	CarID             int64       `db:"car_id" json:"car_id"`
	DriverID          *int64      `db:"driver_id" json:"driver_id,omitempty"`
	Status            string      `db:"status" json:"status"`
	DailyRateSnapshot string      `db:"daily_rate_snapshot" json:"daily_rate_snapshot"`
	Fees              string      `db:"fees" json:"fees"`
	TotalPrice        string      `db:"total_price" json:"total_price"`
	PricingModel      string      `db:"pricing_model" json:"pricing_model"`
	DistanceKm        *string     `db:"distance_km" json:"distance_km,omitempty"`
	CarName           string      `db:"car_name" json:"car_name"`
	CarCategory       *string     `db:"car_category" json:"car_category,omitempty"`
	Driver            *DriverInfo `json:"driver,omitempty"`
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
	ID                           int64  `db:"id"`
	Name                         string `db:"name"`
	DailyRate                    string `db:"daily_rate"`
	OutsideAntananarivoDailyRate string `db:"outside_antananarivo_daily_rate"`
	Status                       string `db:"status"`
	CategoryID                   int64  `db:"category_id"`
	IsCargoTransport             bool   `db:"is_cargo_transport"`
	CargoPerKmRate               string `db:"cargo_per_km_rate"`
	CargoMinimumRate             string `db:"cargo_minimum_rate"`
}

type driverRow struct {
	ID       int64  `db:"id"`
	FullName string `db:"full_name"`
	Phone    string `db:"phone"`
	Status   string `db:"status"`
}

// --- request DTOs ---

type CreateBookingRequest struct {
	CarID               int64     `json:"car_id"`
	StartAt             time.Time `json:"start_at"` // RFC3339
	EndAt               time.Time `json:"end_at"`   // RFC3339
	PickupLocation      string    `json:"pickup_location"`
	PickupLatitude      *float64  `json:"pickup_latitude"`
	PickupLongitude     *float64  `json:"pickup_longitude"`
	PickupReference     *string   `json:"pickup_reference"`
	DropoffLocation     *string   `json:"dropoff_location"`
	DropoffLatitude     *float64  `json:"dropoff_latitude"`
	DropoffLongitude    *float64  `json:"dropoff_longitude"`
	DropoffReference    *string   `json:"dropoff_reference"`
	DistanceKm          *string   `json:"distance_km"`
	OutsideAntananarivo *bool     `json:"outside_antananarivo"`
	ContactPhone        string    `json:"contact_phone"`
	Note                *string   `json:"note"`
}

// CreateBookingBatchRequest reserves several physical cars under one booking
// reference and one payment. Each physical car remains a separate internal
// item so availability and driver assignment remain per vehicle.
type CreateBookingBatchRequest struct {
	CarIDs              []int64   `json:"car_ids"`
	StartAt             time.Time `json:"start_at"`
	EndAt               time.Time `json:"end_at"`
	PickupLocation      string    `json:"pickup_location"`
	PickupLatitude      *float64  `json:"pickup_latitude"`
	PickupLongitude     *float64  `json:"pickup_longitude"`
	PickupReference     *string   `json:"pickup_reference"`
	DropoffLocation     *string   `json:"dropoff_location"`
	DropoffLatitude     *float64  `json:"dropoff_latitude"`
	DropoffLongitude    *float64  `json:"dropoff_longitude"`
	DropoffReference    *string   `json:"dropoff_reference"`
	DistanceKm          *string   `json:"distance_km"`
	OutsideAntananarivo *bool     `json:"outside_antananarivo"`
	ContactPhone        string    `json:"contact_phone"`
	Note                *string   `json:"note"`
}

func (r CreateBookingBatchRequest) bookingFor(carID int64) CreateBookingRequest {
	return CreateBookingRequest{
		CarID:               carID,
		StartAt:             r.StartAt,
		EndAt:               r.EndAt,
		PickupLocation:      r.PickupLocation,
		PickupLatitude:      r.PickupLatitude,
		PickupLongitude:     r.PickupLongitude,
		PickupReference:     r.PickupReference,
		DropoffLocation:     r.DropoffLocation,
		DropoffLatitude:     r.DropoffLatitude,
		DropoffLongitude:    r.DropoffLongitude,
		DropoffReference:    r.DropoffReference,
		DistanceKm:          r.DistanceKm,
		OutsideAntananarivo: r.OutsideAntananarivo,
		ContactPhone:        r.ContactPhone,
		Note:                r.Note,
	}
}

type AdminCreateBookingRequest struct {
	UserID              *int64    `json:"user_id"`
	CustomerName        string    `json:"customer_name"`
	CarID               int64     `json:"car_id"`
	DriverID            *int64    `json:"driver_id"`
	StartAt             time.Time `json:"start_at"`
	EndAt               time.Time `json:"end_at"`
	PickupLocation      string    `json:"pickup_location"`
	PickupLatitude      *float64  `json:"pickup_latitude"`
	PickupLongitude     *float64  `json:"pickup_longitude"`
	PickupReference     *string   `json:"pickup_reference"`
	DropoffLocation     *string   `json:"dropoff_location"`
	DropoffLatitude     *float64  `json:"dropoff_latitude"`
	DropoffLongitude    *float64  `json:"dropoff_longitude"`
	DropoffReference    *string   `json:"dropoff_reference"`
	DistanceKm          *string   `json:"distance_km"`
	OutsideAntananarivo *bool     `json:"outside_antananarivo"`
	ContactPhone        string    `json:"contact_phone"`
	Note                *string   `json:"note"`
}

type AssignDriverRequest struct {
	DriverID      int64  `json:"driver_id"`
	BookingItemID *int64 `json:"booking_item_id"`
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

// BookedRange is an occupied window on a car's calendar, exposed publicly so
// clients can grey out unavailable dates before submitting a booking.
type BookedRange struct {
	StartAt time.Time `db:"start_at" json:"start_at"`
	EndAt   time.Time `db:"end_at" json:"end_at"`
}
