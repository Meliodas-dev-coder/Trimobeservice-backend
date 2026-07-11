package catalog

// Product templates ("types") drive category-specific product attributes. A
// category picks a template_key; products of that category then carry the
// template's ProductFields in products.attributes, and each variant carries the
// VariantAxes in product_variants.attributes. Filterable fields are denormalised
// into product_facets for storefront faceting.
//
// This registry is the single source of truth: the admin frontend fetches it via
// GET /admin/product-templates and renders the form from it, and the service
// validates submitted attributes against it. Adding a new type = editing this file.

// Field types a template field can take.
const (
	FieldText   = "text"
	FieldNumber = "number"
	FieldBool   = "bool"
	FieldSelect = "select"
)

// DefaultTemplateKey is assigned to categories that don't pick a type.
const DefaultTemplateKey = "generic"

// Departments group product types into separate back-office and storefront
// sections (tech vs fashion). Every template belongs to exactly one department;
// categories inherit it from their template, products from their category.
const (
	DepartmentTech    = "tech"
	DepartmentFashion = "fashion"
	DepartmentCoffee  = "coffee"
)

// Department is a storefront/back-office grouping of product types.
type Department struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

var departments = []Department{
	{Key: DepartmentTech, Label: "Tech"},
	{Key: DepartmentFashion, Label: "Fashion"},
	{Key: DepartmentCoffee, Label: "Coffee"},
}

// Departments returns all departments in display order.
func Departments() []Department { return departments }

// IsValidDepartment reports whether key names a known department.
func IsValidDepartment(key string) bool {
	for _, d := range departments {
		if d.Key == key {
			return true
		}
	}
	return false
}

// TemplateKeysForDepartment returns the template keys that belong to a department.
func TemplateKeysForDepartment(dept string) []string {
	var keys []string
	for _, t := range productTemplates {
		if t.Department == dept {
			keys = append(keys, t.Key)
		}
	}
	return keys
}

// TemplateField describes one dynamic attribute (a product spec or a variant axis).
type TemplateField struct {
	Key        string   `json:"key"`
	Label      string   `json:"label"`
	Type       string   `json:"type"`
	Options    []string `json:"options,omitempty"` // select only
	Unit       string   `json:"unit,omitempty"`    // display hint, e.g. "GB"
	Required   bool     `json:"required,omitempty"`
	Filterable bool     `json:"filterable,omitempty"` // denormalised into product_facets
}

// ProductTemplate is a product "type". ProductFields describe the product;
// VariantAxes describe what distinguishes each sellable SKU.
type ProductTemplate struct {
	Key           string          `json:"key"`
	Label         string          `json:"label"`
	Department    string          `json:"department"`
	ProductFields []TemplateField `json:"product_fields"`
	VariantAxes   []TemplateField `json:"variant_axes"`
}

var productTemplates = []ProductTemplate{
	{
		Key:        "phone",
		Label:      "Phone",
		Department: DepartmentTech,
		ProductFields: []TemplateField{
			{Key: "model", Label: "Model", Type: FieldText},
			{Key: "screen_size", Label: "Screen size", Type: FieldNumber, Unit: "in", Filterable: true},
			{Key: "chipset", Label: "Chipset", Type: FieldText},
			{Key: "ram", Label: "RAM", Type: FieldNumber, Unit: "GB", Filterable: true},
			{Key: "battery", Label: "Battery", Type: FieldNumber, Unit: "mAh"},
			{Key: "is_5g", Label: "5G", Type: FieldBool, Filterable: true},
			{Key: "dual_sim", Label: "Dual SIM", Type: FieldBool},
			{Key: "os", Label: "OS", Type: FieldSelect, Options: []string{"Android", "iOS"}},
		},
		VariantAxes: []TemplateField{
			{Key: "color", Label: "Color", Type: FieldText},
			{Key: "storage", Label: "Storage", Type: FieldNumber, Unit: "GB", Filterable: true},
		},
	},
	{
		Key:        "laptop",
		Label:      "Laptop",
		Department: DepartmentTech,
		ProductFields: []TemplateField{
			{Key: "cpu", Label: "CPU", Type: FieldText, Filterable: true},
			{Key: "gpu", Label: "GPU", Type: FieldText},
			{Key: "ram", Label: "RAM", Type: FieldNumber, Unit: "GB", Filterable: true},
			{Key: "storage_type", Label: "Storage type", Type: FieldSelect, Options: []string{"SSD", "HDD", "eMMC"}},
			{Key: "screen_size", Label: "Screen size", Type: FieldNumber, Unit: "in", Filterable: true},
			{Key: "os", Label: "OS", Type: FieldSelect, Options: []string{"Windows", "macOS", "Linux", "ChromeOS"}},
		},
		VariantAxes: []TemplateField{
			{Key: "color", Label: "Color", Type: FieldText},
			{Key: "storage", Label: "Storage", Type: FieldNumber, Unit: "GB", Filterable: true},
		},
	},
	{
		Key:        "tablet",
		Label:      "Tablet",
		Department: DepartmentTech,
		ProductFields: []TemplateField{
			{Key: "screen_size", Label: "Screen size", Type: FieldNumber, Unit: "in", Filterable: true},
			{Key: "chipset", Label: "Chipset", Type: FieldText},
			{Key: "ram", Label: "RAM", Type: FieldNumber, Unit: "GB", Filterable: true},
			{Key: "cellular", Label: "Cellular", Type: FieldBool, Filterable: true},
			{Key: "os", Label: "OS", Type: FieldSelect, Options: []string{"Android", "iPadOS"}},
		},
		VariantAxes: []TemplateField{
			{Key: "color", Label: "Color", Type: FieldText},
			{Key: "storage", Label: "Storage", Type: FieldNumber, Unit: "GB", Filterable: true},
		},
	},
	{
		Key:        "audio",
		Label:      "Audio",
		Department: DepartmentTech,
		ProductFields: []TemplateField{
			{Key: "form", Label: "Form", Type: FieldSelect, Options: []string{"In-ear", "On-ear", "Over-ear"}, Filterable: true},
			{Key: "wireless", Label: "Wireless", Type: FieldBool, Filterable: true},
			{Key: "anc", Label: "ANC (noise cancelling)", Type: FieldBool, Filterable: true},
			{Key: "battery_life", Label: "Battery life", Type: FieldNumber, Unit: "h"},
		},
		VariantAxes: []TemplateField{
			{Key: "color", Label: "Color", Type: FieldText},
		},
	},
	{
		Key:        "accessory",
		Label:      "Accessory",
		Department: DepartmentTech,
		ProductFields: []TemplateField{
			{Key: "connector", Label: "Connector", Type: FieldSelect, Options: []string{"USB-C", "Lightning", "USB-A", "Micro-USB", "Wireless"}, Filterable: true},
			{Key: "wattage", Label: "Wattage", Type: FieldNumber, Unit: "W", Filterable: true},
			{Key: "length", Label: "Length", Type: FieldNumber, Unit: "cm"},
		},
		VariantAxes: []TemplateField{
			{Key: "color", Label: "Color", Type: FieldText},
		},
	},
	{
		Key:        "clothing",
		Label:      "Clothing",
		Department: DepartmentFashion,
		ProductFields: []TemplateField{
			{Key: "gender", Label: "Audience", Type: FieldSelect, Options: []string{"Men", "Women", "Unisex", "Kids"}, Filterable: true},
			{Key: "material", Label: "Material", Type: FieldSelect, Options: []string{"Cotton", "Polyester", "Wool", "Denim", "Leather", "Linen", "Silk", "Blend"}, Filterable: true},
			{Key: "fit", Label: "Fit", Type: FieldSelect, Options: []string{"Slim", "Regular", "Relaxed", "Oversized"}, Filterable: true},
			{Key: "care", Label: "Care instructions", Type: FieldText},
		},
		// A sellable SKU is a size × color pair, each with its own price and stock.
		// Size is free text so any scheme fits (XS–XXL, EU numbers, "One size").
		VariantAxes: []TemplateField{
			{Key: "size", Label: "Size", Type: FieldText, Filterable: true},
			{Key: "color", Label: "Color", Type: FieldText, Filterable: true},
		},
	},
	{
		Key:        "footwear",
		Label:      "Footwear",
		Department: DepartmentFashion,
		ProductFields: []TemplateField{
			{Key: "gender", Label: "Audience", Type: FieldSelect, Options: []string{"Men", "Women", "Unisex", "Kids"}, Filterable: true},
			{Key: "style", Label: "Style", Type: FieldSelect, Options: []string{"Sneakers", "Boots", "Sandals", "Formal", "Loafers", "Heels", "Flats"}, Filterable: true},
			{Key: "material", Label: "Material", Type: FieldSelect, Options: []string{"Leather", "Suede", "Canvas", "Synthetic", "Mesh", "Rubber"}, Filterable: true},
			{Key: "closure", Label: "Closure", Type: FieldSelect, Options: []string{"Laces", "Slip-on", "Velcro", "Buckle", "Zipper"}},
		},
		VariantAxes: []TemplateField{
			{Key: "size", Label: "Size (EU)", Type: FieldText, Filterable: true},
			{Key: "color", Label: "Color", Type: FieldText, Filterable: true},
		},
	},
	{
		Key:        "coffee",
		Label:      "Coffee",
		Department: DepartmentCoffee,
		ProductFields: []TemplateField{
			{Key: "roast", Label: "Roast", Type: FieldSelect, Options: []string{"Light", "Medium", "Medium-dark", "Dark"}, Filterable: true},
			{Key: "form", Label: "Form", Type: FieldSelect, Options: []string{"Whole bean", "Ground", "Pods", "Instant"}, Filterable: true},
			{Key: "origin", Label: "Origin", Type: FieldText},
			{Key: "process", Label: "Process", Type: FieldSelect, Options: []string{"Washed", "Natural", "Honey"}},
			{Key: "decaf", Label: "Decaf", Type: FieldBool, Filterable: true},
		},
		// A sellable SKU is a pack size, each with its own price and stock. Weight
		// is free text so "250g", "1kg", or "12 × 20g" all fit.
		VariantAxes: []TemplateField{
			{Key: "weight", Label: "Weight / pack", Type: FieldText, Filterable: true},
		},
	},
	{
		Key:           "generic",
		Label:         "Generic",
		Department:    DepartmentTech,
		ProductFields: []TemplateField{},
		VariantAxes: []TemplateField{
			{Key: "color", Label: "Color", Type: FieldText},
		},
	},
}

var templatesByKey = func() map[string]ProductTemplate {
	m := make(map[string]ProductTemplate, len(productTemplates))
	for _, t := range productTemplates {
		m[t.Key] = t
	}
	return m
}()

// Templates returns all product templates in display order.
func Templates() []ProductTemplate { return productTemplates }

// TemplateByKey returns a template and whether it exists.
func TemplateByKey(key string) (ProductTemplate, bool) {
	t, ok := templatesByKey[key]
	return t, ok
}

// IsValidTemplateKey reports whether key names a known template.
func IsValidTemplateKey(key string) bool {
	_, ok := templatesByKey[key]
	return ok
}
