package catalog

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/trimo/backend/internal/authz"
	"github.com/trimo/backend/internal/httpx"
)

const (
	catalogResourceCategories = "categories"
	catalogResourceBrands     = "brands"
	catalogResourceProducts   = "products"

	orderProductLookupCapability = "orders.orders.manage"
)

// catalogAccess reads the effective permissions attached by the route guard.
// It deliberately authorizes a legacy module only for the matching department:
// a legacy `tech` role must never become a wildcard over the shared fashion or
// coffee tables.
type catalogAccess struct {
	current authz.CurrentAccess
	present bool
}

func catalogAccessFromContext(ctx context.Context) catalogAccess {
	current, ok := authz.FromContext(ctx)
	return catalogAccess{current: current, present: ok}
}

func (a catalogAccess) can(department, resource string, manage bool) bool {
	if !a.present || !IsValidDepartment(department) {
		return false
	}
	if a.current.IsSuper {
		return true
	}
	if a.current.Permissions.Has(department) {
		return true
	}
	base := department + "." + resource
	if manage {
		return a.current.Permissions.Has(base + ".manage")
	}
	if a.current.Permissions.Has(base) || a.current.Permissions.Has(base+".manage") {
		return true
	}
	// Order creation needs sellable-product lookup data, but never receives
	// category/brand/template visibility or any catalog mutation right.
	return resource == catalogResourceProducts && a.current.Permissions.Has(orderProductLookupCapability)
}

func (a catalogAccess) canTemplates(department string) bool {
	return a.can(department, catalogResourceCategories, false) ||
		a.can(department, catalogResourceProducts, false)
}

func (a catalogAccess) allowedDepartments(resource string, manage bool) []string {
	allowed := make([]string, 0, len(Departments()))
	for _, department := range Departments() {
		if a.can(department.Key, resource, manage) {
			allowed = append(allowed, department.Key)
		}
	}
	return allowed
}

func (a catalogAccess) allowedTemplateDepartments() []string {
	allowed := make([]string, 0, len(Departments()))
	for _, department := range Departments() {
		if a.canTemplates(department.Key) {
			allowed = append(allowed, department.Key)
		}
	}
	return allowed
}

// GuardKeys holds, per admin catalog route family, the permission keys that
// grant coarse route-level access. Because tech, fashion, and coffee share the
// same endpoints, each set is the union across departments: it only decides
// whether a request may enter the shared catalog routes at all. The handler
// still re-checks the exact department of each row (a `tech` grant must never
// reach fashion or coffee data), so these sets are deliberately permissive.
type GuardKeys struct {
	Templates        []string
	CategoriesRead   []string
	CategoriesManage []string
	BrandsRead       []string
	BrandsManage     []string
	ProductsRead     []string
	ProductsManage   []string
}

// resourceGuardKeys mirrors catalogAccess.can: a legacy department wildcard, the
// resource capability, and (for reads) its manage variant. Product reads also
// accept the order-creation lookup capability.
func resourceGuardKeys(resource string, manage bool) []string {
	keys := make([]string, 0, len(Departments())*3+1)
	for _, department := range Departments() {
		base := department.Key + "." + resource
		keys = append(keys, department.Key)
		if manage {
			keys = append(keys, base+".manage")
		} else {
			keys = append(keys, base, base+".manage")
		}
	}
	if resource == catalogResourceProducts && !manage {
		keys = append(keys, orderProductLookupCapability)
	}
	return keys
}

// BuildGuardKeys returns the coarse route-guard key sets for every admin catalog
// route family. main.go wraps each set with RequireAdmin + audit; the handler
// performs the precise per-department check.
func BuildGuardKeys() GuardKeys {
	categoriesRead := resourceGuardKeys(catalogResourceCategories, false)
	productsRead := resourceGuardKeys(catalogResourceProducts, false)
	return GuardKeys{
		// Templates back category/product creation, so template visibility follows
		// read access to either resource (mirrors catalogAccess.canTemplates).
		Templates:        append(append([]string{}, categoriesRead...), productsRead...),
		CategoriesRead:   categoriesRead,
		CategoriesManage: resourceGuardKeys(catalogResourceCategories, true),
		BrandsRead:       resourceGuardKeys(catalogResourceBrands, false),
		BrandsManage:     resourceGuardKeys(catalogResourceBrands, true),
		ProductsRead:     productsRead,
		ProductsManage:   resourceGuardKeys(catalogResourceProducts, true),
	}
}

// --- department access, reusable by the other department-scoped modules ---
//
// Orders and stock present per-department screens over tables catalog does not
// own, but the rule for "may this admin see coffee data?" must stay identical.
// These exported helpers are that rule; catalog remains its only definition.

// Resource names accepted by CanDepartment / DepartmentGuardKeys. The first
// three are catalog's own; the last two belong to the orders and stock modules,
// which hang off a department the same way and so share its key shape
// (`coffee.stock`, `coffee.stock.manage`, …).
const (
	ResourceCategories = catalogResourceCategories
	ResourceBrands     = catalogResourceBrands
	ResourceProducts   = catalogResourceProducts
	ResourceOrders     = "orders"
	ResourceStock      = "stock"
	ResourceOverview   = "overview"
)

// CanDepartment reports whether the request's effective access covers one
// catalog department for a resource. Callers pass the request context; a
// context without resolved access denies.
func CanDepartment(ctx context.Context, department, resource string, manage bool) bool {
	return catalogAccessFromContext(ctx).can(department, resource, manage)
}

// AllowedDepartments lists the departments the caller may act on, in display
// order. Used to scope an unfiltered list request rather than rejecting it.
func AllowedDepartments(ctx context.Context, resource string, manage bool) []string {
	return catalogAccessFromContext(ctx).allowedDepartments(resource, manage)
}

// DepartmentGuardKeys returns the coarse route-guard keys for a resource across
// every department — the same union BuildGuardKeys uses. The handler must still
// re-check the exact department with CanDepartment.
func DepartmentGuardKeys(resource string, manage bool) []string {
	return resourceGuardKeys(resource, manage)
}

// DepartmentForTemplateKey maps a category template to its department, falling
// back to the default template's department for unknown or empty keys (which is
// what a legacy row with no template carries).
func DepartmentForTemplateKey(key string) string {
	department, err := templateDepartment(key)
	if err != nil {
		return DepartmentTech
	}
	return department
}

func requireCatalogDepartment(w http.ResponseWriter, r *http.Request, department, resource string, manage bool) bool {
	if catalogAccessFromContext(r.Context()).can(department, resource, manage) {
		return true
	}
	httpx.Error(w, http.StatusForbidden, "you do not have access to this catalog department")
	return false
}

// adminDepartmentParam distinguishes an absent filter from an invalid filter.
// Treating an unknown value as an empty filter would accidentally widen a
// department-scoped request to the entire shared catalog.
func adminDepartmentParam(r *http.Request) (string, error) {
	department := strings.TrimSpace(r.URL.Query().Get("department"))
	if department == "" {
		return "", nil
	}
	if !IsValidDepartment(department) {
		return "", &ProblemError{Problems: map[string]string{"department": "unknown department"}}
	}
	return department, nil
}

func departmentSet(departments []string) map[string]bool {
	set := make(map[string]bool, len(departments))
	for _, department := range departments {
		set[department] = true
	}
	return set
}

func templateKeysForDepartments(departments []string) []string {
	keys := []string{}
	for _, department := range departments {
		keys = append(keys, TemplateKeysForDepartment(department)...)
	}
	return keys
}

func templateDepartment(templateKey string) (string, error) {
	templateKey = strings.TrimSpace(templateKey)
	if templateKey == "" {
		templateKey = DefaultTemplateKey
	}
	template, ok := TemplateByKey(templateKey)
	if !ok || !IsValidDepartment(template.Department) {
		return "", &ProblemError{Problems: map[string]string{"template_key": "unknown product type"}}
	}
	return template.Department, nil
}

func categoryDepartment(category *Category) (string, error) {
	if category == nil {
		return "", ErrInvalidCategory
	}
	return templateDepartment(category.TemplateKey)
}

func (s *Service) CategoryDepartment(ctx context.Context, id int64) (string, error) {
	category, err := s.repo.GetCategoryByID(ctx, id)
	if err != nil {
		return "", err
	}
	return categoryDepartment(category)
}

func (s *Service) BrandDepartment(ctx context.Context, id int64) (string, error) {
	brand, err := s.repo.GetBrandByID(ctx, id)
	if err != nil {
		return "", err
	}
	if !IsValidDepartment(brand.Department) {
		return "", ErrInvalidBrand
	}
	return brand.Department, nil
}

func (s *Service) ProductDepartment(ctx context.Context, id int64) (string, error) {
	product, err := s.repo.GetProductByID(ctx, id)
	if err != nil {
		return "", err
	}
	return s.CategoryDepartment(ctx, product.CategoryID)
}

func (s *Service) VariantDepartment(ctx context.Context, id int64) (string, error) {
	variant, err := s.repo.GetVariantByID(ctx, id)
	if err != nil {
		return "", err
	}
	return s.ProductDepartment(ctx, variant.ProductID)
}

func (s *Service) ImageDepartment(ctx context.Context, id int64) (string, error) {
	image, err := s.repo.GetImageByID(ctx, id)
	if err != nil {
		return "", err
	}
	return s.ProductDepartment(ctx, image.ProductID)
}

func isCatalogNotFound(err error) bool {
	return errors.Is(err, ErrCategoryNotFound) ||
		errors.Is(err, ErrBrandNotFound) ||
		errors.Is(err, ErrProductNotFound) ||
		errors.Is(err, ErrVariantNotFound) ||
		errors.Is(err, ErrImageNotFound)
}
