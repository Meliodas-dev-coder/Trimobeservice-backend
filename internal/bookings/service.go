package bookings

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

const carStatusAvailable = "available"

var (
	ErrCarUnavailable       = errors.New("car is not available for hire")
	ErrCarNotFree           = errors.New("car is already booked for the selected dates")
	ErrInvalidDates         = errors.New("end must be after start")
	ErrPastStart            = errors.New("start date must be in the future")
	ErrDriverInactive       = errors.New("driver is not active")
	ErrDriverBusy           = errors.New("driver is already assigned for the selected dates")
	ErrNotAssignable        = errors.New("driver can only be assigned to a confirmed booking")
	ErrInvalidTransition    = errors.New("invalid status transition")
	ErrNotCancellable       = errors.New("booking can no longer be cancelled")
	ErrDistanceRequired     = errors.New("distance_km and dropoff_location are required for cargo bookings")
	ErrInvalidDistance      = errors.New("distance_km must be a positive decimal distance")
	ErrRegionChoiceRequired = errors.New("outside_antananarivo must be selected for standard car bookings")
	ErrBookingHasPayments   = errors.New("bookings with payment history cannot be deleted")
	ErrCarSelectionRequired = errors.New("at least two cars are required")
	ErrDuplicateCar         = errors.New("the same car cannot be booked twice in one request")
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// CheckAvailability reports whether a car is free for a date range (read-only).
func (s *Service) CheckAvailability(ctx context.Context, carID int64, start, end time.Time) (*AvailabilityResult, error) {
	if !end.After(start) {
		return nil, ErrInvalidDates
	}
	car, err := s.repo.GetCar(ctx, carID)
	if err != nil {
		return nil, err
	}
	available := car.Status == carStatusAvailable
	if available {
		overlap, err := s.repo.CarOverlaps(ctx, carID, 0, start, end)
		if err != nil {
			return nil, err
		}
		available = !overlap
	}
	return &AvailabilityResult{CarID: carID, StartAt: start, EndAt: end, Available: available}, nil
}

// BookedRanges lists a car's booking windows for the client calendar: every
// non-cancelled booking that ended within the last ~6 months or is current/
// upcoming, so clients see the car's recent and future schedule (cancelled
// bookings free the car and are excluded).
func (s *Service) BookedRanges(ctx context.Context, carID int64) ([]BookedRange, error) {
	if _, err := s.repo.GetCar(ctx, carID); err != nil {
		return nil, err
	}
	return s.repo.ListBookedRanges(ctx, carID, time.Now().AddDate(0, -6, 0))
}

// Create books a car for a date range. It locks the car row, verifies the car
// is available and free, snapshots the rate, and inserts a confirmed booking —
// all in one transaction. The DB trigger is the backstop against overlaps.
func (s *Service) Create(ctx context.Context, userID int64, req CreateBookingRequest) (*BookingDetail, error) {
	uid := userID
	return s.create(ctx, &uid, "", nil, req)
}

func (s *Service) CreateAdmin(ctx context.Context, req AdminCreateBookingRequest) (*BookingDetail, error) {
	return s.create(ctx, req.UserID, req.CustomerName, req.DriverID, CreateBookingRequest{
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
		ContactPhone:        req.ContactPhone,
		DistanceKm:          req.DistanceKm,
		OutsideAntananarivo: req.OutsideAntananarivo,
		Note:                req.Note,
	})
}

func (s *Service) create(ctx context.Context, userID *int64, customerName string, driverID *int64, req CreateBookingRequest) (*BookingDetail, error) {
	if !req.EndAt.After(req.StartAt) {
		return nil, ErrInvalidDates
	}
	days := billableDays(req.StartAt, req.EndAt)

	var bookingID int64
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		car, err := s.repo.LockCar(ctx, tx, req.CarID)
		if err != nil {
			return err
		}
		if car.Status != carStatusAvailable {
			return ErrCarUnavailable
		}
		if req.StartAt.Before(time.Now()) && !isAllowedTodayCargoWindow(car, req.StartAt, req.EndAt) {
			return ErrPastStart
		}
		overlap, err := s.repo.HasCarOverlap(ctx, tx, car.ID, 0, req.StartAt, req.EndAt)
		if err != nil {
			return err
		}
		if overlap {
			return ErrCarNotFree
		}
		catName, err := s.repo.CategoryName(ctx, tx, car.CategoryID)
		if err != nil {
			return err
		}
		name := strings.TrimSpace(customerName)
		var customerNameSnapshot *string
		if name != "" {
			customerNameSnapshot = &name
		}
		if userID != nil {
			accountName, err := s.repo.CustomerName(ctx, tx, *userID)
			if err != nil {
				return err
			}
			if customerNameSnapshot == nil {
				customerNameSnapshot = accountName
			}
		}
		bookingDays := days
		selectedDailyRate, outsideAntananarivo, err := bookingDailyRate(car, req.OutsideAntananarivo)
		if err != nil {
			return err
		}
		rateCents, err := parseCents(selectedDailyRate)
		if err != nil {
			return err
		}
		pricingModel := PricingDaily
		total := rateCents * int64(bookingDays) // fees are 0 for now
		var distanceKm *string
		var cargoPerKmRate *string
		var cargoMinimumRate *string
		if car.IsCargoTransport {
			if req.DropoffLocation == nil || strings.TrimSpace(*req.DropoffLocation) == "" || req.DistanceKm == nil {
				return ErrDistanceRequired
			}
			distanceHundredths, err := parseDistanceHundredths(*req.DistanceKm)
			if err != nil || distanceHundredths <= 0 {
				return ErrInvalidDistance
			}
			perKmCents, err := parseCents(car.CargoPerKmRate)
			if err != nil {
				return err
			}
			minimumCents, err := parseCents(car.CargoMinimumRate)
			if err != nil {
				return err
			}
			pricingModel = PricingCargoDistance
			bookingDays = 1
			total = cargoTotalCents(distanceHundredths, perKmCents, minimumCents)
			distance := formatHundredths(distanceHundredths)
			distanceKm = &distance
			perKmRate := car.CargoPerKmRate
			minimumRate := car.CargoMinimumRate
			cargoPerKmRate = &perKmRate
			cargoMinimumRate = &minimumRate
		}

		b := &Booking{
			UserID:              userID,
			CustomerName:        customerNameSnapshot,
			CarID:               car.ID,
			BookingNumber:       newBookingNumber(),
			Status:              StatusConfirmed,
			PaymentStatus:       PaymentUnpaid,
			StartAt:             req.StartAt,
			EndAt:               req.EndAt,
			Days:                bookingDays,
			DailyRateSnapshot:   selectedDailyRate,
			OutsideAntananarivo: outsideAntananarivo,
			Fees:                "0.00",
			TotalPrice:          formatCents(total),
			PricingModel:        pricingModel,
			DistanceKm:          distanceKm,
			CargoPerKmRate:      cargoPerKmRate,
			CargoMinimumRate:    cargoMinimumRate,
			CarName:             car.Name,
			CarCategory:         catName,
			PickupLocation:      strings.TrimSpace(req.PickupLocation),
			PickupLatitude:      req.PickupLatitude,
			PickupLongitude:     req.PickupLongitude,
			PickupReference:     req.PickupReference,
			DropoffLocation:     req.DropoffLocation,
			DropoffLatitude:     req.DropoffLatitude,
			DropoffLongitude:    req.DropoffLongitude,
			DropoffReference:    req.DropoffReference,
			ContactPhone:        strings.TrimSpace(req.ContactPhone),
			Note:                req.Note,
		}
		id, err := s.repo.InsertBooking(ctx, tx, b)
		if err != nil {
			return err
		}
		bookingID = id
		if driverID != nil {
			d, err := s.repo.GetDriver(ctx, tx, *driverID)
			if err != nil {
				return err
			}
			if d.Status == "inactive" {
				return ErrDriverInactive
			}
			busy, err := s.repo.HasDriverOverlap(ctx, tx, *driverID, bookingID, req.StartAt, req.EndAt)
			if err != nil {
				return err
			}
			if busy {
				return ErrDriverBusy
			}
			if err := s.repo.AssignDriverTx(ctx, tx, bookingID, *driverID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.detail(ctx, bookingID)
}

// CreateBatch reserves several physical cars for one trip in a single
// transaction. Cars are locked in ID order to keep concurrent batch requests
// deadlock-safe. If any car is unavailable or overlaps, the whole batch rolls
// back so customers never receive a partial reservation.
func (s *Service) CreateBatch(ctx context.Context, userID int64, req CreateBookingBatchRequest) (*BookingDetail, error) {
	carIDs, err := orderedBatchCarIDs(req.CarIDs)
	if err != nil {
		return nil, err
	}
	if !req.EndAt.After(req.StartAt) {
		return nil, ErrInvalidDates
	}

	var bookingID int64
	err = s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		cars := make(map[int64]*carRow, len(carIDs))
		for _, carID := range carIDs {
			car, err := s.repo.LockCar(ctx, tx, carID)
			if err != nil {
				return err
			}
			cars[carID] = car
		}

		customerName, err := s.repo.CustomerName(ctx, tx, userID)
		if err != nil {
			return err
		}
		now := time.Now()
		pending := make([]*Booking, 0, len(req.CarIDs))
		var combinedTotal int64
		for _, carID := range req.CarIDs {
			item := req.bookingFor(carID)
			car := cars[carID]
			if car.Status != carStatusAvailable {
				return ErrCarUnavailable
			}
			if item.StartAt.Before(now) && !isAllowedTodayCargoWindow(car, item.StartAt, item.EndAt) {
				return ErrPastStart
			}
			overlap, err := s.repo.HasCarOverlap(ctx, tx, car.ID, 0, item.StartAt, item.EndAt)
			if err != nil {
				return err
			}
			if overlap {
				return ErrCarNotFree
			}
			catName, err := s.repo.CategoryName(ctx, tx, car.CategoryID)
			if err != nil {
				return err
			}

			bookingDays := billableDays(item.StartAt, item.EndAt)
			selectedDailyRate, outsideAntananarivo, err := bookingDailyRate(car, item.OutsideAntananarivo)
			if err != nil {
				return err
			}
			rateCents, err := parseCents(selectedDailyRate)
			if err != nil {
				return err
			}
			pricingModel := PricingDaily
			total := rateCents * int64(bookingDays)
			var distanceKm *string
			var cargoPerKmRate *string
			var cargoMinimumRate *string
			if car.IsCargoTransport {
				if item.DropoffLocation == nil || strings.TrimSpace(*item.DropoffLocation) == "" || item.DistanceKm == nil {
					return ErrDistanceRequired
				}
				distanceHundredths, err := parseDistanceHundredths(*item.DistanceKm)
				if err != nil || distanceHundredths <= 0 {
					return ErrInvalidDistance
				}
				perKmCents, err := parseCents(car.CargoPerKmRate)
				if err != nil {
					return err
				}
				minimumCents, err := parseCents(car.CargoMinimumRate)
				if err != nil {
					return err
				}
				pricingModel = PricingCargoDistance
				bookingDays = 1
				total = cargoTotalCents(distanceHundredths, perKmCents, minimumCents)
				distance := formatHundredths(distanceHundredths)
				distanceKm = &distance
				perKmRate := car.CargoPerKmRate
				minimumRate := car.CargoMinimumRate
				cargoPerKmRate = &perKmRate
				cargoMinimumRate = &minimumRate
			}

			b := &Booking{
				UserID:              &userID,
				CustomerName:        customerName,
				CarID:               car.ID,
				BookingNumber:       newBookingNumber(),
				Status:              StatusConfirmed,
				PaymentStatus:       PaymentUnpaid,
				StartAt:             item.StartAt,
				EndAt:               item.EndAt,
				Days:                bookingDays,
				DailyRateSnapshot:   selectedDailyRate,
				OutsideAntananarivo: outsideAntananarivo,
				Fees:                "0.00",
				TotalPrice:          formatCents(total),
				PricingModel:        pricingModel,
				DistanceKm:          distanceKm,
				CargoPerKmRate:      cargoPerKmRate,
				CargoMinimumRate:    cargoMinimumRate,
				CarName:             car.Name,
				CarCategory:         catName,
				PickupLocation:      strings.TrimSpace(item.PickupLocation),
				PickupLatitude:      item.PickupLatitude,
				PickupLongitude:     item.PickupLongitude,
				PickupReference:     item.PickupReference,
				DropoffLocation:     item.DropoffLocation,
				DropoffLatitude:     item.DropoffLatitude,
				DropoffLongitude:    item.DropoffLongitude,
				DropoffReference:    item.DropoffReference,
				ContactPhone:        strings.TrimSpace(item.ContactPhone),
				Note:                item.Note,
			}
			pending = append(pending, b)
			combinedTotal += total
		}

		firstID, err := s.repo.InsertBooking(ctx, tx, pending[0])
		if err != nil {
			return err
		}
		bookingID = firstID
		if err := s.repo.InsertBookingGroup(ctx, tx, firstID, pending[0].BookingNumber, formatCents(combinedTotal)); err != nil {
			return err
		}
		if err := s.repo.SetBookingGroup(ctx, tx, firstID, firstID); err != nil {
			return err
		}
		for _, item := range pending[1:] {
			item.BookingGroupID = &firstID
			if _, err := s.repo.InsertBooking(ctx, tx, item); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.detail(ctx, bookingID)
}

func orderedBatchCarIDs(carIDs []int64) ([]int64, error) {
	if len(carIDs) < 2 {
		return nil, ErrCarSelectionRequired
	}
	seen := make(map[int64]struct{}, len(carIDs))
	ordered := make([]int64, 0, len(carIDs))
	for _, carID := range carIDs {
		if carID <= 0 {
			return nil, ErrCarSelectionRequired
		}
		if _, exists := seen[carID]; exists {
			return nil, ErrDuplicateCar
		}
		seen[carID] = struct{}{}
		ordered = append(ordered, carID)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	return ordered, nil
}

// --- customer actions ---

func (s *Service) ListMyBookings(ctx context.Context, userID int64, limit, offset int) ([]Booking, int, error) {
	return s.repo.List(ctx, BookingFilter{UserID: &userID, Limit: limit, Offset: offset})
}

func (s *Service) GetMyBooking(ctx context.Context, userID, id int64) (*BookingDetail, error) {
	b, err := s.repo.GetForUser(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	return s.detail(ctx, bookingRootID(b))
}

func (s *Service) CancelMyBooking(ctx context.Context, userID, id int64) (*BookingDetail, error) {
	b, err := s.repo.GetForUser(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	rootID := bookingRootID(b)
	if rootID != b.ID {
		b, err = s.repo.GetForUser(ctx, userID, rootID)
		if err != nil {
			return nil, err
		}
	}
	if !isCancellable(b.Status) {
		return nil, ErrNotCancellable
	}
	if err := s.repo.SetStatus(ctx, rootID, StatusCancelled); err != nil {
		return nil, err
	}
	return s.detail(ctx, rootID)
}

// --- admin actions ---

func (s *Service) List(ctx context.Context, f BookingFilter) ([]Booking, int, error) {
	return s.repo.List(ctx, f)
}

func (s *Service) Get(ctx context.Context, id int64) (*BookingDetail, error) {
	return s.detail(ctx, id)
}

// Delete permanently removes an unpaid booking. Bookings with paid or refunded
// ledger entries are retained so payment history never points at a missing
// business record. The repository repeats this guard under a row lock.
func (s *Service) Delete(ctx context.Context, id int64) error {
	b, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	rootID := bookingRootID(b)
	if rootID != b.ID {
		b, err = s.repo.GetByID(ctx, rootID)
		if err != nil {
			return err
		}
	}
	if !isDeletable(b) {
		return ErrBookingHasPayments
	}
	return s.repo.DeleteBooking(ctx, rootID)
}

// AssignDriver attaches a driver to a confirmed booking, ensuring the driver is
// active and not already committed to an overlapping booking.
func (s *Service) AssignDriver(ctx context.Context, bookingID, driverID int64) (*BookingDetail, error) {
	b, err := s.repo.GetByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if b.Status != StatusConfirmed && b.Status != StatusDriverAssigned {
		return nil, ErrNotAssignable
	}
	err = s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		d, err := s.repo.GetDriver(ctx, tx, driverID)
		if err != nil {
			return err
		}
		if d.Status == "inactive" {
			return ErrDriverInactive
		}
		busy, err := s.repo.HasDriverOverlap(ctx, tx, driverID, b.ID, b.StartAt, b.EndAt)
		if err != nil {
			return err
		}
		if busy {
			return ErrDriverBusy
		}
		return s.repo.AssignDriverTx(ctx, tx, b.ID, driverID)
	})
	if err != nil {
		return nil, err
	}
	return s.detail(ctx, bookingID)
}

// AssignDriverForBooking assigns a driver to one car item and verifies that
// the item belongs to the booking shown in the admin UI. For single-car
// bookings itemID may be zero, in which case the booking itself is assigned.
func (s *Service) AssignDriverForBooking(ctx context.Context, bookingID, itemID, driverID int64) (*BookingDetail, error) {
	booking, err := s.repo.GetByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	rootID := bookingRootID(booking)
	if itemID == 0 {
		itemID = bookingID
	}
	item, err := s.repo.GetByID(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if bookingRootID(item) != rootID {
		return nil, ErrBookingNotFound
	}
	return s.AssignDriver(ctx, itemID, driverID)
}

func (s *Service) UpdateStatus(ctx context.Context, id int64, target string) (*BookingDetail, error) {
	b, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	rootID := bookingRootID(b)
	if rootID != b.ID {
		b, err = s.repo.GetByID(ctx, rootID)
		if err != nil {
			return nil, err
		}
	}
	if !canTransition(b.Status, target) {
		return nil, ErrInvalidTransition
	}
	if err := s.repo.SetStatus(ctx, rootID, target); err != nil {
		return nil, err
	}
	return s.detail(ctx, rootID)
}

// --- helpers ---

func (s *Service) detail(ctx context.Context, id int64) (*BookingDetail, error) {
	b, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	rootID := bookingRootID(b)
	if rootID != b.ID {
		b, err = s.repo.GetByID(ctx, rootID)
		if err != nil {
			return nil, err
		}
	}
	if b.IsMultiCar {
		cars, err := s.repo.ListBookingCars(ctx, rootID)
		if err != nil {
			return nil, err
		}
		for i := range cars {
			if cars[i].DriverID == nil {
				continue
			}
			if driver, err := s.repo.DriverInfo(ctx, *cars[i].DriverID); err == nil {
				cars[i].Driver = driver
			}
		}
		return &BookingDetail{Booking: *b, Cars: cars}, nil
	}
	return s.attachDriver(ctx, b)
}

func bookingRootID(b *Booking) int64 {
	if b != nil && b.BookingGroupID != nil {
		return *b.BookingGroupID
	}
	if b == nil {
		return 0
	}
	return b.ID
}

func (s *Service) attachDriver(ctx context.Context, b *Booking) (*BookingDetail, error) {
	detail := &BookingDetail{Booking: *b}
	if b.DriverID != nil {
		if di, err := s.repo.DriverInfo(ctx, *b.DriverID); err == nil {
			detail.Driver = di
		}
	}
	return detail, nil
}

func isCancellable(status string) bool {
	return status == StatusConfirmed || status == StatusDriverAssigned
}

func isDeletable(b *Booking) bool {
	return b != nil && b.PaymentStatus == PaymentUnpaid
}

func canTransition(from, to string) bool {
	switch to {
	case StatusActive:
		return from == StatusConfirmed || from == StatusDriverAssigned
	case StatusCompleted:
		return from == StatusActive
	case StatusCancelled:
		return from == StatusConfirmed || from == StatusDriverAssigned
	}
	return false
}

// billableDays counts inclusive calendar days: pickup day is day 1, and the
// return day is billed too even when the elapsed duration is under 24 hours.
func billableDays(start, end time.Time) int {
	startDay := dateOnly(start)
	endDay := dateOnly(end.In(start.Location()))
	days := int(endDay.Sub(startDay).Hours()/24) + 1
	if days < 1 {
		days = 1
	}
	return days
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func isAllowedTodayCargoWindow(car *carRow, start, end time.Time) bool {
	now := time.Now()
	return car.IsCargoTransport && sameLocalDay(start, now) && end.After(now)
}

func sameLocalDay(a, b time.Time) bool {
	aa := a.In(time.Local)
	bb := b.In(time.Local)
	return aa.Year() == bb.Year() && aa.YearDay() == bb.YearDay()
}

func bookingDailyRate(car *carRow, outsideAntananarivo *bool) (string, bool, error) {
	if car.IsCargoTransport {
		return car.DailyRate, false, nil
	}
	if outsideAntananarivo == nil {
		return "", false, ErrRegionChoiceRequired
	}
	if *outsideAntananarivo {
		return car.OutsideAntananarivoDailyRate, true, nil
	}
	return car.DailyRate, false, nil
}

func newBookingNumber() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("BKG-%s-%s", time.Now().Format("20060102"), hex.EncodeToString(b))
}
