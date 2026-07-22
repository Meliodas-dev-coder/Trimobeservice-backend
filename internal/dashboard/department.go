package dashboard

import (
	"context"
	"strings"
	"time"

	"github.com/trimo/backend/internal/catalog"
)

// The department overview is the home screen of a catalog department's back
// office (Coffee, Tech, Fashion). Every number is computed from the same tables
// the operational screens read, so the overview can never quote a figure the
// order or stock list would contradict.
//
// Revenue attribution: the customer pays one order once, so a department's
// revenue is the sum of ITS order lines on paid orders. Shipping fees stay out
// of it — they belong to the order, not to any one department.

// DepartmentOverview is the whole payload for one department's home screen.
type DepartmentOverview struct {
	Department     string              `json:"department"`
	KPIs           DepartmentKPIs      `json:"kpis"`
	RevenueSeries  []DepartmentRevenue `json:"revenue_series"`
	OrdersByStatus []StatusCount       `json:"orders_by_status"`
	TopProducts    []DepartmentProduct `json:"top_products"`
	RecentOrders   []DepartmentOrder   `json:"recent_orders"`
	Attention      DepartmentAttention `json:"attention"`
}

type DepartmentKPIs struct {
	RevenueTotal     string `json:"revenue_total"`
	RevenueMonth     string `json:"revenue_month"`
	RevenuePrevMonth string `json:"revenue_prev_month"`
	UnitsSold        int    `json:"units_sold"`
	UnitsSoldMonth   int    `json:"units_sold_month"`
	OrdersTotal      int    `json:"orders_total"`
	OrdersMonth      int    `json:"orders_month"`
	OrdersOpen       int    `json:"orders_open"`
	OrdersUnpaid     int    `json:"orders_unpaid"`
	UnpaidValue      string `json:"unpaid_value"`
	AverageOrder     string `json:"average_order"`
	ProductsTotal    int    `json:"products_total"`
	ProductsActive   int    `json:"products_active"`
	CategoriesTotal  int    `json:"categories_total"`
	SKUsTotal        int    `json:"skus_total"`
	UnitsOnHand      int    `json:"units_on_hand"`
	StockValue       string `json:"stock_value"`
	LowStock         int    `json:"low_stock"`
	OutOfStock       int    `json:"out_of_stock"`
}

// DepartmentRevenue is one day of this department's share of paid orders.
type DepartmentRevenue struct {
	Date    string `json:"date"` // YYYY-MM-DD
	Revenue string `json:"revenue"`
	Units   int    `json:"units"`
}

type DepartmentProduct struct {
	ProductID   int64  `db:"product_id" json:"product_id"`
	ProductName string `db:"product_name" json:"product_name"`
	Units       int    `db:"units" json:"units"`
	Revenue     string `db:"revenue" json:"revenue"`
}

// DepartmentOrder is one order as this department sees it: the customer's
// single order, narrowed to this department's lines.
type DepartmentOrder struct {
	ID                 int64     `db:"id" json:"id"`
	OrderNumber        string    `db:"order_number" json:"order_number"`
	CustomerName       *string   `db:"customer_name" json:"customer_name,omitempty"`
	Status             string    `db:"status" json:"status"`
	PaymentStatus      string    `db:"payment_status" json:"payment_status"`
	FulfillmentType    string    `db:"fulfillment_type" json:"fulfillment_type"`
	Total              string    `db:"total" json:"total"`
	DepartmentSubtotal string    `db:"department_subtotal" json:"department_subtotal"`
	DepartmentQuantity int       `db:"department_quantity" json:"department_quantity"`
	Departments        *string   `db:"departments" json:"departments,omitempty"`
	CreatedAt          time.Time `db:"created_at" json:"created_at"`
}

// DepartmentStockItem is one SKU in the low-stock queue.
type DepartmentStockItem struct {
	VariantID        int64   `db:"variant_id" json:"variant_id"`
	ProductID        int64   `db:"product_id" json:"product_id"`
	ProductName      string  `db:"product_name" json:"product_name"`
	SKU              string  `db:"sku" json:"sku"`
	Label            *string `db:"label" json:"label,omitempty"`
	StockQuantity    int     `db:"stock_quantity" json:"stock_quantity"`
	ReorderThreshold int     `db:"reorder_threshold" json:"reorder_threshold"`
	Level            string  `db:"level" json:"level"`
}

type DepartmentAttention struct {
	OrdersToFulfil []DepartmentOrder     `json:"orders_to_fulfil"`
	UnpaidOrders   []DepartmentOrder     `json:"unpaid_orders"`
	LowStock       []DepartmentStockItem `json:"low_stock"`
}

// openStatuses are orders still owed something: not cancelled, not expired, and
// not yet handed over.
const openStatuses = `('pending', 'confirmed', 'shipped')`

// cancelledStatuses excludes orders that were called off, so they never inflate a
// revenue or backlog figure.
const cancelledStatuses = `('cancelled', 'expired')`

// departmentLevelExpr mirrors the stock module's level buckets. Kept literal
// here (rather than imported) because it is written against this query's
// aliases; the two must move together.
const departmentLevelExpr = `CASE
	WHEN pv.stock_quantity <= 0 THEN 'out'
	WHEN pv.reorder_threshold > 0 AND pv.stock_quantity <= pv.reorder_threshold THEN 'low'
	ELSE 'ok' END`

// departmentOrderCols narrow an order to one department's share.
const departmentOrderCols = `o.id, o.order_number,
	COALESCE(o.customer_name, (SELECT full_name FROM users u WHERE u.id = o.user_id)) AS customer_name,
	o.status, o.payment_status, o.fulfillment_type, o.total, o.created_at,
	(SELECT COALESCE(SUM(oi.line_total), 0) FROM order_items oi
	   WHERE oi.order_id = o.id AND oi.department = ?) AS department_subtotal,
	(SELECT COALESCE(SUM(oi.quantity), 0) FROM order_items oi
	   WHERE oi.order_id = o.id AND oi.department = ?) AS department_quantity,
	(SELECT GROUP_CONCAT(DISTINCT oi.department ORDER BY oi.department SEPARATOR ',')
	   FROM order_items oi WHERE oi.order_id = o.id) AS departments`

// LoadDepartment builds one department's overview.
func (r *Repository) LoadDepartment(ctx context.Context, department string) (*DepartmentOverview, error) {
	out := &DepartmentOverview{Department: department}
	templateKeys := catalog.TemplateKeysForDepartment(department)

	if err := r.loadDepartmentKPIs(ctx, department, templateKeys, &out.KPIs); err != nil {
		return nil, err
	}
	series, err := r.loadDepartmentRevenue(ctx, department)
	if err != nil {
		return nil, err
	}
	out.RevenueSeries = series

	if err := r.db.SelectContext(ctx, &out.OrdersByStatus, `
		SELECT o.status, COUNT(*) AS count
		FROM orders o
		WHERE EXISTS (SELECT 1 FROM order_items oi WHERE oi.order_id = o.id AND oi.department = ?)
		GROUP BY o.status ORDER BY count DESC`, department); err != nil {
		return nil, err
	}

	if err := r.db.SelectContext(ctx, &out.TopProducts, `
		SELECT COALESCE(pv.product_id, 0) AS product_id, oi.product_name,
		       SUM(oi.quantity) AS units, SUM(oi.line_total) AS revenue
		FROM order_items oi
		JOIN orders o ON o.id = oi.order_id
		LEFT JOIN product_variants pv ON pv.id = oi.product_variant_id
		WHERE oi.department = ? AND o.status NOT IN `+cancelledStatuses+`
		GROUP BY oi.product_name, pv.product_id
		ORDER BY units DESC, revenue DESC
		LIMIT 5`, department); err != nil {
		return nil, err
	}

	if out.RecentOrders, err = r.departmentOrders(ctx, department, "", 6); err != nil {
		return nil, err
	}
	if out.Attention.OrdersToFulfil, err = r.departmentOrders(ctx, department,
		"AND o.status IN "+openStatuses, 5); err != nil {
		return nil, err
	}
	if out.Attention.UnpaidOrders, err = r.departmentOrders(ctx, department,
		"AND o.payment_status = 'unpaid' AND o.status NOT IN "+cancelledStatuses, 5); err != nil {
		return nil, err
	}
	if out.Attention.LowStock, err = r.departmentLowStock(ctx, templateKeys, 6); err != nil {
		return nil, err
	}

	// Never hand the client a null array.
	if out.OrdersByStatus == nil {
		out.OrdersByStatus = []StatusCount{}
	}
	if out.TopProducts == nil {
		out.TopProducts = []DepartmentProduct{}
	}
	return out, nil
}

func (r *Repository) loadDepartmentKPIs(ctx context.Context, department string, templateKeys []string, k *DepartmentKPIs) error {
	month := monthStart(time.Now())
	prev := month.AddDate(0, -1, 0)

	// Sales: this department's lines, on orders that were actually paid.
	var sales struct {
		Total     string `db:"total"`
		ThisMonth string `db:"this_month"`
		PrevMonth string `db:"prev_month"`
		Units     int    `db:"units"`
		UnitsThis int    `db:"units_this_month"`
	}
	if err := r.db.GetContext(ctx, &sales, `
		SELECT COALESCE(SUM(oi.line_total), 0) AS total,
		       COALESCE(SUM(CASE WHEN o.paid_at >= ? THEN oi.line_total ELSE 0 END), 0) AS this_month,
		       COALESCE(SUM(CASE WHEN o.paid_at >= ? AND o.paid_at < ? THEN oi.line_total ELSE 0 END), 0) AS prev_month,
		       COALESCE(SUM(oi.quantity), 0) AS units,
		       COALESCE(SUM(CASE WHEN o.paid_at >= ? THEN oi.quantity ELSE 0 END), 0) AS units_this_month
		FROM order_items oi
		JOIN orders o ON o.id = oi.order_id
		WHERE oi.department = ? AND o.payment_status = 'paid'`,
		month, prev, month, month, department); err != nil {
		return err
	}
	k.RevenueTotal, k.RevenueMonth, k.RevenuePrevMonth = sales.Total, sales.ThisMonth, sales.PrevMonth
	k.UnitsSold, k.UnitsSoldMonth = sales.Units, sales.UnitsThis

	// Order counts and the unpaid backlog, valued at this department's share.
	var orderStats struct {
		Total       int    `db:"total"`
		Month       int    `db:"month"`
		Open        int    `db:"open"`
		Unpaid      int    `db:"unpaid"`
		UnpaidValue string `db:"unpaid_value"`
	}
	if err := r.db.GetContext(ctx, &orderStats, `
		SELECT COUNT(*) AS total,
		       COALESCE(SUM(o.created_at >= ?), 0) AS month,
		       COALESCE(SUM(o.status IN `+openStatuses+`), 0) AS open,
		       COALESCE(SUM(o.payment_status = 'unpaid' AND o.status NOT IN `+cancelledStatuses+`), 0) AS unpaid,
		       COALESCE(SUM(CASE WHEN o.payment_status = 'unpaid' AND o.status NOT IN `+cancelledStatuses+`
		                    THEN (SELECT COALESCE(SUM(oi.line_total), 0) FROM order_items oi
		                            WHERE oi.order_id = o.id AND oi.department = ?)
		                    ELSE 0 END), 0) AS unpaid_value
		FROM orders o
		WHERE EXISTS (SELECT 1 FROM order_items oi WHERE oi.order_id = o.id AND oi.department = ?)`,
		month, department, department); err != nil {
		return err
	}
	k.OrdersTotal, k.OrdersMonth = orderStats.Total, orderStats.Month
	k.OrdersOpen, k.OrdersUnpaid = orderStats.Open, orderStats.Unpaid
	k.UnpaidValue = orderStats.UnpaidValue

	// Average basket = paid revenue over the orders that produced it.
	if err := r.db.GetContext(ctx, &k.AverageOrder, `
		SELECT COALESCE(AVG(share), 0) FROM (
		  SELECT SUM(oi.line_total) AS share
		  FROM order_items oi
		  JOIN orders o ON o.id = oi.order_id
		  WHERE oi.department = ? AND o.payment_status = 'paid'
		  GROUP BY oi.order_id
		) AS baskets`, department); err != nil {
		return err
	}

	// Catalog and shelf, scoped by the department's category templates.
	if len(templateKeys) == 0 {
		return nil
	}
	placeholders := "?" + strings.Repeat(", ?", len(templateKeys)-1)
	args := make([]any, 0, len(templateKeys))
	for _, key := range templateKeys {
		args = append(args, key)
	}

	var catalogStats struct {
		Products   int `db:"products"`
		Active     int `db:"active"`
		Categories int `db:"categories"`
	}
	if err := r.db.GetContext(ctx, &catalogStats, `
		SELECT COUNT(*) AS products,
		       COALESCE(SUM(p.is_active), 0) AS active,
		       COUNT(DISTINCT p.category_id) AS categories
		FROM products p
		JOIN product_categories pc ON pc.id = p.category_id
		WHERE pc.template_key IN (`+placeholders+`)`, args...); err != nil {
		return err
	}
	k.ProductsTotal, k.ProductsActive = catalogStats.Products, catalogStats.Active

	if err := r.db.GetContext(ctx, &k.CategoriesTotal, `
		SELECT COUNT(*) FROM product_categories WHERE template_key IN (`+placeholders+`)`, args...); err != nil {
		return err
	}

	var stockStats struct {
		SKUs  int    `db:"skus"`
		Units int    `db:"units"`
		Value string `db:"value"`
		Low   int    `db:"low"`
		Out   int    `db:"out"`
	}
	if err := r.db.GetContext(ctx, &stockStats, `
		SELECT COUNT(*) AS skus,
		       COALESCE(SUM(pv.stock_quantity), 0) AS units,
		       COALESCE(SUM(pv.price * pv.stock_quantity), 0) AS value,
		       COALESCE(SUM(`+departmentLevelExpr+` = 'low'), 0) AS low,
		       COALESCE(SUM(`+departmentLevelExpr+` = 'out'), 0) AS `+"`out`"+`
		FROM product_variants pv
		JOIN products p ON p.id = pv.product_id
		JOIN product_categories pc ON pc.id = p.category_id
		WHERE pc.template_key IN (`+placeholders+`)`, args...); err != nil {
		return err
	}
	k.SKUsTotal, k.UnitsOnHand, k.StockValue = stockStats.SKUs, stockStats.Units, stockStats.Value
	k.LowStock, k.OutOfStock = stockStats.Low, stockStats.Out
	return nil
}

// loadDepartmentRevenue returns revenueDays points ending today, zero-filled so
// the chart keeps a continuous axis.
func (r *Repository) loadDepartmentRevenue(ctx context.Context, department string) ([]DepartmentRevenue, error) {
	start := dayStart(time.Now()).AddDate(0, 0, -(revenueDays - 1))
	rows := []struct {
		Date    string `db:"date"`
		Revenue string `db:"revenue"`
		Units   int    `db:"units"`
	}{}
	err := r.db.SelectContext(ctx, &rows, `
		SELECT DATE(o.paid_at) AS date,
		       COALESCE(SUM(oi.line_total), 0) AS revenue,
		       COALESCE(SUM(oi.quantity), 0) AS units
		FROM order_items oi
		JOIN orders o ON o.id = oi.order_id
		WHERE oi.department = ? AND o.payment_status = 'paid' AND o.paid_at >= ?
		GROUP BY DATE(o.paid_at)`, department, start)
	if err != nil {
		return nil, err
	}

	byDate := make(map[string]DepartmentRevenue, len(rows))
	for _, row := range rows {
		byDate[row.Date] = DepartmentRevenue{Date: row.Date, Revenue: row.Revenue, Units: row.Units}
	}
	series := make([]DepartmentRevenue, 0, revenueDays)
	for i := 0; i < revenueDays; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		point := DepartmentRevenue{Date: day, Revenue: "0.00"}
		if row, ok := byDate[day]; ok {
			point = row
		}
		series = append(series, point)
	}
	return series, nil
}

// departmentOrders lists orders touching the department, narrowed to its share.
// extraWhere is a trusted, caller-supplied fragment (never user input).
func (r *Repository) departmentOrders(ctx context.Context, department, extraWhere string, limit int) ([]DepartmentOrder, error) {
	out := []DepartmentOrder{}
	err := r.db.SelectContext(ctx, &out, `
		SELECT `+departmentOrderCols+`
		FROM orders o
		WHERE EXISTS (SELECT 1 FROM order_items oi WHERE oi.order_id = o.id AND oi.department = ?)
		  `+extraWhere+`
		ORDER BY o.created_at DESC
		LIMIT ?`, department, department, department, limit)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) departmentLowStock(ctx context.Context, templateKeys []string, limit int) ([]DepartmentStockItem, error) {
	out := []DepartmentStockItem{}
	if len(templateKeys) == 0 {
		return out, nil
	}
	placeholders := "?" + strings.Repeat(", ?", len(templateKeys)-1)
	args := make([]any, 0, len(templateKeys)+1)
	for _, key := range templateKeys {
		args = append(args, key)
	}
	args = append(args, limit)
	err := r.db.SelectContext(ctx, &out, `
		SELECT pv.id AS variant_id, pv.product_id, p.name AS product_name, pv.sku, pv.label,
		       pv.stock_quantity, pv.reorder_threshold, `+departmentLevelExpr+` AS level
		FROM product_variants pv
		JOIN products p ON p.id = pv.product_id
		JOIN product_categories pc ON pc.id = p.category_id
		WHERE pc.template_key IN (`+placeholders+`)
		  AND `+departmentLevelExpr+` IN ('out', 'low')
		ORDER BY FIELD(`+departmentLevelExpr+`, 'out', 'low'), pv.stock_quantity ASC, pv.id ASC
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// LoadDepartment is the service-level entry point.
func (s *Service) LoadDepartment(ctx context.Context, department string) (*DepartmentOverview, error) {
	return s.repo.LoadDepartment(ctx, department)
}
