package api

import (
	"log/slog"
	"net/http"

	"reward-platform/pkg/domain"
)

// api holds the dependencies every handler needs. Handlers are methods on
// *api instead of free functions so they get the engine and logger
// through a normal struct field — no package-level globals, no framework
// magic.
type api struct {
	engine *domain.Engine
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

	player, err := a.engine.RegisterPlayer(r.Context(), req.Username)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toPlayerResponse(player))
}

func (a *api) handleListPlayers(w http.ResponseWriter, r *http.Request) {
	players, err := a.engine.Players(r.Context())
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
	player, err := a.engine.GetPlayer(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toPlayerResponse(player))
}

func (a *api) handleGetBalance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	balance, err := a.engine.Balance(r.Context(), id)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, balanceResponse{PlayerID: id, Balance: balance})
}

func (a *api) handleRecordSession(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[recordSessionRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	entry, err := a.engine.RecordSession(r.Context(), r.PathValue("id"), req.GameID)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toLedgerEntryResponse(entry))
}

func (a *api) handleRedeemReward(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[redeemRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	redemption, err := a.engine.RedeemReward(r.Context(), r.PathValue("id"), req.RewardID)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toRedemptionResponse(redemption))
}

func (a *api) handleCreateGame(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[createGameRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	game, err := a.engine.RegisterGame(r.Context(), req.Name, req.PointsPerSession)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toGameResponse(game))
}

func (a *api) handleListGames(w http.ResponseWriter, r *http.Request) {
	games := a.engine.Games(r.Context())
	out := make([]gameResponse, len(games))
	for i, g := range games {
		out[i] = toGameResponse(g)
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) handleCreateReward(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[createRewardRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	reward, err := a.engine.RegisterReward(r.Context(), req.Name, req.CostPoints, req.Stock)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toRewardResponse(reward))
}

func (a *api) handleListRewards(w http.ResponseWriter, r *http.Request) {
	rewards := a.engine.Rewards(r.Context())
	out := make([]rewardResponse, len(rewards))
	for i, rw := range rewards {
		out[i] = toRewardResponse(rw)
	}
	writeJSON(w, http.StatusOK, out)
}
