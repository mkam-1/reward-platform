package api

import (
	"log/slog"
	"net/http"

	"reward-platform/services/rewards-service/internal/catalog"
)

type api struct {
	catalog *catalog.RewardCatalog
	logger  *slog.Logger
}

func (a *api) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *api) handleCreateReward(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[createRewardRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	reward, err := a.catalog.RegisterReward(r.Context(), req.Name, req.CostPoints, req.Stock)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toRewardResponse(reward))
}

func (a *api) handleListRewards(w http.ResponseWriter, r *http.Request) {
	rewards, err := a.catalog.Rewards(r.Context())
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	out := make([]rewardResponse, len(rewards))
	for i, rw := range rewards {
		out[i] = toRewardResponse(rw)
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) handleRedeemReward(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[redeemRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	redemption, err := a.catalog.RedeemReward(r.Context(), r.PathValue("id"), req.RewardID, idempotencyKey)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toRedemptionResponse(redemption))
}

func (a *api) handleListRedemptions(w http.ResponseWriter, r *http.Request) {
	redemptions, err := a.catalog.ListRedemptions(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	out := make([]redemptionResponse, len(redemptions))
	for i, red := range redemptions {
		out[i] = toRedemptionResponse(red)
	}
	writeJSON(w, http.StatusOK, out)
}
