package bookings

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

const carStatusAvailable = "available"

var (
	ErrCarUnavailable    = errors.New("car is not available for hire")
	ErrCarNotFree        = errors.New("car is already booked for the selected dates")
	ErrInvalidDates      = errors.New("end must be after start")
	ErrPastStart         = errors.New("start date must be in the future")
	ErrDriverInactive    = errors.New("driver is not active")
	ErrDriverBusy        = errors.New("driver is already assigned for the selected dates")
	ErrNotAssignable     = errors.New("driver can only be assigned to a confirmed booking")
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrNotCancellable    = errors.New("booking can no longer be cancelled")
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

// Create books a car for a date range. It locks the car row, verifies the car
// is available and free, snapshots the rate, and inserts a confirmed booking —
// all in one transaction. The DB trigger is the backstop against overlaps.
func (s *Service) Create(ctx context.Context, userID int64, req CreateBookingRequest) (*BookingDetail, error) {
	if !req.EndAt.After(req.StartAt) {
		return nil, ErrInvalidDates
	}
	if req.StartAt.Before(time.Now()) {
		return nil, ErrPastStart
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
		rateCents, err := parseCents(car.DailyRate)
		if err != nil {
			return err
		}
		total := rateCents * int64(days) // fees are 0 for now

		b := &Booking{
			UserID:            userID,
			CarID:             car.ID,
			BookingNumber:     newBookingNumber(),
			Status:            StatusConfirmed,
			PaymentStatus:     PaymentUnpaid,
			StartAt:           req.StartAt,
			EndAt:             req.EndAt,
			Days:              days,
			DailyRateSnapshot: car.DailyRate,
			Fees:              "0.00",
			TotalPrice:        formatCents(total),
			CarName:           car.Name,
			CarCategory:       catName,
			PickupLocation:    strings.TrimSpace(req.PickupLocation),
			DropoffLocation:   req.DropoffLocation,
			ContactPhone:      strings.TrimSpace(req.ContactPhone),
			Note:              req.Note,
		}
		id, err := s.repo.InsertBooking(ctx, tx, b)
		if err != nil {
			return err
		}
		bookingID = id
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.detail(ctx, bookingID)
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
	return s.attachDriver(ctx, b)
}

func (s *Service) CancelMyBooking(ctx context.Context, userID, id int64) (*BookingDetail, error) {
	b, err := s.repo.GetForUser(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if !isCancellable(b.Status) {
		return nil, ErrNotCancellable
	}
	if err := s.repo.SetStatus(ctx, id, StatusCancelled); err != nil {
		return nil, err
	}
	return s.detail(ctx, id)
}

// --- admin actions ---

func (s *Service) List(ctx context.Context, f BookingFilter) ([]Booking, int, error) {
	return s.repo.List(ctx, f)
}

func (s *Service) Get(ctx context.Context, id int64) (*BookingDetail, error) {
	return s.detail(ctx, id)
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

func (s *Service) UpdateStatus(ctx context.Context, id int64, target string) (*BookingDetail, error) {
	b, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canTransition(b.Status, target) {
		return nil, ErrInvalidTransition
	}
	if err := s.repo.SetStatus(ctx, id, target); err != nil {
		return nil, err
	}
	return s.detail(ctx, id)
}

// --- helpers ---

func (s *Service) detail(ctx context.Context, id int64) (*BookingDetail, error) {
	b, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.attachDriver(ctx, b)
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

// billableDays is the number of 24h periods (rounded up), at least 1.
func billableDays(start, end time.Time) int {
	days := int(math.Ceil(end.Sub(start).Hours() / 24))
	if days < 1 {
		days = 1
	}
	return days
}

func newBookingNumber() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("BKG-%s-%s", time.Now().Format("20060102"), hex.EncodeToString(b))
}
