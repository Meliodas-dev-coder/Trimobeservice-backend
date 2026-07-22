package orders

import (
	"context"
	"net/http"
	"strings"

	"github.com/trimo/backend/internal/authz"
	"github.com/trimo/backend/internal/catalog"
	"github.com/trimo/backend/internal/httpx"
)

// Orders are read from two angles. The Orders screen under Finance sees every
// order whole (it reconciles payments and invoices, which are order-level). A
// department back office — Coffee, Tech, Fashion — sees only its own lines,
// through the `coffee.orders` style capability.
//
// The customer experience is untouched by this split: one cart, one checkout,
// one payment, one invoice. The split exists purely so a coffee manager is
// never handed someone else's product lines.

// Scope is a caller's authority over orders: either global (the Finance-side
// Orders screen) or a set of catalog departments.
type Scope struct {
	All         bool
	Departments []string
}

// Covers reports whether a line's department falls inside the scope. A line
// with no department snapshot (its variant was deleted before migration 000035)
// is only visible globally, since there is no way to attribute it.
func (s Scope) Covers(department *string) bool {
	if s.All {
		return true
	}
	if department == nil || *department == "" {
		return false
	}
	for _, allowed := range s.Departments {
		if allowed == *department {
			return true
		}
	}
	return false
}

// CoversAll reports whether every line of an order is inside the scope. Order
// state (status, cancellation) is shared by all its lines, so a department
// manager may only move an order that is entirely theirs.
func (s Scope) CoversAll(items []OrderItem) bool {
	if s.All {
		return true
	}
	if len(items) == 0 {
		return false
	}
	for _, it := range items {
		if !s.Covers(it.Department) {
			return false
		}
	}
	return true
}

// GuardKeys are the coarse route-level keys for the admin order routes: the
// global orders capability plus every department's. The handler then makes the
// precise decision — a `coffee.orders` grant never reads a tech line.
func GuardKeys(manage bool) []string {
	keys := authz.RequiredCapabilityKeys("orders.orders", manage)
	return append(keys, catalog.DepartmentGuardKeys(catalog.ResourceOrders, manage)...)
}

// hasGlobalAccess reports whether the caller holds the cross-department Orders
// capability (or is a super-admin).
func hasGlobalAccess(ctx context.Context, manage bool) bool {
	current, ok := authz.FromContext(ctx)
	if !ok {
		return false
	}
	if current.IsSuper {
		return true
	}
	return current.Permissions.HasAny(authz.RequiredCapabilityKeys("orders.orders", manage)...)
}

// scopeFrom resolves the caller's authority for a read or a write.
func scopeFrom(ctx context.Context, manage bool) Scope {
	if hasGlobalAccess(ctx, manage) {
		return Scope{All: true}
	}
	return Scope{Departments: catalog.AllowedDepartments(ctx, catalog.ResourceOrders, manage)}
}

// departmentParam validates the requested department filter. An unknown value
// is rejected rather than ignored, so a typo can never widen the request to the
// whole order book.
func departmentParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	department := strings.TrimSpace(r.URL.Query().Get("department"))
	if department == "" {
		return "", true
	}
	if !catalog.IsValidDepartment(department) {
		httpx.ValidationError(w, map[string]string{"department": "unknown department"})
		return "", false
	}
	return department, true
}

// authorizeList decides whether the caller may run this list query. A
// department-scoped admin must name their department; only global access may
// list the whole order book.
func authorizeList(w http.ResponseWriter, r *http.Request, department string) bool {
	if department != "" {
		if catalog.CanDepartment(r.Context(), department, catalog.ResourceOrders, false) {
			return true
		}
		httpx.Error(w, http.StatusForbidden, "you do not have access to this catalog department")
		return false
	}
	if hasGlobalAccess(r.Context(), false) {
		return true
	}
	httpx.Error(w, http.StatusForbidden, "select a department to list its orders")
	return false
}

// applyScope trims an order detail to the lines the caller may see and states
// their share of it. The order total is left intact: it is what the customer
// actually pays, and `departments` already says the order reaches wider.
func applyScope(detail *OrderDetail, scope Scope) *OrderDetail {
	if scope.All {
		return detail
	}
	visible := make([]OrderItem, 0, len(detail.Items))
	var subtotal int64
	quantity := 0
	for _, it := range detail.Items {
		if !scope.Covers(it.Department) {
			continue
		}
		cents, _ := parseCents(it.LineTotal)
		subtotal += cents
		quantity += it.Quantity
		visible = append(visible, it)
	}
	if len(visible) == 0 {
		return nil
	}
	share := formatCents(subtotal)
	detail.Items = visible
	detail.DepartmentSubtotal = &share
	detail.DepartmentQuantity = &quantity
	return detail
}
