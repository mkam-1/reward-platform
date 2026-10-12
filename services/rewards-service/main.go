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
	_ "net/http/pprof" //nolint:gosec // G108: registered on DefaultServeMux, but only ever served on a loopback-only listener below
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
	"reward-platform/services/rewards-service/internal/resilience"

	"reward-platform/pkg/domain"
	playerv1 "reward-platform/proto/player/v1"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// pprof on a loopback-only port, never exposed through the ALB/Service
	// — `go tool pprof http://localhost:6061/debug/pprof/profile` from
	// inside the container/pod, or port-forwarded in from outside it.
	// gosec G108 flags the pprof import as a blanket "don't expose this
	// publicly" rule; binding to localhost only (never 0.0.0.0) is
	// exactly the mitigation it's asking for. G114 (no-timeout
	// ListenAndServe) is addressed by using an *http.Server with
	// explicit timeouts instead of the bare package-level helper.
	pprofServer := &http.Server{
		Addr:              "localhost:6061",
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := pprofServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("pprof listener stopped", "error", err)
		}
	}()

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
	//
	// WithDefaultServiceConfig's round_robin policy matters once
	// player-service has more than one replica (its own HPA/Application
	// Auto Scaling target can take it well past one — see infra/k8s and
	// infra/terraform/ecs.tf): without it, gRPC's default "pick_first"
	// balancer sends every single RPC from this process to whichever one
	// replica its name resolution happened to return first, for the
	// entire life of this connection — the other replicas sit idle while
	// that one absorbs 100% of the traffic.
	//
	// round_robin only has multiple addresses to balance across because
	// of the "dns:///" prefix: without a scheme, grpc-go defaults to the
	// "passthrough" resolver, which treats playerServiceAddr as one
	// single, never-re-resolved address — exactly as unbalanced as
	// pick_first, just for a different reason. The "dns:///" resolver
	// actually queries DNS and gets back every address that name
	// currently has — in AWS, every task Cloud Map has registered under
	// PLAYER_SERVICE_GRPC_ADDR's hostname (ecs.tf's
	// aws_service_discovery_service); on a bare k8s ClusterIP Service
	// this degenerates back to one address, since kube-proxy already
	// load-balances at that layer — round_robin only earns its keep
	// against Cloud Map's flat DNS, not a ClusterIP.
	conn, err := grpc.NewClient(
		"dns:///"+playerServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig": [{"round_robin":{}}]}`),
	)
	if err != nil {
		logger.Error("failed to build player-service grpc client", "error", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }() // process is exiting either way; nothing left to do with a close error

	// Decorator chain, outermost first: CircuitBreaker wraps Retrying
	// wraps the raw gRPC client. See resilience's package doc comment
	// for why that order (not the reverse) is what makes the breaker's
	// failure count mean "calls," not "attempts."
	rawPlayerBalance := grpcclient.New(playerv1.NewPlayerLedgerClient(conn))
	retryingPlayerBalance := resilience.NewRetrying(rawPlayerBalance)
	playerBalance := resilience.NewCircuitBreaker(retryingPlayerBalance)

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
	handler := api.NewHandler(rewardCatalog, logger, dynamoClient.Ping)

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
