package mq

// SessionCompleted is published by game-service when a player finishes
// a session, and consumed by player-service, which credits the points.
// IdempotencyKey doubles as the resulting LedgerEntry's ID — SQS is
// "at least once" delivery, so the same message can arrive twice; using
// the message's own idempotency key as the ledger entry's ID is what
// lets PlayerLedger.CreditPoints recognize and skip a redelivery instead
// of crediting twice.
type SessionCompleted struct {
	PlayerID       string `json:"playerId"`
	GameID         string `json:"gameId"`
	Points         int    `json:"points"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotencyKey"`
}

// PointsAwarded is published by player-service after it credits a
// SessionCompleted event, and consumed by rewards-service. Today
// rewards-service only logs receipt of it — a placeholder for real
// future behavior (streak bonuses, "you can now afford X" notifications)
// rather than something it drives yet. Still published for real,
// through the same outbox-backed path as everything else in this flow,
// so the plumbing is there when that behavior is.
type PointsAwarded struct {
	PlayerID string `json:"playerId"`
	Points   int    `json:"points"`
}
