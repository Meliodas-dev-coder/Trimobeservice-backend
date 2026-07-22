// Package stock is the inventory back office for the catalog departments
// (tech / fashion / coffee). `product_variants.stock_quantity` remains the
// single source of truth for what is on hand; this module explains how it got
// there via the `stock_movements` ledger, lets an admin restock or correct a
// SKU, and drives the low-stock queues on the department overview.
//
// Reservation and release movements are written by the orders module inside the
// checkout/cancel transaction (see ledger.go); everything here is the read side
// plus deliberate manual adjustments.
package stock

import "time"

// Movement reasons. The first four are admin-driven, the last two are written
// by the orders module when stock is reserved at checkout or released again.
const (
	ReasonInitial      = "initial"
	ReasonRestock      = "restock"
	ReasonManualAdjust = "manual_adjust"
	ReasonCorrection   = "correction"
	ReasonOrderReserve = "order_reserve"
	ReasonOrderRelease = "order_release"
)

// Levels bucket a SKU for the stock screen and the low-stock queues. A SKU with
// no threshold set is only ever `out` or `ok`.
const (
	LevelOut = "out"
	LevelLow = "low"
	LevelOK  = "ok"
)

// Adjustment modes: `adjust` applies a signed delta, `set` counts the shelf and
// states the resulting quantity (the delta is derived).
const (
	ModeAdjust = "adjust"
	ModeSet    = "set"
)

// levelExpr buckets a variant in SQL. Kept in one place so the list, the
// summary counters, and the low-stock queue can never disagree.
const levelExpr = `CASE
	WHEN pv.stock_quantity <= 0 THEN 'out'
	WHEN pv.reorder_threshold > 0 AND pv.stock_quantity <= pv.reorder_threshold THEN 'low'
	ELSE 'ok' END`

// Item is one sellable SKU as the stock screen shows it.
type Item struct {
	VariantID        int64      `db:"variant_id" json:"variant_id"`
	ProductID        int64      `db:"product_id" json:"product_id"`
	ProductName      string     `db:"product_name" json:"product_name"`
	ProductSlug      string     `db:"product_slug" json:"product_slug"`
	CategoryName     string     `db:"category_name" json:"category_name"`
	BrandName        *string    `db:"brand_name" json:"brand_name,omitempty"`
	TemplateKey      string     `db:"template_key" json:"-"`
	Department       string     `db:"-" json:"department"`
	SKU              string     `db:"sku" json:"sku"`
	Label            *string    `db:"label" json:"label,omitempty"`
	Price            string     `db:"price" json:"price"`
	StockQuantity    int        `db:"stock_quantity" json:"stock_quantity"`
	ReorderThreshold int        `db:"reorder_threshold" json:"reorder_threshold"`
	Level            string     `db:"level" json:"level"`
	StockValue       string     `db:"stock_value" json:"stock_value"`
	IsActive         bool       `db:"is_active" json:"is_active"`
	ProductIsActive  bool       `db:"product_is_active" json:"product_is_active"`
	PrimaryImageURL  *string    `db:"primary_image_url" json:"primary_image_url,omitempty"`
	LastMovementAt   *time.Time `db:"last_movement_at" json:"last_movement_at,omitempty"`
	UpdatedAt        time.Time  `db:"updated_at" json:"updated_at"`
	// Movements is the recent ledger tail, attached only on a single-SKU read so
	// the detail screen can show why the level is what it is.
	Movements []MovementView `db:"-" json:"movements,omitempty"`
}

// Filter drives the SKU list. Departments is the caller's authorized scope and
// is always applied; Department is the narrower filter the admin picked.
type Filter struct {
	Departments []string
	Department  string
	Level       string
	Query       string
	ProductID   *int64
	Limit       int
	Offset      int
}

// Summary is the KPI header of the stock screen and feeds the department
// overview. Counts respect the same authorized scope as the list.
type Summary struct {
	SKUCount       int    `json:"sku_count"`
	ActiveSKUCount int    `json:"active_sku_count"`
	ProductCount   int    `json:"product_count"`
	UnitsOnHand    int    `json:"units_on_hand"`
	StockValue     string `json:"stock_value"`
	LowStock       int    `json:"low_stock"`
	OutOfStock     int    `json:"out_of_stock"`
}

// MovementView is one ledger row joined to the SKU and the admin who wrote it.
type MovementView struct {
	ID               int64     `db:"id" json:"id"`
	ProductVariantID int64     `db:"product_variant_id" json:"product_variant_id"`
	ProductName      string    `db:"product_name" json:"product_name"`
	SKU              string    `db:"sku" json:"sku"`
	Label            *string   `db:"label" json:"label,omitempty"`
	Reason           string    `db:"reason" json:"reason"`
	Delta            int       `db:"delta" json:"delta"`
	QuantityAfter    int       `db:"quantity_after" json:"quantity_after"`
	ReferenceType    *string   `db:"reference_type" json:"reference_type,omitempty"`
	ReferenceID      *int64    `db:"reference_id" json:"reference_id,omitempty"`
	OrderNumber      *string   `db:"order_number" json:"order_number,omitempty"`
	Note             *string   `db:"note" json:"note,omitempty"`
	CreatedByName    *string   `db:"created_by_name" json:"created_by_name,omitempty"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
}

// --- request DTOs ---

// AdjustRequest restocks, corrects, or recounts one SKU. In `adjust` mode
// Quantity is a signed delta; in `set` mode it is the counted shelf quantity.
type AdjustRequest struct {
	Mode     string  `json:"mode"`
	Quantity int     `json:"quantity"`
	Reason   string  `json:"reason"`
	Note     *string `json:"note"`
}

type ThresholdRequest struct {
	ReorderThreshold int `json:"reorder_threshold"`
}
