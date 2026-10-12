package domain

import (
	"context"
	"time"
)

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
	// List returns every player, unbounded — fine for cmd/rewardctl's
	// local demo/simulate commands (the only thing that still calls
	// this), which only ever run against a small in-memory dataset.
	// Never call this from a path that could run against a production
	// Postgres table with a real number of rows — see ListPage.
	List(ctx context.Context) ([]Player, error)
	// ListPage is the production-safe alternative List should have
	// always had: keyset pagination (ordered by CreatedAt, then ID as a
	// tiebreaker for players created in the same instant), capped at
	// limit, resuming after the given cursor. player-service's HTTP
	// handler for GET /players uses this, not List — the version of
	// that endpoint that called List ran one unbounded `SELECT *` that
	// would try to load and JSON-encode the *entire* players table on
	// every single call, which is fine at a few hundred test rows and
	// fails outright (OOM, multi-minute response, or both) the moment
	// the table holds anywhere near "millions of users."
	ListPage(ctx context.Context, limit int, after *PlayerPageCursor) ([]Player, error)
}

// PlayerPageCursor is "resume listing after this player" — the (time,
// id) pair ListPage's keyset pagination needs. CreatedAt alone isn't
// unique enough to resume from (two players can register in the same
// instant); ID alone isn't ordered (IDGenerator mints random IDs, not
// sequential ones — see id.go's doc comment for why). Together they're
// both unique and ordered.
type PlayerPageCursor struct {
	CreatedAt time.Time
	ID        string
}

// LedgerRepository persists the points ledger and answers balance
// queries. Balance is part of the contract rather than "List everything
// and sum in Go" on purpose: a real database answers "sum of deltas for
// this player" with one indexed query, far cheaper than shipping every
// row over the wire first and summing client-side.
//
// Both writes are idempotent, keyed by the entry's own ID, the same
// shape as RedemptionRepository.Create: they report created=false
// instead of erroring when that ID already exists. That's what lets
// phase 6's CreditPoints/DebitPoints (services/player-service/internal/
// ledger) be safely retried — an SQS message redelivered after a
// timeout, or a gRPC call retried by a caller that never saw the first
// response — reuse the same entry ID and land here again rather than
// double-crediting or double-debiting.
type LedgerRepository interface {
	// Append is for unconditional credits: a positive Delta can never
	// overdraw a balance, so no check is needed before writing it.
	Append(ctx context.Context, entry LedgerEntry) (created bool, err error)

	Balance(ctx context.Context, playerID string) (int, error)

	// DebitIfSufficient atomically checks that the player's balance is
	// at least minBalance and, only if so, appends entry (whose Delta
	// must be negative) — the check and the write happen as one atomic
	// operation, so two concurrent debits for the same player can never
	// both pass a balance check that only actually covers one of them.
	// This is what finally closes the race flagged back in phase 4's
	// Engine.RedeemReward comments: once Reward moved to DynamoDB
	// (phase 5) with its own atomic DecrementStock, the ledger side was
	// the one half still unguarded — this method is that other half.
	DebitIfSufficient(ctx context.Context, entry LedgerEntry, minBalance int) (newBalance int, created bool, err error)
}

// RewardRepository persists Rewards. DecrementStock is its own method,
// not "Get, subtract one, Put" from the caller's side, because it has to
// be atomic: two callers decrementing concurrently must never both see
// Stock==1 and succeed. The in-memory implementation gets that from a
// mutex; the DynamoDB-backed one gets it from a conditional UpdateItem —
// same contract, very different mechanism, which is exactly why this is
// an interface method and not exposed as separate Get/Put calls.
type RewardRepository interface {
	Put(ctx context.Context, r Reward) error
	Get(ctx context.Context, id string) (Reward, error)
	List(ctx context.Context) ([]Reward, error)
	DecrementStock(ctx context.Context, id string) error
}

// RedemptionRepository persists Redemptions, keyed by their own ID, which
// doubles as an idempotency key (see Engine.RedeemReward). Create reports
// created=false instead of returning an error when that ID already
// exists — "this was already done" is an expected, successful outcome
// for an idempotent write, not a failure.
type RedemptionRepository interface {
	Create(ctx context.Context, r Redemption) (created bool, err error)
	// Get takes rewardID alongside id because the DynamoDB-backed
	// implementation's primary key is (PK=REWARD#<rewardID>,
	// SK=REDEMPTION#<id>) — every caller already has rewardID in hand
	// (it's a parameter to RedeemReward from the start), so taking it
	// here turns what would otherwise be a full table Scan into a
	// direct, O(1) GetItem.
	Get(ctx context.Context, rewardID, id string) (Redemption, error)
	ListByPlayer(ctx context.Context, playerID string) ([]Redemption, error)
}
