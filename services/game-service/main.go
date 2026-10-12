// Command game-service owns Game registration and publishes
// SessionCompleted events for player-service to consume — phase 6's
// first genuinely independent service (alongside rewards-service),
// communicating with player-service only over the network (SQS here;
// rewards-service uses gRPC), never through shared in-process code.
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

	"reward-platform/mq"
	"reward-platform/pkg/domain"
	"reward-platform/services/game-service/internal/api"
	"reward-platform/services/game-service/internal/catalog"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	sqsEndpoint := os.Getenv("SQS_ENDPOINT") // empty = real AWS SQS
	queueName := os.Getenv("SESSION_COMPLETED_QUEUE")
	if queueName == "" {
		queueName = "session-completed"
	}

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStartup()

	sqsClient, err := mq.NewClient(startupCtx, sqsEndpoint)
	if err != nil {
		logger.Error("failed to build sqs client", "error", err)
		os.Exit(1)
	}
	queueURL, err := sqsClient.EnsureQueue(startupCtx, queueName)
	if err != nil {
		logger.Error("failed to ensure sqs queue", "error", err)
		os.Exit(1)
	}
	logger.Info("sqs queue ready", "queue", queueName, "url", queueURL)

	gameCatalog := catalog.New(domain.SystemClock{}, domain.NewStore[domain.Game](), sqsClient, queueURL)
	handler := api.NewHandler(gameCatalog, logger, nil)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("game-service listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("shutdown complete")
}
