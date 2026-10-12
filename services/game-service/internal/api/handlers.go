package api

import (
	"context"
	"log/slog"
	"net/http"

	"reward-platform/services/game-service/internal/catalog"
)

type api struct {
	catalog *catalog.GameCatalog
	logger  *slog.Logger
	ready   func(context.Context) error
}

func (a *api) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *api) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if a.ready == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
		return
	}
	if err := a.ready(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *api) handleCreateGame(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[createGameRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	game, err := a.catalog.RegisterGame(r.Context(), req.Name, req.PointsPerSession)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toGameResponse(game))
}

func (a *api) handleListGames(w http.ResponseWriter, r *http.Request) {
	games := a.catalog.Games(r.Context())
	out := make([]gameResponse, len(games))
	for i, g := range games {
		out[i] = toGameResponse(g)
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) handleGetGame(w http.ResponseWriter, r *http.Request) {
	game, err := a.catalog.GetGame(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toGameResponse(game))
}

// handleRecordSession responds 202 Accepted, not 201/200 with a result —
// this is the visible, honest consequence of the flow now being
// asynchronous. There's no ledger entry to hand back in the response
// anymore; by the time this handler returns, the SessionCompleted event
// has only been published, not yet necessarily consumed and credited.
func (a *api) handleRecordSession(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[recordSessionRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	if err := a.catalog.RecordSession(r.Context(), req.PlayerID, r.PathValue("id"), idempotencyKey); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}
