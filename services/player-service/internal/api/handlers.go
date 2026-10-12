package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"reward-platform/services/player-service/internal/ledger"
)

// api holds the dependencies every handler needs. Handlers are methods on
// *api instead of free functions so they get the ledger and logger
// through a normal struct field — no package-level globals, no framework
// magic.
type api struct {
	ledger *ledger.PlayerLedger
	logger *slog.Logger
	ready  func(context.Context) error
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

func (a *api) handleCreatePlayer(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[createPlayerRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	player, err := a.ledger.RegisterPlayer(r.Context(), req.Username)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toPlayerResponse(player))
}

// handleListPlayers is paginated (limit + an opaque cursor), not the
// "give me everything" endpoint it used to be — see
// domain.PlayerRepository.ListPage's doc comment for why an unbounded
// list here is a real production bug, not a style preference, once the
// players table holds anywhere near "millions of users."
func (a *api) handleListPlayers(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "limit must be a non-negative integer")
			return
		}
		limit = parsed
	}

	cursor, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	players, err := a.ledger.PlayersPage(r.Context(), limit, cursor)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}

	out := playersPageResponse{Players: make([]playerResponse, len(players))}
	for i, p := range players {
		out.Players[i] = toPlayerResponse(p)
	}
	if len(players) > 0 {
		out.NextCursor = encodeCursor(players[len(players)-1])
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) handleGetPlayer(w http.ResponseWriter, r *http.Request) {
	player, err := a.ledger.GetPlayer(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toPlayerResponse(player))
}

func (a *api) handleGetBalance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	balance, err := a.ledger.Balance(r.Context(), id)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, balanceResponse{PlayerID: id, Balance: balance})
}
