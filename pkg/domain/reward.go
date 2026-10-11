package domain

// Reward is an item players can redeem points for. Stock decrements on
// every redemption; a Reward with Stock 0 cannot be redeemed.
type Reward struct {
	Model
	Name       string
	CostPoints int
	Stock      int
}
