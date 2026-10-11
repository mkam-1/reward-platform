// Command player-service exposes the reward-platform domain engine over
// HTTP. It's phase 3's deliverable: the same Engine from pkg/domain that
// cmd/rewardctl drives from the command line, now wrapped in a real
// network-facing service with routing, middleware, and graceful
// shutdown.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"reward-platform/pkg/domain"
	"reward-platform/services/player-service/internal/api"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	engine := domain.NewSystemEngine()
	handler := api.NewHandler(engine, logger)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// signal.NotifyContext gives us a context that's cancelled the
	// moment the process receives SIGINT (Ctrl+C) or SIGTERM (what
	// Docker/Kubernetes send on a normal stop) — that's the trigger for
	// the graceful shutdown below, instead of the process just dying
	// mid-request.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("player-service listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	// Shutdown stops accepting new connections and waits for in-flight
	// requests to finish, up to this timeout — so a deploy or restart
	// doesn't cut an in-progress request off mid-response.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("shutdown complete")
}
