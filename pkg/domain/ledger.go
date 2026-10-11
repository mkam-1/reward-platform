package domain

// LedgerEntry is one append-only movement of points for a player: a
// positive Delta for points earned, negative for points spent. A player's
// balance is always the sum of their entries — there's no separately
// stored balance field that could drift out of sync with it.
type LedgerEntry struct {
	Model
	PlayerID string
	Delta    int
	Reason   string
}
