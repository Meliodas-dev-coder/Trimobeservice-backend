package bookings

import (
	"errors"
	"reflect"
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

func TestIsDeletableRequiresUnpaidBooking(t *testing.T) {
	tests := []struct {
		name    string
		booking *Booking
		want    bool
	}{
		{name: "unpaid", booking: &Booking{PaymentStatus: PaymentUnpaid}, want: true},
		{name: "paid", booking: &Booking{PaymentStatus: PaymentPaid}, want: false},
		{name: "refunded", booking: &Booking{PaymentStatus: PaymentRefunded}, want: false},
		{name: "missing", booking: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDeletable(tt.booking); got != tt.want {
				t.Fatalf("isDeletable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOrderedBatchCarIDs(t *testing.T) {
	got, err := orderedBatchCarIDs([]int64{9, 2, 5})
	if err != nil {
		t.Fatalf("orderedBatchCarIDs() error = %v", err)
	}
	if want := []int64{2, 5, 9}; !reflect.DeepEqual(got, want) {
		t.Fatalf("orderedBatchCarIDs() = %v, want %v", got, want)
	}

	if _, err := orderedBatchCarIDs([]int64{4, 4}); !errors.Is(err, ErrDuplicateCar) {
		t.Fatalf("duplicate error = %v, want %v", err, ErrDuplicateCar)
	}
	if _, err := orderedBatchCarIDs(nil); !errors.Is(err, ErrCarSelectionRequired) {
		t.Fatalf("empty selection error = %v, want %v", err, ErrCarSelectionRequired)
	}
	if _, err := orderedBatchCarIDs([]int64{4}); !errors.Is(err, ErrCarSelectionRequired) {
		t.Fatalf("single-car selection error = %v, want %v", err, ErrCarSelectionRequired)
	}
}

func TestValidateCreateBookingBatch(t *testing.T) {
	start := time.Now().Add(24 * time.Hour)
	valid := CreateBookingBatchRequest{
		CarIDs:         []int64{1, 2},
		StartAt:        start,
		EndAt:          start.Add(24 * time.Hour),
		PickupLocation: "Ivato Airport",
		ContactPhone:   "+261340000000",
	}
	if problems := validateCreateBookingBatch(valid); len(problems) != 0 {
		t.Fatalf("valid batch problems = %v", problems)
	}

	tests := []struct {
		name   string
		carIDs []int64
	}{
		{name: "one car belongs on single endpoint", carIDs: []int64{1}},
		{name: "duplicate car", carIDs: []int64{1, 1}},
		{name: "invalid car", carIDs: []int64{1, 0}},
		{name: "too many cars", carIDs: []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := valid
			req.CarIDs = tt.carIDs
			if problems := validateCreateBookingBatch(req); problems["car_ids"] == "" {
				t.Fatalf("car_ids problem missing: %v", problems)
			}
		})
	}
}
