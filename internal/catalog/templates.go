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
	ProductFields []TemplateField `json:"product_fields"`
	VariantAxes   []TemplateField `json:"variant_axes"`
}

var productTemplates = []ProductTemplate{
	{
		Key:   "phone",
		Label: "Phone",
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
		Key:   "laptop",
		Label: "Laptop",
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
		Key:   "tablet",
		Label: "Tablet",
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
		Key:   "audio",
		Label: "Audio",
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
		Key:   "accessory",
		Label: "Accessory",
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
		Key:           "generic",
		Label:         "Generic",
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
