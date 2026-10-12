// Package api is player-service's HTTP layer: routes, handlers,
// middleware, and the JSON request/response shapes they speak. It's
// under internal/ so nothing outside this service can import it — the
// only way in is over the network, through NewHandler. As of phase 6,
// this is also the only way *end users* reach player-service — other
// services reach it over gRPC instead (see internal/grpcapi).
package api

import (
	"log/slog"
	"net/http"

	"reward-platform/services/player-service/internal/ledger"
)

// NewHandler builds player-service's HTTP handler: players, and only
// players — recording a session and redeeming a reward both used to be
// HTTP endpoints here, but now happen via SQS (a SessionCompleted event
// from game-service) and gRPC (a DebitPoints call from rewards-service)
// respectively, not direct HTTP calls from an end user.
func NewHandler(playerLedger *ledger.PlayerLedger, logger *slog.Logger) http.Handler {
	a := &api{ledger: playerLedger, logger: logger}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", a.handleHealthz)

	mux.HandleFunc("POST /players", a.handleCreatePlayer)
	mux.HandleFunc("GET /players", a.handleListPlayers)
	mux.HandleFunc("GET /players/{id}", a.handleGetPlayer)
	mux.HandleFunc("GET /players/{id}/balance", a.handleGetBalance)

	return withMiddleware(mux, logger)
}
