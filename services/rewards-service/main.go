// Command rewards-service owns Reward and Redemption. It calls
// player-service synchronously over gRPC during a redemption (it needs
// an answer right now: can this player afford it), and consumes
// PointsAwarded asynchronously over SQS (nothing time-sensitive to react
// to yet — see mq.PointsAwarded's doc comment).
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

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"reward-platform/mq"
	"reward-platform/services/rewards-service/internal/api"
	"reward-platform/services/rewards-service/internal/catalog"
	"reward-platform/services/rewards-service/internal/dynamodb"
	"reward-platform/services/rewards-service/internal/grpcclient"

	playerv1 "reward-platform/proto/player/v1"
	"reward-platform/pkg/domain"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dynamoTable := os.Getenv("DYNAMODB_TABLE")
	if dynamoTable == "" {
		logger.Error("DYNAMODB_TABLE is required")
		os.Exit(1)
	}
	dynamoEndpoint := os.Getenv("DYNAMODB_ENDPOINT")

	playerServiceAddr := os.Getenv("PLAYER_SERVICE_GRPC_ADDR")
	if playerServiceAddr == "" {
		logger.Error("PLAYER_SERVICE_GRPC_ADDR is required")
		os.Exit(1)
	}

	sqsEndpoint := os.Getenv("SQS_ENDPOINT")
	pointsAwardedQueue := os.Getenv("POINTS_AWARDED_QUEUE")
	if pointsAwardedQueue == "" {
		pointsAwardedQueue = "points-awarded"
	}

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStartup()

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

	// grpc.NewClient (not the older, deprecated grpc.Dial) builds a lazy
	// connection: it doesn't actually dial player-service until the
	// first RPC. insecure.NewCredentials() means plaintext — fine inside
	// one AWS VPC's private network (phase 11), not something to carry
	// over a public network unchanged.
	conn, err := grpc.NewClient(playerServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Error("failed to build player-service grpc client", "error", err)
		os.Exit(1)
	}
	defer conn.Close()
	playerBalance := grpcclient.New(playerv1.NewPlayerLedgerClient(conn))

	sqsClient, err := mq.NewClient(startupCtx, sqsEndpoint)
	if err != nil {
		logger.Error("failed to build sqs client", "error", err)
		os.Exit(1)
	}
	pointsAwardedQueueURL, err := sqsClient.EnsureQueue(startupCtx, pointsAwardedQueue)
	if err != nil {
		logger.Error("failed to ensure points-awarded queue", "error", err)
		os.Exit(1)
	}

	rewardCatalog := catalog.New(
		domain.SystemClock{},
		dynamodb.NewRewardRepository(dynamoClient),
		dynamodb.NewRedemptionRepository(dynamoClient),
		playerBalance,
	)
	handler := api.NewHandler(rewardCatalog, logger)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Background consumer for PointsAwarded — see mq.PointsAwarded's doc
	// comment for why this currently just logs receipt.
	go func() {
		err := sqsClient.Consume(ctx, pointsAwardedQueueURL, logger, func(_ context.Context, body string) error {
			logger.Info("points awarded event received", "body", body)
			return nil
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("points-awarded consumer stopped", "error", err)
		}
	}()

	go func() {
		logger.Info("rewards-service listening", "addr", srv.Addr)
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
