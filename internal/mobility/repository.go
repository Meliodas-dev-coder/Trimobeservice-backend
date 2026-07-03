package mobility

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
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

// --- car categories ---

const carCategoryCols = `id, name, slug, description, default_daily_rate, image_url, sort_order, is_active, created_at, updated_at`

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
		`INSERT INTO car_categories (name, slug, description, default_daily_rate, image_url, sort_order, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Slug, c.Description, c.DefaultDailyRate, c.ImageURL, c.SortOrder, c.IsActive)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateCarCategory(ctx context.Context, c *CarCategory) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE car_categories
		 SET name = ?, slug = ?, description = ?, default_daily_rate = ?, image_url = ?, sort_order = ?, is_active = ?
		 WHERE id = ?`,
		c.Name, c.Slug, c.Description, c.DefaultDailyRate, c.ImageURL, c.SortOrder, c.IsActive, c.ID)
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

const carCols = `id, category_id, name, slug, make, model, year, registration_plate, color, seats,
	transmission, fuel_type, daily_rate, attributes, description, status, created_at, updated_at`

func (r *Repository) ListCars(ctx context.Context, f CarFilter) ([]Car, int, error) {
	var where []string
	var args []any
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.CategoryID != nil {
		where = append(where, "category_id = ?")
		args = append(args, *f.CategoryID)
	}
	if f.Search != "" {
		where = append(where, "name LIKE ?")
		args = append(args, "%"+f.Search+"%")
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM cars`+clause, args...); err != nil {
		return nil, 0, err
	}

	listArgs := append(append([]any{}, args...), f.Limit, f.Offset)
	out := []Car{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+carCols+` FROM cars`+clause+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) GetCarByID(ctx context.Context, id int64) (*Car, error) {
	var c Car
	err := r.db.GetContext(ctx, &c, `SELECT `+carCols+` FROM cars WHERE id = ?`, id)
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
	err := r.db.GetContext(ctx, &c, `SELECT `+carCols+` FROM cars WHERE slug = ?`, slug)
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
		 transmission, fuel_type, daily_rate, attributes, description, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.CategoryID, c.Name, c.Slug, c.Make, c.Model, c.Year, c.RegistrationPlate, c.Color, c.Seats,
		c.Transmission, c.FuelType, c.DailyRate, c.Attributes, c.Description, c.Status)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateCar(ctx context.Context, c *Car) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE cars
		 SET category_id = ?, name = ?, slug = ?, make = ?, model = ?, year = ?, registration_plate = ?,
		     color = ?, seats = ?, transmission = ?, fuel_type = ?, daily_rate = ?, attributes = ?,
		     description = ?, status = ?
		 WHERE id = ?`,
		c.CategoryID, c.Name, c.Slug, c.Make, c.Model, c.Year, c.RegistrationPlate, c.Color, c.Seats,
		c.Transmission, c.FuelType, c.DailyRate, c.Attributes, c.Description, c.Status, c.ID)
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

const driverCols = `id, full_name, phone, license_number, status, notes, created_at, updated_at`

func (r *Repository) ListDrivers(ctx context.Context, status string) ([]Driver, error) {
	q := `SELECT ` + driverCols + ` FROM drivers`
	var args []any
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY full_name`
	out := []Driver{}
	if err := r.db.SelectContext(ctx, &out, q, args...); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetDriverByID(ctx context.Context, id int64) (*Driver, error) {
	var d Driver
	err := r.db.GetContext(ctx, &d, `SELECT `+driverCols+` FROM drivers WHERE id = ?`, id)
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
