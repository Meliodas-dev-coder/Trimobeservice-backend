// Package dashboard is a read-only, admin-only aggregate over the whole system:
// KPI counters, a 30-day paid-revenue series, status breakdowns for orders,
// bookings, events, and healthcare requests, the paid-payment method mix, and
// the "needs attention" queues (unpaid orders, bookings to confirm, event and
// healthcare requests to review). Everything is computed in SQL so the admin home can
// render real numbers and charts in a single request.
package dashboard

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// revenueDays is the width of the revenue time series (today included).
const revenueDays = 30

// Dashboard is the full payload the admin home screen renders from.
type Dashboard struct {
	KPIs               KPIs           `json:"kpis"`
	RevenueSeries      []RevenuePoint `json:"revenue_series"`
	OrdersByStatus     []StatusCount  `json:"orders_by_status"`
	BookingsByStatus   []StatusCount  `json:"bookings_by_status"`
	EventsByStatus     []StatusCount  `json:"events_by_status"`
	HealthcareByStatus []StatusCount  `json:"healthcare_by_status"`
	PaymentMethods     []MethodTotal  `json:"payment_methods"`
	Attention          Attention      `json:"attention"`
}

type KPIs struct {
	RevenueTotal       string `json:"revenue_total"`
	RevenueMonth       string `json:"revenue_month"`
	RevenuePrevMonth   string `json:"revenue_prev_month"`
	OrdersTotal        int    `json:"orders_total"`
	OrdersUnpaid       int    `json:"orders_unpaid"`
	BookingsTotal      int    `json:"bookings_total"`
	BookingsToConfirm  int    `json:"bookings_to_confirm"`
	EventRequestsTotal int    `json:"event_requests_total"`
	EventsToReview     int    `json:"events_to_review"`
	CustomersTotal     int    `json:"customers_total"`
	CustomersNewMonth  int    `json:"customers_new_month"`
	ProductsActive     int    `json:"products_active"`
	CarsTotal          int    `json:"cars_total"`
	CarsAvailable      int    `json:"cars_available"`
	HealthcareTotal    int    `json:"healthcare_total"`
	HealthcareToReview int    `json:"healthcare_to_review"`
}

// RevenuePoint is one day of confirmed (paid) revenue, split by source.
type RevenuePoint struct {
	Date       string `json:"date"`       // YYYY-MM-DD
	Orders     string `json:"orders"`     // DECIMAL string
	Bookings   string `json:"bookings"`   // DECIMAL string
	Events     string `json:"events"`     // DECIMAL string
	Healthcare string `json:"healthcare"` // DECIMAL string
}

type StatusCount struct {
	Status string `db:"status" json:"status"`
	Count  int    `db:"count" json:"count"`
}

type MethodTotal struct {
	Method string `db:"method" json:"method"`
	Count  int    `db:"count" json:"count"`
	Amount string `db:"amount" json:"amount"`
}

// AttentionItem is one row in a "needs attention" queue.
type AttentionItem struct {
	Number    string    `db:"number" json:"number"`
	Label     *string   `db:"label" json:"label,omitempty"` // car name (bookings) — nil for orders
	Total     *string   `db:"total" json:"total,omitempty"` // amount (orders) — nil for bookings
	Status    string    `db:"status" json:"status"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type Attention struct {
	UnpaidOrders       []AttentionItem `json:"unpaid_orders"`
	BookingsToConfirm  []AttentionItem `json:"bookings_to_confirm"`
	EventsToReview     []AttentionItem `json:"events_to_review"`
	HealthcareToReview []AttentionItem `json:"healthcare_to_review"`
}

// dayRevenue is the raw per-day split returned by the revenue query.
type dayRevenue struct {
	Date       string `db:"date"`
	Orders     string `db:"orders"`
	Bookings   string `db:"bookings"`
	Events     string `db:"events"`
	Healthcare string `db:"healthcare"`
}

// Repository runs the aggregate queries.
type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Load(ctx context.Context) (*Dashboard, error) {
	out := &Dashboard{}

	if err := r.loadKPIs(ctx, &out.KPIs); err != nil {
		return nil, err
	}

	series, err := r.loadRevenueSeries(ctx)
	if err != nil {
		return nil, err
	}
	out.RevenueSeries = series

	if err := r.db.SelectContext(ctx, &out.OrdersByStatus,
		`SELECT status, COUNT(*) AS count FROM orders GROUP BY status ORDER BY count DESC`); err != nil {
		return nil, err
	}
	if err := r.db.SelectContext(ctx, &out.BookingsByStatus,
		`SELECT status, COUNT(*) AS count
		   FROM bookings
		  WHERE booking_group_id IS NULL OR id = booking_group_id
		  GROUP BY status ORDER BY count DESC`); err != nil {
		return nil, err
	}
	if err := r.db.SelectContext(ctx, &out.EventsByStatus,
		`SELECT status, COUNT(*) AS count FROM event_requests GROUP BY status ORDER BY count DESC`); err != nil {
		return nil, err
	}
	if err := r.db.SelectContext(ctx, &out.HealthcareByStatus,
		`SELECT status, COUNT(*) AS count FROM healthcare_requests GROUP BY status ORDER BY count DESC`); err != nil {
		return nil, err
	}
	if err := r.db.SelectContext(ctx, &out.PaymentMethods,
		`SELECT method, COUNT(*) AS count, COALESCE(SUM(amount), 0) AS amount
		   FROM payments WHERE status = 'paid' GROUP BY method ORDER BY amount DESC`); err != nil {
		return nil, err
	}

	if out.Attention.UnpaidOrders, err = r.loadUnpaidOrders(ctx); err != nil {
		return nil, err
	}
	if out.Attention.BookingsToConfirm, err = r.loadBookingsToConfirm(ctx); err != nil {
		return nil, err
	}
	if out.Attention.EventsToReview, err = r.loadEventsToReview(ctx); err != nil {
		return nil, err
	}
	if out.Attention.HealthcareToReview, err = r.loadHealthcareToReview(ctx); err != nil {
		return nil, err
	}

	// Ensure the JSON arrays are never null.
	if out.OrdersByStatus == nil {
		out.OrdersByStatus = []StatusCount{}
	}
	if out.BookingsByStatus == nil {
		out.BookingsByStatus = []StatusCount{}
	}
	if out.EventsByStatus == nil {
		out.EventsByStatus = []StatusCount{}
	}
	if out.HealthcareByStatus == nil {
		out.HealthcareByStatus = []StatusCount{}
	}
	if out.PaymentMethods == nil {
		out.PaymentMethods = []MethodTotal{}
	}
	return out, nil
}

func (r *Repository) loadKPIs(ctx context.Context, k *KPIs) error {
	// Paid revenue (all-time / this month / previous month), from the ledger.
	const revQuery = `
		SELECT
			COALESCE(SUM(amount), 0) AS total,
			COALESCE(SUM(CASE WHEN marked_paid_at >= ? THEN amount ELSE 0 END), 0) AS this_month,
			COALESCE(SUM(CASE WHEN marked_paid_at >= ? AND marked_paid_at < ? THEN amount ELSE 0 END), 0) AS prev_month
		FROM payments WHERE status = 'paid'`
	monthStart := monthStart(time.Now())
	prevStart := monthStart.AddDate(0, -1, 0)
	var rev struct {
		Total     string `db:"total"`
		ThisMonth string `db:"this_month"`
		PrevMonth string `db:"prev_month"`
	}
	if err := r.db.GetContext(ctx, &rev, revQuery, monthStart, prevStart, monthStart); err != nil {
		return err
	}
	k.RevenueTotal, k.RevenueMonth, k.RevenuePrevMonth = rev.Total, rev.ThisMonth, rev.PrevMonth

	// A batch of independent COUNTs. Each is cheap; keep them readable over clever.
	counts := []struct {
		dst   *int
		query string
		args  []any
	}{
		{&k.OrdersTotal, `SELECT COUNT(*) FROM orders`, nil},
		{&k.OrdersUnpaid, `SELECT COUNT(*) FROM orders WHERE payment_status = 'unpaid' AND status NOT IN ('cancelled','expired')`, nil},
		{&k.BookingsTotal, `SELECT COUNT(*) FROM bookings WHERE booking_group_id IS NULL OR id = booking_group_id`, nil},
		{&k.BookingsToConfirm, `SELECT COUNT(*) FROM bookings WHERE status = 'confirmed' AND (booking_group_id IS NULL OR id = booking_group_id)`, nil},
		{&k.EventRequestsTotal, `SELECT COUNT(*) FROM event_requests`, nil},
		{&k.EventsToReview, `SELECT COUNT(*) FROM event_requests WHERE status IN ('requested','reviewing')`, nil},
		{&k.CustomersTotal, `SELECT COUNT(*) FROM users WHERE role = 'customer'`, nil},
		{&k.CustomersNewMonth, `SELECT COUNT(*) FROM users WHERE role = 'customer' AND created_at >= ?`, []any{monthStart}},
		{&k.ProductsActive, `SELECT COUNT(*) FROM products WHERE is_active = 1`, nil},
		{&k.CarsTotal, `SELECT COUNT(*) FROM cars`, nil},
		{&k.CarsAvailable, `SELECT COUNT(*) FROM cars WHERE status = 'available'`, nil},
		{&k.HealthcareTotal, `SELECT COUNT(*) FROM healthcare_requests`, nil},
		{&k.HealthcareToReview, `SELECT COUNT(*) FROM healthcare_requests WHERE status IN ('requested','reviewing')`, nil},
	}
	for _, c := range counts {
		if err := r.db.GetContext(ctx, c.dst, c.query, c.args...); err != nil {
			return err
		}
	}
	return nil
}

// loadRevenueSeries returns exactly revenueDays points ending today, filling any
// day with no confirmed payments as "0.00" so the chart has a continuous axis.
func (r *Repository) loadRevenueSeries(ctx context.Context) ([]RevenuePoint, error) {
	start := dayStart(time.Now()).AddDate(0, 0, -(revenueDays - 1))
	rows := []dayRevenue{}
	err := r.db.SelectContext(ctx, &rows, `
		SELECT DATE(marked_paid_at) AS date,
			COALESCE(SUM(CASE WHEN payable_type = 'order' THEN amount ELSE 0 END), 0) AS orders,
			COALESCE(SUM(CASE WHEN payable_type = 'booking' THEN amount ELSE 0 END), 0) AS bookings,
			COALESCE(SUM(CASE WHEN payable_type = 'event' THEN amount ELSE 0 END), 0) AS events,
			COALESCE(SUM(CASE WHEN payable_type = 'healthcare' THEN amount ELSE 0 END), 0) AS healthcare
		FROM payments
		WHERE status = 'paid' AND marked_paid_at >= ?
		GROUP BY DATE(marked_paid_at)`, start)
	if err != nil {
		return nil, err
	}

	byDate := make(map[string]dayRevenue, len(rows))
	for _, row := range rows {
		byDate[row.Date] = row
	}

	series := make([]RevenuePoint, 0, revenueDays)
	for i := 0; i < revenueDays; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		point := RevenuePoint{Date: day, Orders: "0.00", Bookings: "0.00", Events: "0.00", Healthcare: "0.00"}
		if row, ok := byDate[day]; ok {
			point.Orders, point.Bookings = row.Orders, row.Bookings
			point.Events, point.Healthcare = row.Events, row.Healthcare
		}
		series = append(series, point)
	}
	return series, nil
}

func (r *Repository) loadUnpaidOrders(ctx context.Context) ([]AttentionItem, error) {
	items := []AttentionItem{}
	err := r.db.SelectContext(ctx, &items, `
		SELECT order_number AS number, NULL AS label, total AS total, status, created_at
		FROM orders
		WHERE payment_status = 'unpaid' AND status NOT IN ('cancelled','expired')
		ORDER BY created_at DESC LIMIT 5`)
	return items, err
}

func (r *Repository) loadBookingsToConfirm(ctx context.Context) ([]AttentionItem, error) {
	items := []AttentionItem{}
	err := r.db.SelectContext(ctx, &items, `
		SELECT COALESCE(bg.booking_number, b.booking_number) AS number,
		       CASE WHEN bg.booking_id IS NOT NULL
		            THEN CONCAT((SELECT COUNT(*) FROM bookings bi WHERE bi.booking_group_id = bg.booking_id), ' cars')
		            ELSE b.car_name END AS label,
		       NULL AS total, b.status, b.created_at
		FROM bookings b
		LEFT JOIN booking_groups bg ON bg.booking_id = b.booking_group_id
		WHERE b.status = 'confirmed'
		  AND (b.booking_group_id IS NULL OR b.id = b.booking_group_id)
		ORDER BY b.start_at ASC LIMIT 5`)
	return items, err
}

func (r *Repository) loadEventsToReview(ctx context.Context) ([]AttentionItem, error) {
	items := []AttentionItem{}
	err := r.db.SelectContext(ctx, &items, `
		SELECT request_number AS number, event_type AS label, quoted_price AS total, status, created_at
		FROM event_requests
		WHERE status IN ('requested','reviewing')
		ORDER BY event_start ASC LIMIT 5`)
	return items, err
}

func (r *Repository) loadHealthcareToReview(ctx context.Context) ([]AttentionItem, error) {
	items := []AttentionItem{}
	err := r.db.SelectContext(ctx, &items, `
		SELECT request_number AS number, COALESCE(service_name, request_type) AS label, NULL AS total, status, created_at
		FROM healthcare_requests
		WHERE status IN ('requested','reviewing')
		ORDER BY created_at DESC LIMIT 5`)
	return items, err
}

// monthStart is midnight on the first day of value's month, local time.
func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// Service is a thin read-only pass-through over the repository.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Load(ctx context.Context) (*Dashboard, error) {
	return s.repo.Load(ctx)
}
