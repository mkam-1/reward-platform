// Package api is game-service's HTTP layer.
package api

import (
	"log/slog"
	"net/http"

	"reward-platform/services/game-service/internal/catalog"
)

func NewHandler(gameCatalog *catalog.GameCatalog, logger *slog.Logger) http.Handler {
	a := &api{catalog: gameCatalog, logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealthz)
	mux.HandleFunc("POST /games", a.handleCreateGame)
	mux.HandleFunc("GET /games", a.handleListGames)
	mux.HandleFunc("GET /games/{id}", a.handleGetGame)
	mux.HandleFunc("POST /games/{id}/sessions", a.handleRecordSession)

	return withMiddleware(mux, logger)
}
