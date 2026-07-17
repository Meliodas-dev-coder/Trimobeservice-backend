package events

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var errBadMoney = errors.New("amount is not a valid decimal")

// parseCents converts a DECIMAL(12,2)-shaped string ("150000" / "150000.50")
// into integer cents, avoiding float rounding when summing line prices.
func parseCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errBadMoney
	}
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	whole, frac, _ := strings.Cut(s, ".")
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, errBadMoney
	}
	cents := w * 100
	if frac != "" {
		if len(frac) > 2 {
			return 0, errBadMoney
		}
		for len(frac) < 2 {
			frac += "0"
		}
		c, err := strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, errBadMoney
		}
		cents += c
	}
	if neg {
		cents = -cents
	}
	return cents, nil
}

// formatCents renders integer cents back to a DECIMAL(12,2) string.
func formatCents(c int64) string {
	neg := c < 0
	if neg {
		c = -c
	}
	s := fmt.Sprintf("%d.%02d", c/100, c%100)
	if neg {
		s = "-" + s
	}
	return s
}
