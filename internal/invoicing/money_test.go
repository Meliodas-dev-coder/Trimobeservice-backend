package invoicing

import "testing"

func TestParseCents(t *testing.T) {
	cases := map[string]int64{
		"":           0,
		"0":          0,
		"120":        12000,
		"120.5":      12050,
		"120.50":     12050,
		"1500000.00": 150000000,
		"-250.25":    -25025,
		"0.99":       99,
		"10.999":     1099, // extra precision truncated
		"20.00":      2000,
	}
	for in, want := range cases {
		got, err := parseCents(in)
		if err != nil {
			t.Fatalf("parseCents(%q) error: %v", in, err)
		}
		if got != want {
			t.Errorf("parseCents(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestFormatCents(t *testing.T) {
	cases := map[int64]string{
		0:         "0.00",
		99:        "0.99",
		12050:     "120.50",
		150000000: "1500000.00",
		-25025:    "-250.25",
		5:         "0.05",
	}
	for in, want := range cases {
		if got := formatCents(in); got != want {
			t.Errorf("formatCents(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestTaxCents(t *testing.T) {
	// 20.00% of 1 000 000.00 = 200 000.00
	if got := taxCents(100000000, 2000); got != 20000000 {
		t.Errorf("20%% tax = %d, want 20000000", got)
	}
	// 0% rate -> no tax
	if got := taxCents(12345, 0); got != 0 {
		t.Errorf("0%% tax = %d, want 0", got)
	}
	// round half up: 20% of 12.51 (1251 cents) = 2.502 -> 250 cents
	if got := taxCents(1251, 2000); got != 250 {
		t.Errorf("rounding = %d, want 250", got)
	}
}

func TestComputeTotals(t *testing.T) {
	lines := []lineInput{
		{LineTotalC: 150000000}, // 1 500 000.00
		{LineTotalC: 50000000},  //   500 000.00
	}
	// no discount, 20% tax
	sub, tax, total := computeTotals(lines, 0, 2000)
	if sub != 200000000 {
		t.Errorf("subtotal = %d, want 200000000", sub)
	}
	if tax != 40000000 {
		t.Errorf("tax = %d, want 40000000", tax)
	}
	if total != 240000000 {
		t.Errorf("total = %d, want 240000000", total)
	}

	// with discount 100 000.00, tax computed on the discounted base
	sub2, tax2, total2 := computeTotals(lines, 10000000, 2000)
	if sub2 != 200000000 {
		t.Errorf("subtotal(discount) = %d, want 200000000", sub2)
	}
	if tax2 != 38000000 { // 20% of 1 900 000.00
		t.Errorf("tax(discount) = %d, want 38000000", tax2)
	}
	if total2 != 228000000 { // 1 900 000 + 380 000
		t.Errorf("total(discount) = %d, want 228000000", total2)
	}

	// no tax at all
	_, tax3, total3 := computeTotals(lines, 0, 0)
	if tax3 != 0 || total3 != 200000000 {
		t.Errorf("no-tax totals = (%d,%d), want (0,200000000)", tax3, total3)
	}
}
