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
