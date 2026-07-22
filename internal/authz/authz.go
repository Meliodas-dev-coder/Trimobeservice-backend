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

// BusinessModule and BusinessCapability describe the stable navigation keys
// used by department/position access. The base capability grants read/visibility
// and the same key with a `.manage` suffix grants mutations. Legacy coarse
// module permissions (for example `mobility`) remain unrestricted wildcards for
// that module until existing roles are migrated.
type BusinessModule struct {
	Key          string               `json:"key"`
	Label        string               `json:"label"`
	Capabilities []BusinessCapability `json:"capabilities"`
}

type BusinessCapability struct {
	Key   string `json:"key"`
	Label string `json:"label"`
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
	PermHR             = "hr"
	PermHRDashboard    = "hr_dashboard"
	PermHREmployees    = "hr_employees"
	PermHROrganization = "hr_organization"
	PermHRLifecycle    = "hr_lifecycle"
	PermHRLeave        = "hr_leave"
	PermHRAttendance   = "hr_attendance"
	PermHRPerformance  = "hr_performance"
	PermHRRecruitment  = "hr_recruitment"
	PermHRExpenses     = "hr_expenses"
	PermHRCompensation = "hr_compensation"
	PermHRDocuments    = "hr_documents"
	PermHRReports      = "hr_reports"
	PermHREmployee     = "hr_employee"
	// PermHRAudit is an effective-policy middleware key, not a legacy role
	// permission. It is surfaced only when the resolver finds audit.view with
	// all scope; the HR service repeats that invariant before returning history.
	PermHRAudit        = "hr_audit_all"
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
	{PermHR, "Human Resources", "Employee lifecycle, leave, attendance, performance, and recruitment.", "Human Resources"},
	{PermHRDashboard, "HR dashboard", "HR workforce totals and headcount metrics.", "Human Resources"},
	{PermHREmployees, "HR employee directory", "Employee profiles and emergency contacts.", "Human Resources"},
	{PermHROrganization, "HR organization", "Departments and positions.", "Human Resources"},
	{PermHRLifecycle, "HR lifecycle", "Onboarding, probation, transfers, promotions, and offboarding.", "Human Resources"},
	{PermHRLeave, "HR leave", "Leave policies, balances, requests, approvals, and calendar.", "Human Resources"},
	{PermHRAttendance, "HR attendance", "Shifts, attendance, timesheets, lateness, and overtime.", "Human Resources"},
	{PermHRPerformance, "HR performance", "Reviews, goals, feedback, and one-to-ones.", "Human Resources"},
	{PermHRRecruitment, "HR recruitment", "Vacancies, candidates, interviews, offers, and hiring.", "Human Resources"},
	{PermHRExpenses, "HR expenses", "Expense approvals and reimbursements.", "Human Resources"},
	{PermHRCompensation, "HR compensation", "Contracts, compensation, and benefits.", "Human Resources"},
	{PermHRDocuments, "HR confidential documents", "Upload, view, and download private employee documents.", "Human Resources"},
	{PermHRReports, "HR reports", "HR reports, CSV exports, and HR audit history.", "Human Resources"},
	{PermHREmployee, "HR employee self-service", "Reserved for the future employee self-service portal.", "Human Resources"},
	{PermOrders, "Orders", "Customer and walk-in orders, fulfillment, and payment state.", "Finance & people"},
	{PermPayments, "Payments", "Record manual payments and refunds.", "Finance & people"},
	{PermInvoices, "Invoices & billing", "Proforma bills, invoices, credit notes, and billing settings.", "Finance & people"},
	{PermCustomers, "Customers", "Read-only view of customer accounts.", "Finance & people"},
	{PermAuditLogs, "Activity log", "Who changed what in the dashboard, and when.", "Finance & people"},
	{PermUserManagement, "Users & roles", "Create admin employees and manage their access roles.", "Administration"},
}

// BusinessModules is the canonical submenu catalog. Keys are deliberately not
// route paths or translated labels, so either can change without rewriting
// stored access policies.
var BusinessModules = []BusinessModule{
	{PermDashboard, "Dashboard", []BusinessCapability{{"dashboard.overview", "Overview"}}},
	// Each catalog department runs its own back office: its overview, its slice
	// of the shared order book, and its shelf. `<dept>.orders` is deliberately
	// distinct from the cross-department `orders.orders` under Finance — the
	// former sees only that department's lines of an order.
	{PermTech, "Tech", []BusinessCapability{{"tech.overview", "Overview"}, {"tech.categories", "Categories"}, {"tech.brands", "Brands"}, {"tech.products", "Products"}, {"tech.orders", "Orders"}, {"tech.stock", "Stock"}}},
	{PermFashion, "Fashion", []BusinessCapability{{"fashion.overview", "Overview"}, {"fashion.categories", "Categories"}, {"fashion.brands", "Brands"}, {"fashion.products", "Products"}, {"fashion.orders", "Orders"}, {"fashion.stock", "Stock"}}},
	{PermCoffee, "Coffee", []BusinessCapability{{"coffee.overview", "Overview"}, {"coffee.categories", "Categories"}, {"coffee.brands", "Brands"}, {"coffee.products", "Products"}, {"coffee.orders", "Orders"}, {"coffee.stock", "Stock"}}},
	{PermMobility, "Mobility", []BusinessCapability{{"mobility.overview", "Overview"}, {"mobility.categories", "Car categories"}, {"mobility.cars", "Fleet"}, {"mobility.drivers", "Drivers"}, {"mobility.bookings", "Bookings"}}},
	{PermEvents, "Events", []BusinessCapability{{"events.overview", "Overview"}, {"events.categories", "Service categories"}, {"events.services", "Services"}, {"events.artists", "Artists"}, {"events.requests", "Requests"}}},
	{PermHealthcare, "Healthcare", []BusinessCapability{{"healthcare.overview", "Overview"}, {"healthcare.practitioners", "Practitioners"}, {"healthcare.categories", "Service categories"}, {"healthcare.services", "Services"}, {"healthcare.requests", "Requests"}, {"healthcare.settings", "Settings"}}},
	{PermOrders, "Orders", []BusinessCapability{{"orders.orders", "Orders"}}},
	{PermPayments, "Payments", []BusinessCapability{{"payments.payments", "Payments"}}},
	{PermInvoices, "Invoices", []BusinessCapability{{"invoices.documents", "Invoices"}, {"invoices.settings", "Billing settings"}}},
	{PermCustomers, "Customers", []BusinessCapability{{"customers.directory", "Customer directory"}}},
	{PermAuditLogs, "Activity log", []BusinessCapability{{"audit_logs.history", "Activity history"}}},
}

var catalogIndex = func() map[string]struct{} {
	m := make(map[string]struct{}, len(Catalog)+64)
	for _, p := range Catalog {
		m[p.Key] = struct{}{}
	}
	for _, module := range BusinessModules {
		for _, capability := range module.Capabilities {
			m[capability.Key] = struct{}{}
			m[capability.Key+".manage"] = struct{}{}
		}
	}
	return m
}()

var capabilityModules = func() map[string]string {
	m := make(map[string]string, 64)
	for _, module := range BusinessModules {
		for _, capability := range module.Capabilities {
			m[capability.Key] = module.Key
		}
	}
	return m
}()

// IsValidKey reports whether key is a known permission.
func IsValidKey(key string) bool {
	_, ok := catalogIndex[key]
	return ok
}

// IsCapabilityKey reports whether key is a base submenu capability. Mutation
// suffixes are intentionally excluded because access configuration stores the
// level separately.
func IsCapabilityKey(key string) bool {
	_, ok := capabilityModules[key]
	return ok
}

// CapabilityModule returns the top-level module containing a capability.
func CapabilityModule(key string) (string, bool) {
	module, ok := capabilityModules[key]
	return module, ok
}

// RequiredCapabilityKeys returns the keys accepted by a route. A legacy module
// grant is always accepted as a full-access compatibility wildcard.
func RequiredCapabilityKeys(key string, manage bool) []string {
	module, ok := CapabilityModule(key)
	if !ok {
		return []string{key}
	}
	if manage {
		return []string{module, key + ".manage"}
	}
	return []string{module, key, key + ".manage"}
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
