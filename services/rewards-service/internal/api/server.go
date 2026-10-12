// Package api is rewards-service's HTTP layer.
package api

import (
	"context"
	"log/slog"
	"net/http"

	"golang.org/x/time/rate"

	"reward-platform/services/rewards-service/internal/catalog"
)

func NewHandler(rewardCatalog *catalog.RewardCatalog, logger *slog.Logger, ready func(context.Context) error) http.Handler {
	a := &api{catalog: rewardCatalog, logger: logger, ready: ready}
	redeemLimiter := newPerIPLimiter(rate.Limit(5), 10) // 5 req/s sustained, burst of 10

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealthz)
	mux.HandleFunc("GET /readyz", a.handleReadyz)
	mux.Handle("GET /metrics", metricsHandler())

	mux.HandleFunc("POST /rewards", a.handleCreateReward)
	mux.HandleFunc("GET /rewards", a.handleListRewards)

	mux.HandleFunc("POST /players/{id}/redemptions", rateLimitMiddleware(redeemLimiter, a.handleRedeemReward))
	mux.HandleFunc("GET /players/{id}/redemptions", a.handleListRedemptions)

	return withMiddleware(mux, logger)
}
