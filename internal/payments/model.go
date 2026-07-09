// Package payments is the manual/offline payment ledger. An admin records a
// payment against an order or a booking (polymorphic); recording it also flips
// that target's payment_status. A real gateway later is just another `method`.
package payments

import "time"

// Payable targets.
const (
	PayableOrder      = "order"
	PayableBooking    = "booking"
	PayableEvent      = "event"
	PayableHealthcare = "healthcare"
)

// Payment methods.
const (
	MethodCash         = "cash"
	MethodBankTransfer = "bank_transfer"
	MethodMobileMoney  = "mobile_money"
	MethodOther        = "other"
)

// Payment ledger statuses.
const (
	StatusPending  = "pending"
	StatusPaid     = "paid"
	StatusRefunded = "refunded"
)

// Target lifecycle values we care about (mirror the orders/bookings modules).
const (
	orderDelivered = "delivered"
	orderPickedUp  = "picked_up"
	orderCancelled = "cancelled"
	orderExpired   = "expired"

	bookingCancelled = "cancelled"

	eventCancelled = "cancelled"

	healthcareCancelled = "cancelled"

	targetPaid = "paid"
)

type Payment struct {
	ID           int64      `db:"id" json:"id"`
	PayableType  string     `db:"payable_type" json:"payable_type"`
	PayableID    int64      `db:"payable_id" json:"payable_id"`
	Amount       string     `db:"amount" json:"amount"`
	Method       string     `db:"method" json:"method"`
	Status       string     `db:"status" json:"status"`
	Reference    *string    `db:"reference" json:"reference,omitempty"`
	MarkedPaidBy *int64     `db:"marked_paid_by" json:"marked_paid_by,omitempty"`
	MarkedPaidAt *time.Time `db:"marked_paid_at" json:"marked_paid_at,omitempty"`
	Note         *string    `db:"note" json:"note,omitempty"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"`
}

type PaymentDetail struct {
	Payment
	Target *PaymentTarget `json:"target,omitempty"`
}

type PaymentTarget struct {
	Type            string              `db:"target_type" json:"type"`
	ID              int64               `db:"id" json:"id"`
	UserID          *int64              `db:"user_id" json:"user_id,omitempty"`
	CustomerName    *string             `db:"customer_name" json:"customer_name,omitempty"`
	Number          string              `db:"number" json:"number"`
	Status          string              `db:"status" json:"status"`
	PaymentStatus   string              `db:"payment_status" json:"payment_status"`
	Total           string              `db:"total" json:"total"`
	FulfillmentType *string             `db:"fulfillment_type" json:"fulfillment_type,omitempty"`
	CarName         *string             `db:"car_name" json:"car_name,omitempty"`
	CarCategory     *string             `db:"car_category" json:"car_category,omitempty"`
	EventType       *string             `db:"event_type" json:"event_type,omitempty"`
	StartAt         *time.Time          `db:"start_at" json:"start_at,omitempty"`
	EndAt           *time.Time          `db:"end_at" json:"end_at,omitempty"`
	PickupLocation  *string             `db:"pickup_location" json:"pickup_location,omitempty"`
	DropoffLocation *string             `db:"dropoff_location" json:"dropoff_location,omitempty"`
	ContactPhone    *string             `db:"contact_phone" json:"contact_phone,omitempty"`
	CreatedAt       time.Time           `db:"created_at" json:"created_at"`
	Items           []PaymentTargetItem `json:"items,omitempty"`
}

type PaymentTargetItem struct {
	ID           int64   `db:"id" json:"id"`
	ProductName  string  `db:"product_name" json:"product_name"`
	VariantLabel *string `db:"variant_label" json:"variant_label,omitempty"`
	SKU          *string `db:"sku" json:"sku,omitempty"`
	UnitPrice    string  `db:"unit_price" json:"unit_price"`
	Quantity     int     `db:"quantity" json:"quantity"`
	LineTotal    string  `db:"line_total" json:"line_total"`
}

// PaymentFilter drives the list query.
type PaymentFilter struct {
	PayableType string
	PayableID   *int64
	Status      string
	Method      string
	Limit       int
	Offset      int
}

// --- request DTOs ---

type RecordPaymentRequest struct {
	PayableType string  `json:"payable_type"` // order | booking
	PayableID   int64   `json:"payable_id"`
	Method      string  `json:"method"`
	Amount      *string `json:"amount"`    // optional; defaults to the target's total
	Reference   *string `json:"reference"` // transfer ref, receipt no, etc.
	Note        *string `json:"note"`
}
