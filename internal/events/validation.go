package events

import (
	"regexp"
	"strings"
	"time"

	"github.com/jmoiron/sqlx/types"
)

// priceRe matches a non-negative decimal that fits DECIMAL(12,2).
var priceRe = regexp.MustCompile(`^\d{1,10}(\.\d{1,2})?$`)

var slugNonWord = regexp.MustCompile(`[^a-z0-9]+`)

// knownEventTypes are the suggested event categories; the column is free text so
// anything non-empty is accepted, but blanks are rejected.
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
	if req.FromPrice != nil {
		if v := strings.TrimSpace(*req.FromPrice); v != "" && !priceRe.MatchString(v) {
			p["from_price"] = "must be a decimal amount, e.g. 500000.00"
		}
	}
	return p
}

func validateCreateRequest(req CreateEventRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.EventType) == "" {
		p["event_type"] = "is required"
	}
	if req.EventStart.IsZero() {
		p["event_start"] = "is required"
	} else if req.EventStart.Before(time.Now().Add(-24 * time.Hour)) {
		p["event_start"] = "must not be in the past"
	}
	if req.EventEnd != nil && !req.EventEnd.IsZero() && req.EventEnd.Before(req.EventStart) {
		p["event_end"] = "must be on or after the start"
	}
	if strings.TrimSpace(req.Location) == "" {
		p["location"] = "is required"
	}
	if strings.TrimSpace(req.ContactPhone) == "" {
		p["contact_phone"] = "is required"
	}
	if len(req.Services) == 0 && len(req.Artists) == 0 {
		p["services"] = "select at least one service or artist"
	}
	if req.Budget != nil {
		if v := strings.TrimSpace(*req.Budget); v != "" && !priceRe.MatchString(v) {
			p["budget"] = "must be a decimal amount"
		}
	}
	return p
}

func validateArtist(req ArtistRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.StageName) == "" {
		p["stage_name"] = "is required"
	}
	if req.FromFee != nil {
		if v := strings.TrimSpace(*req.FromFee); v != "" && !priceRe.MatchString(v) {
			p["from_fee"] = "must be a decimal amount, e.g. 900000.00"
		}
	}
	return p
}

func validateQuote(req QuoteRequest) map[string]string {
	p := map[string]string{}
	if !priceRe.MatchString(strings.TrimSpace(req.QuotedPrice)) {
		p["quoted_price"] = "must be a decimal amount, e.g. 3500000.00"
	}
	return p
}

// slugify builds a URL slug from a name: lowercase, non-alphanumerics to
// hyphens, trimmed. Mirrors the catalog module's slug rules.
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
