// Command api is the entry point for the Trimo backend HTTP server.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"

	"github.com/trimo/backend/internal/auth"
	"github.com/trimo/backend/internal/bookings"
	"github.com/trimo/backend/internal/catalog"
	"github.com/trimo/backend/internal/config"
	"github.com/trimo/backend/internal/customers"
	"github.com/trimo/backend/internal/dashboard"
	"github.com/trimo/backend/internal/database"
	"github.com/trimo/backend/internal/events"
	"github.com/trimo/backend/internal/healthcare"
	"github.com/trimo/backend/internal/mobility"
	"github.com/trimo/backend/internal/orders"
	"github.com/trimo/backend/internal/payments"
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
	customersHandler := customers.NewHandler(customers.NewService(customers.NewRepository(db)))
	dashboardHandler := dashboard.NewHandler(dashboard.NewService(dashboard.NewRepository(db)))

	srv := server.New(cfg, log, db)
	srv.MountAPI(func(r chi.Router) {
		auth.RegisterRoutes(r, authHandler, authMW)
		catalog.RegisterRoutes(r, catalogHandler, authMW.RequireAdmin)
		mobility.RegisterRoutes(r, mobilityHandler, authMW.RequireAdmin)
		orders.RegisterRoutes(r, ordersHandler, authMW.RequireAuth, authMW.RequireAdmin)
		bookings.RegisterRoutes(r, bookingsHandler, authMW.RequireAuth, authMW.RequireAdmin)
		events.RegisterRoutes(r, eventsHandler, authMW.RequireAuth, authMW.RequireAdmin)
		healthcare.RegisterRoutes(r, healthcareHandler, authMW.RequireAuth, authMW.RequireAdmin)
		payments.RegisterRoutes(r, paymentsHandler, authMW.RequireAdmin)
		customers.RegisterRoutes(r, customersHandler, authMW.RequireAdmin)
		dashboard.RegisterRoutes(r, dashboardHandler, authMW.RequireAdmin)
		if uploadsHandler != nil {
			uploads.RegisterRoutes(r, uploadsHandler, authMW.RequireAdmin)
		}
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
