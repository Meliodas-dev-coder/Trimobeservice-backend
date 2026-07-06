package payments

import (
	"context"
	"errors"
	"strings"

	"github.com/jmoiron/sqlx"
)

var (
	ErrInvalidPayable    = errors.New("payable_type must be 'order', 'booking', or 'event'")
	ErrOrderNotFulfilled = errors.New("order payment can only be recorded after delivery or pickup")
	ErrTargetClosed      = errors.New("the order, booking, or event is cancelled or expired")
	ErrAlreadyPaid       = errors.New("the order, booking, or event is already paid")
	ErrEventNotQuoted    = errors.New("set a quote on the event before recording payment")
	ErrNotRefundable     = errors.New("only a paid payment can be refunded")
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// Record creates a paid ledger entry against an order or booking and flips that
// target's payment_status to paid — atomically.
func (s *Service) Record(ctx context.Context, adminID int64, req RecordPaymentRequest) (*Payment, error) {
	info, err := s.targetInfo(ctx, req.PayableType, req.PayableID)
	if err != nil {
		return nil, err
	}

	switch req.PayableType {
	case PayableOrder:
		if info.Status == orderCancelled || info.Status == orderExpired {
			return nil, ErrTargetClosed
		}
		if info.Status != orderDelivered && info.Status != orderPickedUp {
			return nil, ErrOrderNotFulfilled // deliver / hand over first, then confirm payment
		}
	case PayableBooking:
		if info.Status == bookingCancelled {
			return nil, ErrTargetClosed
		}
	case PayableEvent:
		if info.Status == eventCancelled {
			return nil, ErrTargetClosed
		}
		if info.Total == "" { // no quote set yet
			return nil, ErrEventNotQuoted
		}
	}
	if info.PaymentStatus == targetPaid {
		return nil, ErrAlreadyPaid
	}

	amount := info.Total
	if req.Amount != nil {
		if a := strings.TrimSpace(*req.Amount); a != "" {
			amount = a
		}
	}

	var paymentID int64
	err = s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		p := &Payment{
			PayableType:  req.PayableType,
			PayableID:    req.PayableID,
			Amount:       amount,
			Method:       req.Method,
			Status:       StatusPaid,
			Reference:    req.Reference,
			MarkedPaidBy: &adminID,
			Note:         req.Note,
		}
		id, err := s.repo.InsertPaymentTx(ctx, tx, p)
		if err != nil {
			return err
		}
		paymentID = id
		return s.setTargetPayment(ctx, tx, req.PayableType, req.PayableID, targetPaid)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, paymentID)
}

// Refund marks a paid payment refunded and flips the target back to refunded.
func (s *Service) Refund(ctx context.Context, paymentID int64) (*Payment, error) {
	p, err := s.repo.GetByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	if p.Status != StatusPaid {
		return nil, ErrNotRefundable
	}
	err = s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		if err := s.repo.SetPaymentStatusTx(ctx, tx, paymentID, StatusRefunded); err != nil {
			return err
		}
		return s.setTargetPayment(ctx, tx, p.PayableType, p.PayableID, StatusRefunded)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, paymentID)
}

func (s *Service) List(ctx context.Context, f PaymentFilter) ([]Payment, int, error) {
	return s.repo.List(ctx, f)
}

func (s *Service) Get(ctx context.Context, id int64) (*PaymentDetail, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	target, err := s.repo.GetTarget(ctx, p.PayableType, p.PayableID)
	if err != nil {
		return nil, err
	}
	return &PaymentDetail{Payment: *p, Target: target}, nil
}

// --- helpers ---

func (s *Service) targetInfo(ctx context.Context, payableType string, id int64) (*targetInfo, error) {
	switch payableType {
	case PayableOrder:
		return s.repo.GetOrderInfo(ctx, id)
	case PayableBooking:
		return s.repo.GetBookingInfo(ctx, id)
	case PayableEvent:
		return s.repo.GetEventInfo(ctx, id)
	default:
		return nil, ErrInvalidPayable
	}
}

func (s *Service) setTargetPayment(ctx context.Context, tx *sqlx.Tx, payableType string, id int64, status string) error {
	switch payableType {
	case PayableOrder:
		return s.repo.SetOrderPaymentTx(ctx, tx, id, status)
	case PayableEvent:
		return s.repo.SetEventPaymentTx(ctx, tx, id, status)
	default:
		return s.repo.SetBookingPaymentTx(ctx, tx, id, status)
	}
}
