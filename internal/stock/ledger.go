package stock

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// Movement is one entry in the stock ledger. It is written by whoever moves the
// stock, inside the same transaction as the `product_variants.stock_quantity`
// update, so the ledger can never drift from the level it explains.
type Movement struct {
	ProductVariantID int64
	Department       string
	Reason           string
	Delta            int // signed; a zero delta is not a movement
	QuantityAfter    int
	ReferenceType    *string
	ReferenceID      *int64
	Note             *string
	CreatedBy        *int64 // admin who acted; nil for system moves
}

// ReferenceOrder is the only reference kind so far.
const ReferenceOrder = "order"

// OrderMovement builds the reservation (negative delta) or release (positive
// delta) entry the orders module records at checkout, cancellation, and pickup
// expiry. Exported so the reason/reference shape stays owned by this package.
func OrderMovement(variantID int64, department string, delta, quantityAfter int, orderID int64) Movement {
	reason := ReasonOrderReserve
	if delta > 0 {
		reason = ReasonOrderRelease
	}
	ref := ReferenceOrder
	return Movement{
		ProductVariantID: variantID,
		Department:       department,
		Reason:           reason,
		Delta:            delta,
		QuantityAfter:    quantityAfter,
		ReferenceType:    &ref,
		ReferenceID:      &orderID,
	}
}

// RecordTx appends one movement. A zero delta is silently skipped rather than
// rejected: callers loop over order lines and an empty line is not an error.
func RecordTx(ctx context.Context, tx sqlx.ExecerContext, m Movement) error {
	if m.Delta == 0 {
		return nil
	}
	var department any
	if m.Department != "" {
		department = m.Department
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO stock_movements
		 (product_variant_id, department, reason, delta, quantity_after, reference_type, reference_id, note, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ProductVariantID, department, m.Reason, m.Delta, m.QuantityAfter,
		m.ReferenceType, m.ReferenceID, m.Note, m.CreatedBy)
	return err
}
