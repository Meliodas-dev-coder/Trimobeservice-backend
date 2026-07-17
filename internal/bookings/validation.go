package bookings

import "strings"

const maxCarsPerBatch = 10

func validateCreateBooking(req CreateBookingRequest) map[string]string {
	p := map[string]string{}
	if req.CarID <= 0 {
		p["car_id"] = "is required"
	}
	if req.StartAt.IsZero() {
		p["start_at"] = "is required (RFC3339)"
	}
	if req.EndAt.IsZero() {
		p["end_at"] = "is required (RFC3339)"
	}
	if !req.StartAt.IsZero() && !req.EndAt.IsZero() && !req.EndAt.After(req.StartAt) {
		p["end_at"] = "must be after start_at"
	}
	if strings.TrimSpace(req.PickupLocation) == "" {
		p["pickup_location"] = "is required"
	}
	if strings.TrimSpace(req.ContactPhone) == "" {
		p["contact_phone"] = "is required"
	}
	validateBookingCoordinates(p, "pickup", req.PickupLatitude, req.PickupLongitude)
	validateBookingCoordinates(p, "dropoff", req.DropoffLatitude, req.DropoffLongitude)
	return p
}

func validateCreateBookingBatch(req CreateBookingBatchRequest) map[string]string {
	p := validateCreateBooking(req.bookingFor(1))
	delete(p, "car_id")
	if len(req.CarIDs) < 2 || len(req.CarIDs) > maxCarsPerBatch {
		p["car_ids"] = "must contain between 2 and 10 cars"
		return p
	}
	seen := make(map[int64]struct{}, len(req.CarIDs))
	for _, id := range req.CarIDs {
		if id <= 0 {
			p["car_ids"] = "must contain valid car IDs"
			break
		}
		if _, exists := seen[id]; exists {
			p["car_ids"] = "must not contain the same car more than once"
			break
		}
		seen[id] = struct{}{}
	}
	return p
}

func validateAdminCreateBooking(req AdminCreateBookingRequest) map[string]string {
	p := validateCreateBooking(CreateBookingRequest{
		CarID:               req.CarID,
		StartAt:             req.StartAt,
		EndAt:               req.EndAt,
		PickupLocation:      req.PickupLocation,
		PickupLatitude:      req.PickupLatitude,
		PickupLongitude:     req.PickupLongitude,
		PickupReference:     req.PickupReference,
		DropoffLocation:     req.DropoffLocation,
		DropoffLatitude:     req.DropoffLatitude,
		DropoffLongitude:    req.DropoffLongitude,
		DropoffReference:    req.DropoffReference,
		DistanceKm:          req.DistanceKm,
		OutsideAntananarivo: req.OutsideAntananarivo,
		ContactPhone:        req.ContactPhone,
		Note:                req.Note,
	})
	if req.UserID != nil && *req.UserID <= 0 {
		p["user_id"] = "must be greater than zero"
	}
	if req.DriverID != nil && *req.DriverID <= 0 {
		p["driver_id"] = "must be greater than zero"
	}
	if strings.TrimSpace(req.CustomerName) == "" {
		p["customer_name"] = "is required"
	}
	return p
}

func validateBookingCoordinates(p map[string]string, prefix string, latitude, longitude *float64) {
	if (latitude == nil) != (longitude == nil) {
		p[prefix+"_coordinates"] = "latitude and longitude must be provided together"
		return
	}
	if latitude == nil {
		return
	}
	if *latitude < -90 || *latitude > 90 {
		p[prefix+"_latitude"] = "must be between -90 and 90"
	}
	if *longitude < -180 || *longitude > 180 {
		p[prefix+"_longitude"] = "must be between -180 and 180"
	}
}
