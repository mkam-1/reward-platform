package api

import (
	"time"

	"reward-platform/pkg/domain"
)

type createRewardRequest struct {
	Name       string `json:"name"`
	CostPoints int    `json:"costPoints"`
	Stock      int    `json:"stock"`
}

type rewardResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	CostPoints int       `json:"costPoints"`
	Stock      int       `json:"stock"`
	CreatedAt  time.Time `json:"createdAt"`
}

func toRewardResponse(r domain.Reward) rewardResponse {
	return rewardResponse{ID: r.ID, Name: r.Name, CostPoints: r.CostPoints, Stock: r.Stock, CreatedAt: r.CreatedAt}
}

type redeemRequest struct {
	RewardID string `json:"rewardId"`
}

type redemptionResponse struct {
	ID          string    `json:"id"`
	PlayerID    string    `json:"playerId"`
	RewardID    string    `json:"rewardId"`
	PointsSpent int       `json:"pointsSpent"`
	CreatedAt   time.Time `json:"createdAt"`
}

func toRedemptionResponse(r domain.Redemption) redemptionResponse {
	return redemptionResponse{ID: r.ID, PlayerID: r.PlayerID, RewardID: r.RewardID, PointsSpent: r.PointsSpent, CreatedAt: r.CreatedAt}
}
