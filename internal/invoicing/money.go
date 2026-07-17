package invoicing

import (
	"errors"
	"strconv"
	"strings"
)

// Money is handled as int64 minor units ("cents") internally so line sums and
// tax are exact, then formatted back to a DECIMAL(12,2) string for storage/JSON.
// MGA has no practical subdivision, but the schema keeps 2 decimals for
// consistency with every other money column in the platform.

var errBadAmount = errors.New("invalid decimal amount")

// parseCents converts a decimal string ("1500000.00", "-250.5", "") to int64
// cents. Empty/blank parses to 0.
func parseCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	neg := false
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		neg = true
		s = s[1:]
	}
	intPart, fracPart, hasDot := strings.Cut(s, ".")
	if intPart == "" {
		intPart = "0"
	}
	// Normalize the fractional part to exactly 2 digits.
	if hasDot {
		if len(fracPart) > 2 {
			fracPart = fracPart[:2] // truncate extra precision
		}
		for len(fracPart) < 2 {
			fracPart += "0"
		}
	} else {
		fracPart = "00"
	}
	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, errBadAmount
	}
	frac, err := strconv.ParseInt(fracPart, 10, 64)
	if err != nil {
		return 0, errBadAmount
	}
	cents := whole*100 + frac
	if neg {
		cents = -cents
	}
	return cents, nil
}

// formatCents renders int64 cents as a DECIMAL(12,2) string ("1500000.00").
func formatCents(c int64) string {
	neg := c < 0
	if neg {
		c = -c
	}
	whole := c / 100
	frac := c % 100
	s := strconv.FormatInt(whole, 10) + "." + pad2(frac)
	if neg {
		return "-" + s
	}
	return s
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

// taxCents computes tax on baseCents at a percentage rate given in basis-of-100
// cents (e.g. "20.00" -> rateCents 2000 meaning 20.00%), rounded half-up.
func taxCents(baseCents, rateCents int64) int64 {
	if baseCents == 0 || rateCents == 0 {
		return 0
	}
	// baseCents * (rateCents/100) / 100  == baseCents * rateCents / 10000
	neg := (baseCents < 0) != (rateCents < 0)
	b := baseCents
	if b < 0 {
		b = -b
	}
	r := rateCents
	if r < 0 {
		r = -r
	}
	num := b*r + 5000 // + half of 10000 for round-half-up
	res := num / 10000
	if neg {
		return -res
	}
	return res
}
