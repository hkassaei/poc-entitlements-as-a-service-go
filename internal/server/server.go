package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/config"
)

// Server is the HTTP server for the ECS.
type Server struct {
	router     *chi.Mux
	httpServer *http.Server
	cfg        config.Config
}

// NewServer creates a new chi-based HTTP server with middleware and routes.
func NewServer(cfg config.Config, entitlementHandler *EntitlementHandler) *Server {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(chiMiddleware.RealIP)
	r.Use(RecoverMiddleware)
	r.Use(RequestLogger)
	r.Use(UserAgentParser)
	r.Use(VersionCheck(cfg))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Entitlement routes
	r.Post("/entitlement", entitlementHandler.HandlePost)
	r.Get("/entitlement", entitlementHandler.HandleGet)

	return &Server{
		router: r,
		cfg:    cfg,
		httpServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Handler:      r,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	slog.Info("Starting ECS server", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("Shutting down server")
	return s.httpServer.Shutdown(ctx)
}
