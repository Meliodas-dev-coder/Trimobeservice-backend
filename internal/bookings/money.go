package bookings

import (
	"fmt"
	"strconv"
	"strings"
)

// parseCents converts a DECIMAL(12,2) string into integer cents.
func parseCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")

	parts := strings.SplitN(s, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount %q", s)
	}
	cents := whole * 100
	if len(parts) == 2 {
		frac := parts[1]
		if len(frac) == 1 {
			frac += "0"
		} else if len(frac) > 2 {
			frac = frac[:2]
		}
		f, err := strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid amount %q", s)
		}
		cents += f
	}
	if neg {
		cents = -cents
	}
	return cents, nil
}

// formatCents renders integer cents back to a "d.dd" string.
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

// parseDistanceHundredths converts a kilometer distance to hundredths of a km.
func parseDistanceHundredths(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("invalid distance %q", s)
	}
	parts := strings.SplitN(s, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid distance %q", s)
	}
	out := whole * 100
	if len(parts) == 2 {
		frac := parts[1]
		if len(frac) == 1 {
			frac += "0"
		} else if len(frac) > 2 {
			frac = frac[:2]
		}
		f, err := strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid distance %q", s)
		}
		out += f
	}
	return out, nil
}

func formatHundredths(n int64) string {
	return fmt.Sprintf("%d.%02d", n/100, n%100)
}

func cargoTotalCents(distanceHundredths, perKmCents, minimumCents int64) int64 {
	const includedKmHundredths = 1000 // first 10km are covered by the minimum.
	if distanceHundredths <= includedKmHundredths {
		return minimumCents
	}
	extra := distanceHundredths - includedKmHundredths
	return minimumCents + ((perKmCents*extra)+99)/100
}
