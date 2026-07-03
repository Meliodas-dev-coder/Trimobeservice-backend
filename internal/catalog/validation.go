package catalog

import (
	"regexp"
	"strings"
)

// matches a non-negative decimal with up to 10 integer and 2 fractional digits,
// i.e. it fits DECIMAL(12,2).
var priceRe = regexp.MustCompile(`^\d{1,10}(\.\d{1,2})?$`)

func validateCategory(req CategoryRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.Name) == "" {
		p["name"] = "is required"
	}
	return p
}

func validateBrand(req BrandRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.Name) == "" {
		p["name"] = "is required"
	}
	return p
}

func validateProduct(req ProductRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.Name) == "" {
		p["name"] = "is required"
	}
	if req.CategoryID <= 0 {
		p["category_id"] = "is required"
	}
	return p
}

func validateVariant(req VariantRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.SKU) == "" {
		p["sku"] = "is required"
	}
	if !priceRe.MatchString(req.Price) {
		p["price"] = "must be a decimal amount, e.g. 699.00"
	}
	if req.StockQuantity < 0 {
		p["stock_quantity"] = "must be zero or greater"
	}
	return p
}

func validateImage(req ImageRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.URL) == "" {
		p["url"] = "is required"
	}
	return p
}
