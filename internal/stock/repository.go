package stock

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jmoiron/sqlx"
)

var (
	// ErrVariantNotFound is returned for an unknown or out-of-scope SKU.
	ErrVariantNotFound = errors.New("sku not found")
	// ErrNegativeStock guards against an adjustment that would push a SKU below zero.
	ErrNegativeStock = errors.New("stock cannot go below zero")
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// itemCols joins each SKU to the product context an admin needs to recognise it
// and derives its level and shelf value in SQL.
const itemCols = `pv.id AS variant_id, pv.product_id, p.name AS product_name, p.slug AS product_slug,
	p.is_active AS product_is_active, pc.name AS category_name, pc.template_key,
	b.name AS brand_name, pv.sku, pv.label, pv.price, pv.stock_quantity, pv.reorder_threshold,
	pv.is_active, pv.updated_at,
	(pv.price * pv.stock_quantity) AS stock_value,
	` + levelExpr + ` AS level,
	COALESCE(
	  (SELECT url FROM product_images pi WHERE pi.variant_id = pv.id
	     ORDER BY pi.is_primary DESC, pi.sort_order, pi.id LIMIT 1),
	  (SELECT url FROM product_images pi WHERE pi.product_id = p.id
	     ORDER BY pi.is_primary DESC, pi.sort_order, pi.id LIMIT 1)
	) AS primary_image_url,
	(SELECT MAX(sm.created_at) FROM stock_movements sm WHERE sm.product_variant_id = pv.id) AS last_movement_at`

const itemFrom = `FROM product_variants pv
	JOIN products p ON p.id = pv.product_id
	JOIN product_categories pc ON pc.id = p.category_id
	LEFT JOIN brands b ON b.id = p.brand_id`

// scopeClause turns a filter into a WHERE fragment. Departments are expressed
// as the set of category template keys that belong to them, resolved by the
// caller — see Service.templateKeys.
func scopeClause(f Filter, templateKeys []string) (string, []any) {
	var where []string
	var args []any

	// An empty template set means "no authorized department": match nothing
	// rather than silently widening to the whole catalog.
	if len(templateKeys) == 0 {
		return " WHERE 1 = 0", nil
	}
	where = append(where, "pc.template_key IN (?"+strings.Repeat(", ?", len(templateKeys)-1)+")")
	for _, key := range templateKeys {
		args = append(args, key)
	}

	if f.Level != "" {
		where = append(where, levelExpr+" = ?")
		args = append(args, f.Level)
	}
	if f.ProductID != nil {
		where = append(where, "pv.product_id = ?")
		args = append(args, *f.ProductID)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		where = append(where, "(p.name LIKE ? OR pv.sku LIKE ? OR pv.label LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	return " WHERE " + strings.Join(where, " AND "), args
}

func (r *Repository) ListItems(ctx context.Context, f Filter, templateKeys []string) ([]Item, int, error) {
	clause, args := scopeClause(f, templateKeys)

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) `+itemFrom+clause, args...); err != nil {
		return nil, 0, err
	}

	// Out of stock first, then low: the screen opens on what needs a decision.
	listArgs := append(append([]any{}, args...), f.Limit, f.Offset)
	out := []Item{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+itemCols+` `+itemFrom+clause+`
		 ORDER BY FIELD(`+levelExpr+`, 'out', 'low', 'ok'), pv.stock_quantity ASC, p.name ASC, pv.id ASC
		 LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) GetItem(ctx context.Context, variantID int64) (*Item, error) {
	var item Item
	err := r.db.GetContext(ctx, &item,
		`SELECT `+itemCols+` `+itemFrom+` WHERE pv.id = ?`, variantID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrVariantNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) Summary(ctx context.Context, templateKeys []string) (*Summary, error) {
	clause, args := scopeClause(Filter{}, templateKeys)
	var s Summary
	err := r.db.GetContext(ctx, &s, `
		SELECT COUNT(*) AS sku_count,
		       COALESCE(SUM(pv.is_active AND p.is_active), 0) AS active_sku_count,
		       COUNT(DISTINCT pv.product_id) AS product_count,
		       COALESCE(SUM(pv.stock_quantity), 0) AS units_on_hand,
		       COALESCE(SUM(pv.price * pv.stock_quantity), 0) AS stock_value,
		       COALESCE(SUM(`+levelExpr+` = 'low'), 0) AS low_stock,
		       COALESCE(SUM(`+levelExpr+` = 'out'), 0) AS out_of_stock
		`+itemFrom+clause, args...)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// LowStockItems is the attention queue: everything out of or running low on
// stock, worst first. Used by the stock screen and the department overview.
func (r *Repository) LowStockItems(ctx context.Context, templateKeys []string, limit int) ([]Item, error) {
	clause, args := scopeClause(Filter{}, templateKeys)
	args = append(args, limit)
	out := []Item{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+itemCols+` `+itemFrom+clause+`
		   AND `+levelExpr+` IN ('out', 'low')
		 ORDER BY FIELD(`+levelExpr+`, 'out', 'low'), pv.stock_quantity ASC, pv.id ASC
		 LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) ListMovements(ctx context.Context, variantID int64, limit, offset int) ([]MovementView, int, error) {
	var total int
	if err := r.db.GetContext(ctx, &total,
		`SELECT COUNT(*) FROM stock_movements WHERE product_variant_id = ?`, variantID); err != nil {
		return nil, 0, err
	}
	out := []MovementView{}
	err := r.db.SelectContext(ctx, &out, `
		SELECT sm.id, sm.product_variant_id, p.name AS product_name, pv.sku, pv.label,
		       sm.reason, sm.delta, sm.quantity_after, sm.reference_type, sm.reference_id,
		       o.order_number, sm.note, u.full_name AS created_by_name, sm.created_at
		FROM stock_movements sm
		JOIN product_variants pv ON pv.id = sm.product_variant_id
		JOIN products p ON p.id = pv.product_id
		LEFT JOIN users u ON u.id = sm.created_by
		LEFT JOIN orders o ON sm.reference_type = 'order' AND o.id = sm.reference_id
		WHERE sm.product_variant_id = ?
		ORDER BY sm.id DESC LIMIT ? OFFSET ?`, variantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// RecentMovements is the department-wide ledger tail shown on the overview.
func (r *Repository) RecentMovements(ctx context.Context, departments []string, limit int) ([]MovementView, error) {
	if len(departments) == 0 {
		return []MovementView{}, nil
	}
	args := make([]any, 0, len(departments)+1)
	for _, d := range departments {
		args = append(args, d)
	}
	args = append(args, limit)
	out := []MovementView{}
	err := r.db.SelectContext(ctx, &out, `
		SELECT sm.id, sm.product_variant_id, p.name AS product_name, pv.sku, pv.label,
		       sm.reason, sm.delta, sm.quantity_after, sm.reference_type, sm.reference_id,
		       o.order_number, sm.note, u.full_name AS created_by_name, sm.created_at
		FROM stock_movements sm
		JOIN product_variants pv ON pv.id = sm.product_variant_id
		JOIN products p ON p.id = pv.product_id
		LEFT JOIN users u ON u.id = sm.created_by
		LEFT JOIN orders o ON sm.reference_type = 'order' AND o.id = sm.reference_id
		WHERE sm.department IN (?`+strings.Repeat(", ?", len(departments)-1)+`)
		ORDER BY sm.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Apply moves one SKU's stock and appends the matching ledger entry in a single
// transaction, locking the variant row so a concurrent checkout cannot
// interleave and leave the ledger disagreeing with the level.
func (r *Repository) Apply(ctx context.Context, variantID int64, mode string, quantity int, reason string, note *string, actor *int64, department string) (*Item, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var current int
	err = tx.GetContext(ctx, &current,
		`SELECT stock_quantity FROM product_variants WHERE id = ? FOR UPDATE`, variantID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrVariantNotFound
	}
	if err != nil {
		return nil, err
	}

	delta := quantity
	if mode == ModeSet {
		delta = quantity - current
	}
	after := current + delta
	if after < 0 {
		return nil, ErrNegativeStock
	}

	if delta != 0 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE product_variants SET stock_quantity = ? WHERE id = ?`, after, variantID); err != nil {
			return nil, err
		}
		if err := RecordTx(ctx, tx, Movement{
			ProductVariantID: variantID,
			Department:       department,
			Reason:           reason,
			Delta:            delta,
			QuantityAfter:    after,
			Note:             note,
			CreatedBy:        actor,
		}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetItem(ctx, variantID)
}

// SetThreshold writes the reorder point. Existence is established by the
// service's prior read (which also authorizes the department), so a zero
// rows-affected here just means the value was already the requested one.
func (r *Repository) SetThreshold(ctx context.Context, variantID int64, threshold int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE product_variants SET reorder_threshold = ? WHERE id = ?`, threshold, variantID)
	return err
}
