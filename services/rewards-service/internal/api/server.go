// Package api is rewards-service's HTTP layer.
package api

import (
	"log/slog"
	"net/http"

	"reward-platform/services/rewards-service/internal/catalog"
)

func NewHandler(rewardCatalog *catalog.RewardCatalog, logger *slog.Logger) http.Handler {
	a := &api{catalog: rewardCatalog, logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealthz)

	mux.HandleFunc("POST /rewards", a.handleCreateReward)
	mux.HandleFunc("GET /rewards", a.handleListRewards)

	mux.HandleFunc("POST /players/{id}/redemptions", a.handleRedeemReward)
	mux.HandleFunc("GET /players/{id}/redemptions", a.handleListRedemptions)

	return withMiddleware(mux, logger)
}
