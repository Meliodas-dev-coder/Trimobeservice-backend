package invoicing

import (
	"fmt"
	"strings"
)

const dateFR = "02/01/2006"

// strPtr returns nil for a blank string, else a pointer to the trimmed value.
func strPtr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// coalesceStr returns the first non-blank pointer among the arguments.
func coalesceStr(vals ...*string) *string {
	for _, v := range vals {
		if v != nil {
			if s := strings.TrimSpace(*v); s != "" {
				return &s
			}
		}
	}
	return nil
}

// joinAddress joins the non-blank address parts into a single block, comma-separated.
func joinAddress(parts ...*string) *string {
	var out []string
	for _, p := range parts {
		if p != nil {
			if s := strings.TrimSpace(*p); s != "" {
				out = append(out, s)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	joined := strings.Join(out, ", ")
	return &joined
}

// variantDetail combines a product variant label and SKU into a line subtitle.
func variantDetail(label, sku *string) *string {
	var parts []string
	if label != nil && strings.TrimSpace(*label) != "" {
		parts = append(parts, strings.TrimSpace(*label))
	}
	if sku != nil && strings.TrimSpace(*sku) != "" {
		parts = append(parts, "réf. "+strings.TrimSpace(*sku))
	}
	if len(parts) == 0 {
		return nil
	}
	joined := strings.Join(parts, " · ")
	return &joined
}

func bookingLineDetail(c bookingCarRow) string {
	var parts []string
	if c.CarCategory != nil && strings.TrimSpace(*c.CarCategory) != "" {
		parts = append(parts, strings.TrimSpace(*c.CarCategory))
	}
	parts = append(parts, fmt.Sprintf("%d jour(s)", c.Days))
	parts = append(parts, fmt.Sprintf("%s → %s", c.StartAt.Format(dateFR), c.EndAt.Format(dateFR)))
	return strings.Join(parts, " · ")
}

func composeEventDetail(services, artists []string) *string {
	var parts []string
	if len(services) > 0 {
		parts = append(parts, "Prestations : "+strings.Join(services, ", "))
	}
	if len(artists) > 0 {
		parts = append(parts, "Artistes : "+strings.Join(artists, ", "))
	}
	if len(parts) == 0 {
		return nil
	}
	joined := strings.Join(parts, ". ")
	return &joined
}

func composeHealthcareDetail(h healthcareRow, staff []string) *string {
	var parts []string
	if strings.TrimSpace(h.PatientName) != "" {
		parts = append(parts, "Patient : "+strings.TrimSpace(h.PatientName))
	}
	switch {
	case h.StartAt != nil && h.EndAt != nil:
		parts = append(parts, fmt.Sprintf("Période : %s → %s", h.StartAt.Format(dateFR), h.EndAt.Format(dateFR)))
	case h.PreferredAt != nil:
		parts = append(parts, "Visite : "+h.PreferredAt.Format(dateFR))
	}
	if len(staff) > 0 {
		parts = append(parts, "Équipe : "+strings.Join(staff, ", "))
	}
	if len(parts) == 0 {
		return nil
	}
	joined := strings.Join(parts, ". ")
	return &joined
}
