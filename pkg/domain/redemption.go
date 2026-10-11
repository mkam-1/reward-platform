package domain

// Redemption records a player exchanging points for a reward.
type Redemption struct {
	Model
	PlayerID    string
	RewardID    string
	PointsSpent int
}
