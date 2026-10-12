// Package api is player-service's HTTP layer: routes, handlers,
// middleware, and the JSON request/response shapes they speak. It's
// under internal/ so nothing outside this service can import it — the
// only way in is over the network, through NewHandler.
package api

import (
	"log/slog"
	"net/http"

	"reward-platform/pkg/domain"
)

// NewHandler builds player-service's complete HTTP handler: every route
// wired to its handler, wrapped in the middleware chain every request
// passes through.
//
// Games and rewards get bare-bones endpoints here too (create + list,
// no update/delete) purely so this service is self-contained enough to
// exercise end to end — a session can't be recorded without a game to
// record it against. Phase 6 gives games and rewards their own services;
// until then, player-service fronts the whole engine.
func NewHandler(engine *domain.Engine, logger *slog.Logger) http.Handler {
	a := &api{engine: engine, logger: logger}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", a.handleHealthz)

	mux.HandleFunc("POST /players", a.handleCreatePlayer)
	mux.HandleFunc("GET /players", a.handleListPlayers)
	mux.HandleFunc("GET /players/{id}", a.handleGetPlayer)
	mux.HandleFunc("GET /players/{id}/balance", a.handleGetBalance)
	mux.HandleFunc("POST /players/{id}/sessions", a.handleRecordSession)
	mux.HandleFunc("POST /players/{id}/redemptions", a.handleRedeemReward)
	mux.HandleFunc("GET /players/{id}/redemptions", a.handleListRedemptions)

	mux.HandleFunc("POST /games", a.handleCreateGame)
	mux.HandleFunc("GET /games", a.handleListGames)

	mux.HandleFunc("POST /rewards", a.handleCreateReward)
	mux.HandleFunc("GET /rewards", a.handleListRewards)

	return withMiddleware(mux, logger)
}
