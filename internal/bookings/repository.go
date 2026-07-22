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

func (r *Repository) ListBookingCarOptions(ctx context.Context) ([]BookingCarOption, error) {
	items := []BookingCarOption{}
	err := r.db.SelectContext(ctx, &items, `
		SELECT c.id,c.name,cc.name AS category_name,c.status
		FROM cars c JOIN car_categories cc ON cc.id=c.category_id
		ORDER BY c.name`)
	return items, err
}

func (r *Repository) ListBookingDriverOptions(ctx context.Context) ([]BookingDriverOption, error) {
	items := []BookingDriverOption{}
	err := r.db.SelectContext(ctx, &items, `
		SELECT id,full_name,status FROM drivers ORDER BY full_name`)
	return items, err
}

// --- car / driver lookups ---

// LockCar reads a car FOR UPDATE, serializing concurrent bookings for that car.
func (r *Repository) LockCar(ctx context.Context, tx *sqlx.Tx, carID int64) (*carRow, error) {
	var c carRow
	err := sqlx.GetContext(ctx, tx, &c,
		`SELECT c.id, c.name, c.daily_rate, c.outside_antananarivo_daily_rate, c.status, c.category_id,
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
		`SELECT c.id, c.name, c.daily_rate, c.outside_antananarivo_daily_rate, c.status, c.category_id,
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
		`INSERT INTO bookings (booking_group_id, user_id, customer_name, car_id, driver_id, booking_number, status, payment_status,
		 start_at, end_at, days, daily_rate_snapshot, outside_antananarivo, fees, total_price,
		 pricing_model, distance_km, cargo_per_km_rate_snapshot, cargo_minimum_rate_snapshot,
		 car_name, car_category,
		 pickup_location, pickup_latitude, pickup_longitude, pickup_reference,
		 dropoff_location, dropoff_latitude, dropoff_longitude, dropoff_reference,
		 contact_phone, note)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.BookingGroupID, b.UserID, b.CustomerName, b.CarID, b.DriverID, b.BookingNumber, b.Status, b.PaymentStatus,
		b.StartAt, b.EndAt, b.Days, b.DailyRateSnapshot, b.OutsideAntananarivo, b.Fees, b.TotalPrice,
		b.PricingModel, b.DistanceKm, b.CargoPerKmRate, b.CargoMinimumRate,
		b.CarName, b.CarCategory,
		b.PickupLocation, b.PickupLatitude, b.PickupLongitude, b.PickupReference,
		b.DropoffLocation, b.DropoffLatitude, b.DropoffLongitude, b.DropoffReference,
		b.ContactPhone, b.Note)
	if err != nil {
		return 0, mapTriggerErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) InsertBookingGroup(ctx context.Context, tx *sqlx.Tx, bookingID int64, bookingNumber, totalPrice string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO booking_groups (booking_id, booking_number, total_price) VALUES (?, ?, ?)`,
		bookingID, bookingNumber, totalPrice)
	return err
}

func (r *Repository) SetBookingGroup(ctx context.Context, tx *sqlx.Tx, bookingID, groupID int64) error {
	res, err := tx.ExecContext(ctx, `UPDATE bookings SET booking_group_id = ? WHERE id = ?`, groupID, bookingID)
	if err != nil {
		return err
	}
	return notFoundIfNoRows(res, ErrBookingNotFound)
}

func (r *Repository) AssignDriverTx(ctx context.Context, tx *sqlx.Tx, id, driverID int64) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE bookings
		    SET driver_id = ?,
		        status = CASE WHEN booking_group_id IS NULL THEN 'driver_assigned' ELSE status END
		  WHERE id = ?`, driverID, id)
	if err != nil {
		return mapTriggerErr(err)
	}
	if err := notFoundIfNoRows(res, ErrBookingNotFound); err != nil {
		return err
	}

	var groupID *int64
	if err := tx.GetContext(ctx, &groupID, `SELECT booking_group_id FROM bookings WHERE id = ?`, id); err != nil {
		return err
	}
	if groupID == nil {
		return nil
	}
	var unassigned int
	if err := tx.GetContext(ctx, &unassigned,
		`SELECT COUNT(*) FROM bookings WHERE booking_group_id = ? AND driver_id IS NULL`, *groupID); err != nil {
		return err
	}
	if unassigned == 0 {
		_, err = tx.ExecContext(ctx,
			`UPDATE bookings SET status = 'driver_assigned' WHERE booking_group_id = ?`, *groupID)
		return mapTriggerErr(err)
	}
	return nil
}

// CarOverlaps is the non-transactional (read-only) availability check.
func (r *Repository) CarOverlaps(ctx context.Context, carID, excludeID int64, start, end time.Time) (bool, error) {
	return r.HasCarOverlap(ctx, r.db, carID, excludeID, start, end)
}

// ListBookedRanges returns every booking window for a car (for the client
// calendar), excluding only cancelled ones — a cancelled booking frees the car,
// so it must not paint days red. `from` bounds how far back we look so the
// calendar shows recent history plus everything current/upcoming. This is a
// display feed only; double-booking enforcement uses CarOverlaps + the DB
// triggers (the occupying-status set), which is intentionally narrower.
func (r *Repository) ListBookedRanges(ctx context.Context, carID int64, from time.Time) ([]BookedRange, error) {
	out := []BookedRange{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT start_at, end_at FROM bookings
		 WHERE car_id = ?
		   AND status <> 'cancelled'
		   AND end_at > ?
		 ORDER BY start_at`, carID, from)
	return out, err
}

func (r *Repository) SetStatus(ctx context.Context, id int64, status string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE bookings
		    SET status = ?
		  WHERE id = ?
		     OR booking_group_id = (SELECT group_id FROM (SELECT booking_group_id AS group_id FROM bookings WHERE id = ?) source)`,
		status, id, id)
	if err != nil {
		return mapTriggerErr(err)
	}
	return notFoundIfNoRows(res, ErrBookingNotFound)
}

func (r *Repository) MarkPaid(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE bookings
		    SET payment_status = 'paid', paid_at = NOW()
		  WHERE id = ?
		     OR booking_group_id = (SELECT group_id FROM (SELECT booking_group_id AS group_id FROM bookings WHERE id = ?) source)`,
		id, id)
	if err != nil {
		return err
	}
	return notFoundIfNoRows(res, ErrBookingNotFound)
}

// DeleteBooking removes a booking only while it has no payment history. The
// booking row is locked so a concurrent payment cannot be recorded between the
// eligibility check and the delete.
func (r *Repository) DeleteBooking(ctx context.Context, id int64) error {
	return r.InTx(ctx, func(tx *sqlx.Tx) error {
		var target struct {
			ID            int64  `db:"id"`
			GroupID       *int64 `db:"booking_group_id"`
			PaymentStatus string `db:"payment_status"`
		}
		err := tx.GetContext(ctx, &target,
			`SELECT id, booking_group_id, payment_status FROM bookings WHERE id = ? FOR UPDATE`, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrBookingNotFound
		}
		if err != nil {
			return err
		}

		var paymentCount int
		if err := tx.GetContext(ctx, &paymentCount,
			`SELECT COUNT(*) FROM payments WHERE payable_type = 'booking' AND payable_id = ?`, id); err != nil {
			return err
		}
		rootID := target.ID
		if target.GroupID != nil {
			rootID = *target.GroupID
		}
		if rootID != id {
			if err := tx.GetContext(ctx, &target.PaymentStatus,
				`SELECT payment_status FROM bookings WHERE id = ? FOR UPDATE`, rootID); err != nil {
				return err
			}
			if err := tx.GetContext(ctx, &paymentCount,
				`SELECT COUNT(*) FROM payments WHERE payable_type = 'booking' AND payable_id = ?`, rootID); err != nil {
				return err
			}
		}
		if target.PaymentStatus != PaymentUnpaid || paymentCount > 0 {
			return ErrBookingHasPayments
		}

		if target.GroupID != nil {
			if _, err := tx.ExecContext(ctx, `DELETE FROM bookings WHERE booking_group_id = ? AND id <> ?`, rootID, rootID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE bookings SET booking_group_id = NULL WHERE id = ?`, rootID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM booking_groups WHERE booking_id = ?`, rootID); err != nil {
				return err
			}
		}

		res, err := tx.ExecContext(ctx, `DELETE FROM bookings WHERE id = ?`, rootID)
		if err != nil {
			return err
		}
		return notFoundIfNoRows(res, ErrBookingNotFound)
	})
}

// --- booking reads ---

const bookingCols = `id, booking_group_id, user_id,
	COALESCE(customer_name, (SELECT full_name FROM users u WHERE u.id = bookings.user_id)) AS customer_name,
	car_id, driver_id,
	COALESCE((SELECT bg.booking_number FROM booking_groups bg WHERE bg.booking_id = bookings.booking_group_id), booking_number) AS booking_number,
	status, payment_status,
	start_at, end_at, days, daily_rate_snapshot, outside_antananarivo, fees,
	COALESCE((SELECT bg.total_price FROM booking_groups bg WHERE bg.booking_id = bookings.booking_group_id), total_price) AS total_price,
	pricing_model, distance_km, cargo_per_km_rate_snapshot, cargo_minimum_rate_snapshot,
	CASE WHEN booking_group_id IS NOT NULL
		THEN CONCAT((SELECT COUNT(*) FROM bookings bi WHERE bi.booking_group_id = bookings.booking_group_id), ' cars')
		ELSE car_name END AS car_name,
	CASE WHEN booking_group_id IS NOT NULL THEN NULL ELSE car_category END AS car_category,
	pickup_location, pickup_latitude, pickup_longitude, pickup_reference,
	dropoff_location, dropoff_latitude, dropoff_longitude, dropoff_reference,
	contact_phone, note,
	paid_at, created_at, updated_at,
	(booking_group_id IS NOT NULL) AS is_multi_car,
	CASE WHEN booking_group_id IS NOT NULL
		THEN (SELECT COUNT(*) FROM bookings bi WHERE bi.booking_group_id = bookings.booking_group_id)
		ELSE 1 END AS car_count`

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
	where := []string{"(booking_group_id IS NULL OR id = booking_group_id)"}
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
		where = append(where, "(car_id = ? OR EXISTS (SELECT 1 FROM bookings car_item WHERE car_item.booking_group_id = bookings.id AND car_item.car_id = ?))")
		args = append(args, *f.CarID)
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

func (r *Repository) ListBookingCars(ctx context.Context, groupID int64) ([]BookingCarDetail, error) {
	out := []BookingCarDetail{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT id, car_id, driver_id, status, daily_rate_snapshot, fees, total_price,
		        pricing_model, distance_km, car_name, car_category
		   FROM bookings
		  WHERE booking_group_id = ?
		  ORDER BY id`, groupID)
	return out, err
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
