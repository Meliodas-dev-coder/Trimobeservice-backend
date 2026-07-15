package bookings

import (
	"errors"
	"testing"
)

func TestBookingDailyRate(t *testing.T) {
	local := false
	outside := true
	standard := &carRow{
		DailyRate:                    "150000.00",
		OutsideAntananarivoDailyRate: "225000.00",
	}

	tests := []struct {
		name        string
		car         *carRow
		choice      *bool
		wantRate    string
		wantOutside bool
		wantErr     error
	}{
		{name: "local standard trip", car: standard, choice: &local, wantRate: "150000.00"},
		{name: "outside standard trip", car: standard, choice: &outside, wantRate: "225000.00", wantOutside: true},
		{name: "standard trip requires choice", car: standard, wantErr: ErrRegionChoiceRequired},
		{
			name:     "cargo ignores region choice",
			car:      &carRow{IsCargoTransport: true, DailyRate: "0.00", OutsideAntananarivoDailyRate: "999999.00"},
			choice:   &outside,
			wantRate: "0.00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate, selectedOutside, err := bookingDailyRate(tt.car, tt.choice)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("bookingDailyRate() error = %v, want %v", err, tt.wantErr)
			}
			if rate != tt.wantRate || selectedOutside != tt.wantOutside {
				t.Fatalf("bookingDailyRate() = (%q, %v), want (%q, %v)", rate, selectedOutside, tt.wantRate, tt.wantOutside)
			}
		})
	}
}
