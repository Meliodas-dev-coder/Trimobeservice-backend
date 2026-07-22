package orders

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/trimo/backend/internal/catalog"
	"github.com/trimo/backend/internal/stock"
)

var (
	ErrCartEmpty          = errors.New("cart is empty")
	ErrVariantInactive    = errors.New("a product in your cart is no longer available")
	ErrInsufficientStock  = errors.New("insufficient stock")
	ErrInvalidFulfillment = errors.New("invalid fulfillment type")
	ErrNotCancellable     = errors.New("order can no longer be cancelled")
	ErrInvalidTransition  = errors.New("invalid status transition")
	ErrNoItems            = errors.New("order has no items")
	// ErrForbiddenDepartment is returned when a department-scoped admin tries to
	// act on lines outside their catalog department.
	ErrForbiddenDepartment = errors.New("you do not have access to every line of this order")
)

type Service struct {
	repo *Repository
	log  *slog.Logger
}

func NewService(repo *Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// --- cart ---

func (s *Service) GetCart(ctx context.Context, userID int64) (*CartView, error) {
	cart, err := s.repo.GetOrCreateActiveCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.cartView(ctx, cart.ID)
}

func (s *Service) AddCartItem(ctx context.Context, userID int64, req AddCartItemRequest) (*CartView, error) {
	cart, err := s.repo.GetOrCreateActiveCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	vs, err := s.repo.GetVariantForCart(ctx, req.ProductVariantID)
	if err != nil {
		if errors.Is(err, ErrVariantMissing) {
			return nil, ErrVariantMissing
		}
		return nil, err
	}
	if !vs.IsActive {
		return nil, ErrVariantInactive
	}
	if vs.StockQuantity < req.Quantity {
		return nil, ErrInsufficientStock
	}
	if err := s.repo.AddCartItem(ctx, cart.ID, req.ProductVariantID, req.Quantity); err != nil {
		return nil, err
	}
	return s.cartView(ctx, cart.ID)
}

func (s *Service) UpdateCartItem(ctx context.Context, userID, itemID int64, req UpdateCartItemRequest) (*CartView, error) {
	cart, err := s.ownedCartItemCart(ctx, userID, itemID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateCartItemQty(ctx, itemID, req.Quantity); err != nil {
		return nil, err
	}
	return s.cartView(ctx, cart.ID)
}

func (s *Service) RemoveCartItem(ctx context.Context, userID, itemID int64) (*CartView, error) {
	cart, err := s.ownedCartItemCart(ctx, userID, itemID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.DeleteCartItem(ctx, itemID); err != nil {
		return nil, err
	}
	return s.cartView(ctx, cart.ID)
}

func (s *Service) ClearCart(ctx context.Context, userID int64) (*CartView, error) {
	cart, err := s.repo.GetOrCreateActiveCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.ClearCart(ctx, cart.ID); err != nil {
		return nil, err
	}
	return s.cartView(ctx, cart.ID)
}

// ownedCartItemCart verifies the cart item belongs to the user's active cart.
func (s *Service) ownedCartItemCart(ctx context.Context, userID, itemID int64) (*Cart, error) {
	cart, err := s.repo.GetOrCreateActiveCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.GetCartItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if item.CartID != cart.ID {
		return nil, ErrCartItemNotFound
	}
	return cart, nil
}

func (s *Service) cartView(ctx context.Context, cartID int64) (*CartView, error) {
	items, err := s.repo.ListCartItemViews(ctx, cartID)
	if err != nil {
		return nil, err
	}
	var subtotal int64
	var count int
	for _, it := range items {
		c, _ := parseCents(it.LineTotal)
		subtotal += c
		count += it.Quantity
	}
	return &CartView{CartID: cartID, Items: items, Subtotal: formatCents(subtotal), ItemCount: count}, nil
}

// --- checkout ---

func (s *Service) Checkout(ctx context.Context, userID int64, req CheckoutRequest) (*OrderDetail, error) {
	if req.FulfillmentType != FulfillmentDelivery && req.FulfillmentType != FulfillmentPickup {
		return nil, ErrInvalidFulfillment
	}

	cart, err := s.repo.GetOrCreateActiveCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListCartItems(ctx, cart.ID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrCartEmpty
	}

	var orderID int64
	err = s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		var subtotal int64
		orderItems := make([]OrderItem, 0, len(items))
		reservations := make([]stock.Movement, 0, len(items))

		for _, ci := range items {
			vs, err := s.repo.LockVariant(ctx, tx, ci.ProductVariantID)
			if err != nil {
				return err // ErrVariantMissing bubbles up
			}
			if !vs.IsActive {
				return ErrVariantInactive
			}
			if vs.StockQuantity < ci.Quantity {
				return fmt.Errorf("%w: %s", ErrInsufficientStock, vs.ProductName)
			}
			if err := s.repo.AdjustStock(ctx, tx, vs.ID, -ci.Quantity); err != nil {
				return err
			}
			priceCents, err := parseCents(vs.Price)
			if err != nil {
				return err
			}
			line := priceCents * int64(ci.Quantity)
			subtotal += line
			variantID := vs.ID
			sku := vs.SKU
			department := catalog.DepartmentForTemplateKey(vs.TemplateKey)
			orderItems = append(orderItems, OrderItem{
				ProductVariantID: &variantID,
				Department:       &department,
				ProductName:      vs.ProductName,
				VariantLabel:     vs.Label,
				SKU:              &sku,
				UnitPrice:        vs.Price,
				Quantity:         ci.Quantity,
				LineTotal:        formatCents(line),
			})
			// The row stays locked for the rest of the transaction, so the level
			// we just wrote is the level the ledger records. The order id is only
			// known after the insert below, hence the deferred write.
			reservations = append(reservations,
				stock.OrderMovement(vs.ID, department, -ci.Quantity, vs.StockQuantity-ci.Quantity, 0))
		}

		shipping := int64(0) // flat/no shipping fee for now
		now := time.Now()
		customerNameSnapshot, err := s.repo.CustomerName(ctx, tx, userID)
		if err != nil {
			return err
		}
		if customerNameSnapshot != nil {
			name := strings.TrimSpace(*customerNameSnapshot)
			customerNameSnapshot = nil
			if name != "" {
				customerNameSnapshot = &name
			}
		}
		order := &Order{
			UserID:          &userID,
			CustomerName:    customerNameSnapshot,
			OrderNumber:     newOrderNumber(),
			FulfillmentType: req.FulfillmentType,
			Status:          StatusPending,
			PaymentStatus:   PaymentUnpaid,
			Subtotal:        formatCents(subtotal),
			ShippingFee:     formatCents(shipping),
			Total:           formatCents(subtotal + shipping),
			Note:            req.Note,
			PlacedAt:        &now,
		}
		if req.FulfillmentType == FulfillmentPickup {
			until := now.Add(PickupHold)
			order.ReservedUntil = &until
		} else {
			applyShippingAddress(order, req.ShippingAddress)
		}

		id, err := s.repo.InsertOrder(ctx, tx, order)
		if err != nil {
			return err
		}
		orderID = id
		for i := range orderItems {
			orderItems[i].OrderID = id
			if err := s.repo.InsertOrderItem(ctx, tx, &orderItems[i]); err != nil {
				return err
			}
		}
		if err := recordReservations(ctx, tx, reservations, id); err != nil {
			return err
		}
		return s.repo.MarkCartConverted(ctx, tx, cart.ID)
	})
	if err != nil {
		return nil, err
	}
	return s.orderDetail(ctx, orderID)
}

// --- customer order actions ---

func (s *Service) ListMyOrders(ctx context.Context, userID int64, limit, offset int) ([]Order, int, error) {
	return s.repo.ListOrders(ctx, OrderFilter{UserID: &userID, Limit: limit, Offset: offset})
}

func (s *Service) GetMyOrder(ctx context.Context, userID, orderID int64) (*OrderDetail, error) {
	o, err := s.repo.GetOrderForUser(ctx, userID, orderID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListOrderItems(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	return &OrderDetail{Order: *o, Items: items}, nil
}

func (s *Service) CancelMyOrder(ctx context.Context, userID, orderID int64) (*OrderDetail, error) {
	o, err := s.repo.GetOrderForUser(ctx, userID, orderID)
	if err != nil {
		return nil, err
	}
	if !isCancellable(o.Status) {
		return nil, ErrNotCancellable
	}
	if err := s.releaseAndSetStatus(ctx, o.ID, StatusCancelled); err != nil {
		return nil, err
	}
	return s.orderDetail(ctx, o.ID)
}

// --- admin actions ---

// AdminCreateOrder builds a phone/walk-in order from explicit line items (rather
// than a cart), reserving stock and snapshotting prices like customer checkout.
func (s *Service) AdminCreateOrder(ctx context.Context, req AdminCreateOrderRequest, scope Scope) (*OrderDetail, error) {
	if req.FulfillmentType != FulfillmentDelivery && req.FulfillmentType != FulfillmentPickup {
		return nil, ErrInvalidFulfillment
	}
	if len(req.Items) == 0 {
		return nil, ErrNoItems
	}

	var orderID int64
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		var subtotal int64
		orderItems := make([]OrderItem, 0, len(req.Items))
		reservations := make([]stock.Movement, 0, len(req.Items))

		for _, li := range req.Items {
			if li.Quantity <= 0 {
				return ErrNoItems
			}
			vs, err := s.repo.LockVariant(ctx, tx, li.ProductVariantID)
			if err != nil {
				return err // ErrVariantMissing bubbles up
			}
			if !vs.IsActive {
				return ErrVariantInactive
			}
			if vs.StockQuantity < li.Quantity {
				return fmt.Errorf("%w: %s", ErrInsufficientStock, vs.ProductName)
			}
			if err := s.repo.AdjustStock(ctx, tx, vs.ID, -li.Quantity); err != nil {
				return err
			}
			priceCents, err := parseCents(vs.Price)
			if err != nil {
				return err
			}
			line := priceCents * int64(li.Quantity)
			subtotal += line
			variantID := vs.ID
			sku := vs.SKU
			department := catalog.DepartmentForTemplateKey(vs.TemplateKey)
			// A department admin may only sell their own catalog.
			if !scope.Covers(&department) {
				return ErrForbiddenDepartment
			}
			orderItems = append(orderItems, OrderItem{
				ProductVariantID: &variantID,
				Department:       &department,
				ProductName:      vs.ProductName,
				VariantLabel:     vs.Label,
				SKU:              &sku,
				UnitPrice:        vs.Price,
				Quantity:         li.Quantity,
				LineTotal:        formatCents(line),
			})
			reservations = append(reservations,
				stock.OrderMovement(vs.ID, department, -li.Quantity, vs.StockQuantity-li.Quantity, 0))
		}

		now := time.Now()
		order := &Order{
			UserID:          req.UserID,
			OrderNumber:     newOrderNumber(),
			FulfillmentType: req.FulfillmentType,
			Status:          StatusPending,
			PaymentStatus:   PaymentUnpaid,
			Subtotal:        formatCents(subtotal),
			ShippingFee:     formatCents(0),
			Total:           formatCents(subtotal),
			Note:            req.Note,
			PlacedAt:        &now,
		}
		if name := strings.TrimSpace(req.CustomerName); name != "" {
			order.CustomerName = &name
		}
		if req.FulfillmentType == FulfillmentPickup {
			until := now.Add(PickupHold)
			order.ReservedUntil = &until
		} else {
			applyShippingAddress(order, req.ShippingAddress)
		}

		id, err := s.repo.InsertOrder(ctx, tx, order)
		if err != nil {
			return err
		}
		orderID = id
		for i := range orderItems {
			orderItems[i].OrderID = id
			if err := s.repo.InsertOrderItem(ctx, tx, &orderItems[i]); err != nil {
				return err
			}
		}
		return recordReservations(ctx, tx, reservations, id)
	})
	if err != nil {
		return nil, err
	}
	return s.orderDetail(ctx, orderID)
}

func (s *Service) ListOrders(ctx context.Context, f OrderFilter) ([]Order, int, error) {
	return s.repo.ListOrders(ctx, f)
}

func (s *Service) GetOrder(ctx context.Context, orderID int64) (*OrderDetail, error) {
	return s.orderDetail(ctx, orderID)
}

func (s *Service) UpdateStatus(ctx context.Context, orderID int64, target string, scope Scope) (*OrderDetail, error) {
	o, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	// Status is a property of the whole order, so a department admin may only
	// move an order that contains nothing but their own lines.
	if !scope.All {
		items, err := s.repo.ListOrderItems(ctx, o.ID)
		if err != nil {
			return nil, err
		}
		if !scope.CoversAll(items) {
			return nil, ErrForbiddenDepartment
		}
	}
	if !canTransition(o.FulfillmentType, o.Status, target) {
		return nil, ErrInvalidTransition
	}
	if target == StatusCancelled {
		// releasing reserved stock as we cancel
		if err := s.releaseAndSetStatus(ctx, o.ID, StatusCancelled); err != nil {
			return nil, err
		}
	} else if err := s.repo.SetOrderStatus(ctx, o.ID, target); err != nil {
		return nil, err
	}
	return s.orderDetail(ctx, o.ID)
}

// ExpireStalePickups releases stock for pickup orders whose 24h hold has lapsed.
// Intended to be called periodically by a background sweeper.
func (s *Service) ExpireStalePickups(ctx context.Context) (int, error) {
	ids, err := s.repo.FindExpiredPickups(ctx, time.Now())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if err := s.releaseAndSetStatus(ctx, id, StatusExpired); err != nil {
			s.log.Error("expire pickup order", "order_id", id, "error", err)
			continue
		}
		n++
	}
	return n, nil
}

// --- helpers ---

// recordReservations writes the checkout reservations to the stock ledger once
// the order id exists to reference.
func recordReservations(ctx context.Context, tx *sqlx.Tx, movements []stock.Movement, orderID int64) error {
	for _, m := range movements {
		m.ReferenceID = &orderID
		if err := stock.RecordTx(ctx, tx, m); err != nil {
			return err
		}
	}
	return nil
}

// releaseAndSetStatus restores each line's stock and sets the terminal status,
// all in one transaction. Each return is mirrored into the stock ledger so the
// shelf history explains cancellations and lapsed pickup holds too.
func (s *Service) releaseAndSetStatus(ctx context.Context, orderID int64, status string) error {
	return s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		items, err := s.repo.ListOrderItemsTx(ctx, tx, orderID)
		if err != nil {
			return err
		}
		for _, it := range items {
			if it.ProductVariantID == nil {
				continue
			}
			department := ""
			if it.Department != nil {
				department = *it.Department
			}
			if err := s.repo.ReleaseStockTx(ctx, tx, *it.ProductVariantID, it.Quantity, department, orderID); err != nil {
				return err
			}
		}
		return s.repo.SetOrderStatusTx(ctx, tx, orderID, status)
	})
}

func (s *Service) orderDetail(ctx context.Context, orderID int64) (*OrderDetail, error) {
	o, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListOrderItems(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	return &OrderDetail{Order: *o, Items: items}, nil
}

func applyShippingAddress(o *Order, a *ShippingAddress) {
	if a == nil {
		return
	}
	recipient := strings.TrimSpace(a.RecipientName)
	phone := strings.TrimSpace(a.Phone)
	line1 := strings.TrimSpace(a.Line1)
	city := strings.TrimSpace(a.City)
	country := strings.TrimSpace(a.Country)
	o.ShipRecipientName = &recipient
	o.ShipPhone = &phone
	o.ShipLine1 = &line1
	o.ShipLine2 = a.Line2
	o.ShipCity = &city
	o.ShipRegion = a.Region
	o.ShipCountry = &country
	o.ShipPostalCode = a.PostalCode
	o.ShipLatitude = a.Latitude
	o.ShipLongitude = a.Longitude
	o.ShipLocationRef = a.LocationReference
}

func isCancellable(status string) bool {
	return status == StatusPending || status == StatusConfirmed
}

// canTransition encodes the fulfillment-specific state machine.
func canTransition(fulfillment, from, to string) bool {
	if to == StatusCancelled {
		return from == StatusPending || from == StatusConfirmed
	}
	if fulfillment == FulfillmentDelivery {
		switch from {
		case StatusPending:
			return to == StatusConfirmed
		case StatusConfirmed:
			return to == StatusShipped
		case StatusShipped:
			return to == StatusDelivered
		}
		return false
	}
	// pickup
	switch from {
	case StatusPending:
		return to == StatusPickedUp
	}
	return false
}

func newOrderNumber() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("ORD-%s-%s", time.Now().Format("20060102"), hex.EncodeToString(b))
}
