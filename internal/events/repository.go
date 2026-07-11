package events

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var (
	ErrCategoryNotFound = errors.New("event service category not found")
	ErrServiceNotFound  = errors.New("event service not found")
	ErrArtistNotFound   = errors.New("artist not found")
	ErrRequestNotFound  = errors.New("event request not found")

	// ErrConflict is a duplicate unique value (slug); ErrInUse is a referenced
	// row that cannot be deleted.
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

// --- service categories ---

const categoryCols = `id, name, slug, description, translations, icon, image_url, sort_order, is_active, created_at, updated_at`

func (r *Repository) ListCategories(ctx context.Context, activeOnly bool) ([]ServiceCategory, error) {
	q := `SELECT ` + categoryCols + `,
		(SELECT COUNT(*) FROM event_services s WHERE s.category_id = event_service_categories.id) AS service_count
		FROM event_service_categories`
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
	err := r.db.GetContext(ctx, &c, `SELECT `+categoryCols+`, 0 AS service_count FROM event_service_categories WHERE id = ?`, id)
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
	err := r.db.GetContext(ctx, &ok, `SELECT EXISTS(SELECT 1 FROM event_service_categories WHERE id = ?)`, id)
	return ok, err
}

func (r *Repository) CategorySlugExists(ctx context.Context, slug string, excludeID int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok,
		`SELECT EXISTS(SELECT 1 FROM event_service_categories WHERE slug = ? AND id <> ?)`, slug, excludeID)
	return ok, err
}

func (r *Repository) CreateCategory(ctx context.Context, c *ServiceCategory) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO event_service_categories (name, slug, description, translations, icon, image_url, sort_order, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Slug, c.Description, nullableJSON(c.Translations), c.Icon, c.ImageURL, c.SortOrder, c.IsActive)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateCategory(ctx context.Context, c *ServiceCategory) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE event_service_categories
		 SET name = ?, slug = ?, description = ?, translations = ?, icon = ?, image_url = ?, sort_order = ?, is_active = ?
		 WHERE id = ?`,
		c.Name, c.Slug, c.Description, nullableJSON(c.Translations), c.Icon, c.ImageURL, c.SortOrder, c.IsActive, c.ID)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrCategoryNotFound)
}

func (r *Repository) DeleteCategory(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM event_service_categories WHERE id = ?`, id)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrCategoryNotFound)
}

// --- services ---

const serviceCols = `id, category_id, name, slug, description, translations, from_price, price_unit, image_url, attributes, sort_order, is_active, created_at, updated_at`

// serviceListCols joins the category name for the admin table / public grouping.
const serviceListCols = serviceCols + `,
	(SELECT name FROM event_service_categories c WHERE c.id = event_services.category_id) AS category_name`

func (r *Repository) ListServices(ctx context.Context, f ServiceFilter) ([]EventService, int, error) {
	var where []string
	var args []any
	if f.ActiveOnly {
		where = append(where, "is_active = TRUE")
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
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM event_services`+clause, args...); err != nil {
		return nil, 0, err
	}

	q := `SELECT ` + serviceListCols + ` FROM event_services` + clause + ` ORDER BY sort_order, name`
	listArgs := append([]any{}, args...)
	if f.Limit > 0 {
		q += ` LIMIT ? OFFSET ?`
		listArgs = append(listArgs, f.Limit, f.Offset)
	}
	out := []EventService{}
	if err := r.db.SelectContext(ctx, &out, q, listArgs...); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) GetServiceByID(ctx context.Context, id int64) (*EventService, error) {
	var s EventService
	err := r.db.GetContext(ctx, &s, `SELECT `+serviceListCols+` FROM event_services WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrServiceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) GetServiceBySlug(ctx context.Context, slug string) (*EventService, error) {
	var s EventService
	err := r.db.GetContext(ctx, &s, `SELECT `+serviceListCols+` FROM event_services WHERE slug = ?`, slug)
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
		`SELECT EXISTS(SELECT 1 FROM event_services WHERE slug = ? AND id <> ?)`, slug, excludeID)
	return ok, err
}

func (r *Repository) CreateService(ctx context.Context, s *EventService) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO event_services (category_id, name, slug, description, translations, from_price, price_unit, image_url, attributes, sort_order, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.CategoryID, s.Name, s.Slug, s.Description, nullableJSON(s.Translations), s.FromPrice, s.PriceUnit, s.ImageURL, nullableJSON(s.Attributes), s.SortOrder, s.IsActive)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateService(ctx context.Context, s *EventService) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE event_services
		 SET category_id = ?, name = ?, slug = ?, description = ?, translations = ?, from_price = ?, price_unit = ?, image_url = ?, attributes = ?, sort_order = ?, is_active = ?
		 WHERE id = ?`,
		s.CategoryID, s.Name, s.Slug, s.Description, nullableJSON(s.Translations), s.FromPrice, s.PriceUnit, s.ImageURL, nullableJSON(s.Attributes), s.SortOrder, s.IsActive, s.ID)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrServiceNotFound)
}

func (r *Repository) DeleteService(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM event_services WHERE id = ?`, id)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrServiceNotFound)
}

// GetServiceRowsTx reads the snapshot subset of the given services (active state
// + category name) inside a transaction, so a request freezes a consistent view.
func (r *Repository) GetServiceRowsTx(ctx context.Context, tx *sqlx.Tx, ids []int64) ([]serviceRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query, args, err := sqlx.In(
		`SELECT s.id, s.name, s.from_price, s.is_active,
			(SELECT name FROM event_service_categories c WHERE c.id = s.category_id) AS category_name
		 FROM event_services s WHERE s.id IN (?)`, ids)
	if err != nil {
		return nil, err
	}
	query = tx.Rebind(query)
	out := []serviceRow{}
	if err := tx.SelectContext(ctx, &out, query, args...); err != nil {
		return nil, err
	}
	return out, nil
}

// --- artists ---

const artistCols = `id, stage_name, slug, tagline, bio, translations, home_base, photo_url, group_size,
	genres, formats, languages, occasions, sample_links, social_links, from_fee,
	is_featured, sort_order, is_active, created_at, updated_at`

func (r *Repository) ListArtists(ctx context.Context, f ArtistFilter) ([]Artist, int, error) {
	var where []string
	var args []any
	if f.ActiveOnly {
		where = append(where, "is_active = TRUE")
	}
	if f.FeaturedOnly {
		where = append(where, "is_featured = TRUE")
	}
	if f.Search != "" {
		where = append(where, "(stage_name LIKE ? OR genres LIKE ?)")
		args = append(args, "%"+f.Search+"%", "%"+f.Search+"%")
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM artists`+clause, args...); err != nil {
		return nil, 0, err
	}

	// Featured first, then manual sort order, then name.
	q := `SELECT ` + artistCols + ` FROM artists` + clause + ` ORDER BY is_featured DESC, sort_order, stage_name`
	listArgs := append([]any{}, args...)
	if f.Limit > 0 {
		q += ` LIMIT ? OFFSET ?`
		listArgs = append(listArgs, f.Limit, f.Offset)
	}
	out := []Artist{}
	if err := r.db.SelectContext(ctx, &out, q, listArgs...); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) GetArtistByID(ctx context.Context, id int64) (*Artist, error) {
	var a Artist
	err := r.db.GetContext(ctx, &a, `SELECT `+artistCols+` FROM artists WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrArtistNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repository) GetArtistBySlug(ctx context.Context, slug string) (*Artist, error) {
	var a Artist
	err := r.db.GetContext(ctx, &a, `SELECT `+artistCols+` FROM artists WHERE slug = ?`, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrArtistNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repository) ArtistSlugExists(ctx context.Context, slug string, excludeID int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok,
		`SELECT EXISTS(SELECT 1 FROM artists WHERE slug = ? AND id <> ?)`, slug, excludeID)
	return ok, err
}

func (r *Repository) CreateArtist(ctx context.Context, a *Artist) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO artists (stage_name, slug, tagline, bio, translations, home_base, photo_url, group_size,
			genres, formats, languages, occasions, sample_links, social_links, from_fee, is_featured, sort_order, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.StageName, a.Slug, a.Tagline, a.Bio, nullableJSON(a.Translations), a.HomeBase, a.PhotoURL, a.GroupSize,
		a.Genres, a.Formats, a.Languages, a.Occasions, a.SampleLinks, a.SocialLinks, a.FromFee, a.IsFeatured, a.SortOrder, a.IsActive)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateArtist(ctx context.Context, a *Artist) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE artists
		 SET stage_name = ?, slug = ?, tagline = ?, bio = ?, translations = ?, home_base = ?, photo_url = ?, group_size = ?,
			 genres = ?, formats = ?, languages = ?, occasions = ?, sample_links = ?, social_links = ?,
			 from_fee = ?, is_featured = ?, sort_order = ?, is_active = ?
		 WHERE id = ?`,
		a.StageName, a.Slug, a.Tagline, a.Bio, nullableJSON(a.Translations), a.HomeBase, a.PhotoURL, a.GroupSize,
		a.Genres, a.Formats, a.Languages, a.Occasions, a.SampleLinks, a.SocialLinks,
		a.FromFee, a.IsFeatured, a.SortOrder, a.IsActive, a.ID)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrArtistNotFound)
}

func (r *Repository) DeleteArtist(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM artists WHERE id = ?`, id)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrArtistNotFound)
}

// GetArtistRowsTx reads the snapshot subset of the given artists inside a tx.
func (r *Repository) GetArtistRowsTx(ctx context.Context, tx *sqlx.Tx, ids []int64) ([]artistRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query, args, err := sqlx.In(
		`SELECT id, stage_name, from_fee, is_active FROM artists WHERE id IN (?)`, ids)
	if err != nil {
		return nil, err
	}
	query = tx.Rebind(query)
	out := []artistRow{}
	if err := tx.SelectContext(ctx, &out, query, args...); err != nil {
		return nil, err
	}
	return out, nil
}

// --- event requests ---

const requestCols = `id, user_id, customer_name, request_number, event_type, status, payment_status, paid_at,
	event_start, event_end, location, guest_count, budget, quoted_price, contact_phone, contact_email,
	note, admin_note, created_at, updated_at`

const requestListCols = requestCols + `,
	(SELECT COUNT(*) FROM event_request_services ers WHERE ers.request_id = event_requests.id) AS service_count`

func (r *Repository) InsertRequest(ctx context.Context, tx *sqlx.Tx, e *EventRequest) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO event_requests
			(user_id, customer_name, request_number, event_type, status, payment_status,
			 event_start, event_end, location, guest_count, budget, contact_phone, contact_email, note)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.UserID, e.CustomerName, e.RequestNumber, e.EventType, e.Status, e.PaymentStatus,
		e.EventStart, e.EventEnd, e.Location, e.GuestCount, e.Budget, e.ContactPhone, e.ContactEmail, e.Note)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) InsertRequestService(ctx context.Context, tx *sqlx.Tx, s *RequestService) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO event_request_services
			(request_id, service_id, service_name, category_name, from_price_snapshot, quantity, note)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.RequestID, s.ServiceID, s.ServiceName, s.CategoryName, s.FromPriceSnapshot, s.Quantity, s.Note)
	return err
}

func (r *Repository) InsertRequestArtist(ctx context.Context, tx *sqlx.Tx, a *RequestArtist) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO event_request_artists (request_id, artist_id, artist_name, fee_snapshot, note)
		 VALUES (?, ?, ?, ?, ?)`,
		a.RequestID, a.ArtistID, a.ArtistName, a.FeeSnapshot, a.Note)
	return err
}

func (r *Repository) ListRequestArtists(ctx context.Context, requestID int64) ([]RequestArtist, error) {
	out := []RequestArtist{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT id, request_id, artist_id, artist_name, fee_snapshot, note, created_at
		 FROM event_request_artists WHERE request_id = ? ORDER BY id`, requestID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) List(ctx context.Context, f EventRequestFilter) ([]EventRequest, int, error) {
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
	if f.EventType != "" {
		where = append(where, "event_type = ?")
		args = append(args, f.EventType)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM event_requests`+clause, args...); err != nil {
		return nil, 0, err
	}

	listArgs := append(append([]any{}, args...), f.Limit, f.Offset)
	out := []EventRequest{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+requestListCols+` FROM event_requests`+clause+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*EventRequest, error) {
	var e EventRequest
	err := r.db.GetContext(ctx, &e, `SELECT `+requestListCols+` FROM event_requests WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRequestNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *Repository) GetForUser(ctx context.Context, userID, id int64) (*EventRequest, error) {
	var e EventRequest
	err := r.db.GetContext(ctx, &e,
		`SELECT `+requestListCols+` FROM event_requests WHERE id = ? AND user_id = ?`, id, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRequestNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *Repository) ListRequestServices(ctx context.Context, requestID int64) ([]RequestService, error) {
	out := []RequestService{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT id, request_id, service_id, service_name, category_name, from_price_snapshot, quantity, note, created_at
		 FROM event_request_services WHERE request_id = ? ORDER BY id`, requestID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) SetStatus(ctx context.Context, id int64, status string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE event_requests SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return err
	}
	return notFoundIfNoRows(res, ErrRequestNotFound)
}

// SetQuote sets the quoted price (and optionally an admin note) and, when the
// request is still in an early state, advances it to 'quoted'.
func (r *Repository) SetQuote(ctx context.Context, id int64, price string, adminNote *string, advanceToQuoted bool) error {
	q := `UPDATE event_requests SET quoted_price = ?`
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
