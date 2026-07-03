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
		`SELECT status, payment_status, total_price AS total FROM bookings WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTargetNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
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
	q := `UPDATE bookings SET payment_status = ? WHERE id = ?`
	if status == targetPaid {
		q = `UPDATE bookings SET payment_status = ?, paid_at = NOW() WHERE id = ?`
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
