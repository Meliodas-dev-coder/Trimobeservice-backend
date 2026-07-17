// Package orders is the e-commerce transaction module: per-user cart, checkout
// (delivery or on-site pickup), stock reservation, and the order lifecycle.
package orders

import "time"

// Fulfillment modes.
const (
	FulfillmentDelivery = "delivery"
	FulfillmentPickup   = "pickup"
)

// Order fulfillment lifecycle (distinct from payment).
const (
	StatusPending   = "pending"
	StatusConfirmed = "confirmed"
	StatusShipped   = "shipped"   // delivery only
	StatusDelivered = "delivered" // delivery only
	StatusPickedUp  = "picked_up" // pickup only
	StatusCancelled = "cancelled"
	StatusExpired   = "expired" // pickup hold lapsed
)

// Payment lifecycle — confirmed by an admin AFTER hand-over.
const (
	PaymentUnpaid   = "unpaid"
	PaymentPaid     = "paid"
	PaymentRefunded = "refunded"
)

// PickupHold is how long a pickup order reserves stock before it expires.
const PickupHold = 24 * time.Hour

// --- domain models ---

type Cart struct {
	ID        int64     `db:"id" json:"id"`
	UserID    int64     `db:"user_id" json:"user_id"`
	Status    string    `db:"status" json:"status"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

type CartItem struct {
	ID               int64 `db:"id"`
	CartID           int64 `db:"cart_id"`
	ProductVariantID int64 `db:"product_variant_id"`
	Quantity         int   `db:"quantity"`
}

// CartItemView is a cart line enriched with live product/variant data for display.
type CartItemView struct {
	ID               int64   `db:"id" json:"id"`
	ProductVariantID int64   `db:"product_variant_id" json:"product_variant_id"`
	ProductID        int64   `db:"product_id" json:"product_id"`
	ProductName      string  `db:"product_name" json:"product_name"`
	VariantLabel     *string `db:"variant_label" json:"variant_label,omitempty"`
	SKU              string  `db:"sku" json:"sku"`
	UnitPrice        string  `db:"unit_price" json:"unit_price"`
	Quantity         int     `db:"quantity" json:"quantity"`
	LineTotal        string  `db:"line_total" json:"line_total"`
	InStock          int     `db:"in_stock" json:"in_stock"`
	IsActive         bool    `db:"is_active" json:"is_active"`
}

type CartView struct {
	CartID    int64          `json:"cart_id"`
	Items     []CartItemView `json:"items"`
	Subtotal  string         `json:"subtotal"`
	ItemCount int            `json:"item_count"`
}

type Order struct {
	ID                int64      `db:"id" json:"id"`
	UserID            *int64     `db:"user_id" json:"user_id,omitempty"`             // nil for admin walk-in orders
	CustomerName      *string    `db:"customer_name" json:"customer_name,omitempty"` // account or walk-in name snapshot
	OrderNumber       string     `db:"order_number" json:"order_number"`
	FulfillmentType   string     `db:"fulfillment_type" json:"fulfillment_type"`
	Status            string     `db:"status" json:"status"`
	PaymentStatus     string     `db:"payment_status" json:"payment_status"`
	Subtotal          string     `db:"subtotal" json:"subtotal"`
	ShippingFee       string     `db:"shipping_fee" json:"shipping_fee"`
	Total             string     `db:"total" json:"total"`
	ReservedUntil     *time.Time `db:"reserved_until" json:"reserved_until,omitempty"`
	ShipRecipientName *string    `db:"ship_recipient_name" json:"ship_recipient_name,omitempty"`
	ShipPhone         *string    `db:"ship_phone" json:"ship_phone,omitempty"`
	ShipLine1         *string    `db:"ship_line1" json:"ship_line1,omitempty"`
	ShipLine2         *string    `db:"ship_line2" json:"ship_line2,omitempty"`
	ShipCity          *string    `db:"ship_city" json:"ship_city,omitempty"`
	ShipRegion        *string    `db:"ship_region" json:"ship_region,omitempty"`
	ShipCountry       *string    `db:"ship_country" json:"ship_country,omitempty"`
	ShipPostalCode    *string    `db:"ship_postal_code" json:"ship_postal_code,omitempty"`
	ShipLatitude      *float64   `db:"ship_latitude" json:"ship_latitude,omitempty"`
	ShipLongitude     *float64   `db:"ship_longitude" json:"ship_longitude,omitempty"`
	ShipLocationRef   *string    `db:"ship_location_reference" json:"ship_location_reference,omitempty"`
	Note              *string    `db:"note" json:"note,omitempty"`
	PlacedAt          *time.Time `db:"placed_at" json:"placed_at,omitempty"`
	PaidAt            *time.Time `db:"paid_at" json:"paid_at,omitempty"`
	CreatedAt         time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time  `db:"updated_at" json:"updated_at"`
}

type OrderItem struct {
	ID               int64     `db:"id" json:"id"`
	OrderID          int64     `db:"order_id" json:"order_id"`
	ProductVariantID *int64    `db:"product_variant_id" json:"product_variant_id,omitempty"`
	ProductName      string    `db:"product_name" json:"product_name"`
	VariantLabel     *string   `db:"variant_label" json:"variant_label,omitempty"`
	SKU              *string   `db:"sku" json:"sku,omitempty"`
	UnitPrice        string    `db:"unit_price" json:"unit_price"`
	Quantity         int       `db:"quantity" json:"quantity"`
	LineTotal        string    `db:"line_total" json:"line_total"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
}

type OrderDetail struct {
	Order
	Items []OrderItem `json:"items"`
}

// OrderFilter drives order list queries (admin and per-user).
type OrderFilter struct {
	UserID          *int64 // scope to a single customer (client list)
	Status          string
	PaymentStatus   string
	FulfillmentType string
	Limit           int
	Offset          int
}

// variantStock is the locked/read subset of a variant used during checkout.
type variantStock struct {
	ID            int64   `db:"id"`
	SKU           string  `db:"sku"`
	Label         *string `db:"label"`
	Price         string  `db:"price"`
	StockQuantity int     `db:"stock_quantity"`
	IsActive      bool    `db:"is_active"`
	ProductID     int64   `db:"product_id"`
	ProductName   string  `db:"product_name"`
}

// --- request DTOs ---

type AddCartItemRequest struct {
	ProductVariantID int64 `json:"product_variant_id"`
	Quantity         int   `json:"quantity"`
}

type UpdateCartItemRequest struct {
	Quantity int `json:"quantity"`
}

type ShippingAddress struct {
	RecipientName     string   `json:"recipient_name"`
	Phone             string   `json:"phone"`
	Line1             string   `json:"line1"`
	Line2             *string  `json:"line2"`
	City              string   `json:"city"`
	Region            *string  `json:"region"`
	Country           string   `json:"country"`
	PostalCode        *string  `json:"postal_code"`
	Latitude          *float64 `json:"latitude"`
	Longitude         *float64 `json:"longitude"`
	LocationReference *string  `json:"location_reference"`
}

type CheckoutRequest struct {
	FulfillmentType string           `json:"fulfillment_type"`
	ShippingAddress *ShippingAddress `json:"shipping_address"` // required for delivery, ignored for pickup
	Note            *string          `json:"note"`
}

type UpdateStatusRequest struct {
	Status string `json:"status"`
}

// OrderLine is one line item in an admin-created order.
type OrderLine struct {
	ProductVariantID int64 `json:"product_variant_id"`
	Quantity         int   `json:"quantity"`
}

// AdminCreateOrderRequest is an admin manually creating an order for a phone or
// walk-in customer (with or without a linked account).
type AdminCreateOrderRequest struct {
	CustomerName    string           `json:"customer_name"`
	UserID          *int64           `json:"user_id"` // optional linked account
	FulfillmentType string           `json:"fulfillment_type"`
	Items           []OrderLine      `json:"items"`
	ShippingAddress *ShippingAddress `json:"shipping_address"` // required for delivery
	Note            *string          `json:"note"`
}
