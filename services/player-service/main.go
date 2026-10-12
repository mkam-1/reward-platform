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
	"reward-platform/services/player-service/internal/dynamodb"
	"reward-platform/services/player-service/internal/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// DATABASE_URL is required, not defaulted — a service silently
	// falling back to in-memory storage because a connection string was
	// missing would be a much worse bug than refusing to start at all.
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "services/player-service/migrations"
	}

	// DYNAMODB_TABLE is required the same way DATABASE_URL is.
	// DYNAMODB_ENDPOINT is optional — set it to point at DynamoDB Local
	// for development; leave it empty against real AWS (phase 11), where
	// the SDK resolves the real regional endpoint itself.
	dynamoTable := os.Getenv("DYNAMODB_TABLE")
	if dynamoTable == "" {
		logger.Error("DYNAMODB_TABLE is required")
		os.Exit(1)
	}
	dynamoEndpoint := os.Getenv("DYNAMODB_ENDPOINT")

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStartup()

	db, err := postgres.Open(startupCtx, databaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := postgres.Migrate(db, migrationsDir); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	logger.Info("migrations applied")

	dynamoClient, err := dynamodb.NewClient(startupCtx, dynamoEndpoint, dynamoTable)
	if err != nil {
		logger.Error("failed to build dynamodb client", "error", err)
		os.Exit(1)
	}
	if err := dynamoClient.EnsureTable(startupCtx); err != nil {
		logger.Error("failed to ensure dynamodb table", "error", err)
		os.Exit(1)
	}
	logger.Info("dynamodb table ready", "table", dynamoTable)

	engine := domain.NewEngineWithRepositories(domain.SystemClock{}, domain.Repositories{
		Players:     postgres.NewPlayerRepository(db),
		Ledger:      postgres.NewLedgerRepository(db),
		Rewards:     dynamodb.NewRewardRepository(dynamoClient),
		Redemptions: dynamodb.NewRedemptionRepository(dynamoClient),
	})
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
