package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx/types"
)

var (
	ErrSKUTaken        = errors.New("SKU already exists")
	ErrInvalidCategory = errors.New("referenced category does not exist")
	ErrInvalidBrand    = errors.New("referenced brand does not exist")
)

// ImageDeleter removes an image's backing file from object storage. Optional
// (nil when uploads are disabled).
type ImageDeleter interface {
	DeleteByURL(ctx context.Context, url string) error
}

type Service struct {
	repo   *Repository
	images ImageDeleter
}

func NewService(repo *Repository, images ImageDeleter) *Service {
	return &Service{repo: repo, images: images}
}

// --- categories ---

// ListCategories returns categories, optionally scoped to a department. A
// category's department is inherited from its product-type template, so the
// filter keeps only categories whose template belongs to the department.
func (s *Service) ListCategories(ctx context.Context, activeOnly bool, department string) ([]Category, error) {
	cats, err := s.repo.ListCategories(ctx, activeOnly)
	if err != nil {
		return nil, err
	}
	if department == "" {
		return cats, nil
	}
	allowed := map[string]bool{}
	for _, k := range TemplateKeysForDepartment(department) {
		allowed[k] = true
	}
	out := make([]Category, 0, len(cats))
	for _, c := range cats {
		if allowed[c.TemplateKey] {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *Service) CreateCategory(ctx context.Context, req CategoryRequest) (*Category, error) {
	if req.ParentID != nil {
		if err := s.mustCategoryExist(ctx, *req.ParentID); err != nil {
			return nil, err
		}
	}
	slug, err := s.uniqueSlug(ctx, req.Name, 0, s.repo.CategorySlugExists)
	if err != nil {
		return nil, err
	}
	templateKey, err := templateKeyOrProblem(req.TemplateKey)
	if err != nil {
		return nil, err
	}
	c := &Category{
		ParentID:     req.ParentID,
		Name:         strings.TrimSpace(req.Name),
		Slug:         slug,
		TemplateKey:  templateKey,
		Description:  req.Description,
		Translations: req.Translations,
		ImageURL:     req.ImageURL,
		SortOrder:    req.SortOrder,
		IsActive:     derefBool(req.IsActive, true),
	}
	id, err := s.repo.CreateCategory(ctx, c)
	if err != nil {
		return nil, err
	}
	return s.repo.GetCategoryByID(ctx, id)
}

func (s *Service) UpdateCategory(ctx context.Context, id int64, req CategoryRequest) (*Category, error) {
	c, err := s.repo.GetCategoryByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.ParentID != nil {
		if *req.ParentID == id {
			return nil, ErrInvalidCategory // a category cannot be its own parent
		}
		if err := s.mustCategoryExist(ctx, *req.ParentID); err != nil {
			return nil, err
		}
	}
	slug, err := s.uniqueSlug(ctx, req.Name, id, s.repo.CategorySlugExists)
	if err != nil {
		return nil, err
	}
	templateKey, err := templateKeyOrProblem(req.TemplateKey)
	if err != nil {
		return nil, err
	}
	c.ParentID = req.ParentID
	c.Name = strings.TrimSpace(req.Name)
	c.Slug = slug
	c.TemplateKey = templateKey
	c.Description = req.Description
	c.Translations = req.Translations
	c.ImageURL = req.ImageURL
	c.SortOrder = req.SortOrder
	if req.IsActive != nil {
		c.IsActive = *req.IsActive
	}
	if err := s.repo.UpdateCategory(ctx, c); err != nil {
		return nil, err
	}
	return s.repo.GetCategoryByID(ctx, id)
}

func (s *Service) DeleteCategory(ctx context.Context, id int64) error {
	return s.repo.DeleteCategory(ctx, id)
}

// --- brands ---

func (s *Service) ListBrands(ctx context.Context, activeOnly bool, department string) ([]Brand, error) {
	return s.repo.ListBrands(ctx, activeOnly, department)
}

func (s *Service) CreateBrand(ctx context.Context, req BrandRequest) (*Brand, error) {
	slug, err := s.uniqueSlug(ctx, req.Name, 0, s.repo.BrandSlugExists)
	if err != nil {
		return nil, err
	}
	b := &Brand{
		Name:         strings.TrimSpace(req.Name),
		Slug:         slug,
		Translations: req.Translations,
		Department:   departmentOrDefault(req.Department),
		LogoURL:      req.LogoURL,
		IsActive:     derefBool(req.IsActive, true),
	}
	id, err := s.repo.CreateBrand(ctx, b)
	if err != nil {
		return nil, err
	}
	return s.repo.GetBrandByID(ctx, id)
}

func (s *Service) UpdateBrand(ctx context.Context, id int64, req BrandRequest) (*Brand, error) {
	b, err := s.repo.GetBrandByID(ctx, id)
	if err != nil {
		return nil, err
	}
	slug, err := s.uniqueSlug(ctx, req.Name, id, s.repo.BrandSlugExists)
	if err != nil {
		return nil, err
	}
	b.Name = strings.TrimSpace(req.Name)
	b.Slug = slug
	b.Translations = req.Translations
	b.Department = departmentOrDefault(req.Department)
	b.LogoURL = req.LogoURL
	if req.IsActive != nil {
		b.IsActive = *req.IsActive
	}
	if err := s.repo.UpdateBrand(ctx, b); err != nil {
		return nil, err
	}
	return s.repo.GetBrandByID(ctx, id)
}

func (s *Service) DeleteBrand(ctx context.Context, id int64) error {
	return s.repo.DeleteBrand(ctx, id)
}

// --- products ---

func (s *Service) ListProducts(ctx context.Context, f ProductFilter) ([]Product, int, error) {
	return s.repo.ListProducts(ctx, f)
}

func (s *Service) GetProductBySlug(ctx context.Context, slug string, publicOnly bool) (*ProductDetail, error) {
	p, err := s.repo.GetProductBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if publicOnly && !p.IsActive {
		return nil, ErrProductNotFound
	}
	return s.assembleDetail(ctx, p, publicOnly)
}

func (s *Service) GetProductByID(ctx context.Context, id int64) (*ProductDetail, error) {
	p, err := s.repo.GetProductByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.assembleDetail(ctx, p, false)
}

func (s *Service) CreateProduct(ctx context.Context, req ProductRequest) (*ProductDetail, error) {
	if err := s.validateProductRefs(ctx, req.CategoryID, req.BrandID); err != nil {
		return nil, err
	}
	slug, err := s.uniqueSlug(ctx, req.Name, 0, s.repo.ProductSlugExists)
	if err != nil {
		return nil, err
	}
	attrs, err := s.normalizeProductAttributes(ctx, req.CategoryID, req.Attributes)
	if err != nil {
		return nil, err
	}
	p := &Product{
		CategoryID:   req.CategoryID,
		BrandID:      req.BrandID,
		Name:         strings.TrimSpace(req.Name),
		Slug:         slug,
		Description:  req.Description,
		Translations: req.Translations,
		Attributes:   attrs,
		IsActive:     derefBool(req.IsActive, true),
	}
	id, err := s.repo.CreateProduct(ctx, p)
	if err != nil {
		return nil, err
	}
	if err := s.rebuildFacets(ctx, id); err != nil {
		return nil, err
	}
	return s.GetProductByID(ctx, id)
}

func (s *Service) UpdateProduct(ctx context.Context, id int64, req ProductRequest) (*ProductDetail, error) {
	p, err := s.repo.GetProductByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.validateProductRefs(ctx, req.CategoryID, req.BrandID); err != nil {
		return nil, err
	}
	slug, err := s.uniqueSlug(ctx, req.Name, id, s.repo.ProductSlugExists)
	if err != nil {
		return nil, err
	}
	attrs, err := s.normalizeProductAttributes(ctx, req.CategoryID, req.Attributes)
	if err != nil {
		return nil, err
	}
	p.CategoryID = req.CategoryID
	p.BrandID = req.BrandID
	p.Name = strings.TrimSpace(req.Name)
	p.Slug = slug
	p.Description = req.Description
	p.Translations = req.Translations
	p.Attributes = attrs
	if req.IsActive != nil {
		p.IsActive = *req.IsActive
	}
	if err := s.repo.UpdateProduct(ctx, p); err != nil {
		return nil, err
	}
	if err := s.rebuildFacets(ctx, id); err != nil {
		return nil, err
	}
	return s.GetProductByID(ctx, id)
}

func (s *Service) DeleteProduct(ctx context.Context, id int64) error {
	return s.repo.DeleteProduct(ctx, id)
}

// --- variants ---

func (s *Service) CreateVariant(ctx context.Context, productID int64, req VariantRequest) (*Variant, error) {
	prod, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return nil, err
	}
	attrs, err := s.normalizeVariantAttributes(ctx, prod.CategoryID, req.Attributes)
	if err != nil {
		return nil, err
	}
	sku := strings.TrimSpace(req.SKU)
	taken, err := s.repo.SKUExists(ctx, sku, 0)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, ErrSKUTaken
	}
	v := &Variant{
		ProductID:     productID,
		SKU:           sku,
		Label:         req.Label,
		Color:         req.Color,
		Storage:       req.Storage,
		Attributes:    attrs,
		Price:         req.Price,
		StockQuantity: req.StockQuantity,
		IsActive:      derefBool(req.IsActive, true),
	}
	id, err := s.repo.CreateVariant(ctx, v)
	if err != nil {
		return nil, err
	}
	if err := s.rebuildFacets(ctx, productID); err != nil {
		return nil, err
	}
	return s.repo.GetVariantByID(ctx, id)
}

func (s *Service) UpdateVariant(ctx context.Context, id int64, req VariantRequest) (*Variant, error) {
	v, err := s.repo.GetVariantByID(ctx, id)
	if err != nil {
		return nil, err
	}
	prod, err := s.repo.GetProductByID(ctx, v.ProductID)
	if err != nil {
		return nil, err
	}
	attrs, err := s.normalizeVariantAttributes(ctx, prod.CategoryID, req.Attributes)
	if err != nil {
		return nil, err
	}
	sku := strings.TrimSpace(req.SKU)
	if sku != v.SKU {
		taken, err := s.repo.SKUExists(ctx, sku, id)
		if err != nil {
			return nil, err
		}
		if taken {
			return nil, ErrSKUTaken
		}
	}
	v.SKU = sku
	v.Label = req.Label
	v.Color = req.Color
	v.Storage = req.Storage
	v.Attributes = attrs
	v.Price = req.Price
	v.StockQuantity = req.StockQuantity
	if req.IsActive != nil {
		v.IsActive = *req.IsActive
	}
	if err := s.repo.UpdateVariant(ctx, v); err != nil {
		return nil, err
	}
	if err := s.rebuildFacets(ctx, v.ProductID); err != nil {
		return nil, err
	}
	return s.repo.GetVariantByID(ctx, id)
}

func (s *Service) DeleteVariant(ctx context.Context, id int64) error {
	v, err := s.repo.GetVariantByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteVariant(ctx, id); err != nil {
		return err
	}
	return s.rebuildFacets(ctx, v.ProductID)
}

// --- images ---

func (s *Service) CreateImage(ctx context.Context, productID int64, req ImageRequest) (*Image, error) {
	if _, err := s.repo.GetProductByID(ctx, productID); err != nil {
		return nil, err
	}
	if req.VariantID != nil {
		v, err := s.repo.GetVariantByID(ctx, *req.VariantID)
		if err != nil {
			return nil, err
		}
		if v.ProductID != productID {
			return nil, ErrVariantNotFound // variant belongs to a different product
		}
	}
	im := &Image{
		ProductID: productID,
		VariantID: req.VariantID,
		URL:       strings.TrimSpace(req.URL),
		AltText:   req.AltText,
		IsPrimary: req.IsPrimary,
		SortOrder: req.SortOrder,
	}
	id, err := s.repo.CreateImage(ctx, im)
	if err != nil {
		return nil, err
	}
	im.ID = id
	// A product-level primary image is the cover; keep it unique.
	if im.IsPrimary && im.VariantID == nil {
		if err := s.repo.ClearProductCover(ctx, im.ProductID, id); err != nil {
			return nil, err
		}
	}
	return im, nil
}

func (s *Service) UpdateImage(ctx context.Context, id int64, req ImageUpdateRequest) (*Image, error) {
	im, err := s.repo.GetImageByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.AltText != nil {
		im.AltText = req.AltText
	}
	if req.IsPrimary != nil {
		im.IsPrimary = *req.IsPrimary
	}
	if err := s.repo.UpdateImage(ctx, im); err != nil {
		return nil, err
	}
	if im.IsPrimary && im.VariantID == nil {
		if err := s.repo.ClearProductCover(ctx, im.ProductID, im.ID); err != nil {
			return nil, err
		}
	}
	return im, nil
}

func (s *Service) DeleteImage(ctx context.Context, id int64) error {
	img, err := s.repo.GetImageByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteImage(ctx, id); err != nil {
		return err
	}
	// Best-effort: the DB record is already gone; an orphaned file is preferable
	// to failing the request.
	if s.images != nil && img.URL != "" {
		_ = s.images.DeleteByURL(ctx, img.URL)
	}
	return nil
}

// --- internal helpers ---

func (s *Service) assembleDetail(ctx context.Context, p *Product, publicOnly bool) (*ProductDetail, error) {
	variants, err := s.repo.ListVariantsByProduct(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if publicOnly {
		active := make([]Variant, 0, len(variants))
		for _, v := range variants {
			if v.IsActive {
				active = append(active, v)
			}
		}
		variants = active
	}
	images, err := s.repo.ListImagesByProduct(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	detail := &ProductDetail{Product: *p, Variants: variants, Images: images}
	if cat, err := s.repo.GetCategoryByID(ctx, p.CategoryID); err == nil {
		detail.Category = cat
	}
	if p.BrandID != nil {
		if b, err := s.repo.GetBrandByID(ctx, *p.BrandID); err == nil {
			detail.Brand = b
		}
	}
	return detail, nil
}

func (s *Service) validateProductRefs(ctx context.Context, categoryID int64, brandID *int64) error {
	ok, err := s.repo.CategoryExists(ctx, categoryID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCategory
	}
	if brandID != nil {
		ok, err := s.repo.BrandExists(ctx, *brandID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrInvalidBrand
		}
	}
	return nil
}

func (s *Service) mustCategoryExist(ctx context.Context, id int64) error {
	ok, err := s.repo.CategoryExists(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCategory
	}
	return nil
}

// --- attributes & facets ---

// templateForCategory resolves the product template a category points at. An
// unknown/legacy key yields the zero template (no fields), which cleanly drops
// any submitted attributes.
func (s *Service) templateForCategory(ctx context.Context, categoryID int64) (ProductTemplate, error) {
	cat, err := s.repo.GetCategoryByID(ctx, categoryID)
	if err != nil {
		return ProductTemplate{}, err
	}
	tmpl, _ := TemplateByKey(cat.TemplateKey)
	return tmpl, nil
}

func (s *Service) normalizeProductAttributes(ctx context.Context, categoryID int64, raw types.JSONText) (types.JSONText, error) {
	tmpl, err := s.templateForCategory(ctx, categoryID)
	if err != nil {
		return nil, err
	}
	cleaned, problems := normalizeAttributes(tmpl.ProductFields, raw)
	if len(problems) > 0 {
		return nil, &ProblemError{Problems: problems}
	}
	return cleaned, nil
}

func (s *Service) normalizeVariantAttributes(ctx context.Context, categoryID int64, raw types.JSONText) (types.JSONText, error) {
	tmpl, err := s.templateForCategory(ctx, categoryID)
	if err != nil {
		return nil, err
	}
	cleaned, problems := normalizeAttributes(tmpl.VariantAxes, raw)
	if len(problems) > 0 {
		return nil, &ProblemError{Problems: problems}
	}
	return cleaned, nil
}

// rebuildFacets rewrites a product's product_facets rows from its product-level
// filterable specs plus the axis values of its active variants (deduplicated).
func (s *Service) rebuildFacets(ctx context.Context, productID int64) error {
	p, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return err
	}
	tmpl, err := s.templateForCategory(ctx, p.CategoryID)
	if err != nil {
		return err
	}
	variants, err := s.repo.ListVariantsByProduct(ctx, productID)
	if err != nil {
		return err
	}

	seen := map[string]bool{}
	var facets []Facet
	add := func(rows []Facet) {
		for _, f := range rows {
			key := f.Key + "\x00" + f.Value
			if !seen[key] {
				seen[key] = true
				facets = append(facets, f)
			}
		}
	}
	add(templateFacets(tmpl.ProductFields, p.Attributes))
	for _, v := range variants {
		if v.IsActive {
			add(templateFacets(tmpl.VariantAxes, v.Attributes))
		}
	}
	return s.repo.ReplaceProductFacets(ctx, productID, facets)
}

// departmentOrDefault normalizes a brand's department, defaulting to tech when
// blank. Unknown values are rejected earlier by validateBrand.
func departmentOrDefault(dept string) string {
	dept = strings.TrimSpace(dept)
	if dept == "" {
		return DepartmentTech
	}
	return dept
}

// templateKeyOrProblem validates a category's requested product type, defaulting
// to generic when blank and rejecting unknown keys.
func templateKeyOrProblem(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return DefaultTemplateKey, nil
	}
	if !IsValidTemplateKey(key) {
		return "", &ProblemError{Problems: map[string]string{"template_key": "unknown product type"}}
	}
	return key, nil
}

// uniqueSlug builds a slug from name and appends -2, -3, … until it is free.
// existsFn checks a table for the slug excluding the given id.
func (s *Service) uniqueSlug(ctx context.Context, name string, excludeID int64, existsFn func(context.Context, string, int64) (bool, error)) (string, error) {
	base := Slugify(name)
	slug := base
	for i := 2; ; i++ {
		exists, err := existsFn(ctx, slug, excludeID)
		if err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}
