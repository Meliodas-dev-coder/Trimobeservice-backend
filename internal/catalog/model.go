// Package catalog is the phones & accessories module: categories, brands,
// products, sellable variants (SKUs), and product images.
package catalog

import (
	"time"

	"github.com/jmoiron/sqlx/types"
)

// --- domain models (map to tables) ---

type Category struct {
	ID          int64     `db:"id" json:"id"`
	ParentID    *int64    `db:"parent_id" json:"parent_id,omitempty"`
	Name        string    `db:"name" json:"name"`
	Slug        string    `db:"slug" json:"slug"`
	TemplateKey string    `db:"template_key" json:"template_key"`
	Description *string   `db:"description" json:"description,omitempty"`
	ImageURL    *string   `db:"image_url" json:"image_url,omitempty"`
	SortOrder   int       `db:"sort_order" json:"sort_order"`
	IsActive    bool      `db:"is_active" json:"is_active"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

type Brand struct {
	ID         int64     `db:"id" json:"id"`
	Name       string    `db:"name" json:"name"`
	Slug       string    `db:"slug" json:"slug"`
	Department string    `db:"department" json:"department"`
	LogoURL    *string   `db:"logo_url" json:"logo_url,omitempty"`
	IsActive   bool      `db:"is_active" json:"is_active"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time `db:"updated_at" json:"updated_at"`
}

type Product struct {
	ID          int64          `db:"id" json:"id"`
	CategoryID  int64          `db:"category_id" json:"category_id"`
	BrandID     *int64         `db:"brand_id" json:"brand_id,omitempty"`
	Name        string         `db:"name" json:"name"`
	Slug        string         `db:"slug" json:"slug"`
	Description *string        `db:"description" json:"description,omitempty"`
	Attributes  types.JSONText `db:"attributes" json:"attributes,omitempty"`
	IsActive    bool           `db:"is_active" json:"is_active"`
	CreatedAt   time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at" json:"updated_at"`

	// List-only aggregates: populated by ListProducts so the admin table/cards can
	// show an image, variant count, and price range without a per-row fetch.
	// Zero/omitted on single-product reads (which return the full Variants/Images).
	PrimaryImageURL *string `db:"primary_image_url" json:"primary_image_url,omitempty"`
	VariantCount    int     `db:"variant_count" json:"variant_count"`
	PriceMin        *string `db:"price_min" json:"price_min,omitempty"`
	PriceMax        *string `db:"price_max" json:"price_max,omitempty"`
}

type Variant struct {
	ID            int64          `db:"id" json:"id"`
	ProductID     int64          `db:"product_id" json:"product_id"`
	SKU           string         `db:"sku" json:"sku"`
	Label         *string        `db:"label" json:"label,omitempty"`
	Color         *string        `db:"color" json:"color,omitempty"`
	Storage       *string        `db:"storage" json:"storage,omitempty"`
	Attributes    types.JSONText `db:"attributes" json:"attributes,omitempty"`
	Price         string         `db:"price" json:"price"` // DECIMAL(12,2) carried as string to preserve precision
	StockQuantity int            `db:"stock_quantity" json:"stock_quantity"`
	IsActive      bool           `db:"is_active" json:"is_active"`
	CreatedAt     time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at" json:"updated_at"`
}

type Image struct {
	ID        int64     `db:"id" json:"id"`
	ProductID int64     `db:"product_id" json:"product_id"`
	VariantID *int64    `db:"variant_id" json:"variant_id,omitempty"`
	URL       string    `db:"url" json:"url"`
	AltText   *string   `db:"alt_text" json:"alt_text,omitempty"`
	IsPrimary bool      `db:"is_primary" json:"is_primary"`
	SortOrder int       `db:"sort_order" json:"sort_order"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// ProductDetail is a product with its related data, returned by detail reads.
type ProductDetail struct {
	Product
	Category *Category `json:"category,omitempty"`
	Brand    *Brand    `json:"brand,omitempty"`
	Variants []Variant `json:"variants"`
	Images   []Image   `json:"images"`
}

// ProductFilter drives the product list query.
type ProductFilter struct {
	CategoryID         *int64
	BrandID            *int64
	TemplateKey        string
	ExcludeTemplateKey string
	TemplateKeys       []string // restrict to categories whose template is in this set (e.g. a department)
	Search             string
	ActiveOnly         bool
	Limit              int
	Offset             int
}

// --- request DTOs ---

type CategoryRequest struct {
	Name        string  `json:"name"`
	ParentID    *int64  `json:"parent_id"`
	TemplateKey string  `json:"template_key"`
	Description *string `json:"description"`
	ImageURL    *string `json:"image_url"`
	SortOrder   int     `json:"sort_order"`
	IsActive    *bool   `json:"is_active"`
}

type BrandRequest struct {
	Name       string  `json:"name"`
	Department string  `json:"department"`
	LogoURL    *string `json:"logo_url"`
	IsActive   *bool   `json:"is_active"`
}

type ProductRequest struct {
	CategoryID  int64          `json:"category_id"`
	BrandID     *int64         `json:"brand_id"`
	Name        string         `json:"name"`
	Description *string        `json:"description"`
	Attributes  types.JSONText `json:"attributes"`
	IsActive    *bool          `json:"is_active"`
}

type VariantRequest struct {
	SKU           string         `json:"sku"`
	Label         *string        `json:"label"`
	Color         *string        `json:"color"`
	Storage       *string        `json:"storage"`
	Attributes    types.JSONText `json:"attributes"`
	Price         string         `json:"price"`
	StockQuantity int            `json:"stock_quantity"`
	IsActive      *bool          `json:"is_active"`
}

type ImageRequest struct {
	VariantID *int64  `json:"variant_id"`
	URL       string  `json:"url"`
	AltText   *string `json:"alt_text"`
	IsPrimary bool    `json:"is_primary"`
	SortOrder int     `json:"sort_order"`
}

// ImageUpdateRequest patches an existing image; used mainly to promote a gallery
// image to the product cover (is_primary).
type ImageUpdateRequest struct {
	AltText   *string `json:"alt_text"`
	IsPrimary *bool   `json:"is_primary"`
}

func derefBool(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}
