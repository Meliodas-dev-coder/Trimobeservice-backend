// Command api is the entry point for the Trimo backend HTTP server.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"

	"github.com/trimo/backend/internal/adminusers"
	"github.com/trimo/backend/internal/audit"
	"github.com/trimo/backend/internal/auth"
	"github.com/trimo/backend/internal/authz"
	"github.com/trimo/backend/internal/bookings"
	"github.com/trimo/backend/internal/catalog"
	"github.com/trimo/backend/internal/config"
	"github.com/trimo/backend/internal/customers"
	"github.com/trimo/backend/internal/dashboard"
	"github.com/trimo/backend/internal/database"
	"github.com/trimo/backend/internal/events"
	"github.com/trimo/backend/internal/healthcare"
	"github.com/trimo/backend/internal/hr"
	"github.com/trimo/backend/internal/invoicing"
	"github.com/trimo/backend/internal/mobility"
	"github.com/trimo/backend/internal/orders"
	"github.com/trimo/backend/internal/payments"
	"github.com/trimo/backend/internal/ratelimit"
	"github.com/trimo/backend/internal/realtime"
	"github.com/trimo/backend/internal/server"
	"github.com/trimo/backend/internal/stock"
	"github.com/trimo/backend/internal/storage"
	"github.com/trimo/backend/internal/uploads"
	"github.com/trimo/backend/migrations"
)

func main() {

	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Load .env in development; ignored if the file is absent.
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := newLogger(cfg)
	slog.SetDefault(log)

	// Apply schema migrations before opening the main pool.
	if err := database.Migrate(cfg.DB.DSN(), migrations.FS); err != nil {
		return err
	}
	log.Info("migrations applied")

	db, err := database.Connect(cfg.DB.DSN(), cfg.DB.MaxOpenConns, cfg.DB.MaxIdleConns, cfg.DB.ConnMaxLifetime)
	if err != nil {
		return err
	}
	defer db.Close()
	log.Info("database connected", "name", cfg.DB.Name)

	// Optional object storage for image uploads (disabled when STORAGE_BUCKET is unset).
	// `store` stays nil when disabled; services treat a nil ImageDeleter as "skip".
	var store storage.Storage
	var uploadsHandler *uploads.Handler
	if cfg.Storage.Bucket != "" {
		gcsStore, serr := storage.NewGCS(context.Background(), cfg.Storage.Bucket, cfg.Storage.CredentialsFile, cfg.Storage.PublicBaseURL)
		if serr != nil {
			log.Warn("image uploads disabled: storage init failed", "error", serr)
		} else {
			store = gcsStore
			uploadsHandler = uploads.NewHandler(store, cfg.Storage.MaxUploadBytes)
			log.Info("image uploads enabled", "bucket", cfg.Storage.Bucket)
		}
	} else {
		log.Info("image uploads disabled: STORAGE_BUCKET not set")
	}

	// --- dependency wiring ---
	tokens := auth.NewTokenManager(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	authMW := auth.NewMiddleware(tokens)
	authRepo := auth.NewRepository(db)
	authMW.SetAccountStatusLoader(authRepo)
	authHandler := auth.NewHandler(auth.NewService(authRepo, tokens))

	catalogHandler := catalog.NewHandler(catalog.NewService(catalog.NewRepository(db), store))
	mobilityHandler := mobility.NewHandler(mobility.NewService(mobility.NewRepository(db), store))
	ordersSvc := orders.NewService(orders.NewRepository(db), log)
	ordersHandler := orders.NewHandler(ordersSvc)
	bookingsHandler := bookings.NewHandler(bookings.NewService(bookings.NewRepository(db)))
	eventsHandler := events.NewHandler(events.NewService(events.NewRepository(db), store))
	healthcareHandler := healthcare.NewHandler(healthcare.NewService(healthcare.NewRepository(db), store))
	hrSvc := hr.NewService(hr.NewRepository(db))
	hrHandler := hr.NewHandler(hrSvc)
	paymentsHandler := payments.NewHandler(payments.NewService(payments.NewRepository(db)))
	invoicingHandler := invoicing.NewHandler(invoicing.NewService(invoicing.NewRepository(db)))
	customersHandler := customers.NewHandler(customers.NewService(customers.NewRepository(db)))
	dashboardHandler := dashboard.NewHandler(dashboard.NewService(dashboard.NewRepository(db)))
	stockHandler := stock.NewHandler(stock.NewService(stock.NewRepository(db)))
	// adminUsersSvc manages admin employees + roles AND resolves a user's
	// effective permissions for the authz enforcement middleware below.
	adminUsersSvc := adminusers.NewService(adminusers.NewRepository(db))
	adminUsersHandler := adminusers.NewHandler(adminUsersSvc)

	// Audit trail: one service feeds both the read endpoint and the permission
	// wrappers below that record successful admin writes.
	auditSvc := audit.NewService(audit.NewRepository(db), log)
	auditHandler := audit.NewHandler(auditSvc)

	// Screen-level access control. Beyond RequireAdmin, each admin module also
	// requires the section permission(s) for the area it serves; super-admins
	// bypass. permGate is the read-only variant; auditedPerm additionally records
	// the write. adminUsersSvc is the authz.PermissionLoader (one query/request).
	permGate := func(keys ...string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return authMW.RequireAdmin(authz.RequirePermission(adminUsersSvc, keys...)(next))
		}
	}
	auditedPerm := func(keys ...string) func(http.Handler) http.Handler {
		record := audit.NewMiddleware(auditSvc).Record
		return func(next http.Handler) http.Handler {
			return authMW.RequireAdmin(authz.RequirePermission(adminUsersSvc, keys...)(record(next)))
		}
	}
	auditedCapability := func(key string, manage bool) func(http.Handler) http.Handler {
		record := audit.NewMiddleware(auditSvc).Record
		return func(next http.Handler) http.Handler {
			return authMW.RequireAdmin(authz.RequireCapability(adminUsersSvc, key, manage)(record(next)))
		}
	}
	auditedSuper := func(next http.Handler) http.Handler {
		return authMW.RequireAdmin(authz.RequireSuperAdmin(adminUsersSvc)(audit.NewMiddleware(auditSvc).Record(next)))
	}
	// auditedAll requires a grant from every group (each group itself is OR).
	// It is used for HR reports: `hr` bypasses both groups, while a restricted
	// user needs both hr_reports and the report's sensitive domain capability.
	auditedAll := func(groups ...[]string) func(http.Handler) http.Handler {
		record := audit.NewMiddleware(auditSvc).Record
		return func(next http.Handler) http.Handler {
			var gated http.Handler = record(next)
			for i := len(groups) - 1; i >= 0; i-- {
				gated = authz.RequirePermission(adminUsersSvc, groups[i]...)(gated)
			}
			return authMW.RequireAdmin(gated)
		}
	}
	realtimePermissions := []string{authz.PermDashboard, authz.PermTech, authz.PermFashion, authz.PermCoffee, authz.PermMobility, authz.PermEvents, authz.PermHealthcare, authz.PermOrders, authz.PermPayments}
	for _, module := range authz.BusinessModules {
		for _, capability := range module.Capabilities {
			realtimePermissions = append(realtimePermissions, capability.Key, capability.Key+".manage")
		}
	}

	// Realtime hub: streams newly created orders/bookings/requests to connected
	// admins over SSE. Handlers publish through it (nil-safe if left unset).
	hub := realtime.NewHub()
	ordersHandler.SetPublisher(hub)
	bookingsHandler.SetPublisher(hub)
	eventsHandler.SetPublisher(hub)
	healthcareHandler.SetPublisher(hub)
	paymentsHandler.SetPublisher(hub)

	// Strict per-IP limiter for the credential endpoints (login/register/refresh).
	authThrottle := ratelimit.New(cfg.HTTP.AuthRateLimitRPS, cfg.HTTP.AuthRateLimitBurst).Middleware

	srv := server.New(cfg, log, db)
	srv.MountAPI(func(r chi.Router) {
		auth.RegisterRoutes(r, authHandler, authMW, authThrottle)
		// Mutating admin modules use audited permission wrappers. Read-only modules
		// still enforce their matching permission server-side.
		// Catalog serves tech/fashion/coffee over shared endpoints. The route guards
		// admit any user with catalog access; the handler re-checks each row's
		// department (a tech grant never reaches fashion/coffee data).
		catalogGuardKeys := catalog.BuildGuardKeys()
		catalog.RegisterRoutes(r, catalogHandler, catalog.AdminGuards{
			TemplatesRead:    auditedPerm(catalogGuardKeys.Templates...),
			CategoriesRead:   auditedPerm(catalogGuardKeys.CategoriesRead...),
			CategoriesManage: auditedPerm(catalogGuardKeys.CategoriesManage...),
			BrandsRead:       auditedPerm(catalogGuardKeys.BrandsRead...),
			BrandsManage:     auditedPerm(catalogGuardKeys.BrandsManage...),
			ProductsRead:     auditedPerm(catalogGuardKeys.ProductsRead...),
			ProductsManage:   auditedPerm(catalogGuardKeys.ProductsManage...),
		})
		mobility.RegisterRoutes(r, mobilityHandler, mobility.AdminGuards{
			CategoriesRead: auditedCapability("mobility.categories", false), CategoriesManage: auditedCapability("mobility.categories", true),
			CarsRead: auditedCapability("mobility.cars", false), CarsManage: auditedCapability("mobility.cars", true),
			DriversRead: auditedCapability("mobility.drivers", false), DriversManage: auditedCapability("mobility.drivers", true),
		})
		// Orders are entered either from Finance (the whole order book) or from a
		// department back office (its own lines of the same orders). The guard
		// admits both; the handler then trims what a department admin may see.
		orders.RegisterRoutes(r, ordersHandler, authMW.RequireAuth, orders.AdminGuards{
			Read: auditedPerm(orders.GuardKeys(false)...), Manage: auditedPerm(orders.GuardKeys(true)...),
		})
		stock.RegisterRoutes(r, stockHandler, stock.AdminGuards{
			Read:   auditedPerm(catalog.DepartmentGuardKeys(catalog.ResourceStock, false)...),
			Manage: auditedPerm(catalog.DepartmentGuardKeys(catalog.ResourceStock, true)...),
		})
		bookings.RegisterRoutes(r, bookingsHandler, authMW.RequireAuth, bookings.AdminGuards{Read: auditedCapability("mobility.bookings", false), Manage: auditedCapability("mobility.bookings", true)})
		events.RegisterRoutes(r, eventsHandler, authMW.RequireAuth, events.AdminGuards{
			CategoriesRead: auditedCapability("events.categories", false), CategoriesManage: auditedCapability("events.categories", true),
			ServicesRead: auditedCapability("events.services", false), ServicesManage: auditedCapability("events.services", true),
			ArtistsRead: auditedCapability("events.artists", false), ArtistsManage: auditedCapability("events.artists", true),
			RequestsRead: auditedCapability("events.requests", false), RequestsManage: auditedCapability("events.requests", true),
		})
		healthcare.RegisterRoutes(r, healthcareHandler, authMW.RequireAuth, healthcare.AdminGuards{
			PractitionersRead: auditedCapability("healthcare.practitioners", false), PractitionersManage: auditedCapability("healthcare.practitioners", true),
			CategoriesRead: auditedCapability("healthcare.categories", false), CategoriesManage: auditedCapability("healthcare.categories", true),
			ServicesRead: auditedCapability("healthcare.services", false), ServicesManage: auditedCapability("healthcare.services", true),
			RequestsRead: auditedCapability("healthcare.requests", false), RequestsManage: auditedCapability("healthcare.requests", true),
			SettingsRead: auditedCapability("healthcare.settings", false), SettingsManage: auditedCapability("healthcare.settings", true),
		})
		hr.RegisterRoutes(r, hrHandler, hr.Guards{
			Any:                auditedPerm(authz.PermHR, authz.PermHREmployee, authz.PermHRDashboard, authz.PermHREmployees, authz.PermHROrganization, authz.PermHRLifecycle, authz.PermHRLeave, authz.PermHRAttendance, authz.PermHRPerformance, authz.PermHRRecruitment, authz.PermHRExpenses, authz.PermHRCompensation, authz.PermHRDocuments, authz.PermHRReports),
			Lookups:            auditedPerm(authz.PermHR, authz.PermHREmployee, authz.PermHREmployees, authz.PermHROrganization, authz.PermHRLifecycle, authz.PermHRLeave, authz.PermHRAttendance, authz.PermHRPerformance, authz.PermHRRecruitment, authz.PermHRExpenses, authz.PermHRCompensation, authz.PermHRDocuments),
			Dashboard:          auditedPerm(authz.PermHR, authz.PermHREmployee, authz.PermHRDashboard),
			Employees:          auditedPerm(authz.PermHR, authz.PermHREmployee, authz.PermHREmployees),
			Organization:       auditedPerm(authz.PermHR, authz.PermHROrganization),
			Lifecycle:          auditedPerm(authz.PermHR, authz.PermHRLifecycle),
			Leave:              auditedPerm(authz.PermHR, authz.PermHREmployee, authz.PermHRLeave),
			Attendance:         auditedPerm(authz.PermHR, authz.PermHREmployee, authz.PermHRAttendance),
			Performance:        auditedPerm(authz.PermHR, authz.PermHREmployee, authz.PermHRPerformance),
			Recruitment:        auditedPerm(authz.PermHR, authz.PermHRRecruitment),
			Expenses:           auditedPerm(authz.PermHR, authz.PermHREmployee, authz.PermHRExpenses),
			Compensation:       auditedPerm(authz.PermHR, authz.PermHREmployee, authz.PermHRCompensation),
			Documents:          auditedPerm(authz.PermHR, authz.PermHREmployee, authz.PermHRDocuments),
			Audit:              auditedPerm(authz.PermHR, authz.PermHRAudit),
			ReportWorkforce:    auditedAll([]string{authz.PermHR, authz.PermHRReports}, []string{authz.PermHR, authz.PermHREmployees}),
			ReportLeave:        auditedAll([]string{authz.PermHR, authz.PermHRReports}, []string{authz.PermHR, authz.PermHRLeave}),
			ReportAttendance:   auditedAll([]string{authz.PermHR, authz.PermHRReports}, []string{authz.PermHR, authz.PermHRAttendance}),
			ReportPerformance:  auditedAll([]string{authz.PermHR, authz.PermHRReports}, []string{authz.PermHR, authz.PermHRPerformance}),
			ReportRecruitment:  auditedAll([]string{authz.PermHR, authz.PermHRReports}, []string{authz.PermHR, authz.PermHRRecruitment}),
			ReportExpenses:     auditedAll([]string{authz.PermHR, authz.PermHRReports}, []string{authz.PermHR, authz.PermHRExpenses}),
			ReportCompensation: auditedAll([]string{authz.PermHR, authz.PermHRReports}, []string{authz.PermHR, authz.PermHRCompensation}),
		})
		payments.RegisterRoutes(r, paymentsHandler, payments.AdminGuards{
			Read: auditedCapability("payments.payments", false), Manage: auditedCapability("payments.payments", true),
		})
		invoicing.RegisterRoutes(r, invoicingHandler, invoicing.AdminGuards{
			DocumentsRead: auditedCapability("invoices.documents", false), DocumentsManage: auditedCapability("invoices.documents", true),
			SettingsRead: auditedCapability("invoices.settings", false), SettingsManage: auditedCapability("invoices.settings", true),
		})
		customers.RegisterRoutes(r, customersHandler, permGate(authz.PermCustomers, "customers.directory", "customers.directory.manage"))
		dashboard.RegisterRoutes(r, dashboardHandler,
			permGate(authz.PermDashboard, "dashboard.overview", "dashboard.overview.manage"),
			permGate(catalog.DepartmentGuardKeys(catalog.ResourceOverview, false)...))
		audit.RegisterRoutes(r, auditHandler, permGate(authz.PermAuditLogs, "audit_logs.history", "audit_logs.history.manage"))
		adminusers.RegisterRoutes(r, adminUsersHandler, auditedSuper)
		// The SSE connection requires at least one operational capability and the
		// stream itself filters each event by its domain capability.
		realtime.RegisterRoutes(r, hub, permGate(realtimePermissions...))
		if uploadsHandler != nil {
			uploads.RegisterRoutes(r, uploadsHandler, auditedPerm(authz.PermTech, authz.PermFashion, authz.PermCoffee, authz.PermMobility, authz.PermEvents, authz.PermHealthcare,
				"tech.products.manage", "fashion.products.manage", "coffee.products.manage", "mobility.cars.manage", "events.services.manage", "events.artists.manage", "healthcare.services.manage"))
		}
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// On shutdown signal, end open SSE streams promptly so graceful shutdown
	// doesn't wait on long-lived connections.
	context.AfterFunc(ctx, hub.Close)

	// Background sweeper: release stock from pickup orders whose 24h hold lapsed.
	go runPickupSweeper(ctx, log, ordersSvc)
	// Background sweeper: apply scheduled HR lifecycle events once their date is due.
	go runLifecycleSweeper(ctx, log, hrSvc)

	return srv.Run(ctx)
}

// runLifecycleSweeper periodically applies scheduled HR lifecycle events whose
// effective date has arrived. It runs once shortly after boot and hourly after,
// stopping when ctx is cancelled.
func runLifecycleSweeper(ctx context.Context, log *slog.Logger, svc *hr.Service) {
	sweep := func() {
		n, err := svc.SweepDueLifecycleEvents(ctx)
		if err != nil {
			log.Error("lifecycle sweep failed", "error", err)
			return
		}
		if n > 0 {
			log.Info("applied scheduled lifecycle events", "count", n)
		}
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
		sweep()
	}
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}

// runPickupSweeper periodically expires stale pickup reservations, returning
// their held stock. It stops when ctx is cancelled.
func runPickupSweeper(ctx context.Context, log *slog.Logger, svc *orders.Service) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := svc.ExpireStalePickups(ctx)
			if err != nil {
				log.Error("pickup sweep failed", "error", err)
				continue
			}
			if n > 0 {
				log.Info("expired stale pickup orders", "count", n)
			}
		}
	}
}

func newLogger(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.Env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}
