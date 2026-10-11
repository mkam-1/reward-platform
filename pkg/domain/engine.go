package domain

import (
	"context"
	"fmt"
	"sync"
)

// Engine is the application core: it owns players, games, rewards and
// the points ledger, and enforces the rules that connect them (you can't
// redeem a reward you can't afford, or one that's out of stock). HTTP
// transport is layered on top of this (services/player-service); Engine
// itself knows nothing about it.
//
// Players and the ledger are stored behind repository interfaces
// (PlayerRepository, LedgerRepository) rather than Engine owning their
// storage directly — that's what lets player-service swap in Postgres-
// backed persistence while every method below stays exactly as written.
// Games and Rewards haven't needed that yet (phase 5 gives Reward the
// same treatment with DynamoDB), so they're still plain in-memory Stores.
type Engine struct {
	clock Clock
	ids   *idGenerator

	players PlayerRepository
	games   *Store[Game]
	rewards *Store[Reward]
	ledger  LedgerRepository

	// redeemMu serializes RedeemReward end to end, same reasoning as
	// before (see git history / phase 2) — but worth being honest about
	// a gap this introduces now that players/ledger can live in a shared
	// Postgres database: this mutex only protects against races *within
	// one process*. If two player-service instances ran against the same
	// database, each would have its own redeemMu, and both could still
	// race each other on the same reward's Stock field, because Stock
	// itself is still only in this process's memory, not in the shared
	// database. Closing that gap for real means moving Reward into a
	// datastore too and using a database transaction (or row lock)
	// instead of an in-process mutex — left for when Reward gets its own
	// persistent store.
	redeemMu sync.Mutex
}

// NewEngine builds an Engine against the default in-memory
// PlayerRepository and LedgerRepository. Tests pass a fake Clock for
// deterministic timestamps; see NewSystemEngine for production/CLI use,
// and NewEngineWithRepositories for wiring in real persistence.
func NewEngine(clock Clock) *Engine {
	return NewEngineWithRepositories(clock, NewMemoryPlayerRepository(), NewMemoryLedgerRepository())
}

// NewEngineWithRepositories builds an Engine against caller-supplied
// PlayerRepository and LedgerRepository implementations. This is what
// lets player-service's main.go wire in Postgres-backed persistence in
// place of the in-memory default, without any code in this file
// changing — the entire reason those two dependencies are interfaces.
func NewEngineWithRepositories(clock Clock, players PlayerRepository, ledger LedgerRepository) *Engine {
	return &Engine{
		clock:   clock,
		ids:     newIDGenerator(),
		players: players,
		games:   NewStore[Game](),
		rewards: NewStore[Reward](),
		ledger:  ledger,
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
		Model:    Model{ID: e.ids.next("player"), CreatedAt: e.clock.Now()},
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
		Model:            Model{ID: e.ids.next("game"), CreatedAt: e.clock.Now()},
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
		Model:      Model{ID: e.ids.next("reward"), CreatedAt: e.clock.Now()},
		Name:       name,
		CostPoints: costPoints,
		Stock:      stock,
	}
	e.rewards.Put(ctx, r.ID, r)
	return r, nil
}

func (e *Engine) Rewards(ctx context.Context) []Reward { return e.rewards.List(ctx) }

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
		Model:    Model{ID: e.ids.next("entry"), CreatedAt: e.clock.Now()},
		PlayerID: playerID,
		Delta:    game.PointsPerSession,
		Reason:   fmt.Sprintf("played %s", game.Name),
	}

	if err := e.ledger.Append(ctx, entry); err != nil {
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

// RedeemReward exchanges a player's points for a reward, failing if they
// can't afford it or it's out of stock. See redeemMu's comment on the
// Engine struct for what "safe" does and doesn't cover here.
func (e *Engine) RedeemReward(ctx context.Context, playerID, rewardID string) (Redemption, error) {
	e.redeemMu.Lock()
	defer e.redeemMu.Unlock()

	if _, err := e.players.Get(ctx, playerID); err != nil {
		return Redemption{}, fmt.Errorf("redeem: %w", err)
	}
	reward, err := e.rewards.Get(ctx, rewardID)
	if err != nil {
		return Redemption{}, fmt.Errorf("redeem: %w", err)
	}
	if reward.Stock <= 0 {
		return Redemption{}, fmt.Errorf("redeem: reward %q: %w", reward.Name, ErrOutOfStock)
	}

	balance, err := e.Balance(ctx, playerID)
	if err != nil {
		return Redemption{}, err
	}
	if balance < reward.CostPoints {
		return Redemption{}, fmt.Errorf("redeem: have %d, need %d: %w", balance, reward.CostPoints, ErrInsufficientBalance)
	}

	reward.Stock--
	e.rewards.Put(ctx, reward.ID, reward)

	entry := LedgerEntry{
		Model:    Model{ID: e.ids.next("entry"), CreatedAt: e.clock.Now()},
		PlayerID: playerID,
		Delta:    -reward.CostPoints,
		Reason:   fmt.Sprintf("redeemed %s", reward.Name),
	}
	if err := e.ledger.Append(ctx, entry); err != nil {
		return Redemption{}, fmt.Errorf("redeem: %w", err)
	}

	redemption := Redemption{
		Model:       Model{ID: e.ids.next("redemption"), CreatedAt: e.clock.Now()},
		PlayerID:    playerID,
		RewardID:    rewardID,
		PointsSpent: reward.CostPoints,
	}
	return redemption, nil
}
