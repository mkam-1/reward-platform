package domain

// Game is something a player can complete a session of to earn points.
type Game struct {
	Model
	Name             string
	PointsPerSession int
}
