package api

import (
	"time"

	"reward-platform/pkg/domain"
)

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

type recordSessionRequest struct {
	PlayerID string `json:"playerId"`
}
