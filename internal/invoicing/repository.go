package invoicing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var (
	ErrInvoiceNotFound = errors.New("invoice not found")
	ErrSourceNotFound  = errors.New("the order, booking, event, or request was not found")
	ErrNoQuote         = errors.New("set a quote on the event or healthcare request before invoicing")
	ErrConflict        = errors.New("duplicate value")
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

// --- org settings ---

const orgSettingsCols = `id, legal_name, brand_name, address, city, phone, email, website, logo_url,
	nif, stat, rcs, currency, tax_label, default_tax_rate, payment_terms, bank_details, mobile_money,
	invoice_prefix, proforma_prefix, credit_note_prefix, footer_text, updated_at`

func (r *Repository) GetOrgSettings(ctx context.Context) (*OrgSettings, error) {
	var s OrgSettings
	err := r.db.GetContext(ctx, &s, `SELECT `+orgSettingsCols+` FROM org_settings WHERE id = 1`)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvoiceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) UpdateOrgSettings(ctx context.Context, s *OrgSettings) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE org_settings SET
			legal_name = ?, brand_name = ?, address = ?, city = ?, phone = ?, email = ?, website = ?, logo_url = ?,
			nif = ?, stat = ?, rcs = ?, currency = ?, tax_label = ?, default_tax_rate = ?,
			payment_terms = ?, bank_details = ?, mobile_money = ?,
			invoice_prefix = ?, proforma_prefix = ?, credit_note_prefix = ?, footer_text = ?
		 WHERE id = 1`,
		s.LegalName, s.BrandName, s.Address, s.City, s.Phone, s.Email, s.Website, s.LogoURL,
		s.NIF, s.STAT, s.RCS, s.Currency, s.TaxLabel, s.DefaultTaxRate,
		s.PaymentTerms, s.BankDetails, s.MobileMoney,
		s.InvoicePrefix, s.ProformaPrefix, s.CreditNotePrefix, s.FooterText)
	return err
}

// --- invoices ---

const invoiceCols = `id, invoiceable_type, invoiceable_id, source_number, kind, source_invoice_id,
	invoice_number, status, seller_snapshot, buyer_name, buyer_phone, buyer_email, buyer_address,
	currency, issue_date, due_date, subtotal, discount, tax_rate, tax_amount, total, notes, terms,
	issued_by, created_at, updated_at`

func (r *Repository) InsertInvoiceTx(ctx context.Context, tx *sqlx.Tx, inv *Invoice) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO invoices (invoiceable_type, invoiceable_id, source_number, kind, source_invoice_id,
			seller_snapshot, buyer_name, buyer_phone, buyer_email, buyer_address, currency,
			due_date, subtotal, discount, tax_rate, tax_amount, total, notes, terms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.InvoiceableType, inv.InvoiceableID, inv.SourceNumber, inv.Kind, inv.SourceInvoiceID,
		nullableJSON(inv.SellerSnapshot), inv.BuyerName, inv.BuyerPhone, inv.BuyerEmail, inv.BuyerAddress, inv.Currency,
		inv.DueDate, inv.Subtotal, inv.Discount, inv.TaxRate, inv.TaxAmount, inv.Total, inv.Notes, inv.Terms)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) InsertLineTx(ctx context.Context, tx *sqlx.Tx, invoiceID int64, l *InvoiceLine) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO invoice_lines (invoice_id, description, detail, quantity, unit_price, line_total, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		invoiceID, l.Description, l.Detail, l.Quantity, l.UnitPrice, l.LineTotal, l.SortOrder)
	return err
}

func (r *Repository) GetInvoice(ctx context.Context, id int64) (*Invoice, error) {
	var inv Invoice
	err := r.db.GetContext(ctx, &inv, `SELECT `+invoiceCols+` FROM invoices WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvoiceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// GetInvoiceForUpdateTx locks the invoice row so issue/void can't race.
func (r *Repository) GetInvoiceForUpdateTx(ctx context.Context, tx *sqlx.Tx, id int64) (*Invoice, error) {
	var inv Invoice
	err := tx.GetContext(ctx, &inv, `SELECT `+invoiceCols+` FROM invoices WHERE id = ? FOR UPDATE`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvoiceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *Repository) ListLines(ctx context.Context, invoiceID int64) ([]InvoiceLine, error) {
	out := []InvoiceLine{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT id, invoice_id, description, detail, quantity, unit_price, line_total, sort_order, created_at
		 FROM invoice_lines WHERE invoice_id = ? ORDER BY sort_order, id`, invoiceID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) List(ctx context.Context, f InvoiceFilter) ([]Invoice, int, error) {
	var where []string
	var args []any
	if f.InvoiceableType != "" {
		where = append(where, "invoiceable_type = ?")
		args = append(args, f.InvoiceableType)
	}
	if f.InvoiceableID != nil {
		where = append(where, "invoiceable_id = ?")
		args = append(args, *f.InvoiceableID)
	}
	if f.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, f.Kind)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM invoices`+clause, args...); err != nil {
		return nil, 0, err
	}
	listArgs := append(append([]any{}, args...), f.Limit, f.Offset)
	out := []Invoice{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+invoiceCols+` FROM invoices`+clause+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// UpdateFieldsTx updates the mutable fields of a draft invoice and its recomputed
// totals (lines are unchanged).
func (r *Repository) UpdateFieldsTx(ctx context.Context, tx *sqlx.Tx, inv *Invoice) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE invoices SET due_date = ?, discount = ?, tax_rate = ?, tax_amount = ?, total = ?,
			notes = ?, terms = ?
		 WHERE id = ?`,
		inv.DueDate, inv.Discount, inv.TaxRate, inv.TaxAmount, inv.Total, inv.Notes, inv.Terms, inv.ID)
	return err
}

// AssignNumberTx stamps the number, issue date, issuer, and status=issued.
func (r *Repository) AssignNumberTx(ctx context.Context, tx *sqlx.Tx, id int64, number string, issuedBy int64) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE invoices SET invoice_number = ?, issue_date = CURRENT_DATE(), issued_by = ?, status = ?
		 WHERE id = ?`,
		number, issuedBy, StatusIssued, id)
	return mapWriteErr(err)
}

func (r *Repository) SetStatusTx(ctx context.Context, tx *sqlx.Tx, id int64, status string) error {
	_, err := tx.ExecContext(ctx, `UPDATE invoices SET status = ? WHERE id = ?`, status, id)
	return err
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM invoices WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvoiceNotFound
	}
	return nil
}

// nextSequenceTx atomically increments and returns the per-(kind, year) counter.
// The LAST_INSERT_ID() sequence pattern holds the row lock to end-of-tx, so
// concurrent issues get distinct, gapless numbers.
func (r *Repository) nextSequenceTx(ctx context.Context, tx *sqlx.Tx, kind string, year int) (int, error) {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO invoice_sequences (kind, year, last_number)
		 VALUES (?, ?, LAST_INSERT_ID(1))
		 ON DUPLICATE KEY UPDATE last_number = LAST_INSERT_ID(last_number + 1)`, kind, year)
	if err != nil {
		return 0, err
	}
	var n int
	if err := tx.GetContext(ctx, &n, `SELECT LAST_INSERT_ID()`); err != nil {
		return 0, err
	}
	return n, nil
}

// AmountPaid sums confirmed payments recorded against the target in the ledger.
func (r *Repository) AmountPaid(ctx context.Context, invoiceableType string, invoiceableID int64) (string, error) {
	var paid string
	err := r.db.GetContext(ctx, &paid,
		`SELECT COALESCE(SUM(amount), 0) FROM payments
		 WHERE payable_type = ? AND payable_id = ? AND status = 'paid'`, invoiceableType, invoiceableID)
	if err != nil {
		return "0.00", err
	}
	return paid, nil
}

// ================== source composition (per domain) ==================

// LoadSource builds the buyer, reference number, and priced lines for a
// transaction. Amounts are parsed to cents in the returned lineInput.
func (r *Repository) LoadSource(ctx context.Context, invoiceableType string, id int64) (*invoiceSource, error) {
	switch invoiceableType {
	case TypeOrder:
		return r.loadOrderSource(ctx, id)
	case TypeBooking:
		return r.loadBookingSource(ctx, id)
	case TypeEvent:
		return r.loadEventSource(ctx, id)
	case TypeHealthcare:
		return r.loadHealthcareSource(ctx, id)
	default:
		return nil, ErrSourceNotFound
	}
}

type orderHeadRow struct {
	OrderNumber string  `db:"order_number"`
	CustomerNm  *string `db:"customer_name"`
	UserName    *string `db:"user_name"`
	UserEmail   *string `db:"user_email"`
	ShipPhone   *string `db:"ship_phone"`
	ShipName    *string `db:"ship_recipient_name"`
	ShipLine1   *string `db:"ship_line1"`
	ShipLine2   *string `db:"ship_line2"`
	ShipCity    *string `db:"ship_city"`
	ShipRegion  *string `db:"ship_region"`
	ShipCountry *string `db:"ship_country"`
	ShipPostal  *string `db:"ship_postal_code"`
	ShippingFee string  `db:"shipping_fee"`
}

type orderItemRow struct {
	ProductName  string  `db:"product_name"`
	VariantLabel *string `db:"variant_label"`
	SKU          *string `db:"sku"`
	UnitPrice    string  `db:"unit_price"`
	Quantity     int     `db:"quantity"`
	LineTotal    string  `db:"line_total"`
}

func (r *Repository) loadOrderSource(ctx context.Context, id int64) (*invoiceSource, error) {
	var h orderHeadRow
	err := r.db.GetContext(ctx, &h,
		`SELECT o.order_number, o.customer_name, u.full_name AS user_name, u.email AS user_email,
			o.ship_phone, o.ship_recipient_name, o.ship_line1, o.ship_line2, o.ship_city,
			o.ship_region, o.ship_country, o.ship_postal_code, o.shipping_fee
		 FROM orders o LEFT JOIN users u ON u.id = o.user_id WHERE o.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSourceNotFound
	}
	if err != nil {
		return nil, err
	}
	var items []orderItemRow
	if err := r.db.SelectContext(ctx, &items,
		`SELECT product_name, variant_label, sku, unit_price, quantity, line_total
		 FROM order_items WHERE order_id = ? ORDER BY id`, id); err != nil {
		return nil, err
	}

	src := &invoiceSource{
		Number:       h.OrderNumber,
		BuyerName:    coalesceStr(h.CustomerNm, h.UserName),
		BuyerPhone:   h.ShipPhone,
		BuyerEmail:   h.UserEmail,
		BuyerAddress: joinAddress(h.ShipName, h.ShipLine1, h.ShipLine2, h.ShipCity, h.ShipRegion, h.ShipPostal, h.ShipCountry),
	}
	for _, it := range items {
		unit, _ := parseCents(it.UnitPrice)
		total, _ := parseCents(it.LineTotal)
		src.Lines = append(src.Lines, lineInput{
			Description: it.ProductName,
			Detail:      variantDetail(it.VariantLabel, it.SKU),
			QuantityC:   int64(it.Quantity) * 100,
			UnitPriceC:  unit,
			LineTotalC:  total,
		})
	}
	if fee, _ := parseCents(h.ShippingFee); fee > 0 {
		src.Lines = append(src.Lines, lineInput{
			Description: "Livraison",
			QuantityC:   100,
			UnitPriceC:  fee,
			LineTotalC:  fee,
		})
	}
	return src, nil
}

type bookingBuyerRow struct {
	BookingNumber string  `db:"booking_number"`
	GroupID       *int64  `db:"booking_group_id"`
	CustomerNm    *string `db:"customer_name"`
	UserName      *string `db:"user_name"`
	UserEmail     *string `db:"user_email"`
	ContactPhone  string  `db:"contact_phone"`
	PickupLoc     string  `db:"pickup_location"`
}

type bookingCarRow struct {
	CarName     string    `db:"car_name"`
	CarCategory *string   `db:"car_category"`
	Days        int       `db:"days"`
	DailyRate   string    `db:"daily_rate_snapshot"`
	Fees        string    `db:"fees"`
	StartAt     time.Time `db:"start_at"`
	EndAt       time.Time `db:"end_at"`
}

func (r *Repository) loadBookingSource(ctx context.Context, id int64) (*invoiceSource, error) {
	var b bookingBuyerRow
	err := r.db.GetContext(ctx, &b,
		`SELECT b.booking_number, b.booking_group_id, b.customer_name,
			u.full_name AS user_name, u.email AS user_email, b.contact_phone, b.pickup_location
		 FROM bookings b LEFT JOIN users u ON u.id = b.user_id WHERE b.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSourceNotFound
	}
	if err != nil {
		return nil, err
	}

	// Resolve the group head; multi-car bookings all share booking_group_id = head.
	headID := id
	number := b.BookingNumber
	isGroup := b.GroupID != nil
	if isGroup {
		headID = *b.GroupID
		var groupNumber string
		if err := r.db.GetContext(ctx, &groupNumber,
			`SELECT booking_number FROM booking_groups WHERE booking_id = ?`, headID); err == nil {
			number = groupNumber
		}
	}

	var cars []bookingCarRow
	carQuery := `SELECT car_name, car_category, days, daily_rate_snapshot, fees, start_at, end_at FROM bookings WHERE %s ORDER BY id`
	if isGroup {
		err = r.db.SelectContext(ctx, &cars, fmt.Sprintf(carQuery, "booking_group_id = ?"), headID)
	} else {
		err = r.db.SelectContext(ctx, &cars, fmt.Sprintf(carQuery, "id = ?"), id)
	}
	if err != nil {
		return nil, err
	}

	src := &invoiceSource{
		Number:       number,
		BuyerName:    coalesceStr(b.CustomerNm, b.UserName),
		BuyerPhone:   strPtr(b.ContactPhone),
		BuyerEmail:   b.UserEmail,
		BuyerAddress: strPtr(b.PickupLoc),
	}
	for _, c := range cars {
		rate, _ := parseCents(c.DailyRate)
		src.Lines = append(src.Lines, lineInput{
			Description: "Location de véhicule — " + c.CarName,
			Detail:      strPtr(bookingLineDetail(c)),
			QuantityC:   int64(c.Days) * 100,
			UnitPriceC:  rate,
			LineTotalC:  rate * int64(c.Days),
		})
		if fee, _ := parseCents(c.Fees); fee > 0 {
			src.Lines = append(src.Lines, lineInput{
				Description: "Frais annexes — " + c.CarName,
				QuantityC:   100,
				UnitPriceC:  fee,
				LineTotalC:  fee,
			})
		}
	}
	return src, nil
}

type eventRow struct {
	RequestNumber string  `db:"request_number"`
	EventType     string  `db:"event_type"`
	Location      string  `db:"location"`
	ContactPhone  string  `db:"contact_phone"`
	ContactEmail  *string `db:"contact_email"`
	CustomerNm    *string `db:"customer_name"`
	UserName      *string `db:"user_name"`
	QuotedPrice   *string `db:"quoted_price"`
}

type eventServiceRow struct {
	ServiceName     string  `db:"service_name"`
	CategoryName    *string `db:"category_name"`
	QuotedUnitPrice *string `db:"quoted_unit_price"`
	Quantity        int     `db:"quantity"`
}

type eventArtistRow struct {
	ArtistName string  `db:"artist_name"`
	QuotedFee  *string `db:"quoted_fee"`
}

func (r *Repository) loadEventSource(ctx context.Context, id int64) (*invoiceSource, error) {
	var e eventRow
	err := r.db.GetContext(ctx, &e,
		`SELECT e.request_number, e.event_type, e.location, e.contact_phone, e.contact_email,
			e.customer_name, u.full_name AS user_name, e.quoted_price
		 FROM event_requests e LEFT JOIN users u ON u.id = e.user_id WHERE e.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSourceNotFound
	}
	if err != nil {
		return nil, err
	}
	if e.QuotedPrice == nil {
		return nil, ErrNoQuote
	}

	src := &invoiceSource{
		Number:       e.RequestNumber,
		BuyerName:    coalesceStr(e.CustomerNm, e.UserName),
		BuyerPhone:   strPtr(e.ContactPhone),
		BuyerEmail:   e.ContactEmail,
		BuyerAddress: strPtr(e.Location),
	}

	var svcRows []eventServiceRow
	if err := r.db.SelectContext(ctx, &svcRows,
		`SELECT service_name, category_name, quoted_unit_price, quantity
		 FROM event_request_services WHERE request_id = ? ORDER BY id`, id); err != nil {
		return nil, err
	}
	var artRows []eventArtistRow
	if err := r.db.SelectContext(ctx, &artRows,
		`SELECT artist_name, quoted_fee FROM event_request_artists WHERE request_id = ? ORDER BY id`, id); err != nil {
		return nil, err
	}

	// Itemized when at least one line carries an agreed price. Otherwise the
	// request was quoted the legacy single-amount way → one lump line.
	itemized := false
	for _, s := range svcRows {
		if s.QuotedUnitPrice != nil {
			itemized = true
			break
		}
	}
	if !itemized {
		for _, a := range artRows {
			if a.QuotedFee != nil {
				itemized = true
				break
			}
		}
	}

	if itemized {
		for _, s := range svcRows {
			qty := s.Quantity
			if qty < 1 {
				qty = 1
			}
			unit := centsOrZero(s.QuotedUnitPrice)
			src.Lines = append(src.Lines, lineInput{
				Description: s.ServiceName,
				Detail:      s.CategoryName,
				QuantityC:   int64(qty) * 100,
				UnitPriceC:  unit,
				LineTotalC:  unit * int64(qty),
			})
		}
		for _, a := range artRows {
			fee := centsOrZero(a.QuotedFee)
			src.Lines = append(src.Lines, lineInput{
				Description: "Artiste — " + a.ArtistName,
				QuantityC:   100,
				UnitPriceC:  fee,
				LineTotalC:  fee,
			})
		}
		return src, nil
	}

	quote, _ := parseCents(*e.QuotedPrice)
	services := namesOf(svcRows)
	artists := artistNamesOf(artRows)
	src.Lines = []lineInput{{
		Description: "Organisation d'événement — " + e.EventType,
		Detail:      composeEventDetail(services, artists),
		QuantityC:   100,
		UnitPriceC:  quote,
		LineTotalC:  quote,
	}}
	return src, nil
}

// centsOrZero parses an optional decimal string to cents, treating nil/invalid
// as 0 (an unpriced itemized line).
func centsOrZero(s *string) int64 {
	if s == nil {
		return 0
	}
	c, _ := parseCents(*s)
	return c
}

func namesOf(rows []eventServiceRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if n := strings.TrimSpace(r.ServiceName); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func artistNamesOf(rows []eventArtistRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if n := strings.TrimSpace(r.ArtistName); n != "" {
			out = append(out, n)
		}
	}
	return out
}

type healthcareRow struct {
	RequestNumber string     `db:"request_number"`
	RequestType   string     `db:"request_type"`
	ServiceName   *string    `db:"service_name"`
	CustomerNm    *string    `db:"customer_name"`
	UserName      *string    `db:"user_name"`
	QuotedPrice   *string    `db:"quoted_price"`
	PatientName   string     `db:"patient_name"`
	Address       string     `db:"address"`
	ContactPhone  string     `db:"contact_phone"`
	ContactEmail  *string    `db:"contact_email"`
	PreferredAt   *time.Time `db:"preferred_at"`
	StartAt       *time.Time `db:"start_at"`
	EndAt         *time.Time `db:"end_at"`
}

func (r *Repository) loadHealthcareSource(ctx context.Context, id int64) (*invoiceSource, error) {
	var h healthcareRow
	err := r.db.GetContext(ctx, &h,
		`SELECT hr.request_number, hr.request_type, hr.service_name, hr.customer_name,
			u.full_name AS user_name, hr.quoted_price, hr.patient_name, hr.address,
			hr.contact_phone, hr.contact_email, hr.preferred_at, hr.start_at, hr.end_at
		 FROM healthcare_requests hr LEFT JOIN users u ON u.id = hr.user_id WHERE hr.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSourceNotFound
	}
	if err != nil {
		return nil, err
	}
	if h.QuotedPrice == nil {
		return nil, ErrNoQuote
	}
	quote, _ := parseCents(*h.QuotedPrice)

	staff := r.namesList(ctx,
		`SELECT CONCAT(practitioner_name, ' (', practitioner_type, ')') FROM healthcare_request_assignments WHERE request_id = ? ORDER BY id`, id)

	desc := "Soins à domicile"
	if h.ServiceName != nil && strings.TrimSpace(*h.ServiceName) != "" {
		desc = *h.ServiceName
	} else if h.RequestType == "consultation" {
		desc = "Consultation à domicile"
	} else {
		desc = "Forfait de soins à domicile"
	}

	src := &invoiceSource{
		Number:       h.RequestNumber,
		BuyerName:    coalesceStr(h.CustomerNm, h.UserName),
		BuyerPhone:   strPtr(h.ContactPhone),
		BuyerEmail:   h.ContactEmail,
		BuyerAddress: strPtr(h.Address),
		Lines: []lineInput{{
			Description: desc,
			Detail:      composeHealthcareDetail(h, staff),
			QuantityC:   100,
			UnitPriceC:  quote,
			LineTotalC:  quote,
		}},
	}
	return src, nil
}

// namesList runs a single-column query and returns the values (skipping blanks).
func (r *Repository) namesList(ctx context.Context, query string, id int64) []string {
	var rows []string
	if err := r.db.SelectContext(ctx, &rows, query, id); err != nil {
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, s := range rows {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// --- helpers ---

func mapWriteErr(err error) error {
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1062 {
		return ErrConflict
	}
	return err
}

func nullableJSON(j []byte) any {
	if len(strings.TrimSpace(string(j))) == 0 {
		return nil
	}
	return string(j)
}
