package domain

import "context"

// PlayerRepository persists Players. Two implementations exist: an
// in-memory one (memory_player_repo.go, used by tests and cmd/rewardctl)
// and a Postgres-backed one (services/player-service/internal/postgres).
// Engine depends on this interface, not a concrete store, specifically so
// it can run against either one without any of its own code changing —
// that's the entire point of the repository pattern, not an abstraction
// for its own sake.
type PlayerRepository interface {
	Put(ctx context.Context, p Player) error
	Get(ctx context.Context, id string) (Player, error)
	List(ctx context.Context) ([]Player, error)
}

// LedgerRepository persists the points ledger and answers balance
// queries. Balance is part of the contract rather than "List everything
// and sum in Go" on purpose: a real database answers "sum of deltas for
// this player" with one indexed query, far cheaper than shipping every
// row over the wire first and summing client-side.
type LedgerRepository interface {
	Append(ctx context.Context, entry LedgerEntry) error
	Balance(ctx context.Context, playerID string) (int, error)
}
