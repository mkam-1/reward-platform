// Command player-service owns Player and the points ledger. As of
// phase 6 it speaks three protocols at once, each for a different
// caller: HTTP for end users (internal/api), gRPC for rewards-service's
// synchronous balance/debit calls during a redemption (internal/grpcapi),
// and SQS for game-service's asynchronous SessionCompleted events
// (consumed here in main, credited via internal/ledger, outbox-relayed
// back out as PointsAwarded via internal/outbox).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof" //nolint:gosec // G108: registered on DefaultServeMux, but only ever served on a loopback-only listener below
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"reward-platform/mq"
	"reward-platform/pkg/domain"
	"reward-platform/services/player-service/internal/api"
	"reward-platform/services/player-service/internal/grpcapi"
	"reward-platform/services/player-service/internal/ledger"
	"reward-platform/services/player-service/internal/outbox"
	"reward-platform/services/player-service/internal/postgres"

	playerv1 "reward-platform/proto/player/v1"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// pprof on a loopback-only port, never exposed through the ALB/Service.
	// See rewards-service/main.go's identical block for the gosec
	// G108/G114 reasoning.
	pprofServer := &http.Server{
		Addr:              "localhost:6061",
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := pprofServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("pprof listener stopped", "error", err)
		}
	}()

	httpPort := os.Getenv("PORT")
	if httpPort == "" {
		httpPort = "8080"
	}
	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "9090"
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "services/player-service/migrations"
	}

	sqsEndpoint := os.Getenv("SQS_ENDPOINT")
	sessionCompletedQueue := os.Getenv("SESSION_COMPLETED_QUEUE")
	if sessionCompletedQueue == "" {
		sessionCompletedQueue = "session-completed"
	}
	pointsAwardedQueue := os.Getenv("POINTS_AWARDED_QUEUE")
	if pointsAwardedQueue == "" {
		pointsAwardedQueue = "points-awarded"
	}

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStartup()

	db, err := postgres.Open(startupCtx, databaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }() // process is exiting either way; nothing left to do with a close error

	if err := postgres.Migrate(db, migrationsDir); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	logger.Info("migrations applied")

	sqsClient, err := mq.NewClient(startupCtx, sqsEndpoint)
	if err != nil {
		logger.Error("failed to build sqs client", "error", err)
		os.Exit(1)
	}
	sessionCompletedURL, err := sqsClient.EnsureQueue(startupCtx, sessionCompletedQueue)
	if err != nil {
		logger.Error("failed to ensure session-completed queue", "error", err)
		os.Exit(1)
	}
	pointsAwardedURL, err := sqsClient.EnsureQueue(startupCtx, pointsAwardedQueue)
	if err != nil {
		logger.Error("failed to ensure points-awarded queue", "error", err)
		os.Exit(1)
	}

	ledgerRepo := postgres.NewLedgerRepository(db)
	playerLedger := ledger.New(domain.SystemClock{}, postgres.NewPlayerRepository(db), ledgerRepo)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// SQS consumer: credits points for every SessionCompleted event
	// game-service publishes.
	go func() {
		err := sqsClient.Consume(ctx, sessionCompletedURL, logger, func(ctx context.Context, body string) error {
			return handleSessionCompleted(ctx, playerLedger, body)
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("session-completed consumer stopped", "error", err)
		}
	}()

	// Outbox relay: publishes PointsAwarded rows CreditPoints wrote,
	// every 2s. See internal/outbox's doc comment for why this runs
	// separately instead of publishing inline.
	relay := outbox.NewRelay(ledgerRepo, sqsClient, pointsAwardedURL, logger)
	go relay.Run(ctx, 2*time.Second)

	// gRPC server: rewards-service's synchronous balance/debit calls.
	grpcServer := grpc.NewServer()
	playerv1.RegisterPlayerLedgerServer(grpcServer, grpcapi.NewServer(playerLedger))
	grpcListener, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		logger.Error("failed to listen for grpc", "error", err)
		os.Exit(1)
	}
	go func() {
		logger.Info("player-service grpc listening", "addr", grpcListener.Addr().String())
		if err := grpcServer.Serve(grpcListener); err != nil {
			logger.Error("grpc server error", "error", err)
		}
	}()

	// HTTP server: end users.
	handler := api.NewHandler(playerLedger, logger, db.PingContext)
	httpServer := &http.Server{
		Addr:         ":" + httpPort,
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() {
		logger.Info("player-service http listening", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("shutdown complete")
}

// handleSessionCompleted decodes a game-service SessionCompleted
// message and credits the points. An error here leaves the SQS message
// on the queue to be redelivered (see mq.Client.Consume) — safe because
// CreditPoints is idempotent on the event's own IdempotencyKey.
func handleSessionCompleted(ctx context.Context, playerLedger *ledger.PlayerLedger, body string) error {
	var event mq.SessionCompleted
	if err := json.Unmarshal([]byte(body), &event); err != nil {
		return fmt.Errorf("unmarshal session completed event: %w", err)
	}
	_, err := playerLedger.CreditPoints(ctx, event.PlayerID, event.Points, event.Reason, event.IdempotencyKey)
	return err
}
