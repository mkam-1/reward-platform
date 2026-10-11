package api

import (
	"time"

	"reward-platform/pkg/domain"
)

// Request/response DTOs live separately from domain.Player et al. on
// purpose: the JSON shape the outside world sees is an API contract that
// has to stay stable, while the domain types are free to change as the
// domain model evolves. Collapsing the two together would mean every
// internal refactor risks becoming a breaking API change.

type createPlayerRequest struct {
	Username string `json:"username"`
}

type playerResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"createdAt"`
}

func toPlayerResponse(p domain.Player) playerResponse {
	return playerResponse{ID: p.ID, Username: p.Username, CreatedAt: p.CreatedAt}
}

type balanceResponse struct {
	PlayerID string `json:"playerId"`
	Balance  int    `json:"balance"`
}

type createGameRequest struct {
	Name             string `json:"name"`
	PointsPerSession int    `json:"pointsPerSession"`
}

type gameResponse struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	PointsPerSession int       `json:"pointsPerSession"`
	CreatedAt        time.Time `json:"createdAt"`
}

func toGameResponse(g domain.Game) gameResponse {
	return gameResponse{ID: g.ID, Name: g.Name, PointsPerSession: g.PointsPerSession, CreatedAt: g.CreatedAt}
}

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

type recordSessionRequest struct {
	GameID string `json:"gameId"`
}

type ledgerEntryResponse struct {
	ID        string    `json:"id"`
	PlayerID  string    `json:"playerId"`
	Delta     int       `json:"delta"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
}

func toLedgerEntryResponse(e domain.LedgerEntry) ledgerEntryResponse {
	return ledgerEntryResponse{ID: e.ID, PlayerID: e.PlayerID, Delta: e.Delta, Reason: e.Reason, CreatedAt: e.CreatedAt}
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
