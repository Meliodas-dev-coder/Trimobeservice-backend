package bookings

import "testing"

func TestCargoTotalCents(t *testing.T) {
	tests := []struct {
		name              string
		distanceHundredth int64
		perKmCents        int64
		minimumCents      int64
		want              int64
	}{
		{
			name:              "under ten kilometers uses minimum",
			distanceHundredth: 500,
			perKmCents:        10000,
			minimumCents:      50000,
			want:              50000,
		},
		{
			name:              "ten kilometers uses minimum",
			distanceHundredth: 1000,
			perKmCents:        10000,
			minimumCents:      50000,
			want:              50000,
		},
		{
			name:              "above ten kilometers bills extra distance only",
			distanceHundredth: 1250,
			perKmCents:        10000,
			minimumCents:      50000,
			want:              75000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cargoTotalCents(tt.distanceHundredth, tt.perKmCents, tt.minimumCents)
			if got != tt.want {
				t.Fatalf("cargoTotalCents() = %d, want %d", got, tt.want)
			}
		})
	}
}
