package orders

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

var (
	ErrCartNotFound     = errors.New("cart not found")
	ErrCartItemNotFound = errors.New("cart item not found")
	ErrOrderNotFound    = errors.New("order not found")
	ErrVariantMissing   = errors.New("variant not found")
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// InTx runs fn inside a transaction, committing on success and rolling back on
// error or panic.
func (r *Repository) InTx(ctx context.Context, fn func(tx *sqlx.Tx) error) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// --- cart ---

func (r *Repository) GetOrCreateActiveCart(ctx context.Context, userID int64) (*Cart, error) {
	var c Cart
	err := r.db.GetContext(ctx, &c,
		`SELECT id, user_id, status, created_at, updated_at
		 FROM carts WHERE user_id = ? AND status = 'active' ORDER BY id DESC LIMIT 1`, userID)
	if err == nil {
		return &c, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO carts (user_id, status) VALUES (?, 'active')`, userID)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.getCartByID(ctx, id)
}

func (r *Repository) getCartByID(ctx context.Context, id int64) (*Cart, error) {
	var c Cart
	err := r.db.GetContext(ctx, &c,
		`SELECT id, user_id, status, created_at, updated_at FROM carts WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCartNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) ListCartItemViews(ctx context.Context, cartID int64) ([]CartItemView, error) {
	out := []CartItemView{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT ci.id, ci.product_variant_id, pv.product_id, p.name AS product_name,
		        pv.label AS variant_label, pv.sku, pv.price AS unit_price, ci.quantity,
		        (pv.price * ci.quantity) AS line_total, pv.stock_quantity AS in_stock, pv.is_active
		 FROM cart_items ci
		 JOIN product_variants pv ON pv.id = ci.product_variant_id
		 JOIN products p ON p.id = pv.product_id
		 WHERE ci.cart_id = ?
		 ORDER BY ci.id`, cartID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) ListCartItems(ctx context.Context, cartID int64) ([]CartItem, error) {
	out := []CartItem{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT id, cart_id, product_variant_id, quantity FROM cart_items WHERE cart_id = ? ORDER BY id`, cartID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetCartItem(ctx context.Context, id int64) (*CartItem, error) {
	var ci CartItem
	err := r.db.GetContext(ctx, &ci,
		`SELECT id, cart_id, product_variant_id, quantity FROM cart_items WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCartItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ci, nil
}

// GetVariantForCart reads variant availability data (no lock) for add-to-cart
// validation.
func (r *Repository) GetVariantForCart(ctx context.Context, variantID int64) (*variantStock, error) {
	var vs variantStock
	err := r.db.GetContext(ctx, &vs,
		`SELECT pv.id, pv.sku, pv.label, pv.price, pv.stock_quantity, pv.is_active,
		        pv.product_id, p.name AS product_name
		 FROM product_variants pv JOIN products p ON p.id = pv.product_id
		 WHERE pv.id = ?`, variantID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrVariantMissing
	}
	if err != nil {
		return nil, err
	}
	return &vs, nil
}

func (r *Repository) AddCartItem(ctx context.Context, cartID, variantID int64, qty int) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO cart_items (cart_id, product_variant_id, quantity)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE quantity = quantity + VALUES(quantity)`,
		cartID, variantID, qty)
	return err
}

func (r *Repository) UpdateCartItemQty(ctx context.Context, id int64, qty int) error {
	res, err := r.db.ExecContext(ctx, `UPDATE cart_items SET quantity = ? WHERE id = ?`, qty, id)
	if err != nil {
		return err
	}
	return notFoundIfNoRows(res, ErrCartItemNotFound)
}

func (r *Repository) DeleteCartItem(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM cart_items WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return notFoundIfNoRows(res, ErrCartItemNotFound)
}

func (r *Repository) ClearCart(ctx context.Context, cartID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM cart_items WHERE cart_id = ?`, cartID)
	return err
}

// --- checkout primitives (transaction-scoped) ---

// LockVariant reads a variant FOR UPDATE so concurrent checkouts serialize on it.
func (r *Repository) LockVariant(ctx context.Context, tx *sqlx.Tx, variantID int64) (*variantStock, error) {
	var vs variantStock
	err := sqlx.GetContext(ctx, tx, &vs,
		`SELECT pv.id, pv.sku, pv.label, pv.price, pv.stock_quantity, pv.is_active,
		        pv.product_id, p.name AS product_name
		 FROM product_variants pv JOIN products p ON p.id = pv.product_id
		 WHERE pv.id = ? FOR UPDATE`, variantID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrVariantMissing
	}
	if err != nil {
		return nil, err
	}
	return &vs, nil
}

func (r *Repository) AdjustStock(ctx context.Context, tx *sqlx.Tx, variantID int64, delta int) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE product_variants SET stock_quantity = stock_quantity + ? WHERE id = ?`, delta, variantID)
	return err
}

func (r *Repository) InsertOrder(ctx context.Context, tx *sqlx.Tx, o *Order) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO orders (user_id, customer_name, order_number, fulfillment_type, status, payment_status,
		 subtotal, shipping_fee, total, reserved_until,
		 ship_recipient_name, ship_phone, ship_line1, ship_line2, ship_city, ship_region, ship_country, ship_postal_code,
		 note, placed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.UserID, o.CustomerName, o.OrderNumber, o.FulfillmentType, o.Status, o.PaymentStatus,
		o.Subtotal, o.ShippingFee, o.Total, o.ReservedUntil,
		o.ShipRecipientName, o.ShipPhone, o.ShipLine1, o.ShipLine2, o.ShipCity, o.ShipRegion, o.ShipCountry, o.ShipPostalCode,
		o.Note, o.PlacedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) InsertOrderItem(ctx context.Context, tx *sqlx.Tx, it *OrderItem) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO order_items (order_id, product_variant_id, product_name, variant_label, sku, unit_price, quantity, line_total)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		it.OrderID, it.ProductVariantID, it.ProductName, it.VariantLabel, it.SKU, it.UnitPrice, it.Quantity, it.LineTotal)
	return err
}

func (r *Repository) MarkCartConverted(ctx context.Context, tx *sqlx.Tx, cartID int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM cart_items WHERE cart_id = ?`, cartID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE carts SET status = 'converted' WHERE id = ?`, cartID)
	return err
}

func (r *Repository) ListOrderItemsTx(ctx context.Context, tx *sqlx.Tx, orderID int64) ([]OrderItem, error) {
	out := []OrderItem{}
	err := sqlx.SelectContext(ctx, tx, &out,
		`SELECT `+orderItemCols+` FROM order_items WHERE order_id = ? ORDER BY id`, orderID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) SetOrderStatusTx(ctx context.Context, tx *sqlx.Tx, orderID int64, status string) error {
	_, err := tx.ExecContext(ctx, `UPDATE orders SET status = ? WHERE id = ?`, status, orderID)
	return err
}

// --- order reads / simple writes ---

const orderCols = `id, user_id, customer_name, order_number, fulfillment_type, status, payment_status,
	subtotal, shipping_fee, total, reserved_until,
	ship_recipient_name, ship_phone, ship_line1, ship_line2, ship_city, ship_region, ship_country, ship_postal_code,
	note, placed_at, paid_at, created_at, updated_at`

const orderItemCols = `id, order_id, product_variant_id, product_name, variant_label, sku, unit_price, quantity, line_total, created_at`

func (r *Repository) ListOrders(ctx context.Context, f OrderFilter) ([]Order, int, error) {
	var where []string
	var args []any
	if f.UserID != nil {
		where = append(where, "user_id = ?")
		args = append(args, *f.UserID)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.PaymentStatus != "" {
		where = append(where, "payment_status = ?")
		args = append(args, f.PaymentStatus)
	}
	if f.FulfillmentType != "" {
		where = append(where, "fulfillment_type = ?")
		args = append(args, f.FulfillmentType)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM orders`+clause, args...); err != nil {
		return nil, 0, err
	}

	listArgs := append(append([]any{}, args...), f.Limit, f.Offset)
	out := []Order{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+orderCols+` FROM orders`+clause+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) GetOrderByID(ctx context.Context, id int64) (*Order, error) {
	var o Order
	err := r.db.GetContext(ctx, &o, `SELECT `+orderCols+` FROM orders WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *Repository) GetOrderForUser(ctx context.Context, userID, id int64) (*Order, error) {
	var o Order
	err := r.db.GetContext(ctx, &o,
		`SELECT `+orderCols+` FROM orders WHERE id = ? AND user_id = ?`, id, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *Repository) ListOrderItems(ctx context.Context, orderID int64) ([]OrderItem, error) {
	out := []OrderItem{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+orderItemCols+` FROM order_items WHERE order_id = ? ORDER BY id`, orderID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) SetOrderStatus(ctx context.Context, id int64, status string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE orders SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return err
	}
	return notFoundIfNoRows(res, ErrOrderNotFound)
}

func (r *Repository) MarkPaid(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE orders SET payment_status = 'paid', paid_at = NOW() WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return notFoundIfNoRows(res, ErrOrderNotFound)
}

// FindExpiredPickups returns ids of pending pickup orders whose hold has lapsed.
func (r *Repository) FindExpiredPickups(ctx context.Context, now time.Time) ([]int64, error) {
	ids := []int64{}
	err := r.db.SelectContext(ctx, &ids,
		`SELECT id FROM orders
		 WHERE fulfillment_type = 'pickup' AND status = 'pending'
		   AND reserved_until IS NOT NULL AND reserved_until < ?`, now)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func notFoundIfNoRows(res sql.Result, notFound error) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return notFound
	}
	return nil
}
