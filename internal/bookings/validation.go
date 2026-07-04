package bookings

import "strings"

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
	return p
}

func validateAdminCreateBooking(req AdminCreateBookingRequest) map[string]string {
	p := validateCreateBooking(CreateBookingRequest{
		CarID:           req.CarID,
		StartAt:         req.StartAt,
		EndAt:           req.EndAt,
		PickupLocation:  req.PickupLocation,
		DropoffLocation: req.DropoffLocation,
		DistanceKm:      req.DistanceKm,
		ContactPhone:    req.ContactPhone,
		Note:            req.Note,
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
