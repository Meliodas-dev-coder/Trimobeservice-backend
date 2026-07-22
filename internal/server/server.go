// Package server wires the HTTP router, global middleware, and lifecycle.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jmoiron/sqlx"

	"github.com/trimo/backend/internal/config"
	"github.com/trimo/backend/internal/httpx"
	"github.com/trimo/backend/internal/ratelimit"
)

// Server holds the router and shared dependencies.
type Server struct {
	cfg    *config.Config
	log    *slog.Logger
	db     *sqlx.DB
	router chi.Router
	api    chi.Router
}

// New builds a Server with global middleware, health endpoints, and a
// versioned /api/v1 subrouter ready to mount modules onto.
func New(cfg *config.Config, log *slog.Logger, db *sqlx.DB) *Server {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// RealIP trusts client-supplied forwarding headers, which are spoofable when
	// the service is directly exposed. Only honor them behind a trusted proxy.
	if cfg.HTTP.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(requestLogger(log))
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders(cfg.Env == "production"))
	r.Use(ratelimit.New(cfg.HTTP.RateLimitRPS, cfg.HTTP.RateLimitBurst).Middleware)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.HTTP.CORSOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Content-Disposition"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	s := &Server{cfg: cfg, log: log, db: db, router: r}

	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)

	s.api = chi.NewRouter()
	r.Mount("/api/v1", s.api)

	return s
}

// MountAPI registers module routes onto the /api/v1 subrouter.
func (s *Server) MountAPI(register func(r chi.Router)) {
	register(s.api)
}

// Run starts the HTTP server and blocks until ctx is cancelled, then performs
// a graceful shutdown.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:         ":" + s.cfg.HTTP.Port,
		Handler:      s.router,
		ReadTimeout:  s.cfg.HTTP.ReadTimeout,
		WriteTimeout: s.cfg.HTTP.WriteTimeout,
		IdleTimeout:  s.cfg.HTTP.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.log.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.HTTP.ShutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"status": "ready"})
}
