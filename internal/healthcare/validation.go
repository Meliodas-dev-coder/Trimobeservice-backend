package healthcare

import (
	"regexp"
	"strings"
	"time"

	"github.com/jmoiron/sqlx/types"
)

// priceRe matches a non-negative decimal that fits DECIMAL(12,2).
var priceRe = regexp.MustCompile(`^\d{1,10}(\.\d{1,2})?$`)

var slugNonWord = regexp.MustCompile(`[^a-z0-9]+`)

func validatePractitioner(req PractitionerRequest) map[string]string {
	p := map[string]string{}
	if req.Type != TypeDoctor && req.Type != TypeNurse {
		p["type"] = "must be 'doctor' or 'nurse'"
	}
	if strings.TrimSpace(req.FullName) == "" {
		p["full_name"] = "is required"
	}
	if strings.TrimSpace(req.Phone) == "" {
		p["phone"] = "is required"
	}
	if req.Status != nil && *req.Status != PractitionerActive && *req.Status != PractitionerInactive {
		p["status"] = "must be 'active' or 'inactive'"
	}
	return p
}

func validateCategory(req ServiceCategoryRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.Name) == "" {
		p["name"] = "is required"
	}
	return p
}

func validateService(req ServiceRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.Name) == "" {
		p["name"] = "is required"
	}
	if req.CategoryID <= 0 {
		p["category_id"] = "is required"
	}
	if req.ServiceType != "" && req.ServiceType != ServiceConsultation && req.ServiceType != ServicePackage {
		p["service_type"] = "must be 'consultation' or 'package'"
	}
	if v := trimmedPtr(req.FromPrice); v != "" && !priceRe.MatchString(v) {
		p["from_price"] = "must be a decimal amount, e.g. 50000.00"
	}
	if v := trimmedPtr(req.Price); v != "" && !priceRe.MatchString(v) {
		p["price"] = "must be a decimal amount, e.g. 3000000.00"
	}
	if req.ServiceType == ServicePackage {
		if v := trimmedPtr(req.Price); v == "" {
			p["price"] = "a package needs a fixed price"
		}
		if req.DurationDays != nil && *req.DurationDays <= 0 {
			p["duration_days"] = "must be a positive number of days"
		}
	}
	return p
}

func validateCreateRequest(req CreateRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.PatientName) == "" {
		p["patient_name"] = "is required"
	}
	if strings.TrimSpace(req.Address) == "" {
		p["address"] = "is required"
	}
	if strings.TrimSpace(req.ContactPhone) == "" {
		p["contact_phone"] = "is required"
	}
	validateLocationCoordinates(p, req.LocationLatitude, req.LocationLongitude)
	if req.PatientGender != nil {
		switch strings.TrimSpace(*req.PatientGender) {
		case "", "male", "female", "other":
		default:
			p["patient_gender"] = "must be 'male', 'female', or 'other'"
		}
	}
	if req.PatientAge != nil && (*req.PatientAge < 0 || *req.PatientAge > 130) {
		p["patient_age"] = "must be a plausible age"
	}
	now := time.Now().Add(-24 * time.Hour)
	if req.PreferredAt != nil && !req.PreferredAt.IsZero() && req.PreferredAt.Before(now) {
		p["preferred_at"] = "must not be in the past"
	}
	if req.StartAt != nil && !req.StartAt.IsZero() && req.StartAt.Before(now) {
		p["start_at"] = "must not be in the past"
	}
	if req.StartAt != nil && req.EndAt != nil && !req.EndAt.IsZero() && req.EndAt.Before(*req.StartAt) {
		p["end_at"] = "must be on or after the start"
	}
	return p
}

func validateLocationCoordinates(p map[string]string, latitude, longitude *float64) {
	if (latitude == nil) != (longitude == nil) {
		p["location_coordinates"] = "latitude and longitude must be provided together"
		return
	}
	if latitude == nil {
		return
	}
	if *latitude < -90 || *latitude > 90 {
		p["location_latitude"] = "must be between -90 and 90"
	}
	if *longitude < -180 || *longitude > 180 {
		p["location_longitude"] = "must be between -180 and 180"
	}
}

func validateQuote(req QuoteRequest) map[string]string {
	p := map[string]string{}
	if !priceRe.MatchString(strings.TrimSpace(req.QuotedPrice)) {
		p["quoted_price"] = "must be a decimal amount, e.g. 80000.00"
	}
	return p
}

func validateAssign(req AssignRequest) map[string]string {
	p := map[string]string{}
	if req.PractitionerID <= 0 {
		p["practitioner_id"] = "is required"
	}
	return p
}

func trimmedPtr(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

// slugify builds a URL slug from a name: lowercase, non-alphanumerics to
// hyphens, trimmed. Mirrors the catalog/events slug rules.
func slugify(value string) string {
	s := strings.ToLower(strings.TrimSpace(value))
	s = slugNonWord.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "item"
	}
	return s
}

// nullableJSON returns nil for empty JSON so the column stores SQL NULL rather
// than an invalid empty string.
func nullableJSON(j types.JSONText) any {
	if len(strings.TrimSpace(string(j))) == 0 {
		return nil
	}
	return j
}
