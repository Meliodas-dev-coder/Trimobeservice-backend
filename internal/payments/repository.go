package payments

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jmoiron/sqlx"
)

var (
	ErrTargetNotFound  = errors.New("order or booking not found")
	ErrPaymentNotFound = errors.New("payment not found")
)

// targetInfo is the payment-relevant state of an order or booking.
type targetInfo struct {
	Status        string `db:"status"`
	PaymentStatus string `db:"payment_status"`
	Total         string `db:"total"`
}

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

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

// --- target lookups ---

func (r *Repository) GetOrderInfo(ctx context.Context, id int64) (*targetInfo, error) {
	var t targetInfo
	err := r.db.GetContext(ctx, &t,
		`SELECT status, payment_status, total FROM orders WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTargetNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) GetBookingInfo(ctx context.Context, id int64) (*targetInfo, error) {
	var t targetInfo
	err := r.db.GetContext(ctx, &t,
		`SELECT b.status, b.payment_status, COALESCE(bg.total_price, b.total_price) AS total
		   FROM bookings b
		   LEFT JOIN booking_groups bg ON bg.booking_id = b.booking_group_id
		  WHERE b.id = ?
		    AND (b.booking_group_id IS NULL OR b.id = b.booking_group_id)`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTargetNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) GetEventInfo(ctx context.Context, id int64) (*targetInfo, error) {
	var t targetInfo
	// quoted_price is NULL until an admin sets a quote; expose that as "" so the
	// service can reject payment on an unquoted event.
	err := r.db.GetContext(ctx, &t,
		`SELECT status, payment_status, COALESCE(quoted_price, '') AS total FROM event_requests WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTargetNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) GetHealthcareInfo(ctx context.Context, id int64) (*targetInfo, error) {
	var t targetInfo
	// quoted_price is NULL until priced (packages seed it at creation, an admin
	// quotes consultations); expose that as "" so payment is blocked until set.
	err := r.db.GetContext(ctx, &t,
		`SELECT status, payment_status, COALESCE(quoted_price, '') AS total FROM healthcare_requests WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTargetNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) GetTarget(ctx context.Context, payableType string, id int64) (*PaymentTarget, error) {
	switch payableType {
	case PayableOrder:
		target, err := r.getOrderTarget(ctx, id)
		if err != nil {
			return nil, err
		}
		items, err := r.listOrderTargetItems(ctx, id)
		if err != nil {
			return nil, err
		}
		target.Items = items
		return target, nil
	case PayableBooking:
		return r.getBookingTarget(ctx, id)
	case PayableEvent:
		return r.getEventTarget(ctx, id)
	case PayableHealthcare:
		return r.getHealthcareTarget(ctx, id)
	default:
		return nil, ErrTargetNotFound
	}
}

func (r *Repository) getOrderTarget(ctx context.Context, id int64) (*PaymentTarget, error) {
	var target PaymentTarget
	err := r.db.GetContext(ctx, &target,
		`SELECT
			'order' AS target_type,
			id,
			user_id,
			COALESCE(customer_name, (SELECT full_name FROM users u WHERE u.id = orders.user_id)) AS customer_name,
			order_number AS number,
			status,
			payment_status,
			total,
			fulfillment_type,
			NULL AS car_name,
			NULL AS car_category,
			NULL AS start_at,
			NULL AS end_at,
			NULL AS pickup_location,
			NULL AS pickup_latitude,
			NULL AS pickup_longitude,
			NULL AS pickup_reference,
			NULL AS dropoff_location,
			NULL AS dropoff_latitude,
			NULL AS dropoff_longitude,
			NULL AS dropoff_reference,
			ship_phone AS contact_phone,
			created_at
		   FROM orders
		  WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTargetNotFound
	}
	if err != nil {
		return nil, err
	}
	return &target, nil
}

func (r *Repository) getBookingTarget(ctx context.Context, id int64) (*PaymentTarget, error) {
	var target PaymentTarget
	err := r.db.GetContext(ctx, &target,
		`SELECT
			'booking' AS target_type,
			b.id,
			b.user_id,
			COALESCE(b.customer_name, (SELECT full_name FROM users u WHERE u.id = b.user_id)) AS customer_name,
			COALESCE(bg.booking_number, b.booking_number) AS number,
			b.status,
			b.payment_status,
			COALESCE(bg.total_price, b.total_price) AS total,
			NULL AS fulfillment_type,
			CASE WHEN bg.booking_id IS NOT NULL
				THEN CONCAT((SELECT COUNT(*) FROM bookings bi WHERE bi.booking_group_id = bg.booking_id), ' cars')
				ELSE b.car_name END AS car_name,
			CASE WHEN bg.booking_id IS NOT NULL THEN NULL ELSE b.car_category END AS car_category,
			b.start_at,
			b.end_at,
			b.pickup_location,
			b.pickup_latitude,
			b.pickup_longitude,
			b.pickup_reference,
			b.dropoff_location,
			b.dropoff_latitude,
			b.dropoff_longitude,
			b.dropoff_reference,
			b.contact_phone,
			b.created_at
		   FROM bookings b
		   LEFT JOIN booking_groups bg ON bg.booking_id = b.booking_group_id
		  WHERE b.id = ?
		    AND (b.booking_group_id IS NULL OR b.id = b.booking_group_id)`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTargetNotFound
	}
	if err != nil {
		return nil, err
	}
	return &target, nil
}

func (r *Repository) getEventTarget(ctx context.Context, id int64) (*PaymentTarget, error) {
	var target PaymentTarget
	err := r.db.GetContext(ctx, &target,
		`SELECT
			'event' AS target_type,
			id,
			user_id,
			COALESCE(customer_name, (SELECT full_name FROM users u WHERE u.id = event_requests.user_id)) AS customer_name,
			request_number AS number,
			status,
			payment_status,
			COALESCE(quoted_price, 0) AS total,
			NULL AS fulfillment_type,
			NULL AS car_name,
			NULL AS car_category,
			event_type,
			event_start AS start_at,
			event_end AS end_at,
			location AS pickup_location,
			location_latitude AS pickup_latitude,
			location_longitude AS pickup_longitude,
			location_reference AS pickup_reference,
			NULL AS dropoff_location,
			NULL AS dropoff_latitude,
			NULL AS dropoff_longitude,
			NULL AS dropoff_reference,
			contact_phone,
			created_at
		   FROM event_requests
		  WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTargetNotFound
	}
	if err != nil {
		return nil, err
	}
	return &target, nil
}

func (r *Repository) getHealthcareTarget(ctx context.Context, id int64) (*PaymentTarget, error) {
	var target PaymentTarget
	err := r.db.GetContext(ctx, &target,
		`SELECT
			'healthcare' AS target_type,
			id,
			user_id,
			COALESCE(customer_name, (SELECT full_name FROM users u WHERE u.id = healthcare_requests.user_id)) AS customer_name,
			request_number AS number,
			status,
			payment_status,
			COALESCE(quoted_price, 0) AS total,
			NULL AS fulfillment_type,
			service_name AS car_name,
			category_name AS car_category,
			request_type AS event_type,
			COALESCE(start_at, preferred_at) AS start_at,
			end_at,
			address AS pickup_location,
			location_latitude AS pickup_latitude,
			location_longitude AS pickup_longitude,
			location_reference AS pickup_reference,
			NULL AS dropoff_location,
			NULL AS dropoff_latitude,
			NULL AS dropoff_longitude,
			NULL AS dropoff_reference,
			contact_phone,
			created_at
		   FROM healthcare_requests
		  WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTargetNotFound
	}
	if err != nil {
		return nil, err
	}
	return &target, nil
}

func (r *Repository) listOrderTargetItems(ctx context.Context, orderID int64) ([]PaymentTargetItem, error) {
	out := []PaymentTargetItem{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT id, product_name, variant_label, sku, unit_price, quantity, line_total
		   FROM order_items
		  WHERE order_id = ?
		  ORDER BY id`, orderID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// --- writes (transaction-scoped) ---

func (r *Repository) InsertPaymentTx(ctx context.Context, tx *sqlx.Tx, p *Payment) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO payments (payable_type, payable_id, amount, method, status, reference, marked_paid_by, marked_paid_at, note)
		 VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), ?)`,
		p.PayableType, p.PayableID, p.Amount, p.Method, p.Status, p.Reference, p.MarkedPaidBy, p.Note)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) SetOrderPaymentTx(ctx context.Context, tx *sqlx.Tx, id int64, status string) error {
	q := `UPDATE orders SET payment_status = ? WHERE id = ?`
	if status == targetPaid {
		q = `UPDATE orders SET payment_status = ?, paid_at = NOW() WHERE id = ?`
	}
	_, err := tx.ExecContext(ctx, q, status, id)
	return err
}

func (r *Repository) SetBookingPaymentTx(ctx context.Context, tx *sqlx.Tx, id int64, status string) error {
	q := `UPDATE bookings SET payment_status = ? WHERE id = ? OR booking_group_id = ?`
	if status == targetPaid {
		q = `UPDATE bookings SET payment_status = ?, paid_at = NOW() WHERE id = ? OR booking_group_id = ?`
	}
	res, err := tx.ExecContext(ctx, q, status, id, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrTargetNotFound
	}
	return nil
}

func (r *Repository) SetEventPaymentTx(ctx context.Context, tx *sqlx.Tx, id int64, status string) error {
	q := `UPDATE event_requests SET payment_status = ? WHERE id = ?`
	if status == targetPaid {
		q = `UPDATE event_requests SET payment_status = ?, paid_at = NOW() WHERE id = ?`
	}
	_, err := tx.ExecContext(ctx, q, status, id)
	return err
}

func (r *Repository) SetHealthcarePaymentTx(ctx context.Context, tx *sqlx.Tx, id int64, status string) error {
	q := `UPDATE healthcare_requests SET payment_status = ? WHERE id = ?`
	if status == targetPaid {
		q = `UPDATE healthcare_requests SET payment_status = ?, paid_at = NOW() WHERE id = ?`
	}
	_, err := tx.ExecContext(ctx, q, status, id)
	return err
}

func (r *Repository) SetPaymentStatusTx(ctx context.Context, tx *sqlx.Tx, id int64, status string) error {
	_, err := tx.ExecContext(ctx, `UPDATE payments SET status = ? WHERE id = ?`, status, id)
	return err
}

// --- reads ---

const paymentCols = `id, payable_type, payable_id, amount, method, status, reference,
	marked_paid_by, marked_paid_at, note, created_at, updated_at`

func (r *Repository) GetByID(ctx context.Context, id int64) (*Payment, error) {
	var p Payment
	err := r.db.GetContext(ctx, &p, `SELECT `+paymentCols+` FROM payments WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPaymentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) List(ctx context.Context, f PaymentFilter) ([]Payment, int, error) {
	var where []string
	var args []any
	if f.PayableType != "" {
		where = append(where, "payable_type = ?")
		args = append(args, f.PayableType)
	}
	if f.PayableID != nil {
		where = append(where, "payable_id = ?")
		args = append(args, *f.PayableID)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.Method != "" {
		where = append(where, "method = ?")
		args = append(args, f.Method)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM payments`+clause, args...); err != nil {
		return nil, 0, err
	}

	listArgs := append(append([]any{}, args...), f.Limit, f.Offset)
	out := []Payment{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+paymentCols+` FROM payments`+clause+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}
