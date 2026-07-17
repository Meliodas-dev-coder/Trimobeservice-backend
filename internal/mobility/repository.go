package mobility

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
)

var (
	ErrCarCategoryNotFound = errors.New("car category not found")
	ErrCarNotFound         = errors.New("car not found")
	ErrCarImageNotFound    = errors.New("car image not found")
	ErrDriverNotFound      = errors.New("driver not found")

	ErrConflict = errors.New("duplicate value")
	ErrInUse    = errors.New("resource is referenced by other records")
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

const dayBookedStatuses = "'confirmed','driver_assigned','active','completed'"

// --- car categories ---

const carCategoryCols = `id, name, slug, description, translations, default_daily_rate, default_outside_antananarivo_daily_rate,
	is_cargo_transport, cargo_per_km_rate, cargo_minimum_rate,
	sort_order, is_active, created_at, updated_at`

func (r *Repository) ListCarCategories(ctx context.Context, activeOnly bool) ([]CarCategory, error) {
	q := `SELECT ` + carCategoryCols + ` FROM car_categories`
	if activeOnly {
		q += ` WHERE is_active = TRUE`
	}
	q += ` ORDER BY sort_order, name`
	out := []CarCategory{}
	if err := r.db.SelectContext(ctx, &out, q); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetCarCategoryByID(ctx context.Context, id int64) (*CarCategory, error) {
	var c CarCategory
	err := r.db.GetContext(ctx, &c, `SELECT `+carCategoryCols+` FROM car_categories WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCarCategoryNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) CarCategorySlugExists(ctx context.Context, slug string, excludeID int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok,
		`SELECT EXISTS(SELECT 1 FROM car_categories WHERE slug = ? AND id <> ?)`, slug, excludeID)
	return ok, err
}

func (r *Repository) CreateCarCategory(ctx context.Context, c *CarCategory) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO car_categories (name, slug, description, translations, default_daily_rate, default_outside_antananarivo_daily_rate,
		 is_cargo_transport, cargo_per_km_rate, cargo_minimum_rate, sort_order, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Slug, c.Description, nullableJSON(c.Translations), c.DefaultDailyRate, c.DefaultOutsideAntananarivoDailyRate, c.IsCargoTransport,
		c.CargoPerKmRate, c.CargoMinimumRate, c.SortOrder, c.IsActive)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateCarCategory(ctx context.Context, c *CarCategory) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE car_categories
		 SET name = ?, slug = ?, description = ?, translations = ?, default_daily_rate = ?, default_outside_antananarivo_daily_rate = ?,
		     is_cargo_transport = ?, cargo_per_km_rate = ?, cargo_minimum_rate = ?,
		     sort_order = ?, is_active = ?
		 WHERE id = ?`,
		c.Name, c.Slug, c.Description, nullableJSON(c.Translations), c.DefaultDailyRate, c.DefaultOutsideAntananarivoDailyRate, c.IsCargoTransport,
		c.CargoPerKmRate, c.CargoMinimumRate, c.SortOrder, c.IsActive, c.ID)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrCarCategoryNotFound)
}

func (r *Repository) DeleteCarCategory(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM car_categories WHERE id = ?`, id)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrCarCategoryNotFound)
}

// --- cars ---

const carBookedTodayExpr = `EXISTS (
	SELECT 1 FROM bookings b
	WHERE b.car_id = cars.id
	  AND b.status IN (` + dayBookedStatuses + `)
	  AND b.start_at < DATE_ADD(CURRENT_DATE(), INTERVAL 1 DAY)
	  AND b.end_at > CURRENT_DATE()
)`

const carEffectiveStatusExpr = `CASE
	WHEN cars.status = 'available' AND ` + carBookedTodayExpr + ` THEN 'not_available'
	ELSE cars.status
END`

const carCols = `cars.id, cars.category_id, cars.name, cars.slug, cars.make, cars.model, cars.year,
	cars.registration_plate, cars.color, cars.seats, cars.transmission, cars.fuel_type, cars.daily_rate,
	cars.outside_antananarivo_daily_rate,
	(SELECT cc.is_cargo_transport FROM car_categories cc WHERE cc.id = cars.category_id) AS is_cargo_transport,
	(SELECT cc.cargo_per_km_rate FROM car_categories cc WHERE cc.id = cars.category_id) AS cargo_per_km_rate,
	(SELECT cc.cargo_minimum_rate FROM car_categories cc WHERE cc.id = cars.category_id) AS cargo_minimum_rate,
	cars.attributes, cars.description, cars.translations, cars.status AS base_status, ` + carEffectiveStatusExpr + ` AS status,
	(SELECT ci.url FROM car_images ci WHERE ci.car_id = cars.id ORDER BY ci.is_primary DESC, ci.sort_order, ci.id LIMIT 1) AS primary_image_url,
	cars.created_at, cars.updated_at`

func (r *Repository) ListCars(ctx context.Context, f CarFilter) ([]Car, int, error) {
	var where []string
	var args []any
	if f.Status != "" {
		switch f.Status {
		case CarStatusAvailable:
			if f.IncludeBooked {
				where = append(where, "cars.status = ?")
			} else {
				where = append(where, "cars.status = ? AND NOT "+carBookedTodayExpr)
			}
			args = append(args, CarStatusAvailable)
		case CarStatusNotAvailable:
			where = append(where, "cars.status = ? AND "+carBookedTodayExpr)
			args = append(args, CarStatusAvailable)
		default:
			where = append(where, "cars.status = ?")
			args = append(args, f.Status)
		}
	}
	if f.ExcludeInactive {
		where = append(where, "cars.status <> ?")
		args = append(args, CarStatusInactive)
	}
	if f.CategoryID != nil {
		where = append(where, "cars.category_id = ?")
		args = append(args, *f.CategoryID)
	}
	if f.Search != "" {
		where = append(where, "(cars.name LIKE ? OR cars.make LIKE ? OR cars.model LIKE ? OR cars.registration_plate LIKE ?)")
		pattern := "%" + f.Search + "%"
		args = append(args, pattern, pattern, pattern, pattern)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM cars`+clause, args...); err != nil {
		return nil, 0, err
	}

	selectCols := carCols
	orderBy := "created_at DESC"
	availabilityArgs := []any{}
	if f.AvailableStart != nil && f.AvailableEnd != nil {
		selectCols += `, (cars.status = 'available' AND NOT EXISTS (
			SELECT 1 FROM bookings b
			WHERE b.car_id = cars.id
			  AND b.status IN (` + dayBookedStatuses + `)
			  AND b.start_at < ?
			  AND b.end_at > ?
		)) AS available_for_range`
		availabilityArgs = append(availabilityArgs, *f.AvailableEnd, *f.AvailableStart)
		orderBy = "available_for_range DESC, created_at DESC"
	}
	listArgs := append(append(append([]any{}, availabilityArgs...), args...), f.Limit, f.Offset)
	out := []Car{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+selectCols+` FROM cars`+clause+` ORDER BY `+orderBy+` LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) GetCarByID(ctx context.Context, id int64) (*Car, error) {
	var c Car
	err := r.db.GetContext(ctx, &c, `SELECT `+carCols+` FROM cars WHERE cars.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCarNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) GetCarBySlug(ctx context.Context, slug string) (*Car, error) {
	var c Car
	err := r.db.GetContext(ctx, &c, `SELECT `+carCols+` FROM cars WHERE cars.slug = ?`, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCarNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) CarSlugExists(ctx context.Context, slug string, excludeID int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok,
		`SELECT EXISTS(SELECT 1 FROM cars WHERE slug = ? AND id <> ?)`, slug, excludeID)
	return ok, err
}

func (r *Repository) PlateExists(ctx context.Context, plate string, excludeID int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok,
		`SELECT EXISTS(SELECT 1 FROM cars WHERE registration_plate = ? AND id <> ?)`, plate, excludeID)
	return ok, err
}

func (r *Repository) CreateCar(ctx context.Context, c *Car) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO cars (category_id, name, slug, make, model, year, registration_plate, color, seats,
		 transmission, fuel_type, daily_rate, outside_antananarivo_daily_rate,
		 attributes, description, translations, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.CategoryID, c.Name, c.Slug, c.Make, c.Model, c.Year, c.RegistrationPlate, c.Color, c.Seats,
		c.Transmission, c.FuelType, c.DailyRate, c.OutsideAntananarivoDailyRate,
		c.Attributes, c.Description, nullableJSON(c.Translations), c.Status)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateCar(ctx context.Context, c *Car) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE cars
		 SET category_id = ?, name = ?, slug = ?, make = ?, model = ?, year = ?, registration_plate = ?,
		     color = ?, seats = ?, transmission = ?, fuel_type = ?, daily_rate = ?,
		     outside_antananarivo_daily_rate = ?, attributes = ?,
		     description = ?, translations = ?, status = ?
		 WHERE id = ?`,
		c.CategoryID, c.Name, c.Slug, c.Make, c.Model, c.Year, c.RegistrationPlate, c.Color, c.Seats,
		c.Transmission, c.FuelType, c.DailyRate, c.OutsideAntananarivoDailyRate,
		c.Attributes, c.Description, nullableJSON(c.Translations), c.Status, c.ID)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrCarNotFound)
}

func (r *Repository) DeleteCar(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM cars WHERE id = ?`, id)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrCarNotFound)
}

func (r *Repository) CarUsageStats(ctx context.Context, carID int64) (*CarUsageStats, error) {
	var stats CarUsageStats
	err := r.db.GetContext(ctx, &stats,
		`SELECT
			COALESCE(SUM(CASE WHEN payment_status = 'paid' AND status <> 'cancelled' THEN total_price ELSE 0 END), 0) AS revenue_total,
			COUNT(*) AS total_bookings,
			COALESCE(SUM(CASE WHEN start_at >= NOW() AND status <> 'cancelled' THEN 1 ELSE 0 END), 0) AS future_bookings,
			COALESCE(SUM(CASE WHEN payment_status = 'paid' THEN 1 ELSE 0 END), 0) AS paid_bookings
		 FROM bookings
		 WHERE car_id = ?`, carID)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func (r *Repository) ListBookingsByCar(ctx context.Context, carID int64, limit int) ([]CarBookingSummary, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	out := []CarBookingSummary{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT
			COALESCE(b.booking_group_id, b.id) AS id,
			COALESCE(bg.booking_number, b.booking_number) AS booking_number,
			COALESCE(b.customer_name, u.full_name) AS customer_name,
			d.full_name AS driver_name,
			b.status,
			b.payment_status,
			b.start_at,
			b.end_at,
			b.days,
			b.total_price,
			b.pickup_location,
			b.dropoff_location,
			b.paid_at,
			b.created_at
		 FROM bookings b
		 LEFT JOIN booking_groups bg ON bg.booking_id = b.booking_group_id
		 LEFT JOIN users u ON u.id = b.user_id
		 LEFT JOIN drivers d ON d.id = b.driver_id
		 WHERE b.car_id = ?
		 ORDER BY b.start_at DESC
		 LIMIT ?`, carID, limit)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// --- car images ---

const carImageCols = `id, car_id, url, alt_text, is_primary, sort_order, created_at`

func (r *Repository) ListImagesByCar(ctx context.Context, carID int64) ([]CarImage, error) {
	out := []CarImage{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+carImageCols+` FROM car_images WHERE car_id = ? ORDER BY sort_order, id`, carID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) CreateCarImage(ctx context.Context, im *CarImage) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO car_images (car_id, url, alt_text, is_primary, sort_order) VALUES (?, ?, ?, ?, ?)`,
		im.CarID, im.URL, im.AltText, im.IsPrimary, im.SortOrder)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) GetCarImageByID(ctx context.Context, id int64) (*CarImage, error) {
	var im CarImage
	err := r.db.GetContext(ctx, &im, `SELECT `+carImageCols+` FROM car_images WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCarImageNotFound
	}
	if err != nil {
		return nil, err
	}
	return &im, nil
}

func (r *Repository) DeleteCarImage(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM car_images WHERE id = ?`, id)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrCarImageNotFound)
}

// --- drivers ---

const driverBookedTodayExpr = `EXISTS (
	SELECT 1 FROM bookings b
	WHERE b.driver_id = drivers.id
	  AND b.status IN (` + dayBookedStatuses + `)
	  AND b.start_at < DATE_ADD(CURRENT_DATE(), INTERVAL 1 DAY)
	  AND b.end_at > CURRENT_DATE()
)`

const driverEffectiveStatusExpr = `CASE
	WHEN drivers.status = 'available' AND ` + driverBookedTodayExpr + ` THEN 'assigned'
	ELSE drivers.status
END`

const driverCols = `drivers.id, drivers.full_name, drivers.phone, drivers.license_number,
	drivers.status AS base_status, ` + driverEffectiveStatusExpr + ` AS status,
	drivers.notes, drivers.created_at, drivers.updated_at`

func (r *Repository) ListDrivers(ctx context.Context, status string) ([]Driver, error) {
	q := `SELECT ` + driverCols + ` FROM drivers`
	var args []any
	if status != "" {
		switch status {
		case DriverStatusAvailable:
			q += ` WHERE drivers.status = ? AND NOT ` + driverBookedTodayExpr
			args = append(args, DriverStatusAvailable)
		case DriverStatusAssigned:
			q += ` WHERE drivers.status = ? OR (drivers.status = ? AND ` + driverBookedTodayExpr + `)`
			args = append(args, DriverStatusAssigned, DriverStatusAvailable)
		default:
			q += ` WHERE drivers.status = ?`
			args = append(args, status)
		}
	}
	q += ` ORDER BY drivers.full_name`
	out := []Driver{}
	if err := r.db.SelectContext(ctx, &out, q, args...); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetDriverByID(ctx context.Context, id int64) (*Driver, error) {
	var d Driver
	err := r.db.GetContext(ctx, &d, `SELECT `+driverCols+` FROM drivers WHERE drivers.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDriverNotFound
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *Repository) DriverPhoneExists(ctx context.Context, phone string, excludeID int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok,
		`SELECT EXISTS(SELECT 1 FROM drivers WHERE phone = ? AND id <> ?)`, phone, excludeID)
	return ok, err
}

func (r *Repository) DriverLicenseExists(ctx context.Context, license string, excludeID int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok,
		`SELECT EXISTS(SELECT 1 FROM drivers WHERE license_number = ? AND id <> ?)`, license, excludeID)
	return ok, err
}

func (r *Repository) CreateDriver(ctx context.Context, d *Driver) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO drivers (full_name, phone, license_number, status, notes) VALUES (?, ?, ?, ?, ?)`,
		d.FullName, d.Phone, d.LicenseNumber, d.Status, d.Notes)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateDriver(ctx context.Context, d *Driver) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE drivers SET full_name = ?, phone = ?, license_number = ?, status = ?, notes = ? WHERE id = ?`,
		d.FullName, d.Phone, d.LicenseNumber, d.Status, d.Notes, d.ID)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrDriverNotFound)
}

func (r *Repository) DeleteDriver(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM drivers WHERE id = ?`, id)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrDriverNotFound)
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

func mapWriteErr(err error) error {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		switch me.Number {
		case 1062:
			return ErrConflict
		case 1451:
			return ErrInUse
		}
	}
	return err
}

// nullableJSON returns nil for empty JSON so nullable JSON columns store SQL NULL.
func nullableJSON(j types.JSONText) any {
	if len(strings.TrimSpace(string(j))) == 0 {
		return nil
	}
	return j
}
