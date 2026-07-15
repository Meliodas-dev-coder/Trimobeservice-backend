package bookings

import "testing"

func TestCargoTotalCents(t *testing.T) {
	tests := []struct {
		name               string
		distanceHundredths int64
		perKmCents         int64
		minimumCents       int64
		want               int64
	}{
		{
			name:               "under ten kilometers uses the 120k minimum",
			distanceHundredths: 500,
			perKmCents:         1000000,
			minimumCents:       12000000,
			want:               12000000,
		},
		{
			name:               "ten kilometers uses the 120k minimum",
			distanceHundredths: 1000,
			perKmCents:         1000000,
			minimumCents:       12000000,
			want:               12000000,
		},
		{
			name:               "distance above ten adds 10k per kilometer",
			distanceHundredths: 1250,
			perKmCents:         1000000,
			minimumCents:       12000000,
			want:               14500000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cargoTotalCents(tt.distanceHundredths, tt.perKmCents, tt.minimumCents)
			if got != tt.want {
				t.Fatalf("cargoTotalCents() = %d, want %d", got, tt.want)
			}
		})
	}
}
