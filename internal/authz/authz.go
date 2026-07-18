// Package authz defines the admin permission catalog and the middleware that
// enforces per-screen access. Permissions are coarse "business sections" (one
// per admin area). A role bundles a set of these keys; an employee is assigned
// one role. Super-admins bypass every check.
//
// This package is the single source of truth for the permission keys: the role
// editor validates against Catalog, and the frontend fetches it via
// GET /admin/permissions so the two never drift.
package authz

// Permission describes one grantable admin section.
type Permission struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Group       string `json:"group"`
}

// Permission keys. Keep in sync with the frontend route/nav `permission` tags.
const (
	PermDashboard      = "dashboard"
	PermTech           = "tech"
	PermFashion        = "fashion"
	PermCoffee         = "coffee"
	PermMobility       = "mobility"
	PermEvents         = "events"
	PermHealthcare     = "healthcare"
	PermOrders         = "orders"
	PermPayments       = "payments"
	PermInvoices       = "invoices"
	PermCustomers      = "customers"
	PermAuditLogs      = "audit_logs"
	PermUserManagement = "user_management"
)

// Catalog is the ordered, canonical list of grantable sections. Order drives
// how the role editor renders and how the frontend picks a landing screen.
var Catalog = []Permission{
	{PermDashboard, "Dashboard", "Cross-domain KPIs and the operations home screen.", "Overview"},
	{PermTech, "Tech catalog", "Tech categories, brands, and products.", "Catalog"},
	{PermFashion, "Fashion catalog", "Fashion categories, brands, and products.", "Catalog"},
	{PermCoffee, "Coffee catalog", "Kafe Misiona coffee products.", "Catalog"},
	{PermMobility, "Mobility", "Car categories, cars, drivers, and bookings.", "Operations"},
	{PermEvents, "Events", "Event service categories, services, artists, and requests.", "Operations"},
	{PermHealthcare, "Healthcare", "Practitioners, care services, requests, and emergency contact.", "Operations"},
	{PermOrders, "Orders", "Customer and walk-in orders, fulfillment, and payment state.", "Finance & people"},
	{PermPayments, "Payments", "Record manual payments and refunds.", "Finance & people"},
	{PermInvoices, "Invoices & billing", "Proforma bills, invoices, credit notes, and billing settings.", "Finance & people"},
	{PermCustomers, "Customers", "Read-only view of customer accounts.", "Finance & people"},
	{PermAuditLogs, "Activity log", "Who changed what in the dashboard, and when.", "Finance & people"},
	{PermUserManagement, "Users & roles", "Create admin employees and manage their access roles.", "Administration"},
}

var catalogIndex = func() map[string]struct{} {
	m := make(map[string]struct{}, len(Catalog))
	for _, p := range Catalog {
		m[p.Key] = struct{}{}
	}
	return m
}()

// IsValidKey reports whether key is a known permission.
func IsValidKey(key string) bool {
	_, ok := catalogIndex[key]
	return ok
}

// PermissionSet is a lookup-friendly set of permission keys.
type PermissionSet map[string]struct{}

// NewPermissionSet builds a set from a slice, silently dropping unknown keys.
func NewPermissionSet(keys []string) PermissionSet {
	set := make(PermissionSet, len(keys))
	for _, k := range keys {
		if IsValidKey(k) {
			set[k] = struct{}{}
		}
	}
	return set
}

// Has reports membership of a single key.
func (s PermissionSet) Has(key string) bool {
	_, ok := s[key]
	return ok
}

// HasAny reports whether the set contains at least one of keys. An empty keys
// argument means "no specific permission required" and returns true.
func (s PermissionSet) HasAny(keys ...string) bool {
	if len(keys) == 0 {
		return true
	}
	for _, k := range keys {
		if s.Has(k) {
			return true
		}
	}
	return false
}
