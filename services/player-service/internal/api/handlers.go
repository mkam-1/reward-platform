package api

import (
	"log/slog"
	"net/http"

	"reward-platform/services/player-service/internal/ledger"
)

// api holds the dependencies every handler needs. Handlers are methods on
// *api instead of free functions so they get the ledger and logger
// through a normal struct field — no package-level globals, no framework
// magic.
type api struct {
	ledger *ledger.PlayerLedger
	logger *slog.Logger
}

func (a *api) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

func (a *api) handleListPlayers(w http.ResponseWriter, r *http.Request) {
	players, err := a.ledger.Players(r.Context())
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	out := make([]playerResponse, len(players))
	for i, p := range players {
		out[i] = toPlayerResponse(p)
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
