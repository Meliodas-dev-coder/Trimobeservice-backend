package healthcare

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var (
	ErrPractitionerNotFound = errors.New("practitioner not found")
	ErrCategoryNotFound     = errors.New("healthcare service category not found")
	ErrServiceNotFound      = errors.New("healthcare service not found")
	ErrRequestNotFound      = errors.New("healthcare request not found")
	ErrAssignmentNotFound   = errors.New("assignment not found")

	// ErrConflict is a duplicate unique value; ErrInUse is a referenced row that
	// cannot be deleted.
	ErrConflict = errors.New("duplicate value")
	ErrInUse    = errors.New("resource is referenced by other records")
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

// --- practitioners ---

const practitionerCols = `id, type, full_name, specialty, phone, email, license_number,
	bio, photo_url, status, created_at, updated_at`

func (r *Repository) ListPractitioners(ctx context.Context, f PractitionerFilter) ([]Practitioner, int, error) {
	var where []string
	var args []any
	if f.ActiveOnly {
		where = append(where, "status = 'active'")
	}
	if f.Type != "" {
		where = append(where, "type = ?")
		args = append(args, f.Type)
	}
	if f.Search != "" {
		where = append(where, "(full_name LIKE ? OR specialty LIKE ?)")
		args = append(args, "%"+f.Search+"%", "%"+f.Search+"%")
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM practitioners`+clause, args...); err != nil {
		return nil, 0, err
	}

	q := `SELECT ` + practitionerCols + ` FROM practitioners` + clause + ` ORDER BY type, full_name`
	listArgs := append([]any{}, args...)
	if f.Limit > 0 {
		q += ` LIMIT ? OFFSET ?`
		listArgs = append(listArgs, f.Limit, f.Offset)
	}
	out := []Practitioner{}
	if err := r.db.SelectContext(ctx, &out, q, listArgs...); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) GetPractitionerByID(ctx context.Context, id int64) (*Practitioner, error) {
	var p Practitioner
	err := r.db.GetContext(ctx, &p, `SELECT `+practitionerCols+` FROM practitioners WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPractitionerNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) PractitionerPhoneExists(ctx context.Context, phone string, excludeID int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok,
		`SELECT EXISTS(SELECT 1 FROM practitioners WHERE phone = ? AND id <> ?)`, phone, excludeID)
	return ok, err
}

func (r *Repository) PractitionerLicenseExists(ctx context.Context, license string, excludeID int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok,
		`SELECT EXISTS(SELECT 1 FROM practitioners WHERE license_number = ? AND id <> ?)`, license, excludeID)
	return ok, err
}

func (r *Repository) CreatePractitioner(ctx context.Context, p *Practitioner) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO practitioners (type, full_name, specialty, phone, email, license_number, bio, photo_url, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Type, p.FullName, p.Specialty, p.Phone, p.Email, p.LicenseNumber, p.Bio, p.PhotoURL, p.Status)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) UpdatePractitioner(ctx context.Context, p *Practitioner) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE practitioners
		 SET type = ?, full_name = ?, specialty = ?, phone = ?, email = ?, license_number = ?,
			 bio = ?, photo_url = ?, status = ?
		 WHERE id = ?`,
		p.Type, p.FullName, p.Specialty, p.Phone, p.Email, p.LicenseNumber, p.Bio, p.PhotoURL, p.Status, p.ID)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrPractitionerNotFound)
}

func (r *Repository) DeletePractitioner(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM practitioners WHERE id = ?`, id)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrPractitionerNotFound)
}

// --- service categories ---

const categoryCols = `id, name, slug, description, icon, image_url, sort_order, is_active, created_at, updated_at`

func (r *Repository) ListCategories(ctx context.Context, activeOnly bool) ([]ServiceCategory, error) {
	q := `SELECT ` + categoryCols + `,
		(SELECT COUNT(*) FROM healthcare_services s WHERE s.category_id = healthcare_service_categories.id) AS service_count
		FROM healthcare_service_categories`
	if activeOnly {
		q += ` WHERE is_active = TRUE`
	}
	q += ` ORDER BY sort_order, name`
	out := []ServiceCategory{}
	if err := r.db.SelectContext(ctx, &out, q); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetCategoryByID(ctx context.Context, id int64) (*ServiceCategory, error) {
	var c ServiceCategory
	err := r.db.GetContext(ctx, &c, `SELECT `+categoryCols+`, 0 AS service_count FROM healthcare_service_categories WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCategoryNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) CategoryExists(ctx context.Context, id int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok, `SELECT EXISTS(SELECT 1 FROM healthcare_service_categories WHERE id = ?)`, id)
	return ok, err
}

func (r *Repository) CategorySlugExists(ctx context.Context, slug string, excludeID int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok,
		`SELECT EXISTS(SELECT 1 FROM healthcare_service_categories WHERE slug = ? AND id <> ?)`, slug, excludeID)
	return ok, err
}

func (r *Repository) CreateCategory(ctx context.Context, c *ServiceCategory) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO healthcare_service_categories (name, slug, description, icon, image_url, sort_order, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Slug, c.Description, c.Icon, c.ImageURL, c.SortOrder, c.IsActive)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateCategory(ctx context.Context, c *ServiceCategory) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE healthcare_service_categories
		 SET name = ?, slug = ?, description = ?, icon = ?, image_url = ?, sort_order = ?, is_active = ?
		 WHERE id = ?`,
		c.Name, c.Slug, c.Description, c.Icon, c.ImageURL, c.SortOrder, c.IsActive, c.ID)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrCategoryNotFound)
}

func (r *Repository) DeleteCategory(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM healthcare_service_categories WHERE id = ?`, id)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrCategoryNotFound)
}

// --- services ---

const serviceCols = `id, category_id, name, slug, description, service_type, from_price, price,
	price_unit, duration_days, image_url, attributes, sort_order, is_active, created_at, updated_at`

const serviceListCols = serviceCols + `,
	(SELECT name FROM healthcare_service_categories c WHERE c.id = healthcare_services.category_id) AS category_name,
	COALESCE((SELECT quantity FROM healthcare_package_staff ps WHERE ps.service_id = healthcare_services.id AND ps.practitioner_type = 'doctor'), 0) AS staff_doctors,
	COALESCE((SELECT quantity FROM healthcare_package_staff ps WHERE ps.service_id = healthcare_services.id AND ps.practitioner_type = 'nurse'), 0) AS staff_nurses`

func (r *Repository) ListServices(ctx context.Context, f ServiceFilter) ([]HealthcareService, int, error) {
	var where []string
	var args []any
	if f.ActiveOnly {
		where = append(where, "is_active = TRUE")
	}
	if f.CategoryID != nil {
		where = append(where, "category_id = ?")
		args = append(args, *f.CategoryID)
	}
	if f.ServiceType != "" {
		where = append(where, "service_type = ?")
		args = append(args, f.ServiceType)
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
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM healthcare_services`+clause, args...); err != nil {
		return nil, 0, err
	}

	q := `SELECT ` + serviceListCols + ` FROM healthcare_services` + clause + ` ORDER BY sort_order, name`
	listArgs := append([]any{}, args...)
	if f.Limit > 0 {
		q += ` LIMIT ? OFFSET ?`
		listArgs = append(listArgs, f.Limit, f.Offset)
	}
	out := []HealthcareService{}
	if err := r.db.SelectContext(ctx, &out, q, listArgs...); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) GetServiceByID(ctx context.Context, id int64) (*HealthcareService, error) {
	var s HealthcareService
	err := r.db.GetContext(ctx, &s, `SELECT `+serviceListCols+` FROM healthcare_services WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrServiceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) GetServiceBySlug(ctx context.Context, slug string) (*HealthcareService, error) {
	var s HealthcareService
	err := r.db.GetContext(ctx, &s, `SELECT `+serviceListCols+` FROM healthcare_services WHERE slug = ?`, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrServiceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) ServiceSlugExists(ctx context.Context, slug string, excludeID int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok,
		`SELECT EXISTS(SELECT 1 FROM healthcare_services WHERE slug = ? AND id <> ?)`, slug, excludeID)
	return ok, err
}

// CreateService inserts a service and its package staff in one tx.
func (r *Repository) CreateService(ctx context.Context, s *HealthcareService, staff []PackageStaff) (int64, error) {
	var id int64
	err := r.InTx(ctx, func(tx *sqlx.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO healthcare_services
				(category_id, name, slug, description, service_type, from_price, price, price_unit,
				 duration_days, image_url, attributes, sort_order, is_active)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.CategoryID, s.Name, s.Slug, s.Description, s.ServiceType, s.FromPrice, s.Price, s.PriceUnit,
			s.DurationDays, s.ImageURL, nullableJSON(s.Attributes), s.SortOrder, s.IsActive)
		if err != nil {
			return mapWriteErr(err)
		}
		id, err = res.LastInsertId()
		if err != nil {
			return err
		}
		return insertStaffTx(ctx, tx, id, staff)
	})
	return id, err
}

// UpdateService updates a service and replaces its package staff in one tx.
func (r *Repository) UpdateService(ctx context.Context, s *HealthcareService, staff []PackageStaff) error {
	return r.InTx(ctx, func(tx *sqlx.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE healthcare_services
			 SET category_id = ?, name = ?, slug = ?, description = ?, service_type = ?, from_price = ?,
				 price = ?, price_unit = ?, duration_days = ?, image_url = ?, attributes = ?, sort_order = ?, is_active = ?
			 WHERE id = ?`,
			s.CategoryID, s.Name, s.Slug, s.Description, s.ServiceType, s.FromPrice, s.Price, s.PriceUnit,
			s.DurationDays, s.ImageURL, nullableJSON(s.Attributes), s.SortOrder, s.IsActive, s.ID)
		if err != nil {
			return mapWriteErr(err)
		}
		if err := notFoundIfNoRows(res, ErrServiceNotFound); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM healthcare_package_staff WHERE service_id = ?`, s.ID); err != nil {
			return err
		}
		return insertStaffTx(ctx, tx, s.ID, staff)
	})
}

func insertStaffTx(ctx context.Context, tx *sqlx.Tx, serviceID int64, staff []PackageStaff) error {
	for _, st := range staff {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO healthcare_package_staff (service_id, practitioner_type, quantity) VALUES (?, ?, ?)`,
			serviceID, st.PractitionerType, st.Quantity); err != nil {
			return mapWriteErr(err)
		}
	}
	return nil
}

func (r *Repository) DeleteService(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM healthcare_services WHERE id = ?`, id)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrServiceNotFound)
}

func (r *Repository) ListPackageStaff(ctx context.Context, serviceID int64) ([]PackageStaff, error) {
	out := []PackageStaff{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT id, service_id, practitioner_type, quantity
		 FROM healthcare_package_staff WHERE service_id = ? ORDER BY practitioner_type`, serviceID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetServiceRowTx reads the snapshot subset of a service inside a tx.
func (r *Repository) GetServiceRowTx(ctx context.Context, tx *sqlx.Tx, id int64) (*serviceRow, error) {
	var row serviceRow
	err := tx.GetContext(ctx, &row,
		`SELECT s.id, s.name, s.service_type, s.from_price, s.price, s.duration_days, s.is_active,
			(SELECT name FROM healthcare_service_categories c WHERE c.id = s.category_id) AS category_name
		 FROM healthcare_services s WHERE s.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrServiceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// --- requests ---

const requestCols = `id, user_id, customer_name, request_number, request_type, service_id, service_name,
	category_name, price_snapshot, patient_name, patient_age, patient_gender, preferred_at, start_at, end_at,
	address, symptoms, status, payment_status, paid_at, quoted_price, contact_phone, contact_email,
	note, admin_note, created_at, updated_at`

const requestListCols = requestCols + `,
	(SELECT COUNT(*) FROM healthcare_request_assignments a WHERE a.request_id = healthcare_requests.id) AS assignment_count`

func (r *Repository) InsertRequest(ctx context.Context, tx *sqlx.Tx, e *Request) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO healthcare_requests
			(user_id, customer_name, request_number, request_type, service_id, service_name, category_name,
			 price_snapshot, patient_name, patient_age, patient_gender, preferred_at, start_at, end_at,
			 address, symptoms, status, payment_status, quoted_price, contact_phone, contact_email, note)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.UserID, e.CustomerName, e.RequestNumber, e.RequestType, e.ServiceID, e.ServiceName, e.CategoryName,
		e.PriceSnapshot, e.PatientName, e.PatientAge, e.PatientGender, e.PreferredAt, e.StartAt, e.EndAt,
		e.Address, e.Symptoms, e.Status, e.PaymentStatus, e.QuotedPrice, e.ContactPhone, e.ContactEmail, e.Note)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) List(ctx context.Context, f RequestFilter) ([]Request, int, error) {
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
	if f.RequestType != "" {
		where = append(where, "request_type = ?")
		args = append(args, f.RequestType)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM healthcare_requests`+clause, args...); err != nil {
		return nil, 0, err
	}

	listArgs := append(append([]any{}, args...), f.Limit, f.Offset)
	out := []Request{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+requestListCols+` FROM healthcare_requests`+clause+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*Request, error) {
	var e Request
	err := r.db.GetContext(ctx, &e, `SELECT `+requestListCols+` FROM healthcare_requests WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRequestNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *Repository) GetForUser(ctx context.Context, userID, id int64) (*Request, error) {
	var e Request
	err := r.db.GetContext(ctx, &e,
		`SELECT `+requestListCols+` FROM healthcare_requests WHERE id = ? AND user_id = ?`, id, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRequestNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *Repository) SetStatus(ctx context.Context, id int64, status string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE healthcare_requests SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return err
	}
	return notFoundIfNoRows(res, ErrRequestNotFound)
}

// SetQuote sets the quoted price (and optionally an admin note) and, from an
// early state, advances the request to 'quoted'.
func (r *Repository) SetQuote(ctx context.Context, id int64, price string, adminNote *string, advanceToQuoted bool) error {
	q := `UPDATE healthcare_requests SET quoted_price = ?`
	args := []any{price}
	if adminNote != nil {
		q += `, admin_note = ?`
		args = append(args, *adminNote)
	}
	if advanceToQuoted {
		q += `, status = '` + StatusQuoted + `'`
	}
	q += ` WHERE id = ?`
	args = append(args, id)
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	return notFoundIfNoRows(res, ErrRequestNotFound)
}

// --- assignments ---

func (r *Repository) ListAssignments(ctx context.Context, requestID int64) ([]Assignment, error) {
	out := []Assignment{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT id, request_id, practitioner_id, practitioner_name, practitioner_type, note, assigned_at
		 FROM healthcare_request_assignments WHERE request_id = ? ORDER BY practitioner_type, id`, requestID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) InsertAssignment(ctx context.Context, a *Assignment) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO healthcare_request_assignments (request_id, practitioner_id, practitioner_name, practitioner_type, note)
		 VALUES (?, ?, ?, ?, ?)`,
		a.RequestID, a.PractitionerID, a.PractitionerName, a.PractitionerType, a.Note)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) DeleteAssignment(ctx context.Context, assignmentID int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM healthcare_request_assignments WHERE id = ?`, assignmentID)
	if err != nil {
		return err
	}
	return notFoundIfNoRows(res, ErrAssignmentNotFound)
}

// ActivePractitionerAssignments returns other non-terminal requests the given
// practitioner is currently assigned to (excluding excludeRequestID). Used to
// surface a soft overlap warning at assignment time.
func (r *Repository) ActivePractitionerAssignments(ctx context.Context, practitionerID, excludeRequestID int64) ([]Assignment, error) {
	out := []Assignment{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT a.id, a.request_id, a.practitioner_id, a.practitioner_name, a.practitioner_type, a.note, a.assigned_at
		 FROM healthcare_request_assignments a
		 JOIN healthcare_requests r ON r.id = a.request_id
		 WHERE a.practitioner_id = ? AND a.request_id <> ?
		   AND r.status NOT IN ('completed','cancelled')
		 ORDER BY a.assigned_at DESC`, practitionerID, excludeRequestID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CustomerName returns a customer account's full name (used to snapshot it onto
// admin-created requests linked to an account).
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

// --- settings ---

func (r *Repository) GetSettings(ctx context.Context) (*Settings, error) {
	var s Settings
	err := r.db.GetContext(ctx, &s,
		`SELECT emergency_phone, emergency_hours, emergency_note, updated_at FROM healthcare_settings WHERE id = 1`)
	if errors.Is(err, sql.ErrNoRows) {
		// Row is seeded by migration, but stay nil-safe if it was removed.
		return &Settings{}, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) UpdateSettings(ctx context.Context, s *Settings) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO healthcare_settings (id, emergency_phone, emergency_hours, emergency_note)
		 VALUES (1, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE emergency_phone = VALUES(emergency_phone),
			 emergency_hours = VALUES(emergency_hours), emergency_note = VALUES(emergency_note)`,
		s.EmergencyPhone, s.EmergencyHours, s.EmergencyNote)
	return err
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

// mapWriteErr translates MySQL integrity errors into module sentinels.
func mapWriteErr(err error) error {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		switch me.Number {
		case 1062: // duplicate entry for a unique key
			return ErrConflict
		case 1451, 1452: // FK violation (referenced/references)
			return ErrInUse
		}
	}
	return err
}
