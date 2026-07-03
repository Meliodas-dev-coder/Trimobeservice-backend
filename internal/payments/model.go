// Package payments is the manual/offline payment ledger. An admin records a
// payment against an order or a booking (polymorphic); recording it also flips
// that target's payment_status. A real gateway later is just another `method`.
package payments

import "time"

// Payable targets.
const (
	PayableOrder   = "order"
	PayableBooking = "booking"
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
