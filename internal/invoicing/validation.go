package invoicing

import (
	"regexp"
	"strings"
)

// priceRe matches a positive decimal money/rate string, e.g. "120.00" or "20".
var priceRe = regexp.MustCompile(`^\d{1,12}(\.\d{1,2})?$`)

func validateCreateInvoice(req CreateInvoiceRequest) map[string]string {
	p := map[string]string{}
	if !validType(req.InvoiceableType) {
		p["invoiceable_type"] = "must be one of order, booking, event, healthcare"
	}
	if req.InvoiceableID <= 0 {
		p["invoiceable_id"] = "is required"
	}
	if req.Kind != nil {
		if k := strings.TrimSpace(*req.Kind); k != "" && k != KindProforma && k != KindFinal {
			p["kind"] = "must be 'proforma' or 'final'"
		}
	}
	if req.Discount != nil {
		if v := strings.TrimSpace(*req.Discount); v != "" && !priceRe.MatchString(v) {
			p["discount"] = "must be a decimal amount, e.g. 50000.00"
		}
	}
	if req.TaxRate != nil {
		if v := strings.TrimSpace(*req.TaxRate); v != "" && !priceRe.MatchString(v) {
			p["tax_rate"] = "must be a decimal percentage, e.g. 20.00"
		}
	}
	if req.DueDate != nil {
		if v := strings.TrimSpace(*req.DueDate); v != "" && !dateRe.MatchString(v) {
			p["due_date"] = "must be a date, YYYY-MM-DD"
		}
	}
	return p
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func validateUpdateInvoice(req UpdateInvoiceRequest) map[string]string {
	p := map[string]string{}
	if req.Discount != nil {
		if v := strings.TrimSpace(*req.Discount); v != "" && !priceRe.MatchString(v) {
			p["discount"] = "must be a decimal amount, e.g. 50000.00"
		}
	}
	if req.TaxRate != nil {
		if v := strings.TrimSpace(*req.TaxRate); v != "" && !priceRe.MatchString(v) {
			p["tax_rate"] = "must be a decimal percentage, e.g. 20.00"
		}
	}
	if req.DueDate != nil {
		if v := strings.TrimSpace(*req.DueDate); v != "" && !dateRe.MatchString(v) {
			p["due_date"] = "must be a date, YYYY-MM-DD"
		}
	}
	return p
}

func validateOrgSettings(req OrgSettingsRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.LegalName) == "" {
		p["legal_name"] = "is required"
	}
	if req.DefaultTaxRate != nil {
		if v := strings.TrimSpace(*req.DefaultTaxRate); v != "" && !priceRe.MatchString(v) {
			p["default_tax_rate"] = "must be a decimal percentage, e.g. 20.00"
		}
	}
	return p
}
