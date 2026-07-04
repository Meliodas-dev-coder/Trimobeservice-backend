package bookings

import (
	"testing"
	"time"
)

func TestBillableDaysInclusiveCalendarDays(t *testing.T) {
	loc := time.FixedZone("EAT", 3*60*60)

	tests := []struct {
		name  string
		start time.Time
		end   time.Time
		want  int
	}{
		{
			name:  "same day is one day",
			start: time.Date(2026, 7, 4, 9, 0, 0, 0, loc),
			end:   time.Date(2026, 7, 4, 17, 0, 0, 0, loc),
			want:  1,
		},
		{
			name:  "today to tomorrow is two days",
			start: time.Date(2026, 7, 4, 10, 0, 0, 0, loc),
			end:   time.Date(2026, 7, 5, 9, 0, 0, 0, loc),
			want:  2,
		},
		{
			name:  "short overnight still counts both days",
			start: time.Date(2026, 7, 4, 23, 0, 0, 0, loc),
			end:   time.Date(2026, 7, 5, 1, 0, 0, 0, loc),
			want:  2,
		},
		{
			name:  "three calendar dates are three days",
			start: time.Date(2026, 7, 4, 12, 0, 0, 0, loc),
			end:   time.Date(2026, 7, 6, 8, 0, 0, 0, loc),
			want:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := billableDays(tt.start, tt.end)
			if got != tt.want {
				t.Fatalf("billableDays() = %d, want %d", got, tt.want)
			}
		})
	}
}
