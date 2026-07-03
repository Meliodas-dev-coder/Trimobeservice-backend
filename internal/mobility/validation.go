package mobility

import (
	"regexp"
	"strings"
)

var priceRe = regexp.MustCompile(`^\d{1,10}(\.\d{1,2})?$`)

var validTransmissions = map[string]bool{"manual": true, "automatic": true}
var validCarStatuses = map[string]bool{
	CarStatusAvailable:   true,
	CarStatusMaintenance: true,
	CarStatusInactive:    true,
}
var validDriverStatuses = map[string]bool{
	DriverStatusAvailable: true,
	DriverStatusAssigned:  true,
	DriverStatusInactive:  true,
}

func validateCarCategory(req CarCategoryRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.Name) == "" {
		p["name"] = "is required"
	}
	if r := strings.TrimSpace(req.DefaultDailyRate); r != "" && !priceRe.MatchString(r) {
		p["default_daily_rate"] = "must be a decimal amount, e.g. 120.00"
	}
	return p
}

func validateCar(req CarRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.Name) == "" {
		p["name"] = "is required"
	}
	if req.CategoryID <= 0 {
		p["category_id"] = "is required"
	}
	if r := strings.TrimSpace(req.DailyRate); r != "" && !priceRe.MatchString(r) {
		p["daily_rate"] = "must be a decimal amount, e.g. 120.00"
	}
	if req.Transmission != nil && !validTransmissions[*req.Transmission] {
		p["transmission"] = "must be 'manual' or 'automatic'"
	}
	if req.Status != nil && !validCarStatuses[*req.Status] {
		p["status"] = "must be one of available, maintenance, inactive"
	}
	if req.Year != nil && (*req.Year < 1900 || *req.Year > 2100) {
		p["year"] = "is out of range"
	}
	if req.Seats != nil && *req.Seats < 0 {
		p["seats"] = "must be zero or greater"
	}
	return p
}

func validateDriver(req DriverRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.FullName) == "" {
		p["full_name"] = "is required"
	}
	if strings.TrimSpace(req.Phone) == "" {
		p["phone"] = "is required"
	}
	if strings.TrimSpace(req.LicenseNumber) == "" {
		p["license_number"] = "is required"
	}
	if req.Status != nil && !validDriverStatuses[*req.Status] {
		p["status"] = "must be one of available, assigned, inactive"
	}
	return p
}

func validateCarImage(req CarImageRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.URL) == "" {
		p["url"] = "is required"
	}
	return p
}
