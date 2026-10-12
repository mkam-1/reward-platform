package domain

import (
	"context"
	"errors"
	"fmt"
)

// Repositories bundles every storage dependency Engine needs. It exists
// so NewEngineWithRepositories takes one argument instead of growing a
// new positional parameter every time another entity gets a real
// datastore — Player and Ledger moved to Postgres in phase 4, Reward and
// Redemption move to DynamoDB in phase 5, and Game will get its turn
// eventually too, without this constructor's signature changing again.
type Repositories struct {
	Players     PlayerRepository
	Ledger      LedgerRepository
	Rewards     RewardRepository
	Redemptions RedemptionRepository
}

// Engine is the application core: it owns players, games, rewards and
// the points ledger, and enforces the rules that connect them (you can't
// redeem a reward you can't afford, or one that's out of stock). HTTP
// transport is layered on top of this (services/player-service); Engine
// itself knows nothing about it.
//
// Every entity except Game is stored behind a repository interface
// rather than Engine owning storage directly — that's what lets
// player-service wire in real persistence (Postgres for players/ledger,
// DynamoDB for rewards/redemptions) while every method below stays
// exactly as written.
type Engine struct {
	clock Clock
	ids   *IDGenerator

	players     PlayerRepository
	games       *Store[Game]
	rewards     RewardRepository
	ledger      LedgerRepository
	redemptions RedemptionRepository
}

// NewEngine builds an Engine against the default in-memory repositories.
// Tests pass a fake Clock for deterministic timestamps; see
// NewSystemEngine for production/CLI use, and NewEngineWithRepositories
// for wiring in real persistence.
func NewEngine(clock Clock) *Engine {
	return NewEngineWithRepositories(clock, Repositories{
		Players:     NewMemoryPlayerRepository(),
		Ledger:      NewMemoryLedgerRepository(),
		Rewards:     NewMemoryRewardRepository(),
		Redemptions: NewMemoryRedemptionRepository(),
	})
}

// NewEngineWithRepositories builds an Engine against caller-supplied
// repositories. This is what lets player-service's main.go wire in real
// persistence in place of the in-memory defaults, without any code in
// this file changing — the entire reason these are interfaces.
func NewEngineWithRepositories(clock Clock, repos Repositories) *Engine {
	return &Engine{
		clock:       clock,
		ids:         NewIDGenerator(),
		players:     repos.Players,
		games:       NewStore[Game](),
		rewards:     repos.Rewards,
		ledger:      repos.Ledger,
		redemptions: repos.Redemptions,
	}
}

// NewSystemEngine is a convenience constructor wired to the real wall
// clock and in-memory repositories, for CLI use.
func NewSystemEngine() *Engine {
	return NewEngine(SystemClock{})
}

func (e *Engine) RegisterPlayer(ctx context.Context, username string) (Player, error) {
	if username == "" {
		return Player{}, &ValidationError{Field: "username", Msg: "must not be empty"}
	}
	p := Player{
		Model:    Model{ID: e.ids.Next("player"), CreatedAt: e.clock.Now()},
		Username: username,
	}
	if err := e.players.Put(ctx, p); err != nil {
		return Player{}, fmt.Errorf("register player: %w", err)
	}
	return p, nil
}

// GetPlayer fetches a single player by ID, distinct from Players() (which
// lists everyone) — used by the HTTP service's GET /players/{id}, which
// needs to return 404 for an unknown ID rather than an empty 200 list.
func (e *Engine) GetPlayer(ctx context.Context, id string) (Player, error) {
	player, err := e.players.Get(ctx, id)
	if err != nil {
		return Player{}, fmt.Errorf("get player: %w", err)
	}
	return player, nil
}

func (e *Engine) Players(ctx context.Context) ([]Player, error) {
	players, err := e.players.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list players: %w", err)
	}
	return players, nil
}

func (e *Engine) RegisterGame(ctx context.Context, name string, pointsPerSession int) (Game, error) {
	if name == "" {
		return Game{}, &ValidationError{Field: "name", Msg: "must not be empty"}
	}
	if pointsPerSession <= 0 {
		return Game{}, &ValidationError{Field: "pointsPerSession", Msg: "must be positive"}
	}
	g := Game{
		Model:            Model{ID: e.ids.Next("game"), CreatedAt: e.clock.Now()},
		Name:             name,
		PointsPerSession: pointsPerSession,
	}
	e.games.Put(ctx, g.ID, g)
	return g, nil
}

func (e *Engine) Games(ctx context.Context) []Game { return e.games.List(ctx) }

func (e *Engine) RegisterReward(ctx context.Context, name string, costPoints, stock int) (Reward, error) {
	if name == "" {
		return Reward{}, &ValidationError{Field: "name", Msg: "must not be empty"}
	}
	if costPoints <= 0 {
		return Reward{}, &ValidationError{Field: "costPoints", Msg: "must be positive"}
	}
	if stock < 0 {
		return Reward{}, &ValidationError{Field: "stock", Msg: "must not be negative"}
	}
	r := Reward{
		Model:      Model{ID: e.ids.Next("reward"), CreatedAt: e.clock.Now()},
		Name:       name,
		CostPoints: costPoints,
		Stock:      stock,
	}
	if err := e.rewards.Put(ctx, r); err != nil {
		return Reward{}, fmt.Errorf("register reward: %w", err)
	}
	return r, nil
}

func (e *Engine) Rewards(ctx context.Context) ([]Reward, error) {
	rewards, err := e.rewards.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rewards: %w", err)
	}
	return rewards, nil
}

// RecordSession credits a player with the game's PointsPerSession.
func (e *Engine) RecordSession(ctx context.Context, playerID, gameID string) (LedgerEntry, error) {
	if _, err := e.players.Get(ctx, playerID); err != nil {
		return LedgerEntry{}, fmt.Errorf("record session: player: %w", err)
	}
	game, err := e.games.Get(ctx, gameID)
	if err != nil {
		return LedgerEntry{}, fmt.Errorf("record session: game: %w", err)
	}

	entry := LedgerEntry{
		Model:    Model{ID: e.ids.Next("entry"), CreatedAt: e.clock.Now()},
		PlayerID: playerID,
		Delta:    game.PointsPerSession,
		Reason:   fmt.Sprintf("played %s", game.Name),
	}

	if _, err := e.ledger.Append(ctx, entry); err != nil {
		return LedgerEntry{}, fmt.Errorf("record session: %w", err)
	}
	return entry, nil
}

// Balance asks the ledger repository for the sum of a player's entries.
// It errors if the player doesn't exist at all, distinguishing "unknown
// player" from "known player, zero balance".
func (e *Engine) Balance(ctx context.Context, playerID string) (int, error) {
	if _, err := e.players.Get(ctx, playerID); err != nil {
		return 0, fmt.Errorf("balance: %w", err)
	}

	balance, err := e.ledger.Balance(ctx, playerID)
	if err != nil {
		return 0, fmt.Errorf("balance: %w", err)
	}
	return balance, nil
}

// RedeemReward exchanges a player's points for a reward, identified by
// idempotencyKey — a caller-supplied value (an HTTP client's retry of
// the exact same request should reuse the exact same key). If empty, one
// is generated, which is fine for callers (the CLI, load simulation) that
// don't need retry-safety.
//
// The idempotency check runs *first*, before any validation: a retried
// call with a key that's already been fully processed returns the
// cached result immediately, skipping balance/stock checks entirely.
// That ordering matters, not just style — by the time a retry arrives,
// the player's balance has already dropped by the very redemption being
// replayed, so re-running the balance check on a replay would wrongly
// reject it as insufficient funds. The Get() here is an optimistic
// fast path; Create()'s own conditional write below is the actual
// correctness guarantee for the rare case where two attempts with a
// brand-new key race each other.
//
// Stock-decrement safety comes from RewardRepository.DecrementStock
// being atomic at the datastore layer (a conditional UpdateItem for the
// DynamoDB-backed implementation), and balance safety comes from
// LedgerRepository.DebitIfSufficient the same way (a row-locked
// transaction for the Postgres-backed implementation) — multiple
// player-service instances sharing one table/database can no longer
// both decrement the last unit of stock, or both debit a balance that
// only actually covers one of them. That closes the gap phase 4's
// redeemMu left open (it only ever protected one process) for both
// halves of a redemption.
//
// What's *not* closed: if DecrementStock or DebitIfSufficient fails
// after the redemption record was already created, that record is left
// behind describing a redemption that didn't fully happen — a genuine
// distributed-transaction problem across two independent datastores
// (DynamoDB and Postgres) that nothing here fully solves. A correct fix
// is a saga or outbox pattern; phase 6 applies that pattern to the
// session-completed event flow, but hasn't been retrofitted onto this
// specific path yet.
func (e *Engine) RedeemReward(ctx context.Context, playerID, rewardID, idempotencyKey string) (Redemption, error) {
	if idempotencyKey == "" {
		idempotencyKey = e.ids.Next("redemption")
	}

	if existing, err := e.redemptions.Get(ctx, idempotencyKey); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Redemption{}, fmt.Errorf("redeem: idempotency check: %w", err)
	}

	if _, err := e.players.Get(ctx, playerID); err != nil {
		return Redemption{}, fmt.Errorf("redeem: %w", err)
	}
	reward, err := e.rewards.Get(ctx, rewardID)
	if err != nil {
		return Redemption{}, fmt.Errorf("redeem: %w", err)
	}
	if reward.Stock <= 0 {
		// Fast-path check: DecrementStock below is the real, atomic
		// guarantee, but there's no reason to compute a balance or
		// write a redemption record for a reward that's visibly
		// already out of stock.
		return Redemption{}, fmt.Errorf("redeem: reward %q: %w", reward.Name, ErrOutOfStock)
	}

	balance, err := e.Balance(ctx, playerID)
	if err != nil {
		return Redemption{}, err
	}
	if balance < reward.CostPoints {
		// Fast-path check, same reasoning as the Stock one above:
		// DebitIfSufficient below is the real, atomic guarantee — this
		// just avoids creating a redemption record for a request that's
		// already visibly unaffordable.
		return Redemption{}, fmt.Errorf("redeem: have %d, need %d: %w", balance, reward.CostPoints, ErrInsufficientBalance)
	}

	redemption := Redemption{
		Model:       Model{ID: idempotencyKey, CreatedAt: e.clock.Now()},
		PlayerID:    playerID,
		RewardID:    rewardID,
		PointsSpent: reward.CostPoints,
	}

	created, err := e.redemptions.Create(ctx, redemption)
	if err != nil {
		return Redemption{}, fmt.Errorf("redeem: %w", err)
	}
	if !created {
		// Lost a race with a concurrent call using this same new key —
		// rare, but handled the same way as the fast-path replay above:
		// fetch and return whatever the other call wrote.
		existing, err := e.redemptions.Get(ctx, idempotencyKey)
		if err != nil {
			return Redemption{}, fmt.Errorf("redeem: idempotent replay: %w", err)
		}
		return existing, nil
	}

	if err := e.rewards.DecrementStock(ctx, rewardID); err != nil {
		return Redemption{}, fmt.Errorf("redeem: %w", err)
	}

	entry := LedgerEntry{
		Model:    Model{ID: e.ids.Next("entry"), CreatedAt: e.clock.Now()},
		PlayerID: playerID,
		Delta:    -reward.CostPoints,
		Reason:   fmt.Sprintf("redeemed %s", reward.Name),
	}
	// DebitIfSufficient, not Append: the authoritative, atomic
	// check-and-debit — see its doc comment on LedgerRepository for why
	// this is the piece that actually closes the cross-instance race the
	// earlier fast-path Balance check above can't.
	if _, _, err := e.ledger.DebitIfSufficient(ctx, entry, reward.CostPoints); err != nil {
		return Redemption{}, fmt.Errorf("redeem: %w", err)
	}

	return redemption, nil
}

// ListRedemptions returns a player's redemption history, in whatever
// order the repository defines (the DynamoDB-backed one returns them via
// its GSI, sorted by creation time).
func (e *Engine) ListRedemptions(ctx context.Context, playerID string) ([]Redemption, error) {
	redemptions, err := e.redemptions.ListByPlayer(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("list redemptions: %w", err)
	}
	return redemptions, nil
}
