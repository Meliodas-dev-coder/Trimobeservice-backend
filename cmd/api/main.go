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
	"github.com/trimo/backend/internal/invoicing"
	"github.com/trimo/backend/internal/mobility"
	"github.com/trimo/backend/internal/orders"
	"github.com/trimo/backend/internal/payments"
	"github.com/trimo/backend/internal/ratelimit"
	"github.com/trimo/backend/internal/realtime"
	"github.com/trimo/backend/internal/server"
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
	authHandler := auth.NewHandler(auth.NewService(auth.NewRepository(db), tokens))

	catalogHandler := catalog.NewHandler(catalog.NewService(catalog.NewRepository(db), store))
	mobilityHandler := mobility.NewHandler(mobility.NewService(mobility.NewRepository(db), store))
	ordersSvc := orders.NewService(orders.NewRepository(db), log)
	ordersHandler := orders.NewHandler(ordersSvc)
	bookingsHandler := bookings.NewHandler(bookings.NewService(bookings.NewRepository(db)))
	eventsHandler := events.NewHandler(events.NewService(events.NewRepository(db), store))
	healthcareHandler := healthcare.NewHandler(healthcare.NewService(healthcare.NewRepository(db), store))
	paymentsHandler := payments.NewHandler(payments.NewService(payments.NewRepository(db)))
	invoicingHandler := invoicing.NewHandler(invoicing.NewService(invoicing.NewRepository(db)))
	customersHandler := customers.NewHandler(customers.NewService(customers.NewRepository(db)))
	dashboardHandler := dashboard.NewHandler(dashboard.NewService(dashboard.NewRepository(db)))
	// adminUsersSvc manages admin employees + roles AND resolves a user's
	// effective permissions for the authz enforcement middleware below.
	adminUsersSvc := adminusers.NewService(adminusers.NewRepository(db))
	adminUsersHandler := adminusers.NewHandler(adminUsersSvc)

	// Audit trail: one service feeds both the read endpoint and the middleware
	// that records every admin write. `auditedAdmin` = RequireAdmin + recording,
	// so wrapping it around a route group logs that group's writes automatically.
	auditSvc := audit.NewService(audit.NewRepository(db), log)
	auditHandler := audit.NewHandler(auditSvc)
	auditedAdmin := func(next http.Handler) http.Handler {
		return authMW.RequireAdmin(audit.NewMiddleware(auditSvc).Record(next))
	}

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
		// Mutating admin modules are wrapped with `auditedAdmin` so their writes
		// are recorded. Read-only modules (customers, dashboard, realtime SSE)
		// and the audit reader itself stay on plain RequireAdmin.
		catalog.RegisterRoutes(r, catalogHandler, auditedPerm(authz.PermTech, authz.PermFashion, authz.PermCoffee))
		mobility.RegisterRoutes(r, mobilityHandler, auditedPerm(authz.PermMobility))
		orders.RegisterRoutes(r, ordersHandler, authMW.RequireAuth, auditedPerm(authz.PermOrders))
		bookings.RegisterRoutes(r, bookingsHandler, authMW.RequireAuth, auditedPerm(authz.PermMobility))
		events.RegisterRoutes(r, eventsHandler, authMW.RequireAuth, auditedPerm(authz.PermEvents))
		healthcare.RegisterRoutes(r, healthcareHandler, authMW.RequireAuth, auditedPerm(authz.PermHealthcare))
		payments.RegisterRoutes(r, paymentsHandler, auditedPerm(authz.PermPayments))
		invoicing.RegisterRoutes(r, invoicingHandler, auditedPerm(authz.PermInvoices))
		customers.RegisterRoutes(r, customersHandler, permGate(authz.PermCustomers))
		dashboard.RegisterRoutes(r, dashboardHandler, permGate(authz.PermDashboard))
		audit.RegisterRoutes(r, auditHandler, permGate(authz.PermAuditLogs))
		adminusers.RegisterRoutes(r, adminUsersHandler, auditedPerm(authz.PermUserManagement))
		// The realtime SSE stream and shared image uploads stay any-admin (v1).
		realtime.RegisterRoutes(r, hub, authMW.RequireAdmin)
		if uploadsHandler != nil {
			uploads.RegisterRoutes(r, uploadsHandler, auditedAdmin)
		}
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// On shutdown signal, end open SSE streams promptly so graceful shutdown
	// doesn't wait on long-lived connections.
	context.AfterFunc(ctx, hub.Close)

	// Background sweeper: release stock from pickup orders whose 24h hold lapsed.
	go runPickupSweeper(ctx, log, ordersSvc)

	return srv.Run(ctx)
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
