package auth

import (
	"net/mail"
	"strings"
	"unicode/utf8"
)

func validateRegister(req RegisterRequest) map[string]string {
	problems := map[string]string{}
	if _, err := mail.ParseAddress(strings.TrimSpace(req.Email)); err != nil {
		problems["email"] = "must be a valid email address"
	}
	if utf8.RuneCountInString(req.Password) < 8 {
		problems["password"] = "must be at least 8 characters"
	}
	// bcrypt ignores any bytes past the 72nd, so a longer password would be
	// silently truncated (a weaker credential than the user believes). Reject it.
	if len(req.Password) > 72 {
		problems["password"] = "must be at most 72 bytes"
	}
	if strings.TrimSpace(req.FullName) == "" {
		problems["full_name"] = "is required"
	}
	return problems
}

func validateLogin(req LoginRequest) map[string]string {
	problems := map[string]string{}
	if strings.TrimSpace(req.Email) == "" {
		problems["email"] = "is required"
	}
	if req.Password == "" {
		problems["password"] = "is required"
	}
	return problems
}

func validateAddress(req AddressRequest) map[string]string {
	problems := map[string]string{}
	if strings.TrimSpace(req.RecipientName) == "" {
		problems["recipient_name"] = "is required"
	}
	if strings.TrimSpace(req.Phone) == "" {
		problems["phone"] = "is required"
	}
	if strings.TrimSpace(req.Line1) == "" {
		problems["line1"] = "is required"
	}
	if strings.TrimSpace(req.City) == "" {
		problems["city"] = "is required"
	}
	if strings.TrimSpace(req.Country) == "" {
		problems["country"] = "is required"
	}
	return problems
}
