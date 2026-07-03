package catalog

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var (
	ErrCategoryNotFound = errors.New("category not found")
	ErrBrandNotFound    = errors.New("brand not found")
	ErrProductNotFound  = errors.New("product not found")
	ErrVariantNotFound  = errors.New("variant not found")
	ErrImageNotFound    = errors.New("image not found")

	// ErrConflict is a duplicate unique value (slug, SKU); ErrInUse is a
	// referenced row that cannot be deleted.
	ErrConflict = errors.New("duplicate value")
	ErrInUse    = errors.New("resource is referenced by other records")
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// --- categories ---

const categoryCols = `id, parent_id, name, slug, description, image_url, sort_order, is_active, created_at, updated_at`

func (r *Repository) ListCategories(ctx context.Context, activeOnly bool) ([]Category, error) {
	q := `SELECT ` + categoryCols + ` FROM product_categories`
	if activeOnly {
		q += ` WHERE is_active = TRUE`
	}
	q += ` ORDER BY sort_order, name`
	out := []Category{}
	if err := r.db.SelectContext(ctx, &out, q); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetCategoryByID(ctx context.Context, id int64) (*Category, error) {
	var c Category
	err := r.db.GetContext(ctx, &c, `SELECT `+categoryCols+` FROM product_categories WHERE id = ?`, id)
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
	err := r.db.GetContext(ctx, &ok, `SELECT EXISTS(SELECT 1 FROM product_categories WHERE id = ?)`, id)
	return ok, err
}

func (r *Repository) CategorySlugExists(ctx context.Context, slug string, excludeID int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok,
		`SELECT EXISTS(SELECT 1 FROM product_categories WHERE slug = ? AND id <> ?)`, slug, excludeID)
	return ok, err
}

func (r *Repository) CreateCategory(ctx context.Context, c *Category) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO product_categories (parent_id, name, slug, description, image_url, sort_order, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ParentID, c.Name, c.Slug, c.Description, c.ImageURL, c.SortOrder, c.IsActive)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateCategory(ctx context.Context, c *Category) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE product_categories
		 SET parent_id = ?, name = ?, slug = ?, description = ?, image_url = ?, sort_order = ?, is_active = ?
		 WHERE id = ?`,
		c.ParentID, c.Name, c.Slug, c.Description, c.ImageURL, c.SortOrder, c.IsActive, c.ID)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrCategoryNotFound)
}

func (r *Repository) DeleteCategory(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM product_categories WHERE id = ?`, id)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrCategoryNotFound)
}

// --- brands ---

const brandCols = `id, name, slug, logo_url, is_active, created_at, updated_at`

func (r *Repository) ListBrands(ctx context.Context, activeOnly bool) ([]Brand, error) {
	q := `SELECT ` + brandCols + ` FROM brands`
	if activeOnly {
		q += ` WHERE is_active = TRUE`
	}
	q += ` ORDER BY name`
	out := []Brand{}
	if err := r.db.SelectContext(ctx, &out, q); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetBrandByID(ctx context.Context, id int64) (*Brand, error) {
	var b Brand
	err := r.db.GetContext(ctx, &b, `SELECT `+brandCols+` FROM brands WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBrandNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *Repository) BrandExists(ctx context.Context, id int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok, `SELECT EXISTS(SELECT 1 FROM brands WHERE id = ?)`, id)
	return ok, err
}

func (r *Repository) BrandSlugExists(ctx context.Context, slug string, excludeID int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok,
		`SELECT EXISTS(SELECT 1 FROM brands WHERE slug = ? AND id <> ?)`, slug, excludeID)
	return ok, err
}

func (r *Repository) CreateBrand(ctx context.Context, b *Brand) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO brands (name, slug, logo_url, is_active) VALUES (?, ?, ?, ?)`,
		b.Name, b.Slug, b.LogoURL, b.IsActive)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateBrand(ctx context.Context, b *Brand) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE brands SET name = ?, slug = ?, logo_url = ?, is_active = ? WHERE id = ?`,
		b.Name, b.Slug, b.LogoURL, b.IsActive, b.ID)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrBrandNotFound)
}

func (r *Repository) DeleteBrand(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM brands WHERE id = ?`, id)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrBrandNotFound)
}

// --- products ---

const productCols = `id, category_id, brand_id, name, slug, description, is_active, created_at, updated_at`

func (r *Repository) ListProducts(ctx context.Context, f ProductFilter) ([]Product, int, error) {
	var where []string
	var args []any
	if f.ActiveOnly {
		where = append(where, "is_active = TRUE")
	}
	if f.CategoryID != nil {
		where = append(where, "category_id = ?")
		args = append(args, *f.CategoryID)
	}
	if f.BrandID != nil {
		where = append(where, "brand_id = ?")
		args = append(args, *f.BrandID)
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
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM products`+clause, args...); err != nil {
		return nil, 0, err
	}

	listArgs := append(append([]any{}, args...), f.Limit, f.Offset)
	out := []Product{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+productCols+` FROM products`+clause+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) GetProductByID(ctx context.Context, id int64) (*Product, error) {
	var p Product
	err := r.db.GetContext(ctx, &p, `SELECT `+productCols+` FROM products WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) GetProductBySlug(ctx context.Context, slug string) (*Product, error) {
	var p Product
	err := r.db.GetContext(ctx, &p, `SELECT `+productCols+` FROM products WHERE slug = ?`, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) ProductSlugExists(ctx context.Context, slug string, excludeID int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok,
		`SELECT EXISTS(SELECT 1 FROM products WHERE slug = ? AND id <> ?)`, slug, excludeID)
	return ok, err
}

func (r *Repository) CreateProduct(ctx context.Context, p *Product) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO products (category_id, brand_id, name, slug, description, is_active)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		p.CategoryID, p.BrandID, p.Name, p.Slug, p.Description, p.IsActive)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateProduct(ctx context.Context, p *Product) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE products
		 SET category_id = ?, brand_id = ?, name = ?, slug = ?, description = ?, is_active = ?
		 WHERE id = ?`,
		p.CategoryID, p.BrandID, p.Name, p.Slug, p.Description, p.IsActive, p.ID)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrProductNotFound)
}

func (r *Repository) DeleteProduct(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM products WHERE id = ?`, id)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrProductNotFound)
}

// --- variants ---

const variantCols = `id, product_id, sku, label, color, storage, attributes, price, stock_quantity, is_active, created_at, updated_at`

func (r *Repository) ListVariantsByProduct(ctx context.Context, productID int64) ([]Variant, error) {
	out := []Variant{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+variantCols+` FROM product_variants WHERE product_id = ? ORDER BY id`, productID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetVariantByID(ctx context.Context, id int64) (*Variant, error) {
	var v Variant
	err := r.db.GetContext(ctx, &v, `SELECT `+variantCols+` FROM product_variants WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrVariantNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *Repository) SKUExists(ctx context.Context, sku string, excludeID int64) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok,
		`SELECT EXISTS(SELECT 1 FROM product_variants WHERE sku = ? AND id <> ?)`, sku, excludeID)
	return ok, err
}

func (r *Repository) CreateVariant(ctx context.Context, v *Variant) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO product_variants (product_id, sku, label, color, storage, attributes, price, stock_quantity, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ProductID, v.SKU, v.Label, v.Color, v.Storage, v.Attributes, v.Price, v.StockQuantity, v.IsActive)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) UpdateVariant(ctx context.Context, v *Variant) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE product_variants
		 SET sku = ?, label = ?, color = ?, storage = ?, attributes = ?, price = ?, stock_quantity = ?, is_active = ?
		 WHERE id = ?`,
		v.SKU, v.Label, v.Color, v.Storage, v.Attributes, v.Price, v.StockQuantity, v.IsActive, v.ID)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrVariantNotFound)
}

func (r *Repository) DeleteVariant(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM product_variants WHERE id = ?`, id)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrVariantNotFound)
}

// --- images ---

const imageCols = `id, product_id, variant_id, url, alt_text, is_primary, sort_order, created_at`

func (r *Repository) ListImagesByProduct(ctx context.Context, productID int64) ([]Image, error) {
	out := []Image{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+imageCols+` FROM product_images WHERE product_id = ? ORDER BY sort_order, id`, productID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) CreateImage(ctx context.Context, im *Image) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO product_images (product_id, variant_id, url, alt_text, is_primary, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		im.ProductID, im.VariantID, im.URL, im.AltText, im.IsPrimary, im.SortOrder)
	if err != nil {
		return 0, mapWriteErr(err)
	}
	return res.LastInsertId()
}

func (r *Repository) GetImageByID(ctx context.Context, id int64) (*Image, error) {
	var im Image
	err := r.db.GetContext(ctx, &im, `SELECT `+imageCols+` FROM product_images WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrImageNotFound
	}
	if err != nil {
		return nil, err
	}
	return &im, nil
}

func (r *Repository) DeleteImage(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM product_images WHERE id = ?`, id)
	if err != nil {
		return mapWriteErr(err)
	}
	return notFoundIfNoRows(res, ErrImageNotFound)
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
		case 1451: // cannot delete: row is referenced by a foreign key
			return ErrInUse
		}
	}
	return err
}
