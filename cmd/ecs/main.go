package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/config"
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/db"
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/eapaka"
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/server"
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/services"
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/token"
)

func main() {
	// Structured JSON logging
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg := config.LoadConfig()

	ctx := context.Background()

	// Connect to Postgres
	pool, err := db.NewPool(ctx, cfg.DatabaseURL, cfg.DBPoolSize)
	if err != nil {
		slog.Error("Failed to connect to Postgres", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Connect to Redis
	redisClient, err := db.NewRedisClient(ctx, cfg.RedisURL)
	if err != nil {
		slog.Error("Failed to connect to Redis", "err", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	// Initialize stores
	queries := db.NewQueries(pool)
	sessionStore := eapaka.NewSessionStore(redisClient)
	reauthSessionStore := eapaka.NewReauthSessionStore(redisClient)
	reauthStore := eapaka.NewReauthStore(redisClient, config.ReauthStoreTTLSeconds)
	idempotencyCache := eapaka.NewIdempotencyCache(redisClient)
	hssClient := eapaka.NewHSSClient(cfg.HSSURL)

	// Subscriber lookup function
	subscriberLookup := func(ctx context.Context, imsi string) (string, error) {
		sub, err := queries.FindSubscriberByIMSI(ctx, imsi)
		if err != nil {
			return "", fmt.Errorf("look up subscriber %s: %w", imsi, err)
		}
		return sub.ID, nil
	}

	// Initialize orchestrator and handlers
	orchestrator := eapaka.NewOrchestrator(hssClient, sessionStore, reauthStore, subscriberLookup)
	reauthHandler := eapaka.NewReauthHandler(reauthStore, reauthSessionStore, orchestrator)
	tokenService := token.NewService(queries, redisClient, cfg)
	responseBuilder := services.NewResponseBuilder(queries, cfg)

	entitlementHandler := server.NewEntitlementHandler(
		orchestrator, reauthHandler, reauthStore, reauthSessionStore,
		idempotencyCache, tokenService, responseBuilder, cfg,
	)

	srv := server.NewServer(cfg, entitlementHandler)

	// Graceful shutdown: signal handler goroutine communicates via channel
	shutdownErr := make(chan error, 1)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		sig := <-sigCh
		slog.Info("Received signal, shutting down", "signal", sig)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdownErr <- srv.Shutdown(shutdownCtx)
	}()

	if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("Server failed", "err", err)
		os.Exit(1)
	}

	if err := <-shutdownErr; err != nil {
		slog.Error("Server shutdown error", "err", err)
	}

	slog.Info("Server stopped")
}
