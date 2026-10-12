package api

import (
	"time"

	"reward-platform/pkg/domain"
)

// Request/response DTOs live separately from domain.Player on purpose:
// the JSON shape the outside world sees is an API contract that has to
// stay stable, while the domain type is free to change as the domain
// model evolves.
//
// This file shrank a lot in phase 6: Game, Reward, Redemption and their
// DTOs moved out to game-service and rewards-service along with the
// logic that used them. Player-service's HTTP surface is now just
// players and their balance — everything else about a player's points
// now arrives over gRPC (DebitPoints, from rewards-service) or SQS (a
// SessionCompleted event, from game-service), not HTTP.

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
