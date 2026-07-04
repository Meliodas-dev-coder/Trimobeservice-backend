package bookings

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var (
	ErrBookingNotFound = errors.New("booking not found")
	ErrCarMissing      = errors.New("car not found")
	ErrDriverMissing   = errors.New("driver not found")
	ErrCustomerMissing = errors.New("customer not found")
)

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

// --- car / driver lookups ---

// LockCar reads a car FOR UPDATE, serializing concurrent bookings for that car.
func (r *Repository) LockCar(ctx context.Context, tx *sqlx.Tx, carID int64) (*carRow, error) {
	var c carRow
	err := sqlx.GetContext(ctx, tx, &c,
		`SELECT c.id, c.name, c.daily_rate, c.status, c.category_id,
		        cc.is_cargo_transport, cc.cargo_per_km_rate, cc.cargo_minimum_rate
		   FROM cars c
		   JOIN car_categories cc ON cc.id = c.category_id
		  WHERE c.id = ? FOR UPDATE`, carID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCarMissing
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) GetCar(ctx context.Context, carID int64) (*carRow, error) {
	var c carRow
	err := r.db.GetContext(ctx, &c,
		`SELECT c.id, c.name, c.daily_rate, c.status, c.category_id,
		        cc.is_cargo_transport, cc.cargo_per_km_rate, cc.cargo_minimum_rate
		   FROM cars c
		   JOIN car_categories cc ON cc.id = c.category_id
		  WHERE c.id = ?`, carID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCarMissing
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) CategoryName(ctx context.Context, q sqlx.QueryerContext, categoryID int64) (*string, error) {
	var name string
	err := sqlx.GetContext(ctx, q, &name, `SELECT name FROM car_categories WHERE id = ?`, categoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &name, nil
}

func (r *Repository) CustomerName(ctx context.Context, q sqlx.QueryerContext, userID int64) (*string, error) {
	var name string
	err := sqlx.GetContext(ctx, q, &name, `SELECT full_name FROM users WHERE id = ? AND role = 'customer'`, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCustomerMissing
	}
	if err != nil {
		return nil, err
	}
	return &name, nil
}

func (r *Repository) GetDriver(ctx context.Context, q sqlx.QueryerContext, driverID int64) (*driverRow, error) {
	var d driverRow
	err := sqlx.GetContext(ctx, q, &d,
		`SELECT id, full_name, phone, status FROM drivers WHERE id = ?`, driverID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDriverMissing
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *Repository) DriverInfo(ctx context.Context, driverID int64) (*DriverInfo, error) {
	var d DriverInfo
	err := r.db.GetContext(ctx, &d, `SELECT id, full_name, phone FROM drivers WHERE id = ?`, driverID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDriverMissing
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// --- overlap checks (occupying statuses must mirror the DB triggers) ---

func (r *Repository) HasCarOverlap(ctx context.Context, q sqlx.QueryerContext, carID, excludeID int64, start, end time.Time) (bool, error) {
	var n int
	err := sqlx.GetContext(ctx, q, &n,
		`SELECT COUNT(*) FROM bookings
		 WHERE car_id = ? AND id <> ?
		   AND status IN ('confirmed','driver_assigned','active')
		   AND start_at < ? AND end_at > ?`,
		carID, excludeID, end, start)
	return n > 0, err
}

func (r *Repository) HasDriverOverlap(ctx context.Context, q sqlx.QueryerContext, driverID, excludeID int64, start, end time.Time) (bool, error) {
	var n int
	err := sqlx.GetContext(ctx, q, &n,
		`SELECT COUNT(*) FROM bookings
		 WHERE driver_id = ? AND id <> ?
		   AND status IN ('confirmed','driver_assigned','active')
		   AND start_at < ? AND end_at > ?`,
		driverID, excludeID, end, start)
	return n > 0, err
}

// --- booking writes ---

func (r *Repository) InsertBooking(ctx context.Context, tx *sqlx.Tx, b *Booking) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO bookings (user_id, customer_name, car_id, driver_id, booking_number, status, payment_status,
		 start_at, end_at, days, daily_rate_snapshot, fees, total_price,
		 pricing_model, distance_km, cargo_per_km_rate_snapshot, cargo_minimum_rate_snapshot,
		 car_name, car_category, pickup_location, dropoff_location, contact_phone, note)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.UserID, b.CustomerName, b.CarID, b.DriverID, b.BookingNumber, b.Status, b.PaymentStatus,
		b.StartAt, b.EndAt, b.Days, b.DailyRateSnapshot, b.Fees, b.TotalPrice,
		b.PricingModel, b.DistanceKm, b.CargoPerKmRate, b.CargoMinimumRate,
		b.CarName, b.CarCategory, b.PickupLocation, b.DropoffLocation, b.ContactPhone, b.Note)
	if err != nil {
		return 0, mapTriggerErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) AssignDriverTx(ctx context.Context, tx *sqlx.Tx, id, driverID int64) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE bookings SET driver_id = ?, status = 'driver_assigned' WHERE id = ?`, driverID, id)
	if err != nil {
		return mapTriggerErr(err)
	}
	return notFoundIfNoRows(res, ErrBookingNotFound)
}

// CarOverlaps is the non-transactional (read-only) availability check.
func (r *Repository) CarOverlaps(ctx context.Context, carID, excludeID int64, start, end time.Time) (bool, error) {
	return r.HasCarOverlap(ctx, r.db, carID, excludeID, start, end)
}

func (r *Repository) SetStatus(ctx context.Context, id int64, status string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE bookings SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return mapTriggerErr(err)
	}
	return notFoundIfNoRows(res, ErrBookingNotFound)
}

func (r *Repository) MarkPaid(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE bookings SET payment_status = 'paid', paid_at = NOW() WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return notFoundIfNoRows(res, ErrBookingNotFound)
}

// --- booking reads ---

const bookingCols = `id, user_id,
	COALESCE(customer_name, (SELECT full_name FROM users u WHERE u.id = bookings.user_id)) AS customer_name,
	car_id, driver_id, booking_number, status, payment_status,
	start_at, end_at, days, daily_rate_snapshot, fees, total_price,
	pricing_model, distance_km, cargo_per_km_rate_snapshot, cargo_minimum_rate_snapshot,
	car_name, car_category, pickup_location, dropoff_location, contact_phone, note,
	paid_at, created_at, updated_at`

func (r *Repository) GetByID(ctx context.Context, id int64) (*Booking, error) {
	var b Booking
	err := r.db.GetContext(ctx, &b, `SELECT `+bookingCols+` FROM bookings WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBookingNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *Repository) GetForUser(ctx context.Context, userID, id int64) (*Booking, error) {
	var b Booking
	err := r.db.GetContext(ctx, &b,
		`SELECT `+bookingCols+` FROM bookings WHERE id = ? AND user_id = ?`, id, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBookingNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *Repository) List(ctx context.Context, f BookingFilter) ([]Booking, int, error) {
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
	if f.CarID != nil {
		where = append(where, "car_id = ?")
		args = append(args, *f.CarID)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM bookings`+clause, args...); err != nil {
		return nil, 0, err
	}

	listArgs := append(append([]any{}, args...), f.Limit, f.Offset)
	out := []Booking{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+bookingCols+` FROM bookings`+clause+` ORDER BY start_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// --- helpers ---

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

// mapTriggerErr converts the overlap trigger's SIGNAL (SQLSTATE 45000 →
// MySQL error 1644) into the module's ErrCarNotFree.
func mapTriggerErr(err error) error {
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1644 {
		return ErrCarNotFree
	}
	return err
}
